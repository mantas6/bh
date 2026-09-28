package comment

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
)

func newResolveOpts(t *testing.T, srv *apitest.Server, ios *cmdutil.IOStreams) *ResolveOptions {
	return &ResolveOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(newGitStub(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		CommentID: 9,
	}
}

func TestResolve(t *testing.T) {
	t.Parallel()
	// No PR route: a numeric argument must not fetch the pull request.
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/comments/9/resolve", 200, nil)

	ios, _, out, errOut := cmdutil.TestIOStreams()
	if err := resolveRun(t.Context(), newResolveOpts(t, srv, ios)); err != nil {
		t.Fatalf("resolveRun: %v", err)
	}
	if cmdtest.FindRequest(srv, "POST", "/comments/9/resolve") == nil {
		t.Error("expected POST .../resolve")
	}
	if len(srv.Requests()) != 1 {
		t.Errorf("requests = %v, want only the resolve call", srv.Requests())
	}
	if !strings.Contains(errOut.String(), "Resolved comment #9") {
		t.Errorf("expected success line on stderr, got %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
}

func TestReopen(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("DELETE", "/repositories/myws/myrepo/pullrequests/123/comments/9/resolve", 204, nil)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := newResolveOpts(t, srv, ios)
	opts.Reopen = true
	if err := resolveRun(t.Context(), opts); err != nil {
		t.Fatalf("resolveRun reopen: %v", err)
	}
	if cmdtest.FindRequest(srv, "DELETE", "/comments/9/resolve") == nil {
		t.Error("expected DELETE .../resolve")
	}
	if !strings.Contains(errOut.String(), "Reopened comment #9") {
		t.Errorf("expected success line on stderr, got %q", errOut.String())
	}
}

func TestResolveByBranchLooksUpPR(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		map[string]any{"values": []any{cmdtest.SamplePR()}})
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/comments/9/resolve", 200, nil)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newResolveOpts(t, srv, ios)
	opts.Arg = "feature"
	if err := resolveRun(t.Context(), opts); err != nil {
		t.Fatalf("resolveRun: %v", err)
	}
	if cmdtest.FindRequest(srv, "POST", "/pullrequests/123/comments/9/resolve") == nil {
		t.Error("expected POST .../123/comments/9/resolve")
	}
}

func TestResolveExactArgs(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdResolve(f, func(*ResolveOptions) error { return nil })
	if _, _, err := cmdtest.RunCommand(t, cmd, "123"); err == nil {
		t.Fatal("expected error for missing comment id")
	}
}

func TestReopenParsesArgs(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *ResolveOptions
	cmd := NewCmdReopen(f, func(o *ResolveOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "42", "7"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "42" || captured.CommentID != 7 || !captured.Reopen {
		t.Errorf("parsed = %+v", captured)
	}
}

func TestResolveParsesArgs(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *ResolveOptions
	cmd := NewCmdResolve(f, func(o *ResolveOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "42", "7"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "42" || captured.CommentID != 7 || captured.Reopen {
		t.Errorf("parsed = %+v", captured)
	}
}
