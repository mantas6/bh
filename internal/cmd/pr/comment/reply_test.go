package comment

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
)

func TestReplyBodyHasParent(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &ReplyOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(newGitStub(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		CommentID: 5,
		Body:      "a reply",
	}
	if err := replyRun(t.Context(), opts); err != nil {
		t.Fatalf("replyRun: %v", err)
	}

	body := createBody(t, srv)
	parent, _ := body["parent"].(map[string]any)
	if parent == nil || parent["id"] != float64(5) {
		t.Fatalf("parent.id = %v, want 5", parent)
	}
	content, _ := body["content"].(map[string]any)
	if content["raw"] != "a reply" {
		t.Errorf("content.raw = %v", content["raw"])
	}
	if !strings.Contains(out.String(), "comment-99") {
		t.Errorf("expected URL, got %q", out.String())
	}
}

func TestReplyMissingBody(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	opts := &ReplyOptions{
		IO:        ios,
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		CommentID: 5,
	}
	err := replyRun(t.Context(), opts)
	cmdtest.AssertFlagError(t, err, "comment body is required")
}

func TestReplyExactArgs(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdReply(f, func(*ReplyOptions) error { return nil })
	if _, _, err := cmdtest.RunCommand(t, cmd, "123"); err == nil {
		t.Fatal("expected error for missing comment id")
	}
}

const commentsBase = "/repositories/myws/myrepo/pullrequests/123/comments"

func newReplyResolveOpts(t *testing.T, srv *apitest.Server, ios *cmdutil.IOStreams, commentID int) *ReplyOptions {
	return &ReplyOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(newGitStub(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		CommentID: commentID,
		Body:      "fixed",
		Resolve:   true,
	}
}

func TestReplyResolve(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)
	srv.Handle("GET", commentsBase+"/5", 200, cmdtest.CommentAt(5, "ada", "root", 60))
	srv.Handle("POST", commentsBase+"/5/resolve", 200, nil)

	ios, _, out, errOut := cmdutil.TestIOStreams()
	if err := replyRun(t.Context(), newReplyResolveOpts(t, srv, ios, 5)); err != nil {
		t.Fatalf("replyRun: %v", err)
	}
	if !strings.Contains(out.String(), "comment-99") {
		t.Errorf("expected the reply URL on stdout, got %q", out.String())
	}
	if got := errOut.String(); got != "✓ Resolved comment #5\n" {
		t.Errorf("stderr = %q", got)
	}
	// The reply is posted before the thread is resolved.
	var order []string
	for _, r := range srv.Requests() {
		if r.Method == "POST" {
			order = append(order, r.Path)
		}
	}
	if len(order) != 2 || order[0] != commentsBase || order[1] != commentsBase+"/5/resolve" {
		t.Errorf("POST order = %v", order)
	}
}

func TestReplyResolveNestedResolvesThreadRoot(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)
	nested := cmdtest.CommentAt(7, "bob", "nested", 30)
	nested.Parent = &api.CommentRef{ID: 6}
	mid := cmdtest.CommentAt(6, "ada", "reply", 40)
	mid.Parent = &api.CommentRef{ID: 5}
	srv.Handle("GET", commentsBase+"/7", 200, nested)
	srv.Handle("GET", commentsBase+"/6", 200, mid)
	srv.Handle("GET", commentsBase+"/5", 200, cmdtest.CommentAt(5, "ada", "root", 60))
	srv.Handle("POST", commentsBase+"/5/resolve", 200, nil)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	if err := replyRun(t.Context(), newReplyResolveOpts(t, srv, ios, 7)); err != nil {
		t.Fatalf("replyRun: %v", err)
	}
	if body := createBody(t, srv); body["parent"].(map[string]any)["id"] != float64(7) {
		t.Errorf("reply parent = %v, want 7", body["parent"])
	}
	if got := errOut.String(); got != "✓ Resolved comment #5\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestReplyResolveAlreadyResolved(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)
	root := cmdtest.CommentAt(5, "ada", "root", 60)
	root.Resolution = &api.Resolution{Type: "resolved"}
	srv.Handle("GET", commentsBase+"/5", 200, root)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	if err := replyRun(t.Context(), newReplyResolveOpts(t, srv, ios, 5)); err != nil {
		t.Fatalf("replyRun: %v", err)
	}
	if cmdtest.FindRequest(srv, "POST", "/resolve") != nil {
		t.Error("an already resolved thread should not be resolved again")
	}
	if got := errOut.String(); got != "✓ Resolved comment #5\n" {
		t.Errorf("stderr = %q", got)
	}
}

func TestReplyFailureDoesNotResolve(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handlePR(srv)
	srv.Handle("POST", commentsBase, 400,
		map[string]any{"type": "error", "error": map[string]any{"message": "Bad parent"}})

	ios, _, out, _ := cmdutil.TestIOStreams()
	err := replyRun(t.Context(), newReplyResolveOpts(t, srv, ios, 5))
	if err == nil || !strings.Contains(err.Error(), "Bad parent") {
		t.Fatalf("err = %v, want the reply error", err)
	}
	for _, r := range srv.Requests() {
		if strings.HasPrefix(r.Path, commentsBase+"/") {
			t.Errorf("unexpected request after a failed reply: %s %s", r.Method, r.Path)
		}
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
}

func TestReplyResolveFailureAfterReply(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)
	srv.Handle("GET", commentsBase+"/5", 200, cmdtest.CommentAt(5, "ada", "root", 60))
	srv.Handle("POST", commentsBase+"/5/resolve", 403,
		map[string]any{"type": "error", "error": map[string]any{"message": "Forbidden"}})

	ios, _, out, _ := cmdutil.TestIOStreams()
	err := replyRun(t.Context(), newReplyResolveOpts(t, srv, ios, 5))
	if err == nil || !strings.Contains(err.Error(), "reply posted, but resolving the thread of comment #5 failed") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "comment-99") {
		t.Errorf("the reply URL should still be printed, got %q", out.String())
	}
}

func TestReplyWithoutResolveDoesNotResolve(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := newReplyResolveOpts(t, srv, ios, 5)
	opts.Resolve = false
	if err := replyRun(t.Context(), opts); err != nil {
		t.Fatalf("replyRun: %v", err)
	}
	if n := len(srv.Requests()); n != 2 {
		t.Errorf("requests = %d, want PR lookup and reply only", n)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}

func TestReplyResolveFlag(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	var captured *ReplyOptions
	cmd := NewCmdReply(f, func(o *ReplyOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "123", "5", "--body", "x", "--resolve"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !captured.Resolve || captured.CommentID != 5 {
		t.Errorf("parsed = %+v", captured)
	}
}
