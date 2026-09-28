package comment

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// ListOptions holds the dependencies and flags for `bh pr comment list`.
type ListOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg        string
	JSON       bool
	Unresolved bool
	Limit      int
}

// NewCmdList creates the "pr comment list" command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list [<number> | <url> | <branch>]",
		Short: "List comments on a pull request",
		Args:  cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if opts.Limit < 0 {
				return cmdutil.FlagErrorf("invalid value for --limit: %d", opts.Limit)
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output comments as JSON")
	cmd.Flags().BoolVar(&opts.Unresolved, "unresolved", false, "Show only unresolved comment threads")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 0, "Maximum number of comment threads to show (0 = all)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

	// Fetch every comment: the limit counts threads, which can only be
	// assembled once replies are known.
	comments, err := client.ListComments(ctx, repo.FullName(), pr.ID, 0)
	if err != nil {
		return err
	}

	threads := buildThreads(comments, opts.Unresolved)
	if opts.Limit > 0 && len(threads.roots) > opts.Limit {
		threads.roots = threads.roots[:opts.Limit]
	}

	if opts.JSON {
		return cmdutil.PrintJSON(opts.IO.Out, threads.flatten(comments))
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	return printThreads(opts.IO, now(), pr.ID, threads)
}

// threadSet is the comments of a pull request arranged into threads.
type threadSet struct {
	// roots are the top-level comments, oldest first.
	roots []*api.Comment
	// children maps a comment id to its visible replies, oldest first.
	children map[int][]*api.Comment
}

// buildThreads arranges the non-deleted comments into threads. A reply whose
// parent is deleted or missing becomes a root. With unresolved, threads whose
// root is resolved are dropped.
func buildThreads(comments []api.Comment, unresolved bool) *threadSet {
	byID := make(map[int]*api.Comment, len(comments))
	for i := range comments {
		if !comments[i].Deleted {
			byID[comments[i].ID] = &comments[i]
		}
	}

	ts := &threadSet{children: map[int][]*api.Comment{}}
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
		ts.roots = append(ts.roots, c)
	}

	sortByCreated(ts.roots)
	for _, ch := range ts.children {
		sortByCreated(ch)
	}
	return ts
}

// walk calls fn for every comment in the threads, depth first, with its
// nesting level.
func (ts *threadSet) walk(fn func(c *api.Comment, level int)) {
	var visit func(c *api.Comment, level int)
	visit = func(c *api.Comment, level int) {
		fn(c, level)
		for _, child := range ts.children[c.ID] {
			visit(child, level+1)
		}
	}
	for _, root := range ts.roots {
		visit(root, 0)
	}
}

// flatten returns the comments belonging to the threads, in the API's order.
func (ts *threadSet) flatten(comments []api.Comment) []api.Comment {
	include := map[int]bool{}
	ts.walk(func(c *api.Comment, _ int) { include[c.ID] = true })

	out := make([]api.Comment, 0, len(include))
	for _, c := range comments {
		if !c.Deleted && include[c.ID] {
			out = append(out, c)
		}
	}
	return out
}

func printThreads(ios *cmdutil.IOStreams, now time.Time, prID int, ts *threadSet) error {
	if len(ts.roots) == 0 {
		fmt.Fprintf(ios.Out, "No comments on pull request #%d\n", prID)
		return nil
	}

	cs := ios.ColorScheme()
	var b strings.Builder
	ts.walk(func(c *api.Comment, level int) {
		printComment(&b, cs, now, c, level)
	})
	fmt.Fprint(ios.Out, b.String())
	return nil
}

func printComment(b *strings.Builder, cs *output.ColorScheme, now time.Time, c *api.Comment, level int) {
	indent := strings.Repeat(" ", level*2)

	fmt.Fprintf(b, "%s#%d %s · %s", indent, c.ID, shared.UserName(&c.User), output.RelativeTime(c.CreatedOn, now))
	if s := inlineString(c.Inline); s != "" {
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

// inlineString renders an inline anchor as "path:line". When only the old-file
// line is present it is prefixed with "old ".
func inlineString(in *api.Inline) string {
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
