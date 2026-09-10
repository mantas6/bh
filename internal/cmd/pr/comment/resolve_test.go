package comment

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
)

func newResolveOpts(srv *apitest.Server, ios *cmdutil.IOStreams) *ResolveOptions {
	return &ResolveOptions{
		IO:        ios,
		ApiClient: clientFunc(srv),
		Git:       gitFunc(newGitStub()),
		BaseRepo:  baseRepoFunc(),
		Now:       nowFunc(),
		Arg:       "123",
		CommentID: 9,
	}
}

func TestResolve(t *testing.T) {
	srv := apitest.New(t)
	handlePR(srv)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/comments/9/resolve", 200, nil)

	ios, _, out, _ := cmdutil.TestIOStreams()
	if err := resolveRun(newResolveOpts(srv, ios), false); err != nil {
		t.Fatalf("resolveRun: %v", err)
	}
	if findRequest(srv, "POST", "/comments/9/resolve") == nil {
		t.Error("expected POST .../resolve")
	}
	if !strings.Contains(out.String(), "Resolved comment #9") {
		t.Errorf("expected success line, got %q", out.String())
	}
}

func TestReopen(t *testing.T) {
	srv := apitest.New(t)
	handlePR(srv)
	srv.Handle("DELETE", "/repositories/myws/myrepo/pullrequests/123/comments/9/resolve", 204, nil)

	ios, _, out, _ := cmdutil.TestIOStreams()
	if err := resolveRun(newResolveOpts(srv, ios), true); err != nil {
		t.Fatalf("resolveRun reopen: %v", err)
	}
	if findRequest(srv, "DELETE", "/comments/9/resolve") == nil {
		t.Error("expected DELETE .../resolve")
	}
	if !strings.Contains(out.String(), "Reopened comment #9") {
		t.Errorf("expected success line, got %q", out.String())
	}
}

func TestResolveExactArgs(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdResolve(f, func(*ResolveOptions) error { return nil })
	cmd.SetArgs([]string{"123"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing comment id")
	}
}

func TestReopenParsesArgs(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *ResolveOptions
	cmd := NewCmdReopen(f, func(o *ResolveOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"42", "7"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "42" || captured.CommentID != 7 {
		t.Errorf("parsed = %+v", captured)
	}
}
