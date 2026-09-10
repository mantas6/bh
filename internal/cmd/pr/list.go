package pr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// ListOptions holds the dependencies and flags for `bh pr list`.
type ListOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	Browser   browser
	Now       func() time.Time

	State  string
	Limit  int
	Author string
	Search string
	JSON   bool
	Web    bool
}

// NewCmdList creates the "pr list" command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}
	if f.Browser != nil {
		opts.Browser = f.Browser
	}

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List pull requests",
		Aliases: []string{"ls"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return listRun(opts)
		},
	}

	cmd.Flags().StringVar(&opts.State, "state", "open", "Filter by state: {open|merged|declined|superseded|all}")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of pull requests to fetch")
	cmd.Flags().StringVar(&opts.Author, "author", "", "Filter by author (@me or a nickname)")
	cmd.Flags().StringVarP(&opts.Search, "search", "s", "", "Search pull requests with a query")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output pull requests as JSON")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "List pull requests in the web browser")

	return cmd
}

func listRun(opts *ListOptions) error {
	ctx := context.Background()

	repo, _, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	if opts.Web {
		u := fmt.Sprintf("https://bitbucket.org/%s/pull-requests/", repo.FullName())
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", u)
		}
		return opts.Browser.Browse(u)
	}

	states, err := mapStates(opts.State)
	if err != nil {
		return err
	}

	client, err := opts.ApiClient()
	if err != nil {
		return err
	}

	var qparts []string
	if opts.Author != "" {
		aq, err := authorQuery(ctx, client, opts.Author)
		if err != nil {
			return err
		}
		qparts = append(qparts, aq)
	}
	if opts.Search != "" {
		qparts = append(qparts, opts.Search)
	}

	prs, err := client.ListPullRequests(ctx, repo.FullName(), api.ListPROptions{
		State: states,
		Query: strings.Join(qparts, " AND "),
		Limit: opts.Limit,
	})
	if err != nil {
		return err
	}

	if opts.JSON {
		return output.PrintJSON(opts.IO.Out, prs)
	}

	isTTY := opts.IO.IsStdoutTTY()

	if len(prs) == 0 {
		if isTTY {
			fmt.Fprintf(opts.IO.Out, "No pull requests match your search in %s\n", repo.FullName())
		}
		return nil
	}

	if isTTY {
		fmt.Fprintf(opts.IO.Out, "\n%s in %s\n\n", listHeader(len(prs), opts.State), repo.FullName())
	}

	now := opts.Now
	if now == nil {
		now = time.Now
	}
	cs := output.NewColorScheme(opts.IO.ColorEnabled())

	table := output.NewTable(opts.IO.Out, isTTY)
	for i := range prs {
		pr := prs[i]
		state := pr.State
		if isTTY {
			state = cs.StateColor(pr.State)
		}
		table.AddRow(
			fmt.Sprintf("#%d", pr.ID),
			pr.Title,
			pr.Source.Branch.Name,
			prAuthor(&pr),
			state,
			output.RelativeTime(pr.UpdatedOn, now()),
		)
	}
	return table.Flush()
}

// mapStates converts a --state value into the API's uppercase state list.
func mapStates(state string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "", "open":
		return []string{"OPEN"}, nil
	case "merged":
		return []string{"MERGED"}, nil
	case "declined":
		return []string{"DECLINED"}, nil
	case "superseded":
		return []string{"SUPERSEDED"}, nil
	case "all":
		return []string{"OPEN", "MERGED", "DECLINED", "SUPERSEDED"}, nil
	default:
		return nil, cmdutil.FlagErrorf("invalid state: %s", state)
	}
}

// authorQuery builds the BBQL author filter for --author.
func authorQuery(ctx context.Context, client *api.Client, author string) (string, error) {
	if author == "@me" {
		u, err := client.CurrentUser(ctx)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf(`author.uuid="%s"`, u.UUID), nil
	}
	return fmt.Sprintf(`author.nickname="%s"`, author), nil
}

// listHeader renders the "Showing N [state] pull requests" header line.
func listHeader(n int, state string) string {
	noun := "pull request"
	if n != 1 {
		noun += "s"
	}
	adj := ""
	if s := strings.ToLower(strings.TrimSpace(state)); s != "" && s != "all" {
		adj = s + " "
	}
	return fmt.Sprintf("Showing %d %s%s", n, adj, noun)
}
