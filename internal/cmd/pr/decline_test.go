package pr

import (
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
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
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
		ApiClient:    func() (*api.Client, error) { return srv.Client(), nil },
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
		"checkout main",
		"branch -D feature",
		"push origin --delete feature",
	})
}

func TestDeclineNotOpenErrors(t *testing.T) {
	srv := apitest.New(t)
	pr := declinePR()
	pr.State = "DECLINED"
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &DeclineOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
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
