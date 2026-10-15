package comment

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
)

// jsonIDs decodes `comment list --json` output into its comment ids.
func jsonIDs(t *testing.T, b []byte) []int {
	t.Helper()
	var got []api.Comment
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json: %v\n%s", err, b)
	}
	ids := []int{}
	for _, c := range got {
		ids = append(ids, c.ID)
	}
	return ids
}

// listComments registers the PR lookup and comment list responses.
func listComments(srv *apitest.Server, comments []api.Comment) {
	handlePR(srv)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123/comments", 200,
		map[string]any{"values": comments})
}

func newListOpts(t *testing.T, srv *apitest.Server, ios *cmdutil.IOStreams) *ListOptions {
	return &ListOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(newGitStub(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Now:       cmdtest.FixedNow,
		Arg:       "123",
	}
}

func TestListThreadedWithInlineAndResolved(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	root := cmdtest.CommentAt(1, "ada", "Looks good overall", 60)
	root.Inline = &api.Inline{Path: "main.go", To: new(42)}
	root.Resolution = &api.Resolution{Type: "resolved"}

	reply := cmdtest.CommentAt(2, "bob", "Thanks!", 30)
	reply.Parent = &api.CommentRef{ID: 1}

	deleted := cmdtest.CommentAt(3, "eve", "gone", 10)
	deleted.Deleted = true

	listComments(srv, []api.Comment{reply, root, deleted})

	ios, _, out, _ := cmdutil.TestIOStreams()
	if err := listRun(t.Context(), newListOpts(t, srv, ios)); err != nil {
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
	t.Parallel()
	srv := apitest.New(t)
	c := cmdtest.CommentAt(1, "ada", "old line", 5)
	c.Inline = &api.Inline{Path: "old.go", From: new(7)}
	listComments(srv, []api.Comment{c})

	ios, _, out, _ := cmdutil.TestIOStreams()
	if err := listRun(t.Context(), newListOpts(t, srv, ios)); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "old old.go:7") {
		t.Errorf("expected old-side anchor:\n%s", out.String())
	}
}

func TestListUnresolvedFilter(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	resolved := cmdtest.CommentAt(1, "ada", "resolved thread", 60)
	resolved.Resolution = &api.Resolution{Type: "resolved"}
	open := cmdtest.CommentAt(2, "bob", "open thread", 30)
	listComments(srv, []api.Comment{resolved, open})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.Unresolved = true
	if err := listRun(t.Context(), opts); err != nil {
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
	t.Parallel()
	srv := apitest.New(t)
	listComments(srv, []api.Comment{})

	ios, _, out, _ := cmdutil.TestIOStreams()
	if err := listRun(t.Context(), newListOpts(t, srv, ios)); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "No comments on pull request #123") {
		t.Errorf("expected empty message, got %q", got)
	}
}

func TestListJSON(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	resolved := cmdtest.CommentAt(1, "ada", "resolved", 60)
	resolved.Resolution = &api.Resolution{Type: "resolved"}
	open := cmdtest.CommentAt(2, "bob", "open", 30)
	deleted := cmdtest.CommentAt(3, "eve", "gone", 10)
	deleted.Deleted = true
	listComments(srv, []api.Comment{resolved, open, deleted})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.JSON = true
	opts.Unresolved = true
	if err := listRun(t.Context(), opts); err != nil {
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

func TestListJSONDerivedFields(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	root := cmdtest.CommentAt(1, "ada", "Rename this", 60)
	root.Inline = &api.Inline{Path: "main.go", To: new(42)}
	root.Resolution = &api.Resolution{Type: "resolved"}
	reply := cmdtest.CommentAt(2, "bob", "Done", 30)
	reply.Parent = &api.CommentRef{ID: 1}
	general := cmdtest.CommentAt(3, "cara", "Why?", 10)
	listComments(srv, []api.Comment{root, reply, general})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.JSON = true
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	var got []map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(got) != 3 {
		t.Fatalf("got %d comments, want 3:\n%s", len(got), out.String())
	}
	want := []struct {
		id, thread float64
		resolved   bool
		reply      bool
	}{
		{1, 1, true, false},
		{2, 1, true, true},
		{3, 3, false, false},
	}
	for i, w := range want {
		c := got[i]
		if c["id"] != w.id || c["thread_id"] != w.thread || c["resolved"] != w.resolved || c["is_reply"] != w.reply {
			t.Errorf("comment %d = id %v thread_id %v resolved %v is_reply %v, want %+v",
				i, c["id"], c["thread_id"], c["resolved"], c["is_reply"], w)
		}
	}
	// Raw fields are kept; unset references are omitted, not null.
	if got[0]["resolution"] == nil || got[0]["inline"] == nil {
		t.Errorf("root should keep resolution and inline: %#v", got[0])
	}
	if got[1]["parent"].(map[string]any)["id"] != float64(1) {
		t.Errorf("reply should keep parent: %#v", got[1])
	}
	for _, key := range []string{"parent", "inline", "resolution"} {
		if _, ok := got[2][key]; ok {
			t.Errorf("general comment should omit %q: %#v", key, got[2])
		}
	}
}

func TestListJSONEmptyIsArray(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	listComments(srv, []api.Comment{})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.JSON = true
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got := out.String(); got != "[]\n" {
		t.Errorf("output = %q, want []", got)
	}
}

// threadedComments returns two threads: #1 (with replies #2, #3) and #4.
func threadedComments() []api.Comment {
	root1 := cmdtest.CommentAt(1, "ada", "first thread", 60)
	reply1 := cmdtest.CommentAt(2, "bob", "reply one", 50)
	reply1.Parent = &api.CommentRef{ID: 1}
	reply2 := cmdtest.CommentAt(3, "cara", "reply two", 40)
	reply2.Parent = &api.CommentRef{ID: 1}
	root2 := cmdtest.CommentAt(4, "dan", "second thread", 30)
	return []api.Comment{root1, reply1, reply2, root2}
}

func TestListLimitCountsThreads(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	listComments(srv, threadedComments())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.Limit = 1
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "second thread") {
		t.Errorf("--limit 1 should keep the newest thread:\n%s", got)
	}
	for _, old := range []string{"first thread", "reply one", "reply two"} {
		if strings.Contains(got, old) {
			t.Errorf("%q from the oldest thread should be cut by --limit 1:\n%s", old, got)
		}
	}

	// All comments are fetched; the limit is not passed to the API.
	req := cmdtest.FindRequest(srv, "GET", "/pullrequests/123/comments")
	if pl := req.Query.Get("pagelen"); pl != "50" {
		t.Errorf("pagelen = %q, want 50 (unlimited fetch)", pl)
	}
}

func TestListLimitJSONCountsThreads(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	listComments(srv, threadedComments())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.Limit = 2
	opts.JSON = true
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got := jsonIDs(t, out.Bytes()); !slices.Equal(got, []int{1, 2, 3, 4}) {
		t.Errorf("ids = %v, want [1 2 3 4]", got)
	}

	out.Reset()
	opts.Limit = 1
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got := jsonIDs(t, out.Bytes()); !slices.Equal(got, []int{4}) {
		t.Errorf("ids = %v, want [4] (the newest thread)", got)
	}
}

func TestListNilNow(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	listComments(srv, []api.Comment{cmdtest.CommentAt(1, "ada", "hi", 5)})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.Now = nil
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "#1 ada") {
		t.Errorf("output = %q", out.String())
	}
}

func TestListNegativeLimitFlagError(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdList(f, func(*ListOptions) error {
		t.Error("runF should not be called")
		return nil
	})
	_, _, err := cmdtest.RunCommand(t, cmd, "123", "-L", "-1")
	cmdtest.AssertFlagError(t, err, "")
}

func TestListCurrentBranch(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		map[string]any{"values": []*api.PullRequest{cmdtest.SamplePR()}})
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123/comments", 200,
		map[string]any{"values": []api.Comment{cmdtest.CommentAt(1, "ada", "hi", 5)}})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.Arg = ""
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "#1 ada") {
		t.Errorf("expected comment, got %q", out.String())
	}
}

func TestListFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *ListOptions
	cmd := NewCmdList(f, func(o *ListOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "55", "--json", "--unresolved", "-L", "10"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "55" || !captured.JSON || !captured.Unresolved || captured.Limit != 10 {
		t.Errorf("parsed = %+v", captured)
	}
}

func TestListByBranchWithoutGit(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		map[string]any{"values": []*api.PullRequest{cmdtest.SamplePR()}})
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123/comments", 200,
		map[string]any{"values": []api.Comment{cmdtest.CommentAt(1, "ada", "hi", 5)}})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.Arg = "feature"
	opts.Git = nil // a branch selector must not need git
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); !strings.Contains(q, `source.branch.name="feature"`) {
		t.Errorf("q = %q", q)
	}
	if !strings.Contains(out.String(), "#1 ada") {
		t.Errorf("expected comment, got %q", out.String())
	}
}

func TestListByURLWithoutGitOrBaseRepo(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/other/repo/pullrequests/9", 200, cmdtest.SamplePR())
	srv.Handle("GET", "/repositories/other/repo/pullrequests/123/comments", 200,
		map[string]any{"values": []api.Comment{cmdtest.CommentAt(1, "ada", "hi", 5)}})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.Arg = "https://bitbucket.org/other/repo/pull-requests/9"
	opts.Git = nil
	opts.BaseRepo = nil
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "#1 ada") {
		t.Errorf("expected comment, got %q", out.String())
	}
}

// ttyNoColor makes stdout a terminal without colour.
func ttyNoColor(ios *cmdutil.IOStreams) {
	ios.SetStdoutTTY(true)
	ios.Getenv = func(k string) string {
		if k == "NO_COLOR" {
			return "1"
		}
		return ""
	}
}

// TestListExactOutput locks the text format agents parse:
// `#id author · time · path:line [resolved]`, indented bodies, nested replies.
func TestListExactOutput(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	root := cmdtest.CommentAt(1, "ada", "Rename this", 120)
	root.Inline = &api.Inline{Path: "main.go", To: new(42)}
	root.Resolution = &api.Resolution{Type: "resolved"}
	reply := cmdtest.CommentAt(2, "bob", "Done", 30)
	reply.Parent = &api.CommentRef{ID: 1}
	open := cmdtest.CommentAt(3, "cara", "Why?", 5)
	listComments(srv, []api.Comment{root, reply, open})

	ios, _, out, _ := cmdutil.TestIOStreams()
	ttyNoColor(ios)
	if err := listRun(t.Context(), newListOpts(t, srv, ios)); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	want := "#1 ada · about 2 hours ago · main.go:42 [resolved]\n" +
		"  Rename this\n" +
		"\n" +
		"  #2 bob · 30 minutes ago\n" +
		"    Done\n" +
		"\n" +
		"#3 cara · 5 minutes ago\n" +
		"  Why?\n" +
		"\n"
	if got := out.String(); got != want {
		t.Errorf("output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestListJSONFields(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	root := cmdtest.CommentAt(1, "ada", "Rename this", 60)
	root.Resolution = &api.Resolution{Type: "resolved"}
	reply := cmdtest.CommentAt(2, "bob", "Done", 30)
	reply.Parent = &api.CommentRef{ID: 1}
	listComments(srv, []api.Comment{root, reply})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.JSON = true
	opts.JSONOutput = cmdutil.JSONOutput{Fields: []string{"id", "thread_id", "resolved", "user.nickname"}, JQ: "map([.id, .thread_id, .resolved, .user.nickname])"}
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got, want := out.String(), `[[1,1,true,"ada"],[2,1,true,"bob"]]`+"\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestListAbsoluteTimesWhenPiped(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	root := cmdtest.CommentAt(1, "ada", "Rename this", 120)
	root.Inline = &api.Inline{Path: "main.go", To: new(42)}
	root.Resolution = &api.Resolution{Type: "resolved"}
	reply := cmdtest.CommentAt(2, "bob", "Done", 30)
	reply.Parent = &api.CommentRef{ID: 1}
	listComments(srv, []api.Comment{root, reply})

	ios, _, out, _ := cmdutil.TestIOStreams() // stdout is not a terminal
	if err := listRun(t.Context(), newListOpts(t, srv, ios)); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	stamp := func(c api.Comment) string { return c.CreatedOn.Local().Format(time.RFC3339) }
	want := "#1 ada · " + stamp(root) + " · main.go:42 [resolved]\n" +
		"  Rename this\n" +
		"\n" +
		"  #2 bob · " + stamp(reply) + "\n" +
		"    Done\n" +
		"\n"
	if got := out.String(); got != want {
		t.Errorf("output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

// commentBy returns a comment by the cmdtest.User fixture for nickname, which
// carries a UUID like the API's comments do.
func commentBy(id int, nickname, raw string, minutesAgo int, parent int) api.Comment {
	c := cmdtest.CommentAt(id, nickname, raw, minutesAgo)
	c.User = cmdtest.User(nickname)
	if parent > 0 {
		c.Parent = &api.CommentRef{ID: parent}
	}
	return c
}

// reviewComments returns threads as seen by "me":
//
//	#1 bob, unanswered                      → needs a reply
//	#2 bob, answered by me (#3)             → answered
//	#4 bob, me (#5), bob again (#6)         → needs a reply
//	#7 bob, resolved                        → resolved
//	#8 me, unanswered                       → mine
func reviewComments() []api.Comment {
	resolved := commentBy(7, "bob", "resolved", 20, 0)
	resolved.Resolution = &api.Resolution{Type: "resolved"}
	return []api.Comment{
		commentBy(1, "bob", "unanswered", 100, 0),
		commentBy(2, "bob", "answered", 90, 0),
		commentBy(3, "me", "fixed", 80, 2),
		commentBy(4, "bob", "follow-up", 70, 0),
		commentBy(5, "me", "fixed too", 60, 4),
		commentBy(6, "bob", "not quite", 50, 5),
		resolved,
		commentBy(8, "me", "note to self", 10, 0),
	}
}

func listReview(t *testing.T, setup func(*ListOptions)) ([]int, *apitest.Server) {
	t.Helper()
	srv := apitest.New(t)
	listComments(srv, reviewComments())
	srv.Handle("GET", "/user", 200, cmdtest.User("me"))

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.JSON = true
	setup(opts)
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	return jsonIDs(t, out.Bytes()), srv
}

func TestListNeedsReply(t *testing.T) {
	t.Parallel()
	ids, srv := listReview(t, func(o *ListOptions) { o.NeedsReply = true })
	if want := []int{1, 4, 5, 6}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
	cmdtest.RequireRequest(t, srv, "GET", "/user")
}

func TestListNeedsReplyWithLimitKeepsNewest(t *testing.T) {
	t.Parallel()
	ids, _ := listReview(t, func(o *ListOptions) {
		o.NeedsReply = true
		o.Limit = 1
	})
	if want := []int{4, 5, 6}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestListWithoutUserFiltersSkipsUserLookup(t *testing.T) {
	t.Parallel()
	_, srv := listReview(t, func(o *ListOptions) { o.Author = "bob" })
	if cmdtest.FindRequest(srv, "GET", "/user") != nil {
		t.Error("the current user should only be fetched for --needs-reply or --author @me")
	}
}

func TestListAuthor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		author string
		want   []int
	}{
		{"@me", []int{8}},
		{"bob", []int{1, 2, 3, 4, 5, 6, 7}},
		{"BOB", []int{1, 2, 3, 4, 5, 6, 7}}, // case-insensitive
		{"Me", []int{8}},                    // display name
		{"{me-uuid}", []int{8}},
		{"nobody", []int{}},
	}
	for _, tt := range tests {
		ids, _ := listReview(t, func(o *ListOptions) { o.Author = tt.author })
		if !slices.Equal(ids, tt.want) {
			t.Errorf("--author %s: ids = %v, want %v", tt.author, ids, tt.want)
		}
	}
}

func TestListSince(t *testing.T) {
	t.Parallel()
	now := cmdtest.FixedNow()
	tests := []struct {
		since string
		want  []int
	}{
		// A thread with any newer comment is shown complete.
		{"55m", []int{4, 5, 6, 7, 8}},
		{"15m", []int{8}},
		{now.Add(-25 * time.Minute).Format(time.RFC3339), []int{7, 8}},
		{"5", []int{4, 5, 6, 7, 8}}, // comment id
		{"#6", []int{7, 8}},         // comment id with #
		{"1d", []int{1, 2, 3, 4, 5, 6, 7, 8}},
	}
	for _, tt := range tests {
		ids, _ := listReview(t, func(o *ListOptions) { o.Since = tt.since })
		if !slices.Equal(ids, tt.want) {
			t.Errorf("--since %s: ids = %v, want %v", tt.since, ids, tt.want)
		}
	}
}

func TestListTopLevel(t *testing.T) {
	t.Parallel()
	ids, _ := listReview(t, func(o *ListOptions) { o.TopLevel = true })
	if want := []int{1, 2, 4, 7, 8}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestListTopLevelAppliesAfterNeedsReply(t *testing.T) {
	t.Parallel()
	// #4's last comment (#6) is a reply, yet the thread needs a reply.
	ids, _ := listReview(t, func(o *ListOptions) {
		o.NeedsReply = true
		o.TopLevel = true
	})
	if want := []int{1, 4}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestListTopLevelText(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	listComments(srv, threadedComments())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.TopLevel = true
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	got := out.String()
	if strings.Contains(got, "reply one") || strings.Contains(got, "reply two") {
		t.Errorf("replies should be left out:\n%s", got)
	}
	if !strings.Contains(got, "first thread") || !strings.Contains(got, "second thread") {
		t.Errorf("top-level comments missing:\n%s", got)
	}
}

func TestListFilteredEmptyMessage(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	listComments(srv, reviewComments())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.Author = "nobody"
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got := out.String(); got != "No matching comments on pull request #123\n" {
		t.Errorf("output = %q", got)
	}
}

func TestListCurrentUserError(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handlePR(srv)
	srv.Handle("GET", "/user", 403, map[string]any{"type": "error", "error": map[string]any{"message": "Forbidden"}})

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOpts(t, srv, ios)
	opts.NeedsReply = true
	err := listRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "looking up the current user") {
		t.Fatalf("err = %v", err)
	}
}

func TestParseSince(t *testing.T) {
	t.Parallel()
	now := cmdtest.FixedNow()
	tests := []struct {
		in      string
		after   time.Time
		afterID int
	}{
		{"123", time.Time{}, 123},
		{"#123", time.Time{}, 123},
		{"2h", now.Add(-2 * time.Hour), 0},
		{"90m", now.Add(-90 * time.Minute), 0},
		{"3d", now.AddDate(0, 0, -3), 0},
		{"1w", now.AddDate(0, 0, -7), 0},
		{"2026-09-01T08:00:00Z", time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC), 0},
		{"2026-09-01", time.Date(2026, 9, 1, 0, 0, 0, 0, time.Local), 0},
	}
	for _, tt := range tests {
		got, err := parseSince(tt.in, now)
		if err != nil {
			t.Errorf("parseSince(%q): %v", tt.in, err)
			continue
		}
		if !got.after.Equal(tt.after) || got.afterID != tt.afterID {
			t.Errorf("parseSince(%q) = %+v, want after %v id %d", tt.in, got, tt.after, tt.afterID)
		}
	}
	for _, bad := range []string{"", "yesterday", "-2h", "2026-13-01", "1.5d"} {
		_, err := parseSince(bad, now)
		cmdtest.AssertFlagError(t, err, "invalid value for --since")
	}
}

func TestListNewFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	var captured *ListOptions
	cmd := NewCmdList(f, func(o *ListOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "55", "--needs-reply", "--since", "2h", "--author", "@me", "--top-level"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !captured.NeedsReply || captured.Since != "2h" || captured.Author != "@me" || !captured.TopLevel {
		t.Errorf("parsed = %+v", captured)
	}
}

func TestListInvalidSinceFlagError(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdList(f, func(*ListOptions) error {
		t.Error("runF should not be called")
		return nil
	})
	_, _, err := cmdtest.RunCommand(t, cmd, "123", "--since", "yesterday")
	cmdtest.AssertFlagError(t, err, "invalid value for --since")
}
