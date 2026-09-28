package pr

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func declinePR() *api.PullRequest {
	pr := samplePR()
	pr.State = "OPEN"
	pr.Source.Branch.Name = "feature"
	pr.Source.Repository = &api.Repository{FullName: "myws/myrepo"}
	pr.Destination.Branch.Name = "main"
	return pr
}

func TestDecline(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, declinePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/decline", 200, declinePR())

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &DeclineOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
	if err := declineRun(opts); err != nil {
		t.Fatalf("declineRun: %v", err)
	}
	if findRequest(srv, "POST", "/pullrequests/123/decline") == nil {
		t.Fatal("no decline request")
	}
	if !strings.Contains(errOut.String(), "Declined pull request #123") {
		t.Errorf("message = %q", errOut.String())
	}
}

func TestDeclineDeleteBranch(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, declinePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/decline", 200, declinePR())

	stub := gittest.New()
	stub.Register("feature", nil, "symbolic-ref", "--quiet", "--short", "HEAD")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &DeclineOptions{
		IO:           ios,
		APIClient:    func() (*api.Client, error) { return srv.Client(), nil },
		Git:          gitFunc(stub),
		BaseRepo:     baseRepoFunc(originRemote()),
		Arg:          "123",
		DeleteBranch: true,
	}
	if err := declineRun(opts); err != nil {
		t.Fatalf("declineRun: %v", err)
	}

	assertCalls(t, stub.CallStrings(), []string{
		"symbolic-ref --quiet --short HEAD",
		"checkout --end-of-options main",
		"branch -d --end-of-options feature",
		"push origin --delete feature",
	})
}

// newDeclineDeleteOpts sets up a decline --delete-branch run where the source
// branch exists locally but `git branch -d` refuses to delete it.
func newDeclineDeleteOpts(t *testing.T) (opts *DeclineOptions, stub *gittest.Stub, in, errOut *bytes.Buffer) {
	t.Helper()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, declinePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/decline", 200, declinePR())

	stub = gittest.New()
	stub.Register("main", nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	stub.Register("", gittest.Exit(1), "branch", "-d", "--end-of-options", "feature")

	ios, in, _, errOut := cmdutil.TestIOStreams()
	opts = &DeclineOptions{
		IO:           ios,
		APIClient:    func() (*api.Client, error) { return srv.Client(), nil },
		Git:          gitFunc(stub),
		BaseRepo:     baseRepoFunc(originRemote()),
		Arg:          "123",
		DeleteBranch: true,
	}
	return opts, stub, in, errOut
}

func TestDeclineDeleteBranchUnmergedNonTTYWarns(t *testing.T) {
	opts, stub, _, errOut := newDeclineDeleteOpts(t)

	if err := declineRun(opts); err != nil {
		t.Fatalf("declineRun: %v", err)
	}
	assertCalls(t, stub.CallStrings(), []string{
		"symbolic-ref --quiet --short HEAD",
		"rev-parse --verify --quiet refs/heads/feature",
		"branch -d --end-of-options feature",
		"push origin --delete feature",
	})
	if !strings.Contains(errOut.String(), "Kept local branch feature because it is not fully merged") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestDeclineDeleteBranchUnmergedTTYForceConfirmed(t *testing.T) {
	opts, stub, in, errOut := newDeclineDeleteOpts(t)
	opts.IO.SetStdinTTY(true)
	in.WriteString("y\n")

	if err := declineRun(opts); err != nil {
		t.Fatalf("declineRun: %v", err)
	}
	assertCalls(t, stub.CallStrings(), []string{
		"symbolic-ref --quiet --short HEAD",
		"rev-parse --verify --quiet refs/heads/feature",
		"branch -d --end-of-options feature",
		"branch -D --end-of-options feature",
		"push origin --delete feature",
	})
	got := errOut.String()
	if !strings.Contains(got, "Local branch feature is not fully merged. Delete it anyway? [y/N]") {
		t.Errorf("prompt missing: %q", got)
	}
	if !strings.Contains(got, "Deleted local branch feature") {
		t.Errorf("stderr = %q", got)
	}
}

func TestDeclineDeleteBranchUnmergedTTYDeclined(t *testing.T) {
	opts, stub, in, _ := newDeclineDeleteOpts(t)
	opts.IO.SetStdinTTY(true)
	in.WriteString("n\n")

	if err := declineRun(opts); err != nil {
		t.Fatalf("declineRun: %v", err)
	}
	for _, c := range stub.CallStrings() {
		if c == "branch -D --end-of-options feature" {
			t.Errorf("branch force-deleted despite answering no")
		}
	}
}

func TestDeclineDeleteBranchForkSkipped(t *testing.T) {
	srv := apitest.New(t)
	pr := declinePR()
	pr.Source.Repository = &api.Repository{FullName: "fork/myrepo"}
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/decline", 200, pr)

	stub := gittest.New()
	stub.FailUnstubbed = true

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &DeclineOptions{
		IO:           ios,
		APIClient:    func() (*api.Client, error) { return srv.Client(), nil },
		Git:          gitFunc(stub),
		BaseRepo:     baseRepoFunc(originRemote()),
		Arg:          "123",
		DeleteBranch: true,
	}
	if err := declineRun(opts); err != nil {
		t.Fatalf("declineRun: %v", err)
	}
	if len(stub.Calls) != 0 {
		t.Errorf("unexpected git calls: %v", stub.CallStrings())
	}
	if !strings.Contains(errOut.String(), "Skipped deleting branch feature") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestDeclineNotOpenErrors(t *testing.T) {
	srv := apitest.New(t)
	pr := declinePR()
	pr.State = "DECLINED"
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &DeclineOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
	err := declineRun(opts)
	if err == nil || !strings.Contains(err.Error(), "pull request #123 is declined") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeclineFlagParsingAndAlias(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *DeclineOptions
	cmd := NewCmdDecline(f, func(o *DeclineOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"123", "-d"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "123" || !captured.DeleteBranch {
		t.Errorf("parsed = %+v", captured)
	}

	hasClose := false
	for _, a := range cmd.Aliases {
		if a == "close" {
			hasClose = true
		}
	}
	if !hasClose {
		t.Errorf("expected 'close' alias, got %v", cmd.Aliases)
	}
}

func TestDeclineDeleteBranchWithoutRemoteErrors(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, declinePR())

	stub := gittest.New()
	stub.FailUnstubbed = true

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &DeclineOptions{
		IO:           ios,
		APIClient:    func() (*api.Client, error) { return srv.Client(), nil },
		Git:          gitFunc(stub),
		BaseRepo:     baseRepoFunc(nil),
		Arg:          "123",
		DeleteBranch: true,
	}
	err := declineRun(opts)
	if err == nil || !strings.Contains(err.Error(), "no git remote found for myws/myrepo") {
		t.Fatalf("err = %v", err)
	}
	if findRequest(srv, "POST", "/pullrequests/123/decline") != nil {
		t.Error("PR was declined despite the error")
	}
	if len(stub.Calls) != 0 {
		t.Errorf("unexpected git calls: %v", stub.CallStrings())
	}
}
