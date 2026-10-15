package shared

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/output"
)

func renderThreads(t *testing.T, ts *Threads) string {
	t.Helper()
	var b strings.Builder
	if err := WriteThreads(&b, output.NewColorScheme(false), CommentTime(true, cmdtest.FixedNow()), ts); err != nil {
		t.Fatalf("WriteThreads: %v", err)
	}
	return b.String()
}

// sampleThreads is a resolved inline thread #1 (reply #2, nested reply #4),
// an open thread #3 with a multi-line body, and a deleted comment #5.
func sampleThreads() []api.Comment {
	root := cmdtest.CommentAt(1, "ada", "Rename this", 120)
	root.Inline = &api.Inline{Path: "main.go", To: new(42)}
	root.Resolution = &api.Resolution{Type: "resolved"}
	reply := cmdtest.CommentAt(2, "bob", "Done", 60)
	reply.Parent = &api.CommentRef{ID: 1}
	nested := cmdtest.CommentAt(4, "ada", "Thanks", 30)
	nested.Parent = &api.CommentRef{ID: 2}
	open := cmdtest.CommentAt(3, "cara", "Two\nlines", 90)
	deleted := cmdtest.CommentAt(5, "eve", "gone", 10)
	deleted.Deleted = true
	return []api.Comment{nested, reply, open, root, deleted}
}

func TestWriteThreadsFormat(t *testing.T) {
	t.Parallel()
	got := renderThreads(t, BuildThreads(sampleThreads(), false))
	want := "#1 ada · about 2 hours ago · main.go:42 [resolved]\n" +
		"  Rename this\n" +
		"\n" +
		"  #2 bob · about an hour ago\n" +
		"    Done\n" +
		"\n" +
		"    #4 ada · 30 minutes ago\n" +
		"      Thanks\n" +
		"\n" +
		"#3 cara · about an hour ago\n" +
		"  Two\n" +
		"  lines\n" +
		"\n"
	if got != want {
		t.Errorf("output mismatch\n got:\n%s\nwant:\n%s", got, want)
	}
}

func TestBuildThreadsUnresolved(t *testing.T) {
	t.Parallel()
	ts := BuildThreads(sampleThreads(), true)
	if len(ts.Roots) != 1 || ts.Roots[0].ID != 3 {
		t.Fatalf("roots = %+v, want only #3", ts.Roots)
	}
	if got := renderThreads(t, ts); strings.Contains(got, "#2") || strings.Contains(got, "#4") {
		t.Errorf("replies of a resolved thread should be dropped:\n%s", got)
	}
}

func TestBuildThreadsOrphanReplyBecomesRoot(t *testing.T) {
	t.Parallel()
	parent := cmdtest.CommentAt(1, "ada", "parent", 60)
	parent.Deleted = true
	orphan := cmdtest.CommentAt(2, "bob", "orphan", 30)
	orphan.Parent = &api.CommentRef{ID: 1}

	ts := BuildThreads([]api.Comment{parent, orphan}, false)
	if len(ts.Roots) != 1 || ts.Roots[0].ID != 2 {
		t.Fatalf("roots = %+v, want orphan #2", ts.Roots)
	}
}

func TestThreadsJSON(t *testing.T) {
	t.Parallel()
	comments := sampleThreads()
	ts := BuildThreads(comments, false)

	got := ts.JSON(comments)
	type derived struct {
		id, thread int
		resolved   bool
		reply      bool
	}
	var have []derived
	for _, c := range got {
		have = append(have, derived{c.ID, c.ThreadID, c.Resolved, c.IsReply})
	}
	// API order is preserved and the deleted #5 is dropped. Replies of the
	// resolved thread #1 inherit its resolution and thread id.
	want := []derived{
		{4, 1, true, true},
		{2, 1, true, true},
		{3, 3, false, false},
		{1, 1, true, false},
	}
	if len(have) != len(want) {
		t.Fatalf("got %+v, want %+v", have, want)
	}
	for i := range want {
		if have[i] != want[i] {
			t.Errorf("comment %d: got %+v, want %+v", i, have[i], want[i])
		}
	}
}

func TestThreadsJSONLimitedRoots(t *testing.T) {
	t.Parallel()
	comments := sampleThreads()
	ts := BuildThreads(comments, false)
	ts.Roots = ts.Roots[:1] // keep thread #1 only

	var ids []int
	for _, c := range ts.JSON(comments) {
		ids = append(ids, c.ID)
	}
	// API order is preserved: nested (#4), reply (#2), root (#1).
	if len(ids) != 3 || ids[0] != 4 || ids[1] != 2 || ids[2] != 1 {
		t.Errorf("ids = %v, want [4 2 1]", ids)
	}
}

func TestThreadsJSONEmptyIsNonNil(t *testing.T) {
	t.Parallel()
	if got := BuildThreads(nil, false).JSON(nil); got == nil {
		t.Error("JSON(nil) = nil, want empty slice")
	}
}

func TestThreadsJSONOrphanReply(t *testing.T) {
	t.Parallel()
	parent := cmdtest.CommentAt(1, "ada", "parent", 60)
	parent.Deleted = true
	orphan := cmdtest.CommentAt(2, "bob", "orphan", 30)
	orphan.Parent = &api.CommentRef{ID: 1}

	got := BuildThreads([]api.Comment{parent, orphan}, false).JSON([]api.Comment{parent, orphan})
	if len(got) != 1 || got[0].ThreadID != 2 || got[0].IsReply {
		t.Fatalf("got %+v, want orphan #2 as its own thread", got)
	}
	if got[0].Parent == nil || got[0].Parent.ID != 1 {
		t.Errorf("raw parent should be kept, got %+v", got[0].Parent)
	}
}

func TestInlineString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   *api.Inline
		want string
	}{
		{"nil", nil, ""},
		{"no path", &api.Inline{To: new(1)}, ""},
		{"new side", &api.Inline{Path: "a.go", To: new(3)}, "a.go:3"},
		{"old side", &api.Inline{Path: "a.go", From: new(7)}, "old a.go:7"},
		{"file only", &api.Inline{Path: "a.go"}, "a.go"},
	}
	for _, tt := range tests {
		if got := InlineString(tt.in); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func rootIDs(ts *Threads) []int {
	ids := []int{}
	for _, r := range ts.Roots {
		ids = append(ids, r.ID)
	}
	return ids
}

func TestThreadsThreadAndFilter(t *testing.T) {
	t.Parallel()
	ts := BuildThreads(sampleThreads(), false)

	var thread []int
	for _, c := range ts.Thread(ts.Roots[0]) {
		thread = append(thread, c.ID)
	}
	if !slices.Equal(thread, []int{1, 2, 4}) {
		t.Errorf("Thread(#1) = %v, want [1 2 4]", thread)
	}

	ts.Filter(func(thread []*api.Comment) bool { return len(thread) == 1 })
	if got := rootIDs(ts); !slices.Equal(got, []int{3}) {
		t.Errorf("roots after Filter = %v, want [3]", got)
	}
}

func TestThreadsKeepNewest(t *testing.T) {
	t.Parallel()
	ts := BuildThreads(sampleThreads(), false) // roots #1 (oldest), #3
	ts.KeepNewest(0)
	if got := rootIDs(ts); !slices.Equal(got, []int{1, 3}) {
		t.Errorf("KeepNewest(0) = %v, want all", got)
	}
	ts.KeepNewest(5)
	if got := rootIDs(ts); !slices.Equal(got, []int{1, 3}) {
		t.Errorf("KeepNewest(5) = %v, want all", got)
	}
	ts.KeepNewest(1)
	if got := rootIDs(ts); !slices.Equal(got, []int{3}) {
		t.Errorf("KeepNewest(1) = %v, want [3]", got)
	}
}

func TestThreadsTopLevel(t *testing.T) {
	t.Parallel()
	comments := sampleThreads()
	ts := BuildThreads(comments, false)
	ts.TopLevel()
	got := renderThreads(t, ts)
	if strings.Contains(got, "#2") || strings.Contains(got, "#4") {
		t.Errorf("replies should be dropped:\n%s", got)
	}
	if len(ts.JSON(comments)) != 2 {
		t.Errorf("JSON should hold the two top-level comments only")
	}
}

func TestLastComment(t *testing.T) {
	t.Parallel()
	a := cmdtest.CommentAt(1, "ada", "a", 30)
	b := cmdtest.CommentAt(2, "bob", "b", 10)
	c := cmdtest.CommentAt(3, "cara", "c", 10) // same time as b, higher id
	if got := LastComment([]*api.Comment{&a, &c, &b}); got.ID != 3 {
		t.Errorf("LastComment = #%d, want #3", got.ID)
	}
	if LastComment(nil) != nil {
		t.Error("LastComment(nil) should be nil")
	}
}

func TestSameUser(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b api.User
		want bool
	}{
		{api.User{UUID: "{1}", Nickname: "x"}, api.User{UUID: "{1}", Nickname: "y"}, true},
		{api.User{UUID: "{1}", Nickname: "x"}, api.User{UUID: "{2}", Nickname: "x"}, false},
		{api.User{AccountID: "a"}, api.User{UUID: "{1}", AccountID: "a"}, true},
		{api.User{Nickname: "x"}, api.User{Nickname: "x"}, true},
		{api.User{Nickname: "x"}, api.User{UUID: "{1}"}, false},
		{api.User{}, api.User{}, false},
	}
	for i, tt := range tests {
		if got := SameUser(&tt.a, &tt.b); got != tt.want {
			t.Errorf("%d: SameUser(%+v, %+v) = %v, want %v", i, tt.a, tt.b, got, tt.want)
		}
	}
}

func TestCommentTime(t *testing.T) {
	t.Parallel()
	now := cmdtest.FixedNow()
	at := now.Add(-5 * time.Minute)
	if got := CommentTime(true, now)(at); got != "5 minutes ago" {
		t.Errorf("TTY = %q", got)
	}
	if got, want := CommentTime(false, now)(at), at.Local().Format(time.RFC3339); got != want {
		t.Errorf("non-TTY = %q, want %q", got, want)
	}
}
