package pr

import (
	"context"
	"errors"
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
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
	if err := approveRun(t.Context(), opts); err != nil {
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
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
		Undo:      true,
	}
	if err := approveRun(t.Context(), opts); err != nil {
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

// Icons written to stderr follow stderr's TTY state, so `2>log` stays plain
// even when stdout is a terminal.
func TestApproveStderrIconUncolouredWhenRedirected(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/approve", 200, nil)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(false)
	opts := &ApproveOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
	if err := approveRun(t.Context(), opts); err != nil {
		t.Fatalf("approveRun: %v", err)
	}
	if got := errOut.String(); strings.Contains(got, "\x1b[") || !strings.HasPrefix(got, "✓ ") {
		t.Errorf("stderr = %q, want a plain icon", got)
	}
}

func TestApproveTooManyArgs(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdApprove(f, func(*ApproveOptions) error { return nil })
	cmd.SetArgs([]string{"1", "2"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	var fe *cmdutil.FlagError
	if err := cmd.Execute(); !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}

// The command context reaches the API client: a cancelled context aborts the
// command before any request is sent.
func TestApproveCancelledContext(t *testing.T) {
	srv := apitest.New(t) // no routes: any request that arrives fails the test

	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{
		IOStreams: ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
	}
	cmd := NewCmdApprove(f, nil)
	cmd.SetArgs([]string{"123"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := cmd.ExecuteContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(srv.Requests) != 0 {
		t.Errorf("requests = %v, want none", srv.Requests)
	}
}
