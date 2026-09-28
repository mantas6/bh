package comment

import (
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
)

func newDeleteOpts(t *testing.T, srv *apitest.Server, ios *cmdutil.IOStreams) *DeleteOptions {
	return &DeleteOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(newGitStub(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		CommentID: 9,
	}
}

func TestDeleteNonTTYWithoutYes(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := cmdutil.TestIOStreams()
	// stdin not a TTY by default.
	opts := &DeleteOptions{
		IO:        ios,
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		CommentID: 9,
	}
	err := deleteRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "--yes required when not running interactively") {
		t.Fatalf("err = %v", err)
	}
}

func TestDeleteTTYDecline(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	in.WriteString("n\n")
	opts := newDeleteOpts(t, srv, ios)
	err := deleteRun(t.Context(), opts)
	if !errors.Is(err, cmdutil.ErrCancel) {
		t.Fatalf("err = %v, want ErrCancel", err)
	}
	if !strings.Contains(errOut.String(), "Delete comment #9?") {
		t.Errorf("expected prompt, got %q", errOut.String())
	}
	if cmdtest.FindRequest(srv, "DELETE", "/comments/9") != nil {
		t.Error("no DELETE should be sent on decline")
	}
}

func TestDeleteTTYConfirm(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("DELETE", "/repositories/myws/myrepo/pullrequests/123/comments/9", 204, nil)

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	in.WriteString("y\n")
	opts := newDeleteOpts(t, srv, ios)
	if err := deleteRun(t.Context(), opts); err != nil {
		t.Fatalf("deleteRun: %v", err)
	}
	if cmdtest.FindRequest(srv, "DELETE", "/comments/9") == nil {
		t.Error("expected DELETE request")
	}
	if !strings.Contains(errOut.String(), "Deleted comment #9") {
		t.Errorf("expected success line, got %q", errOut.String())
	}
}

func TestDeleteYesSkipsPrompt(t *testing.T) {
	t.Parallel()
	// No PR route: a numeric argument must not fetch the pull request.
	srv := apitest.New(t)
	srv.Handle("DELETE", "/repositories/myws/myrepo/pullrequests/123/comments/9", 204, nil)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newDeleteOpts(t, srv, ios)
	opts.Yes = true
	if err := deleteRun(t.Context(), opts); err != nil {
		t.Fatalf("deleteRun: %v", err)
	}
	if cmdtest.FindRequest(srv, "DELETE", "/comments/9") == nil {
		t.Error("expected DELETE request")
	}
	if len(srv.Requests()) != 1 {
		t.Errorf("requests = %v, want only the DELETE", srv.Requests())
	}
}

func TestDeleteFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *DeleteOptions
	cmd := NewCmdDelete(f, func(o *DeleteOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "12", "34", "-y"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "12" || captured.CommentID != 34 || !captured.Yes {
		t.Errorf("parsed = %+v", captured)
	}
}
