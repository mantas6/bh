package comment

import (
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
)

func newDeleteOpts(srv *apitest.Server, ios *cmdutil.IOStreams) *DeleteOptions {
	return &DeleteOptions{
		IO:        ios,
		ApiClient: clientFunc(srv),
		Git:       gitFunc(newGitStub()),
		BaseRepo:  baseRepoFunc(),
		Now:       nowFunc(),
		Arg:       "123",
		CommentID: 9,
	}
}

func TestDeleteNonTTYWithoutYes(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	// stdin not a TTY by default.
	opts := &DeleteOptions{
		IO:        ios,
		BaseRepo:  baseRepoFunc(),
		Arg:       "123",
		CommentID: 9,
	}
	err := deleteRun(opts)
	if err == nil || !strings.Contains(err.Error(), "--yes required when not running interactively") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteTTYDecline(t *testing.T) {
	srv := apitest.New(t)
	handlePR(srv)

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	in.WriteString("n\n")
	opts := newDeleteOpts(srv, ios)
	err := deleteRun(opts)
	if !errors.Is(err, cmdutil.ErrCancel) {
		t.Fatalf("err = %v, want ErrCancel", err)
	}
	if !strings.Contains(errOut.String(), "Delete comment #9?") {
		t.Errorf("expected prompt, got %q", errOut.String())
	}
	if findRequest(srv, "DELETE", "/comments/9") != nil {
		t.Error("no DELETE should be sent on decline")
	}
}

func TestDeleteTTYConfirm(t *testing.T) {
	srv := apitest.New(t)
	handlePR(srv)
	srv.Handle("DELETE", "/repositories/myws/myrepo/pullrequests/123/comments/9", 204, nil)

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	in.WriteString("y\n")
	opts := newDeleteOpts(srv, ios)
	if err := deleteRun(opts); err != nil {
		t.Fatalf("deleteRun: %v", err)
	}
	if findRequest(srv, "DELETE", "/comments/9") == nil {
		t.Error("expected DELETE request")
	}
	if !strings.Contains(errOut.String(), "Deleted comment #9") {
		t.Errorf("expected success line, got %q", errOut.String())
	}
}

func TestDeleteYesSkipsPrompt(t *testing.T) {
	srv := apitest.New(t)
	handlePR(srv)
	srv.Handle("DELETE", "/repositories/myws/myrepo/pullrequests/123/comments/9", 204, nil)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newDeleteOpts(srv, ios)
	opts.Yes = true
	if err := deleteRun(opts); err != nil {
		t.Fatalf("deleteRun: %v", err)
	}
	if findRequest(srv, "DELETE", "/comments/9") == nil {
		t.Error("expected DELETE request")
	}
}

func TestDeleteFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *DeleteOptions
	cmd := NewCmdDelete(f, func(o *DeleteOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"12", "34", "-y"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "12" || captured.CommentID != 34 || !captured.Yes {
		t.Errorf("parsed = %+v", captured)
	}
}
