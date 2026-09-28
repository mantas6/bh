package shared

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

// newFinder builds a Finder whose BaseRepo yields TestRepo/OriginRemote and
// whose Git provider returns stub. A nil stub makes Git fail the test, for
// asserting that git is not consulted.
func newFinder(t *testing.T, srv *apitest.Server, stub *gittest.Stub) *Finder {
	t.Helper()
	return NewFinder(
		cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		cmdtest.ClientFunc(srv),
		func() (git.Runner, error) {
			if stub == nil {
				t.Error("git should not be needed")
				return nil, errors.New("no git")
			}
			return stub, nil
		},
	)
}

func TestFindByNumber(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"123", "#123"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

			found, err := newFinder(t, srv, nil).Find(t.Context(), arg)
			if err != nil {
				t.Fatalf("Find: %v", err)
			}
			if found.PR.ID != 123 {
				t.Errorf("id = %d", found.PR.ID)
			}
			if found.Repo != cmdtest.TestRepo() {
				t.Errorf("repo = %v", found.Repo)
			}
			if found.Remote == nil || found.Remote.Remote.Name != "origin" {
				t.Errorf("remote = %v, want origin", found.Remote)
			}
			if found.Client == nil {
				t.Error("client is nil")
			}
		})
	}
}

func TestFindIDSkipsFetchForNumber(t *testing.T) {
	t.Parallel()
	for _, arg := range []string{"123", "https://bitbucket.org/other/repo/pull-requests/123"} {
		t.Run(arg, func(t *testing.T) {
			t.Parallel()
			// No routes: any request fails the test.
			srv := apitest.New(t)

			// A URL only consults git for a matching local remote.
			stub := gittest.New(t).Register("", nil, "remote", "-v")
			found, err := newFinder(t, srv, stub).FindID(t.Context(), arg)
			if err != nil {
				t.Fatalf("FindID: %v", err)
			}
			if found.PR != nil {
				t.Errorf("PR = %+v, want nil", found.PR)
			}
			if found.Number != 123 {
				t.Errorf("Number = %d, want 123", found.Number)
			}
			if found.Client == nil {
				t.Error("client is nil")
			}
			if len(srv.Requests()) != 0 {
				t.Errorf("unexpected requests: %v", srv.Requests())
			}
		})
	}
}

func TestFindIDLooksUpBranch(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		map[string]any{"values": []api.PullRequest{*cmdtest.SamplePR()}})

	found, err := newFinder(t, srv, nil).FindID(t.Context(), "feature")
	if err != nil {
		t.Fatalf("FindID: %v", err)
	}
	if found.PR == nil || found.Number != 123 {
		t.Errorf("found = %+v", found)
	}
}

func TestFindByBranch(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		map[string]any{"values": []api.PullRequest{*cmdtest.SamplePR()}})

	found, err := newFinder(t, srv, nil).Find(t.Context(), "feature")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.PR.ID != 123 {
		t.Errorf("id = %d", found.PR.ID)
	}
	if len(srv.Requests()) != 1 {
		t.Fatalf("requests = %d, want 1", len(srv.Requests()))
	}
	if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); !strings.Contains(q, `source.branch.name="feature"`) {
		t.Errorf("q = %q", q)
	}
}

func TestFindByBranchNoPR(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		map[string]any{"values": []api.PullRequest{}})

	_, err := newFinder(t, srv, nil).Find(t.Context(), "nope")
	if err == nil || !strings.Contains(err.Error(), `no open pull request found for branch "nope"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestFindByURLNeedsNoGitOrBaseRepo(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/other/repo/pullrequests/7", 200, cmdtest.SamplePR())

	f := NewFinder(
		func(context.Context) (git.Repo, *git.ResolvedRemote, error) {
			t.Error("BaseRepo should not be called for a URL")
			return git.Repo{}, nil, errors.New("not a git repository")
		},
		cmdtest.ClientFunc(srv),
		func() (git.Runner, error) { return nil, errors.New("git executable not found") },
	)

	found, err := f.Find(t.Context(), "https://bitbucket.org/other/repo/pull-requests/7")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Repo.FullName() != "other/repo" {
		t.Errorf("repo = %s", found.Repo.FullName())
	}
	if found.Remote != nil {
		t.Errorf("remote = %v, want nil", found.Remote)
	}
}

func TestFindByURLStrictGitOutsideCheckout(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/other/repo/pullrequests/7", 200, cmdtest.SamplePR())

	// Listing remotes fails outside a checkout; nothing else may be run.
	notRepo := &git.Error{Args: []string{"remote", "-v"}, ExitCode: 128, Stderr: "fatal: not a git repository"}
	stub := gittest.New(t).ExpectResponse("", notRepo, "remote", "-v")

	found, err := newFinder(t, srv, stub).Find(t.Context(), "https://bitbucket.org/other/repo/pull-requests/7")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Remote != nil {
		t.Errorf("remote = %v, want nil", found.Remote)
	}
}

func TestFindByURLMatchesLocalRemote(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/other/repo/pullrequests/7", 200, cmdtest.SamplePR())

	stub := gittest.New(t).Register(
		"origin\tgit@bitbucket.org:myws/myrepo.git (fetch)\n"+
			"fork\thttps://bitbucket.org/other/repo.git (fetch)\n",
		nil, "remote", "-v")

	found, err := newFinder(t, srv, stub).Find(t.Context(), "https://bitbucket.org/other/repo/pull-requests/7")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.Remote == nil || found.Remote.Remote.Name != "fork" {
		t.Errorf("remote = %v, want fork", found.Remote)
	}
}

func TestFindCurrentBranch(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		map[string]any{"values": []api.PullRequest{*cmdtest.SamplePR()}})

	stub := gittest.New(t).Register("feature", nil, "symbolic-ref", "--quiet", "--short", "HEAD")

	found, err := newFinder(t, srv, stub).Find(t.Context(), "")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if found.PR.ID != 123 {
		t.Errorf("id = %d", found.PR.ID)
	}
	if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); !strings.Contains(q, `source.branch.name="feature"`) {
		t.Errorf("q = %q", q)
	}
}

func TestFindDetachedHead(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	stub := gittest.New(t).Register("", gittest.Exit(1), "symbolic-ref", "--quiet", "--short", "HEAD")

	_, err := newFinder(t, srv, stub).Find(t.Context(), "")
	if err == nil || err.Error() != "no pull request specified and not on a branch" {
		t.Fatalf("err = %v", err)
	}
}

func TestFindCurrentBranchGitUnavailable(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	f := NewFinder(
		func(context.Context) (git.Repo, *git.ResolvedRemote, error) { return cmdtest.TestRepo(), nil, nil },
		cmdtest.ClientFunc(srv),
		func() (git.Runner, error) { return nil, errors.New("git executable not found") },
	)
	_, err := f.Find(t.Context(), "")
	if err == nil || !strings.Contains(err.Error(), "git executable not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestFindInvalidArg(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	_, err := newFinder(t, srv, nil).Find(t.Context(), "#abc")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(srv.Requests()) != 0 {
		t.Errorf("requests = %d, want 0", len(srv.Requests()))
	}
}
