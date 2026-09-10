package comment

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// ListOptions holds the dependencies and flags for `bh pr comment list`.
type ListOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
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
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list [<number> | <url> | <branch>]",
		Short: "List comments on a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output comments as JSON")
	cmd.Flags().BoolVar(&opts.Unresolved, "unresolved", false, "Show only unresolved comment threads")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 0, "Maximum number of comments to fetch (0 = all)")

	return cmd
}

func listRun(opts *ListOptions) error {
	ctx := context.Background()

	repo, _, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	client, err := opts.ApiClient()
	if err != nil {
		return err
	}
	gitRunner, err := opts.Git()
	if err != nil {
		return err
	}

	pr, repo, err := resolvePR(ctx, client, gitRunner, repo, opts.Arg)
	if err != nil {
		return err
	}

	comments, err := client.ListComments(ctx, repo.FullName(), pr.ID, opts.Limit)
	if err != nil {
		return err
	}

	if opts.JSON {
		return output.PrintJSON(opts.IO.Out, filterComments(comments, opts.Unresolved))
	}

	return printThreads(opts, pr.ID, comments)
}

// filterComments returns the flat list of non-deleted comments, dropping those
// belonging to resolved threads when unresolved is true.
func filterComments(comments []api.Comment, unresolved bool) []api.Comment {
	byID := visibleIndex(comments)

	out := make([]api.Comment, 0, len(comments))
	for i := range comments {
		c := &comments[i]
		if c.Deleted {
			continue
		}
		if unresolved && rootOf(c, byID).Resolution != nil {
			continue
		}
		out = append(out, *c)
	}
	return out
}

// visibleIndex maps id -> comment for all non-deleted comments.
func visibleIndex(comments []api.Comment) map[int]*api.Comment {
	byID := make(map[int]*api.Comment, len(comments))
	for i := range comments {
		if comments[i].Deleted {
			continue
		}
		byID[comments[i].ID] = &comments[i]
	}
	return byID
}

// rootOf follows the parent chain within the visible set. A comment whose
// parent is hidden or missing is its own root.
func rootOf(c *api.Comment, byID map[int]*api.Comment) *api.Comment {
	cur := c
	for cur.Parent != nil {
		p, ok := byID[cur.Parent.ID]
		if !ok {
			break
		}
		cur = p
	}
	return cur
}

func printThreads(opts *ListOptions, prID int, comments []api.Comment) error {
	cs := output.NewColorScheme(opts.IO.ColorEnabled())
	out := opts.IO.Out

	byID := visibleIndex(comments)

	children := map[int][]*api.Comment{}
	var roots []*api.Comment
	for i := range comments {
		c := &comments[i]
		if c.Deleted {
			continue
		}
		if c.Parent != nil {
			if _, ok := byID[c.Parent.ID]; ok {
				children[c.Parent.ID] = append(children[c.Parent.ID], c)
				continue
			}
		}
		roots = append(roots, c)
	}

	sortByCreated(roots)
	for _, ch := range children {
		sortByCreated(ch)
	}

	var b strings.Builder
	printed := 0
	var walk func(c *api.Comment, level int)
	walk = func(c *api.Comment, level int) {
		printComment(&b, cs, opts.Now(), c, level)
		printed++
		for _, child := range children[c.ID] {
			walk(child, level+1)
		}
	}

	for _, root := range roots {
		if opts.Unresolved && root.Resolution != nil {
			continue
		}
		walk(root, 0)
	}

	if printed == 0 {
		fmt.Fprintf(out, "No comments on pull request #%d\n", prID)
		return nil
	}

	fmt.Fprint(out, b.String())
	return nil
}

func printComment(b *strings.Builder, cs *output.ColorScheme, now time.Time, c *api.Comment, level int) {
	indent := strings.Repeat(" ", level*2)

	fmt.Fprintf(b, "%s#%d %s · %s", indent, c.ID, commentAuthor(c.User), output.RelativeTime(c.CreatedOn, now))
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
