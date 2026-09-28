package comment

import (
	"strings"
	"testing"

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
