package comment

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
)

// listComments registers the PR lookup and comment list responses.
func listComments(srv *apitest.Server, comments []api.Comment) {
	handlePR(srv)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123/comments", 200,
		map[string]any{"values": comments})
}

func newListOpts(srv *apitest.Server, ios *cmdutil.IOStreams) *ListOptions {
	return &ListOptions{
		IO:        ios,
		ApiClient: clientFunc(srv),
		Git:       gitFunc(newGitStub()),
		BaseRepo:  baseRepoFunc(),
		Now:       nowFunc(),
		Arg:       "123",
	}
}

func TestListThreadedWithInlineAndResolved(t *testing.T) {
	srv := apitest.New(t)

	root := commentAt(1, "ada", "Looks good overall", 60)
	root.Inline = &api.Inline{Path: "main.go", To: intPtr(42)}
	root.Resolution = &api.Resolution{Type: "resolved"}

	reply := commentAt(2, "bob", "Thanks!", 30)
	reply.Parent = &api.CommentRef{ID: 1}

	deleted := commentAt(3, "eve", "gone", 10)
	deleted.Deleted = true

	listComments(srv, []api.Comment{reply, root, deleted})

	ios, _, out, _ := cmdutil.TestIOStreams()
	if err := listRun(newListOpts(srv, ios)); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "#1 ada") {
		t.Errorf("missing root header:\n%s", got)
	}
	if !strings.Contains(got, "main.go:42") {
		t.Errorf("missing inline anchor:\n%s", got)
	}
	if !strings.Contains(got, "[resolved]") {
		t.Errorf("missing resolved marker:\n%s", got)
	}
	if !strings.Contains(got, "  Looks good overall") {
		t.Errorf("root body not indented:\n%s", got)
	}
	// reply nested: header indented 2, body indented 4.
	if !strings.Contains(got, "  #2 bob") {
		t.Errorf("reply header not indented:\n%s", got)
	}
	if !strings.Contains(got, "    Thanks!") {
		t.Errorf("reply body not indented:\n%s", got)
	}
	if strings.Contains(got, "gone") || strings.Contains(got, "#3") {
		t.Errorf("deleted comment should be hidden:\n%s", got)
	}
	// root must come before reply.
	if strings.Index(got, "#1 ada") > strings.Index(got, "#2 bob") {
		t.Errorf("root should precede reply:\n%s", got)
	}
}

func TestListInlineOldSide(t *testing.T) {
	srv := apitest.New(t)
	c := commentAt(1, "ada", "old line", 5)
	c.Inline = &api.Inline{Path: "old.go", From: intPtr(7)}
	listComments(srv, []api.Comment{c})

	ios, _, out, _ := cmdutil.TestIOStreams()
	if err := listRun(newListOpts(srv, ios)); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "old old.go:7") {
		t.Errorf("expected old-side anchor:\n%s", out.String())
	}
}

func TestListUnresolvedFilter(t *testing.T) {
	srv := apitest.New(t)
	resolved := commentAt(1, "ada", "resolved thread", 60)
	resolved.Resolution = &api.Resolution{Type: "resolved"}
	open := commentAt(2, "bob", "open thread", 30)
	listComments(srv, []api.Comment{resolved, open})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(srv, ios)
	opts.Unresolved = true
	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "resolved thread") {
		t.Errorf("resolved thread should be hidden:\n%s", got)
	}
	if !strings.Contains(got, "open thread") {
		t.Errorf("open thread should be shown:\n%s", got)
	}
}

func TestListEmpty(t *testing.T) {
	srv := apitest.New(t)
	listComments(srv, []api.Comment{})

	ios, _, out, _ := cmdutil.TestIOStreams()
	if err := listRun(newListOpts(srv, ios)); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "No comments on pull request #123") {
		t.Errorf("expected empty message, got %q", got)
	}
}

func TestListJSON(t *testing.T) {
	srv := apitest.New(t)
	resolved := commentAt(1, "ada", "resolved", 60)
	resolved.Resolution = &api.Resolution{Type: "resolved"}
	open := commentAt(2, "bob", "open", 30)
	deleted := commentAt(3, "eve", "gone", 10)
	deleted.Deleted = true
	listComments(srv, []api.Comment{resolved, open, deleted})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(srv, ios)
	opts.JSON = true
	opts.Unresolved = true
	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	var got []api.Comment
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("expected only open comment #2, got %+v", got)
	}
}

func TestListCurrentBranch(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		map[string]any{"values": []*api.PullRequest{samplePR()}})
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123/comments", 200,
		map[string]any{"values": []api.Comment{commentAt(1, "ada", "hi", 5)}})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(srv, ios)
	opts.Arg = ""
	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "#1 ada") {
		t.Errorf("expected comment, got %q", out.String())
	}
}

func TestListFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *ListOptions
	cmd := NewCmdList(f, func(o *ListOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"55", "--json", "--unresolved", "-L", "10"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "55" || !captured.JSON || !captured.Unresolved || captured.Limit != 10 {
		t.Errorf("parsed = %+v", captured)
	}
}
