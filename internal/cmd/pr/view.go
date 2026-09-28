package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// ViewOptions holds the dependencies and flags for `bh pr view`.
type ViewOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Browser   cmdutil.Browser

	Arg  string
	JSON bool
	Web  bool
}

// NewCmdView creates the "pr view" command.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Browser:   f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "view [<number> | <url> | <branch>]",
		Short: "View a pull request",
		Args:  cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return viewRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output the pull request as JSON")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "Open the pull request in the web browser")

	return cmd
}

func viewRun(ctx context.Context, opts *ViewOptions) error {
	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr := found.PR

	if opts.Web {
		return cmdutil.OpenInBrowser(opts.IO, opts.Browser, pr.Links.HTML.Href)
	}

	if opts.JSON {
		return cmdutil.PrintJSON(opts.IO.Out, pr)
	}

	return printPRView(opts, found.Repo, pr)
}

func printPRView(opts *ViewOptions, baseRepo git.Repo, pr *api.PullRequest) error {
	cs := opts.IO.ColorScheme()
	out := opts.IO.Out

	fmt.Fprintf(out, "%s %s\n", cs.Bold(pr.Title), cs.Gray(fmt.Sprintf("#%d", pr.ID)))

	srcPrefix := ""
	if pr.Source.Repository != nil && baseRepo.FullName() != "" && !shared.SameRepoPR(pr, baseRepo) {
		srcPrefix = pr.Source.Repository.FullName + ":"
	}

	comments := fmt.Sprintf("%d comment", pr.CommentCount)
	if pr.CommentCount != 1 {
		comments += "s"
	}

	fmt.Fprintf(out, "%s • %s wants to merge %s%s into %s • %s\n",
		cs.StateColor(pr.State),
		shared.PRAuthor(pr),
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
		name := shared.UserName(&p.User)
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
