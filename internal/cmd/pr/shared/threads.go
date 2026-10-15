package shared

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/output"
)

// Threads is the comments of a pull request arranged into threads, as shown
// by `bh pr comment list` and `bh pr view --comments`.
type Threads struct {
	// Roots are the top-level comments, oldest first.
	Roots []*api.Comment
	// children maps a comment id to its visible replies, oldest first.
	children map[int][]*api.Comment
}

// BuildThreads arranges the non-deleted comments into threads. A reply whose
// parent is deleted or missing becomes a root. With unresolved, threads whose
// root is resolved are dropped.
func BuildThreads(comments []api.Comment, unresolved bool) *Threads {
	byID := make(map[int]*api.Comment, len(comments))
	for i := range comments {
		if !comments[i].Deleted {
			byID[comments[i].ID] = &comments[i]
		}
	}

	ts := &Threads{children: map[int][]*api.Comment{}}
	for i := range comments {
		c := &comments[i]
		if c.Deleted {
			continue
		}
		if c.Parent != nil {
			if _, ok := byID[c.Parent.ID]; ok {
				ts.children[c.Parent.ID] = append(ts.children[c.Parent.ID], c)
				continue
			}
		}
		if unresolved && c.Resolution != nil {
			continue
		}
		ts.Roots = append(ts.Roots, c)
	}

	sortByCreated(ts.Roots)
	for _, ch := range ts.children {
		sortByCreated(ch)
	}
	return ts
}

// Thread returns the comments of root's thread: root first, then its replies
// depth first.
func (ts *Threads) Thread(root *api.Comment) []*api.Comment {
	var out []*api.Comment
	var visit func(c *api.Comment)
	visit = func(c *api.Comment) {
		out = append(out, c)
		for _, child := range ts.children[c.ID] {
			visit(child)
		}
	}
	visit(root)
	return out
}

// Filter keeps only the threads for which keep returns true. keep is given
// the thread's comments as returned by Thread (root first).
func (ts *Threads) Filter(keep func(thread []*api.Comment) bool) {
	roots := ts.Roots[:0:0]
	for _, root := range ts.Roots {
		if keep(ts.Thread(root)) {
			roots = append(roots, root)
		}
	}
	ts.Roots = roots
}

// KeepNewest keeps the n most recently started threads (n > 0), still oldest
// first.
func (ts *Threads) KeepNewest(n int) {
	if n > 0 && len(ts.Roots) > n {
		ts.Roots = ts.Roots[len(ts.Roots)-n:]
	}
}

// TopLevel drops every reply, leaving only the top-level comments.
func (ts *Threads) TopLevel() {
	ts.children = map[int][]*api.Comment{}
}

// LastComment returns the most recently created comment of a thread (the
// higher id wins a tie), or nil for an empty thread.
func LastComment(thread []*api.Comment) *api.Comment {
	var last *api.Comment
	for _, c := range thread {
		if last == nil || c.CreatedOn.After(last.CreatedOn) ||
			(c.CreatedOn.Equal(last.CreatedOn) && c.ID > last.ID) {
			last = c
		}
	}
	return last
}

// SameUser reports whether a and b are the same account, comparing UUIDs,
// then account ids, then nicknames, whichever both carry first.
func SameUser(a, b *api.User) bool {
	switch {
	case a.UUID != "" && b.UUID != "":
		return a.UUID == b.UUID
	case a.AccountID != "" && b.AccountID != "":
		return a.AccountID == b.AccountID
	case a.Nickname != "" && b.Nickname != "":
		return a.Nickname == b.Nickname
	}
	return false
}

// Walk calls fn for every comment in the threads, depth first, with its
// nesting level.
func (ts *Threads) Walk(fn func(c *api.Comment, level int)) {
	ts.walk(func(c, _ *api.Comment, level int) { fn(c, level) })
}

// walk is Walk that also passes the root comment of c's thread.
func (ts *Threads) walk(fn func(c, root *api.Comment, level int)) {
	var visit func(c, root *api.Comment, level int)
	visit = func(c, root *api.Comment, level int) {
		fn(c, root, level)
		for _, child := range ts.children[c.ID] {
			visit(child, root, level+1)
		}
	}
	for _, root := range ts.Roots {
		visit(root, root, 0)
	}
}

// CommentJSON is a comment as printed by `bh pr comment list --json`: the
// raw API fields plus fields derived from the thread it belongs to.
type CommentJSON struct {
	api.Comment
	// Resolved reports whether the comment's thread is resolved. Bitbucket
	// records the resolution on the thread's top-level comment only; replies
	// inherit it here.
	Resolved bool `json:"resolved"`
	// ThreadID is the id of the top-level comment of the thread, the
	// comment's own id for a top-level comment.
	ThreadID int `json:"thread_id"`
	// IsReply reports whether the comment is nested under another comment
	// (ThreadID differs from its id).
	IsReply bool `json:"is_reply"`
}

// JSON returns the comments belonging to the threads, in the API's order,
// with their derived thread fields. The result is never nil.
func (ts *Threads) JSON(comments []api.Comment) []CommentJSON {
	roots := map[int]*api.Comment{}
	ts.walk(func(c, root *api.Comment, _ int) { roots[c.ID] = root })

	out := make([]CommentJSON, 0, len(roots))
	for _, c := range comments {
		root, ok := roots[c.ID]
		if c.Deleted || !ok {
			continue
		}
		out = append(out, CommentJSON{
			Comment:  c,
			Resolved: root.Resolution != nil || c.Resolution != nil,
			ThreadID: root.ID,
			IsReply:  root.ID != c.ID,
		})
	}
	return out
}

// CommentTime returns how WriteThreads renders comment times: relative to now
// on a terminal ("5 minutes ago"), otherwise an RFC 3339 timestamp in local
// time, which stays exact in logs and scripts.
func CommentTime(isTTY bool, now time.Time) func(time.Time) string {
	if isTTY {
		return func(t time.Time) string { return output.RelativeTime(t, now) }
	}
	return func(t time.Time) string { return t.Local().Format(time.RFC3339) }
}

// WriteThreads renders the threads to w: each comment is a header line
// `#id author · time[ · path:line][ [resolved]]` followed by its indented body
// and a blank line; replies are nested two spaces deeper than their parent.
// Times are rendered with formatTime (see CommentTime).
func WriteThreads(w io.Writer, cs *output.ColorScheme, formatTime func(time.Time) string, ts *Threads) error {
	var b strings.Builder
	ts.Walk(func(c *api.Comment, level int) {
		writeComment(&b, cs, formatTime, c, level)
	})
	_, err := io.WriteString(w, b.String())
	return err
}

func writeComment(b *strings.Builder, cs *output.ColorScheme, formatTime func(time.Time) string, c *api.Comment, level int) {
	indent := strings.Repeat(" ", level*2)

	fmt.Fprintf(b, "%s#%d %s · %s", indent, c.ID, UserName(&c.User), formatTime(c.CreatedOn))
	if s := InlineString(c.Inline); s != "" {
		fmt.Fprintf(b, " · %s", s)
	}
	if c.Resolution != nil {
		fmt.Fprintf(b, " %s", cs.Gray("[resolved]"))
	}
	b.WriteByte('\n')

	body := strings.Repeat(" ", level*2+2)
	for _, line := range strings.Split(c.Content.Raw, "\n") {
		fmt.Fprintf(b, "%s%s\n", body, line)
	}
	b.WriteByte('\n')
}

// InlineString renders an inline anchor as "path:line". When only the old-file
// line is present it is prefixed with "old ".
func InlineString(in *api.Inline) string {
	if in == nil || in.Path == "" {
		return ""
	}
	if in.To != nil {
		return fmt.Sprintf("%s:%d", in.Path, *in.To)
	}
	if in.From != nil {
		return fmt.Sprintf("old %s:%d", in.Path, *in.From)
	}
	return in.Path
}

func sortByCreated(cs []*api.Comment) {
	sort.SliceStable(cs, func(i, j int) bool {
		return cs[i].CreatedOn.Before(cs[j].CreatedOn)
	})
}
