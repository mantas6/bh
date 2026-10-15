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

// Walk calls fn for every comment in the threads, depth first, with its
// nesting level.
func (ts *Threads) Walk(fn func(c *api.Comment, level int)) {
	var visit func(c *api.Comment, level int)
	visit = func(c *api.Comment, level int) {
		fn(c, level)
		for _, child := range ts.children[c.ID] {
			visit(child, level+1)
		}
	}
	for _, root := range ts.Roots {
		visit(root, 0)
	}
}

// Flatten returns the comments belonging to the threads, in the API's order.
func (ts *Threads) Flatten(comments []api.Comment) []api.Comment {
	include := map[int]bool{}
	ts.Walk(func(c *api.Comment, _ int) { include[c.ID] = true })

	out := make([]api.Comment, 0, len(include))
	for _, c := range comments {
		if !c.Deleted && include[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

// WriteThreads renders the threads to w: each comment is a header line
// `#id author · time[ · path:line][ [resolved]]` followed by its indented body
// and a blank line; replies are nested two spaces deeper than their parent.
// Times are relative to now.
func WriteThreads(w io.Writer, cs *output.ColorScheme, now time.Time, ts *Threads) error {
	var b strings.Builder
	ts.Walk(func(c *api.Comment, level int) {
		writeComment(&b, cs, now, c, level)
	})
	_, err := io.WriteString(w, b.String())
	return err
}

func writeComment(b *strings.Builder, cs *output.ColorScheme, now time.Time, c *api.Comment, level int) {
	indent := strings.Repeat(" ", level*2)

	fmt.Fprintf(b, "%s#%d %s · %s", indent, c.ID, UserName(&c.User), output.RelativeTime(c.CreatedOn, now))
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
