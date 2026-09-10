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

// ViewOptions holds the dependencies and flags for `bh pr view`.
type ViewOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	Browser   browser
	Now       func() time.Time

	Arg  string
	JSON bool
	Web  bool
}

// NewCmdView creates the "pr view" command.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
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
		Use:   "view [<number> | <url> | <branch>]",
		Short: "View a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output the pull request as JSON")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "Open the pull request in the web browser")

	return cmd
}

func viewRun(opts *ViewOptions) error {
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

	pr, repo, err := FindPR(ctx, client, gitRunner, repo, opts.Arg)
	if err != nil {
		return err
	}

	if opts.Web {
		u := pr.Links.HTML.Href
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", u)
		}
		return opts.Browser.Browse(u)
	}

	if opts.JSON {
		return output.PrintJSON(opts.IO.Out, pr)
	}

	return printPRView(opts, repo, pr)
}

func printPRView(opts *ViewOptions, baseRepo git.Repo, pr *api.PullRequest) error {
	cs := output.NewColorScheme(opts.IO.ColorEnabled())
	out := opts.IO.Out

	fmt.Fprintf(out, "%s %s\n", cs.Bold(pr.Title), cs.Gray(fmt.Sprintf("#%d", pr.ID)))

	srcPrefix := ""
	if pr.Source.Repository != nil && baseRepo.FullName() != "" &&
		!strings.EqualFold(pr.Source.Repository.FullName, baseRepo.FullName()) {
		srcPrefix = pr.Source.Repository.FullName + ":"
	}

	comments := fmt.Sprintf("%d comment", pr.CommentCount)
	if pr.CommentCount != 1 {
		comments += "s"
	}

	fmt.Fprintf(out, "%s • %s wants to merge %s%s into %s • %s\n",
		cs.StateColor(pr.State),
		prAuthor(pr),
		srcPrefix,
		pr.Source.Branch.Name,
		pr.Destination.Branch.Name,
		comments,
	)

	if reviewers := reviewerLines(pr, cs); reviewers != "" {
		fmt.Fprintf(out, "Reviewers: %s\n", reviewers)
	}

	if pr.Draft {
		fmt.Fprintln(out, "Draft: yes")
	}

	fmt.Fprintln(out)
	if strings.TrimSpace(pr.Description) == "" {
		fmt.Fprintln(out, cs.Gray("No description provided"))
	} else {
		fmt.Fprintln(out, pr.Description)
	}
	fmt.Fprintln(out)

	fmt.Fprintf(out, "View this pull request on Bitbucket: %s\n", pr.Links.HTML.Href)
	return nil
}

// reviewerLines renders "name (status)" entries from the PR participants whose
// role is REVIEWER.
func reviewerLines(pr *api.PullRequest, cs *output.ColorScheme) string {
	var parts []string
	for _, p := range pr.Participants {
		if !strings.EqualFold(p.Role, "REVIEWER") {
			continue
		}
		name := p.User.DisplayName
		if name == "" {
			name = p.User.Nickname
		}
		var status string
		switch {
		case p.Approved || strings.EqualFold(p.State, "approved"):
			status = cs.Green("approved ✓")
		case strings.EqualFold(p.State, "changes_requested"):
			status = cs.Red("changes requested")
		default:
			status = cs.Yellow("pending")
		}
		parts = append(parts, fmt.Sprintf("%s (%s)", name, status))
	}
	return strings.Join(parts, ", ")
}
