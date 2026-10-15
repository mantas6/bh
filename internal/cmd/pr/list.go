package pr

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// ListOptions holds the dependencies and flags for `bh pr list`.
type ListOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Browser   cmdutil.Browser
	Now       func() time.Time

	State      string
	Limit      int
	Author     string
	Search     string
	Base       string
	Head       string
	JSON       bool
	JSONOutput cmdutil.JSONOutput
	Web        bool
}

// NewCmdList creates the "pr list" command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		BaseRepo:  f.BaseRepo,
		Browser:   f.Browser,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pull requests",
		Long: `List pull requests in the repository.

Each row shows the number, title, ` + "`source → destination`" + ` branches, author,
state and when the pull request was last updated. When stdout is not a
terminal the fields are tab-separated and the update time is an absolute
RFC 3339 timestamp.

--search takes a Bitbucket BBQL query such as 'title ~ "SUPER-123"'. A
value without any BBQL operator (~ = != > < AND OR) is treated as plain
text and matched against the title, description and source branch, so
--search 4304 works as expected. See
` + bbqlDocsURL + `

When more pull requests match than --limit allows, a notice saying so is
printed on stderr (not with --json).`,
		Aliases: []string{"ls"},
		Args:    cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit <= 0 {
				return cmdutil.FlagErrorf("invalid value for --limit: %d", opts.Limit)
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVar(&opts.State, "state", "open", "Filter by state: {open|merged|declined|superseded|all}")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 30, "Maximum number of pull requests to fetch")
	cmd.Flags().StringVar(&opts.Author, "author", "", "Filter by author (@me or a nickname)")
	cmd.Flags().StringVarP(&opts.Search, "search", "s", "", `Filter with a Bitbucket BBQL query, e.g. 'title ~ "SUPER-123"'; plain text searches title, description and branch`)
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "Filter by destination branch")
	cmd.Flags().StringVarP(&opts.Head, "head", "H", "", "Filter by source branch")
	cmdutil.AddJSONFlags(cmd, &opts.JSON, &opts.JSONOutput, reflect.TypeFor[api.PullRequest](), "pull requests")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "List pull requests in the web browser")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	repo, _, err := opts.BaseRepo(ctx)
	if err != nil {
		return err
	}

	if opts.Web {
		return cmdutil.OpenInBrowser(opts.IO, opts.Browser, repo.WebURL()+"/pull-requests/")
	}

	states, err := mapStates(opts.State)
	if err != nil {
		return err
	}

	client, err := opts.APIClient()
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
	if opts.Base != "" {
		qparts = append(qparts, "destination.branch.name="+api.QuoteBBQL(opts.Base))
	}
	if opts.Head != "" {
		qparts = append(qparts, "source.branch.name="+api.QuoteBBQL(opts.Head))
	}
	if opts.Search != "" {
		switch {
		case !looksLikeBBQL(opts.Search):
			qparts = append(qparts, plainTextSearch(opts.Search))
		case len(qparts) > 0:
			// Raw BBQL; parenthesise it so an OR inside it cannot escape
			// the other filters.
			qparts = append(qparts, "("+opts.Search+")")
		default:
			qparts = append(qparts, opts.Search)
		}
	}

	prs, info, err := client.ListPullRequestsInfo(ctx, repo.FullName(), api.ListPROptions{
		State: states,
		Query: strings.Join(qparts, " AND "),
		Limit: opts.Limit,
	})
	if err != nil {
		if opts.Search != "" && api.IsBadRequest(err) {
			return cmdutil.WithHint(err, "--search takes BBQL, e.g. --search 'title ~ \"SUPER-123\"'; see "+bbqlDocsURL)
		}
		return err
	}

	if opts.JSON {
		return opts.JSONOutput.Print(opts.IO, prs)
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
	cs := opts.IO.ColorScheme()

	table := output.NewTable(opts.IO.Out, isTTY)
	for i := range prs {
		pr := prs[i]
		state := pr.State
		if isTTY {
			state = cs.StateColor(pr.State)
		}
		updated := pr.UpdatedOn.Format(time.RFC3339)
		if isTTY {
			updated = output.RelativeTime(pr.UpdatedOn, now())
		}
		table.AddRow(
			fmt.Sprintf("#%d", pr.ID),
			pr.Title,
			branchPair(&pr),
			shared.PRAuthor(&pr),
			state,
			updated,
		)
	}
	if err := table.Flush(); err != nil {
		return err
	}

	// On stderr so it never mixes with the rows.
	if info.More {
		fmt.Fprintln(opts.IO.ErrOut, truncationNotice(len(prs), info.Total))
	}
	return nil
}

// bbqlDocsURL documents the BBQL syntax --search accepts.
const bbqlDocsURL = "https://developer.atlassian.com/cloud/bitbucket/rest/intro/#filtering"

// bbqlOperators are the substrings whose presence makes a --search value
// BBQL rather than plain text. "!=" is covered by "=".
var bbqlOperators = []string{"~", "=", ">", "<", " AND ", " OR "}

// looksLikeBBQL reports whether a --search value contains a BBQL operator and
// should be passed through verbatim.
func looksLikeBBQL(s string) bool {
	for _, op := range bbqlOperators {
		if strings.Contains(s, op) {
			return true
		}
	}
	return false
}

// plainTextSearch turns plain --search text into a BBQL clause matching it
// against the title, description and source branch name.
func plainTextSearch(s string) string {
	v := api.QuoteBBQL(strings.TrimSpace(s))
	return "(title ~ " + v + " OR description ~ " + v + " OR source.branch.name ~ " + v + ")"
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
		return "author.uuid=" + api.QuoteBBQL(u.UUID), nil
	}
	return "author.nickname=" + api.QuoteBBQL(author), nil
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

// branchPair renders the "source → destination" branch column.
func branchPair(pr *api.PullRequest) string {
	return pr.Source.Branch.Name + " → " + pr.Destination.Branch.Name
}

// truncationNotice tells the user that --limit cut the list short. total is
// the number of matches the API reported, or negative when unknown.
func truncationNotice(shown, total int) string {
	if total > shown {
		return fmt.Sprintf("Showing %d of %s; use --limit to see more", shown, pluralize(total, "result"))
	}
	return fmt.Sprintf("Showing %s; use --limit to see more", pluralize(shown, "result"))
}
