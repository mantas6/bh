package comment

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
)

func TestReplyBodyHasParent(t *testing.T) {
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &ReplyOptions{
		IO:        ios,
		ApiClient: clientFunc(srv),
		Git:       gitFunc(newGitStub()),
		BaseRepo:  baseRepoFunc(),
		Now:       nowFunc(),
		Arg:       "123",
		CommentID: 5,
		Body:      "a reply",
	}
	if err := replyRun(opts); err != nil {
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
	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	opts := &ReplyOptions{
		IO:        ios,
		BaseRepo:  baseRepoFunc(),
		Arg:       "123",
		CommentID: 5,
	}
	err := replyRun(opts)
	if err == nil || !strings.Contains(err.Error(), "comment body is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestReplyExactArgs(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdReply(f, func(*ReplyOptions) error { return nil })
	cmd.SetArgs([]string{"123"}) // only one arg
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing comment id")
	}
}
