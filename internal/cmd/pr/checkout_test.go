package pr

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

// checkoutPR returns a same-repo PR fixture (source lives in the base repo).
func checkoutPR() *api.PullRequest {
	pr := samplePR()
	pr.Source.Branch.Name = "feature"
	pr.Source.Repository = &api.Repository{FullName: "myws/myrepo"}
	pr.Destination.Branch.Name = "main"
	return pr
}

// forkPR returns a PR whose source branch lives in a fork, carrying clone
// links for both protocols.
func forkPR() *api.PullRequest {
	pr := samplePR()
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
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(stub),
		BaseRepo:  baseRepoFunc(rr),
		Arg:       "123",
	}
	if mutate != nil {
		mutate(opts)
	}
	if err := checkoutRun(opts); err != nil {
		t.Fatalf("checkoutRun: %v", err)
	}
	return stub
}

func assertCalls(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("git calls =\n  %v\nwant\n  %v", got, want)
	}
}

func TestCheckoutSameRepoNewBranch(t *testing.T) {
	stub := gittest.New()
	stub.Register("", &git.GitError{ExitCode: 1}, "rev-parse", "--verify", "--quiet", "refs/heads/feature")

	runCheckout(t, checkoutPR(), originRemote(), stub, nil)

	assertCalls(t, stub.CallStrings(), []string{
		"fetch origin +refs/heads/feature:refs/remotes/origin/feature",
		"rev-parse --verify --quiet refs/heads/feature",
		"checkout -b feature --track origin/feature",
	})
}

func TestCheckoutSameRepoExistingBranch(t *testing.T) {
	stub := gittest.New()
	// rev-parse succeeds -> branch exists.
	stub.Register("abc", nil, "rev-parse", "--verify", "--quiet", "refs/heads/feature")

	runCheckout(t, checkoutPR(), originRemote(), stub, nil)

	assertCalls(t, stub.CallStrings(), []string{
		"fetch origin +refs/heads/feature:refs/remotes/origin/feature",
		"rev-parse --verify --quiet refs/heads/feature",
		"checkout feature",
		"merge --ff-only origin/feature",
	})
}

func TestCheckoutSameRepoExistingBranchForce(t *testing.T) {
	stub := gittest.New()
	stub.Register("abc", nil, "rev-parse", "--verify", "--quiet", "refs/heads/feature")

	runCheckout(t, checkoutPR(), originRemote(), stub, func(o *CheckoutOptions) {
		o.Force = true
	})

	assertCalls(t, stub.CallStrings(), []string{
		"fetch origin +refs/heads/feature:refs/remotes/origin/feature",
		"rev-parse --verify --quiet refs/heads/feature",
		"checkout feature",
		"reset --hard origin/feature",
	})
}

func TestCheckoutSameRepoDetach(t *testing.T) {
	stub := gittest.New()

	runCheckout(t, checkoutPR(), originRemote(), stub, func(o *CheckoutOptions) {
		o.Detach = true
	})

	assertCalls(t, stub.CallStrings(), []string{
		"fetch origin feature",
		"checkout --detach FETCH_HEAD",
	})
}

func TestCheckoutSameRepoBranchName(t *testing.T) {
	stub := gittest.New()
	stub.Register("", &git.GitError{ExitCode: 1}, "rev-parse", "--verify", "--quiet", "refs/heads/mine")

	runCheckout(t, checkoutPR(), originRemote(), stub, func(o *CheckoutOptions) {
		o.Branch = "mine"
	})

	assertCalls(t, stub.CallStrings(), []string{
		"fetch origin +refs/heads/feature:refs/remotes/origin/feature",
		"rev-parse --verify --quiet refs/heads/mine",
		"checkout -b mine --track origin/feature",
	})
}

func TestCheckoutForkSSH(t *testing.T) {
	stub := gittest.New()
	stub.Register("", &git.GitError{ExitCode: 1}, "rev-parse", "--verify", "--quiet", "refs/heads/feature")

	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "git@bitbucket.org:myws/myrepo.git"},
		Repo:   testRepo(),
	}
	runCheckout(t, forkPR(), rr, stub, nil)

	assertCalls(t, stub.CallStrings(), []string{
		"fetch git@bitbucket.org:forkws/myrepo.git +refs/heads/feature:refs/remotes/forkws/feature",
		"rev-parse --verify --quiet refs/heads/feature",
		"checkout -b feature --no-track refs/remotes/forkws/feature",
		"config branch.feature.remote git@bitbucket.org:forkws/myrepo.git",
		"config branch.feature.merge refs/heads/feature",
	})
}

func TestCheckoutForkHTTPS(t *testing.T) {
	stub := gittest.New()
	stub.Register("", &git.GitError{ExitCode: 1}, "rev-parse", "--verify", "--quiet", "refs/heads/feature")

	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "https://bitbucket.org/myws/myrepo.git"},
		Repo:   testRepo(),
	}
	runCheckout(t, forkPR(), rr, stub, nil)

	assertCalls(t, stub.CallStrings(), []string{
		"fetch https://bitbucket.org/forkws/myrepo.git +refs/heads/feature:refs/remotes/forkws/feature",
		"rev-parse --verify --quiet refs/heads/feature",
		"checkout -b feature --no-track refs/remotes/forkws/feature",
		"config branch.feature.remote https://bitbucket.org/forkws/myrepo.git",
		"config branch.feature.merge refs/heads/feature",
	})
}

func TestCheckoutForkDetach(t *testing.T) {
	stub := gittest.New()
	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "https://bitbucket.org/myws/myrepo.git"},
		Repo:   testRepo(),
	}
	runCheckout(t, forkPR(), rr, stub, func(o *CheckoutOptions) {
		o.Detach = true
	})

	assertCalls(t, stub.CallStrings(), []string{
		"fetch https://bitbucket.org/forkws/myrepo.git +refs/heads/feature:refs/remotes/forkws/feature",
		"checkout --detach refs/remotes/forkws/feature",
	})
}

func TestCheckoutForkSynthesizedURL(t *testing.T) {
	// No clone links -> URL is synthesized from the protocol + full name.
	pr := samplePR()
	pr.Source.Branch.Name = "feature"
	pr.Source.Repository = &api.Repository{FullName: "forkws/myrepo"}
	pr.Destination.Branch.Name = "main"

	stub := gittest.New()
	stub.Register("", &git.GitError{ExitCode: 1}, "rev-parse", "--verify", "--quiet", "refs/heads/feature")

	rr := &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "ssh://git@bitbucket.org/myws/myrepo.git"},
		Repo:   testRepo(),
	}
	runCheckout(t, pr, rr, stub, nil)

	assertCalls(t, stub.CallStrings(), []string{
		"fetch git@bitbucket.org:forkws/myrepo.git +refs/heads/feature:refs/remotes/forkws/feature",
		"rev-parse --verify --quiet refs/heads/feature",
		"checkout -b feature --no-track refs/remotes/forkws/feature",
		"config branch.feature.remote git@bitbucket.org:forkws/myrepo.git",
		"config branch.feature.merge refs/heads/feature",
	})
}

func TestCheckoutFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *CheckoutOptions
	cmd := NewCmdCheckout(f, func(o *CheckoutOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"123", "-b", "mine", "-f"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "123" || captured.Branch != "mine" || !captured.Force {
		t.Errorf("parsed = %+v", captured)
	}
}

func TestCheckoutBranchDetachConflict(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdCheckout(f, func(o *CheckoutOptions) error { return nil })
	cmd.SetArgs([]string{"123", "-b", "mine", "--detach"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	err := cmd.Execute()
	var fe *cmdutil.FlagError
	if err == nil || !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}
