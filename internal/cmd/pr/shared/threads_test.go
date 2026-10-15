package shared

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/output"
)

func renderThreads(t *testing.T, ts *Threads) string {
	t.Helper()
	var b strings.Builder
	if err := WriteThreads(&b, output.NewColorScheme(false), cmdtest.FixedNow(), ts); err != nil {
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

func TestThreadsFlatten(t *testing.T) {
	t.Parallel()
	comments := sampleThreads()
	ts := BuildThreads(comments, false)
	ts.Roots = ts.Roots[:1] // keep thread #1 only

	var ids []int
	for _, c := range ts.Flatten(comments) {
		ids = append(ids, c.ID)
	}
	// API order is preserved: nested (#4), reply (#2), root (#1).
	if len(ids) != 3 || ids[0] != 4 || ids[1] != 2 || ids[2] != 1 {
		t.Errorf("ids = %v, want [4 2 1]", ids)
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
