package pr

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func TestApprove(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/approve", 200, nil)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &ApproveOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
	if err := approveRun(opts); err != nil {
		t.Fatalf("approveRun: %v", err)
	}

	req := findRequest(srv, "POST", "/pullrequests/123/approve")
	if req == nil {
		t.Fatal("no approve request recorded")
	}
	if !strings.Contains(errOut.String(), "Approved pull request #123") {
		t.Errorf("message = %q", errOut.String())
	}
}

func TestApproveUndo(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("DELETE", "/repositories/myws/myrepo/pullrequests/123/approve", 204, nil)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &ApproveOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
		Undo:      true,
	}
	if err := approveRun(opts); err != nil {
		t.Fatalf("approveRun: %v", err)
	}

	req := findRequest(srv, "DELETE", "/pullrequests/123/approve")
	if req == nil {
		t.Fatal("no unapprove request recorded")
	}
	if !strings.Contains(errOut.String(), "Removed approval from pull request #123") {
		t.Errorf("message = %q", errOut.String())
	}
}

func TestApproveFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *ApproveOptions
	cmd := NewCmdApprove(f, func(o *ApproveOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"123", "--undo"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "123" || !captured.Undo {
		t.Errorf("parsed = %+v", captured)
	}
}
