package pr

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

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
	Now       func() time.Time

	Arg        string
	JSON       bool
	JSONOutput cmdutil.JSONOutput
	Web        bool
	Comments   bool
	Unresolved bool
}

// NewCmdView creates the "pr view" command.
func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Browser:   f.Browser,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "view [<number> | <url> | <branch>]",
		Short: "View a pull request",
		Long: `View a pull request: a summary header, the description and, with
--comments, the comment threads in the same format as ` + "`bh pr comment list`" + `.

When builds have reported on the pull request, the header includes a line such
as "Checks: 2 passed, 1 failed"; see ` + "`bh pr checks`" + ` for the details.

Without --comments, a pull request that has comments gets a hint line
pointing at ` + "`bh pr comment list`" + `, just above the final URL line.`,
		Args: cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			// --unresolved only makes sense for the comment threads.
			if opts.Unresolved {
				opts.Comments = true
			}
			if err := cmdutil.MutuallyExclusive(
				"--comments cannot be combined with --json; use `bh pr comment list --json` for comments as JSON",
				opts.Comments, opts.JSON); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("--comments cannot be combined with --web",
				opts.Comments, opts.Web); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return viewRun(cmd.Context(), opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.JSON, &opts.JSONOutput, reflect.TypeFor[api.PullRequest](), "the pull request")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "Open the pull request in the web browser")
	cmd.Flags().BoolVarP(&opts.Comments, "comments", "c", false, "Show the comment threads after the description")
	cmd.Flags().BoolVar(&opts.Unresolved, "unresolved", false, "With --comments, show only unresolved comment threads (implies --comments)")

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
		return opts.JSONOutput.Print(opts.IO, pr)
	}

	var threads *shared.Threads
	if opts.Comments {
		comments, err := found.Client.ListComments(ctx, found.Repo.FullName(), pr.ID, 0)
		if err != nil {
			return err
		}
		threads = shared.BuildThreads(comments, opts.Unresolved)
	}

	checks, err := viewChecks(ctx, found)
	if err != nil {
		return err
	}

	return printPRView(opts, found.Repo, pr, threads, checks)
}

// viewChecks fetches the build statuses for the Checks line. They are
// supplementary, so a failed request (a token without access to statuses,
// say) just leaves the line out; only an interrupt is reported.
func viewChecks(ctx context.Context, found *shared.FoundPR) ([]api.CommitStatus, error) {
	statuses, err := found.Client.PullRequestStatuses(ctx, found.Repo.FullName(), found.Number)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, nil
	}
	return latestStatuses(statuses), nil
}

// commentListRef is the pull request argument to suggest for
// `bh pr comment list`: the number, or the URL when the pull request was
// named by URL (it may live in another repository).
func commentListRef(arg string, pr *api.PullRequest) string {
	if sel, err := shared.ParsePRArg(arg); err == nil && sel.Repo != nil && pr.Links.HTML.Href != "" {
		return pr.Links.HTML.Href
	}
	return strconv.Itoa(pr.ID)
}

// printPRView renders the text view. threads is nil unless --comments was
// given; checks are the build statuses for the Checks line (omitted when
// empty).
func printPRView(opts *ViewOptions, baseRepo git.Repo, pr *api.PullRequest, threads *shared.Threads, checks []api.CommitStatus) error {
	cs := opts.IO.ColorScheme()
	out := opts.IO.Out

	fmt.Fprintf(out, "%s %s\n", cs.Bold(pr.Title), cs.Gray(fmt.Sprintf("#%d", pr.ID)))

	srcPrefix := ""
	if pr.Source.Repository != nil && baseRepo.FullName() != "" && !shared.SameRepoPR(pr, baseRepo) {
		srcPrefix = pr.Source.Repository.FullName + ":"
	}

	summary := participantSummary(pr)

	fmt.Fprintf(out, "%s • %s %s %s%s into %s • %s • %s\n",
		cs.StateColor(pr.State),
		shared.PRAuthor(pr),
		mergeVerb(pr.State),
		srcPrefix,
		pr.Source.Branch.Name,
		pr.Destination.Branch.Name,
		pluralize(len(summary.approvedBy), "approval"),
		pluralize(pr.CommentCount, "comment"),
	)

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	fmt.Fprintln(out, refsLine(pr, opts.IO.IsStdoutTTY(), now()))

	fmt.Fprintf(out, "Reviewers: %s\n", reviewerLine(summary.reviewers, cs))
	if len(summary.approvedBy) > 0 {
		fmt.Fprintf(out, "Approved by: %s\n", strings.Join(summary.approvedBy, ", "))
	}
	if len(summary.changesRequestedBy) > 0 {
		fmt.Fprintf(out, "Changes requested by: %s\n", strings.Join(summary.changesRequestedBy, ", "))
	}

	if len(checks) > 0 {
		fmt.Fprintf(out, "Checks: %s\n", checksSummary(checks, cs))
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

	switch {
	case threads != nil:
		if err := printViewComments(opts, cs, pr, threads); err != nil {
			return err
		}
	case pr.CommentCount > 0:
		// Point at the comments without printing them. The hint goes
		// before the URL line so that line stays last for scripts.
		fmt.Fprintf(out, "%s · bh pr comment list %s\n", pluralize(pr.CommentCount, "comment"), commentListRef(opts.Arg, pr))
	}

	fmt.Fprintf(out, "View this pull request on Bitbucket: %s\n", pr.Links.HTML.Href)
	return nil
}

// refsLine renders "Head: <hash> · Base: <branch> <hash> · Updated: <time>",
// leaving out whatever the API did not report. The time is RFC 3339 in the
// local zone, followed on a terminal by the relative time.
func refsLine(pr *api.PullRequest, isTTY bool, now time.Time) string {
	var parts []string
	if pr.Source.Commit != nil && pr.Source.Commit.Hash != "" {
		parts = append(parts, "Head: "+shortHash(pr.Source.Commit.Hash))
	}
	base := "Base: " + pr.Destination.Branch.Name
	if pr.Destination.Commit != nil && pr.Destination.Commit.Hash != "" {
		base += " " + shortHash(pr.Destination.Commit.Hash)
	}
	parts = append(parts, base)
	if !pr.UpdatedOn.IsZero() {
		updated := "Updated: " + pr.UpdatedOn.Local().Format(time.RFC3339)
		if isTTY {
			updated += " (" + output.RelativeTime(pr.UpdatedOn, now) + ")"
		}
		parts = append(parts, updated)
	}
	return strings.Join(parts, " · ")
}

// printViewComments prints the --comments section: a heading, then the
// threads exactly as `bh pr comment list` renders them (each comment ends
// with a blank line).
func printViewComments(opts *ViewOptions, cs *output.ColorScheme, pr *api.PullRequest, threads *shared.Threads) error {
	out := opts.IO.Out
	if len(threads.Roots) == 0 {
		if opts.Unresolved {
			fmt.Fprintln(out, cs.Gray("No unresolved comments"))
		} else {
			fmt.Fprintln(out, cs.Gray("No comments"))
		}
		fmt.Fprintln(out)
		return nil
	}

	heading := "Comments"
	if opts.Unresolved {
		heading = "Unresolved comments"
	}
	fmt.Fprintln(out, cs.Bold(heading+":"))

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	return shared.WriteThreads(out, cs, shared.CommentTime(opts.IO.IsStdoutTTY(), now()), threads)
}

// mergeVerb describes what the author did (or wants to do) with the source
// branch, depending on the pull request state.
func mergeVerb(state string) string {
	switch strings.ToUpper(state) {
	case "MERGED":
		return "merged"
	case "DECLINED":
		return "declined"
	default:
		return "wants to merge"
	}
}

type reviewStatus int

const (
	reviewPending reviewStatus = iota
	reviewApproved
	reviewChangesRequested
)

type reviewer struct {
	name   string
	status reviewStatus
}

// prParticipants is the review state of a pull request as shown by
// `bh pr view`.
type prParticipants struct {
	// reviewers are the requested reviewers with their review status.
	reviewers []reviewer
	// approvedBy and changesRequestedBy cover every participant, whatever
	// their role: on Bitbucket anyone with access can approve.
	approvedBy         []string
	changesRequestedBy []string
}

func participantStatus(p *api.Participant) reviewStatus {
	switch {
	case p.Approved || strings.EqualFold(p.State, "approved"):
		return reviewApproved
	case strings.EqualFold(p.State, "changes_requested"):
		return reviewChangesRequested
	default:
		return reviewPending
	}
}

// userKey identifies a user for de-duplication across the reviewers and
// participants lists.
func userKey(u *api.User) string {
	switch {
	case u.UUID != "":
		return "uuid:" + u.UUID
	case u.AccountID != "":
		return "account:" + u.AccountID
	default:
		return "name:" + shared.UserName(u)
	}
}

func participantSummary(pr *api.PullRequest) prParticipants {
	var s prParticipants
	seen := map[string]bool{}
	for i := range pr.Participants {
		p := &pr.Participants[i]
		name := shared.UserName(&p.User)
		status := participantStatus(p)
		if strings.EqualFold(p.Role, "REVIEWER") {
			s.reviewers = append(s.reviewers, reviewer{name: name, status: status})
			seen[userKey(&p.User)] = true
		}
		switch status {
		case reviewApproved:
			s.approvedBy = append(s.approvedBy, name)
		case reviewChangesRequested:
			s.changesRequestedBy = append(s.changesRequestedBy, name)
		}
	}
	// Requested reviewers who have not interacted yet may be missing from
	// participants; list them as pending.
	for i := range pr.Reviewers {
		u := &pr.Reviewers[i]
		if seen[userKey(u)] {
			continue
		}
		seen[userKey(u)] = true
		s.reviewers = append(s.reviewers, reviewer{name: shared.UserName(u), status: reviewPending})
	}
	return s
}

// reviewerLine renders "name (status)" entries for the requested reviewers,
// or "none".
func reviewerLine(reviewers []reviewer, cs *output.ColorScheme) string {
	if len(reviewers) == 0 {
		return "none"
	}
	parts := make([]string, len(reviewers))
	for i, r := range reviewers {
		var status string
		switch r.status {
		case reviewApproved:
			status = cs.Green("approved ✓")
		case reviewChangesRequested:
			status = cs.Red("changes requested")
		default:
			status = cs.Yellow("pending")
		}
		parts[i] = fmt.Sprintf("%s (%s)", r.name, status)
	}
	return strings.Join(parts, ", ")
}
