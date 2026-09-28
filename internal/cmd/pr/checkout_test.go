package pr

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

// checkoutPR returns a same-repo PR fixture (source lives in the base repo).
func checkoutPR() *api.PullRequest {
	pr := cmdtest.SamplePR()
	pr.Source.Branch.Name = "feature"
	pr.Source.Repository = &api.Repository{FullName: "myws/myrepo"}
	pr.Destination.Branch.Name = "main"
	return pr
}

// forkPR returns a PR whose source branch lives in a fork, carrying clone
// links for both protocols.
func forkPR() *api.PullRequest {
	pr := cmdtest.SamplePR()
	pr.Source.Branch.Name = "feature"
	pr.Source.Repository = &api.Repository{
		FullName: "forkws/myrepo",
		Links: api.Links{Clone: []api.CloneLink{
			{Name: "https", Href: "https://bitbucket.org/forkws/myrepo.git"},
			{Name: "ssh", Href: "git@bitbucket.org:forkws/myrepo.git"},
		}},
	}
	pr.Destination.Branch.Name = "main"
	return pr
}

func runCheckout(t *testing.T, pr *api.PullRequest, rr *git.ResolvedRemote, stub *gittest.Stub, mutate func(*CheckoutOptions)) *gittest.Stub {
	t.Helper()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CheckoutOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(rr),
		Arg:       "123",
	}
	if mutate != nil {
		mutate(opts)
	}
	if err := checkoutRun(t.Context(), opts); err != nil {
		t.Fatalf("checkoutRun: %v", err)
	}
	return stub
}

func assertCalls(t testing.TB, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("git calls =\n  %v\nwant\n  %v", got, want)
	}
}

func TestCheckoutSameRepoNewBranch(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "origin", "+refs/heads/feature:refs/remotes/origin/feature")
	stub.ExpectResponse("", gittest.Exit(1), "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Expect("checkout", "-b", "feature", "--track", "origin/feature")

	runCheckout(t, checkoutPR(), cmdtest.OriginRemote(), stub, nil)
}

func TestCheckoutSameRepoExistingBranch(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "origin", "+refs/heads/feature:refs/remotes/origin/feature")
	// rev-parse succeeds -> branch exists.
	stub.ExpectResponse("abc", nil, "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Expect("checkout", "--end-of-options", "feature")
	stub.Expect("merge", "--ff-only", "origin/feature")

	runCheckout(t, checkoutPR(), cmdtest.OriginRemote(), stub, nil)
}

func TestCheckoutSameRepoExistingBranchForce(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "origin", "+refs/heads/feature:refs/remotes/origin/feature")
	stub.ExpectResponse("abc", nil, "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Expect("checkout", "--end-of-options", "feature")
	stub.Expect("reset", "--hard", "--end-of-options", "origin/feature")

	runCheckout(t, checkoutPR(), cmdtest.OriginRemote(), stub, func(o *CheckoutOptions) {
		o.Force = true
	})
}

func TestCheckoutSameRepoDetach(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "origin", "feature")
	stub.Expect("checkout", "--detach", "FETCH_HEAD")

	runCheckout(t, checkoutPR(), cmdtest.OriginRemote(), stub, func(o *CheckoutOptions) {
		o.Detach = true
	})
}

func TestCheckoutSameRepoBranchName(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "origin", "+refs/heads/feature:refs/remotes/origin/feature")
	stub.ExpectResponse("", gittest.Exit(1), "rev-parse", "--verify", "--quiet", "refs/heads/mine")
	stub.Expect("checkout", "-b", "mine", "--track", "origin/feature")

	runCheckout(t, checkoutPR(), cmdtest.OriginRemote(), stub, func(o *CheckoutOptions) {
		o.Branch = "mine"
	})
}

func TestCheckoutForkSSH(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "git@bitbucket.org:forkws/myrepo.git", "+refs/heads/feature:refs/remotes/forkws/feature")
	stub.ExpectResponse("", gittest.Exit(1), "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Expect("checkout", "-b", "feature", "--no-track", "refs/remotes/forkws/feature")
	stub.Expect("config", "branch.feature.remote", "git@bitbucket.org:forkws/myrepo.git")
	stub.Expect("config", "branch.feature.merge", "refs/heads/feature")

	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "git@bitbucket.org:myws/myrepo.git"},
		Repo:   cmdtest.TestRepo(),
	}
	runCheckout(t, forkPR(), rr, stub, nil)
}

func TestCheckoutForkHTTPS(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "https://bitbucket.org/forkws/myrepo.git", "+refs/heads/feature:refs/remotes/forkws/feature")
	stub.ExpectResponse("", gittest.Exit(1), "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Expect("checkout", "-b", "feature", "--no-track", "refs/remotes/forkws/feature")
	stub.Expect("config", "branch.feature.remote", "https://bitbucket.org/forkws/myrepo.git")
	stub.Expect("config", "branch.feature.merge", "refs/heads/feature")

	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "https://bitbucket.org/myws/myrepo.git"},
		Repo:   cmdtest.TestRepo(),
	}
	runCheckout(t, forkPR(), rr, stub, nil)
}

func TestCheckoutForkDetach(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "https://bitbucket.org/forkws/myrepo.git", "+refs/heads/feature:refs/remotes/forkws/feature")
	stub.Expect("checkout", "--detach", "refs/remotes/forkws/feature")
	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "https://bitbucket.org/myws/myrepo.git"},
		Repo:   cmdtest.TestRepo(),
	}
	runCheckout(t, forkPR(), rr, stub, func(o *CheckoutOptions) {
		o.Detach = true
	})
}

func TestCheckoutForkSynthesizedURL(t *testing.T) {
	t.Parallel()
	// No clone links -> URL is synthesized from the protocol + full name.
	pr := cmdtest.SamplePR()
	pr.Source.Branch.Name = "feature"
	pr.Source.Repository = &api.Repository{FullName: "forkws/myrepo"}
	pr.Destination.Branch.Name = "main"

	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "git@bitbucket.org:forkws/myrepo.git", "+refs/heads/feature:refs/remotes/forkws/feature")
	stub.ExpectResponse("", gittest.Exit(1), "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Expect("checkout", "-b", "feature", "--no-track", "refs/remotes/forkws/feature")
	stub.Expect("config", "branch.feature.remote", "git@bitbucket.org:forkws/myrepo.git")
	stub.Expect("config", "branch.feature.merge", "refs/heads/feature")

	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "ssh://git@bitbucket.org/myws/myrepo.git"},
		Repo:   cmdtest.TestRepo(),
	}
	runCheckout(t, pr, rr, stub, nil)
}

func TestCheckoutForkSynthesizedURLGitPlusSSH(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Source.Branch.Name = "feature"
	pr.Source.Repository = &api.Repository{FullName: "forkws/myrepo"}

	stub := gittest.New(t)
	// The SSH clone URL is used for a git+ssh:// remote.
	stub.Expect("fetch", "--end-of-options", "git@bitbucket.org:forkws/myrepo.git", "+refs/heads/feature:refs/remotes/forkws/feature")
	stub.ExpectResponse("", gittest.Exit(1), "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Register("", nil, "checkout", "-b", "feature", "--no-track", "refs/remotes/forkws/feature")
	stub.Register("", nil, "config", "branch.feature.remote", "git@bitbucket.org:forkws/myrepo.git")
	stub.Register("", nil, "config", "branch.feature.merge", "refs/heads/feature")

	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "git+ssh://git@bitbucket.org/myws/myrepo.git"},
		Repo:   cmdtest.TestRepo(),
	}
	runCheckout(t, pr, rr, stub, nil)
}

func TestCheckoutDeletedForkErrors(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	pr := checkoutPR()
	pr.Source.Repository = nil
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	stub := gittest.New(t)
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CheckoutOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Arg:       "123",
	}
	err := checkoutRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "source repository of pull request #123 is no longer available") {
		t.Fatalf("err = %v", err)
	}
	if len(stub.Calls()) != 0 {
		t.Errorf("unexpected git calls: %v", stub.CallStrings())
	}
}

func TestCheckoutURLRepoUsesItsOwnRemote(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	pr := checkoutPR()
	pr.Source.Repository = &api.Repository{FullName: "other/repo"}
	srv.Handle("GET", "/repositories/other/repo/pullrequests/123", 200, pr)

	stub := gittest.New(t)
	stub.ExpectResponse("origin\tgit@bitbucket.org:myws/myrepo.git (fetch)\n"+
		"origin\tgit@bitbucket.org:myws/myrepo.git (push)\n"+
		"other\tgit@bitbucket.org:other/repo.git (fetch)\n"+
		"other\tgit@bitbucket.org:other/repo.git (push)\n", nil, "remote", "-v")
	stub.Expect("fetch", "--end-of-options", "other", "+refs/heads/feature:refs/remotes/other/feature")
	stub.ExpectResponse("", gittest.Exit(1), "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Expect("checkout", "-b", "feature", "--track", "other/feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CheckoutOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Arg:       "https://bitbucket.org/other/repo/pull-requests/123",
	}
	if err := checkoutRun(t.Context(), opts); err != nil {
		t.Fatalf("checkoutRun: %v", err)
	}
}

func TestCheckoutSameRepoWithoutRemoteFetchesByURL(t *testing.T) {
	t.Parallel()
	stub := gittest.New(t)
	stub.Expect("fetch", "--end-of-options", "https://bitbucket.org/myws/myrepo.git", "+refs/heads/feature:refs/remotes/myws/feature")
	stub.ExpectResponse("", gittest.Exit(1), "rev-parse", "--verify", "--quiet", "refs/heads/feature")
	stub.Expect("checkout", "-b", "feature", "--no-track", "refs/remotes/myws/feature")
	stub.Expect("config", "branch.feature.remote", "https://bitbucket.org/myws/myrepo.git")
	stub.Expect("config", "branch.feature.merge", "refs/heads/feature")

	runCheckout(t, checkoutPR(), nil, stub, nil)
}

func TestCheckoutFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *CheckoutOptions
	cmd := NewCmdCheckout(f, func(o *CheckoutOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "123", "-b", "mine", "-f"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "123" || captured.Branch != "mine" || !captured.Force {
		t.Errorf("parsed = %+v", captured)
	}
}

func TestCheckoutBranchDetachConflict(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdCheckout(f, func(o *CheckoutOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "123", "-b", "mine", "--detach")
	cmdtest.AssertFlagError(t, err, "")
}
