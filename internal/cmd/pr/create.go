package pr

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// CreateOptions holds the dependencies and flags for `bh pr create`.
type CreateOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Browser   cmdutil.Browser
	// Getenv looks up BH_BASE; nil means it is unset.
	Getenv func(string) string

	Title              string
	Body               string
	BodyFile           string
	Base               string
	Head               string
	Draft              bool
	Reviewers          []string
	NoDefaultReviewers bool
	CloseSourceBranch  bool
	Fill               bool
	Push               bool
	Web                bool
}

// NewCmdCreate creates the "pr create" command.
func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Browser:   f.Browser,
		Getenv:    os.Getenv,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: cmdutil.Heredoc(`
			Create a pull request on Bitbucket.

			The head branch defaults to the current branch. When it has not been
			pushed yet you are prompted to push it (use --push to skip the prompt,
			required when not running interactively). When standard output is not
			a terminal the push is quiet: git's output is shown only if it fails.

			The base branch defaults to, in order: git config bh.base (repository,
			then global), the BH_BASE environment variable, and the repository's
			main branch.

			Reviewers are given by nickname, display name, {uuid} or account ID.
			Names are resolved by listing the workspace members, which needs the
			read:workspace:bitbucket token scope; a {uuid} or account ID is used
			as is, without that lookup. The repository's default reviewers
			(including those inherited from its project) are added too, except
			you; pass --no-default-reviewers to leave them out. When they cannot
			be fetched a warning is printed and the pull request is created
			without them.

			The pull request URL is the only line printed on standard output; a
			summary line goes to standard error.
		`),
		Example: cmdutil.Heredoc(`
			# Interactively create a PR from the current branch
			$ bh pr create

			# Fill the title and body from the branch's commits
			$ bh pr create --fill

			# Create a PR against a specific base with reviewers
			$ bh pr create --base main --reviewer alice,bob --title "Fix bug"

			# Always target "dev" in this repository unless --base is given
			$ git config bh.base dev

			# Open the create page in the browser instead
			$ bh pr create --web
		`),
		Args: cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive("specify only one of --body or --body-file",
				cmd.Flags().Changed("body"), cmd.Flags().Changed("body-file")); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return createRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title for the pull request")
	cmdutil.AddBodyFlags(cmd, &opts.Body, &opts.BodyFile)
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "The branch to merge into (default: bh.base git config, $BH_BASE, or the main branch)")
	cmd.Flags().StringVarP(&opts.Head, "head", "H", "", "The branch that contains commits (defaults to current branch)")
	cmd.Flags().BoolVar(&opts.Draft, "draft", false, "Mark the pull request as a draft")
	cmd.Flags().StringSliceVarP(&opts.Reviewers, "reviewer", "r", nil, "Request reviews from people by name, {uuid} or account ID (comma-separated)")
	cmd.Flags().BoolVar(&opts.NoDefaultReviewers, "no-default-reviewers", false, "Do not add the repository's default reviewers")
	cmd.Flags().BoolVar(&opts.CloseSourceBranch, "close-source-branch", false, "Close the source branch when the PR merges")
	cmd.Flags().BoolVarP(&opts.Fill, "fill", "f", false, "Use commit info for the title and body")
	cmd.Flags().BoolVar(&opts.Push, "push", false, "Push the branch to its upstream remote (default origin) before creating")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the create page in the web browser")

	return cmd
}

func createRun(ctx context.Context, opts *CreateOptions) error {
	repo, resolvedRemote, err := opts.BaseRepo(ctx)
	if err != nil {
		return err
	}
	gitRunner, err := opts.Git()
	if err != nil {
		return err
	}

	head := opts.Head
	if head == "" {
		head, err = git.CurrentBranch(ctx, gitRunner)
		if err != nil {
			if errors.Is(err, git.ErrNotOnBranch) {
				return errors.New("could not determine the current branch; specify --head")
			}
			return err
		}
	}

	client, err := opts.APIClient()
	if err != nil {
		return err
	}

	base := opts.Base
	if base == "" {
		if base, err = configuredBase(ctx, gitRunner, opts.Getenv); err != nil {
			return err
		}
	}

	// The effective base is needed for --fill commit ranges and the --web
	// URL. When no base was given or configured we look up the repo's
	// default branch, but we still omit destination from the API create body
	// so the server applies its own default.
	effectiveBase := base
	if base == "" && (opts.Fill || opts.Web) {
		r, err := client.Repository(ctx, repo.FullName())
		if err != nil {
			return err
		}
		if r.MainBranch != nil {
			effectiveBase = r.MainBranch.Name
		}
	}
	if opts.Fill && effectiveBase == "" {
		return fmt.Errorf("cannot use --fill: %s has no main branch configured; specify --base", repo.FullName())
	}

	if err := ensurePushed(ctx, opts, gitRunner, head); err != nil {
		return err
	}

	if opts.Web {
		u := repo.WebURL() + "/pull-requests/new?source=" + url.QueryEscape(head)
		if effectiveBase != "" {
			u += "&dest=" + url.QueryEscape(effectiveBase)
		}
		return cmdutil.OpenInBrowser(opts.IO, opts.Browser, u)
	}

	title := opts.Title
	body := opts.Body
	if opts.BodyFile != "" {
		b, err := shared.ReadBodyFile(opts.IO, opts.BodyFile)
		if err != nil {
			return err
		}
		body = b
	}

	if opts.Fill {
		// Compare against the remote's copy of the base branch: the local
		// one may be stale or not exist at all. Without a known remote for
		// the base repository the local branch is the best available.
		baseRef := effectiveBase
		if resolvedRemote != nil && resolvedRemote.Remote.Name != "" {
			baseRef = resolvedRemote.Remote.Name + "/" + effectiveBase
		}
		t, b, err := fillFromCommits(ctx, gitRunner, baseRef, head)
		if err != nil {
			return err
		}
		if title == "" {
			title = t
		}
		if body == "" {
			body = b
		}
	}

	if title == "" {
		if !opts.IO.IsStdinTTY() {
			return cmdutil.FlagErrorf("title is required when not running interactively; use --title or --fill")
		}
		title, err = opts.IO.Prompter().Input("Title", "")
		if err != nil {
			return err
		}
		if strings.TrimSpace(title) == "" {
			return errors.New("title is required")
		}
	}

	reviewerIDs, err := resolveReviewers(ctx, client, repo.Workspace, opts.Reviewers)
	if err != nil {
		return err
	}
	if !opts.NoDefaultReviewers {
		reviewerIDs = addDefaultReviewers(ctx, opts.IO, client, repo.FullName(), reviewerIDs)
	}

	pr, err := client.CreatePullRequest(ctx, repo.FullName(), api.CreatePRInput{
		Title:             title,
		Description:       body,
		SourceBranch:      head,
		DestinationBranch: base,
		Reviewers:         reviewerIDs,
		Draft:             opts.Draft,
		CloseSourceBranch: opts.CloseSourceBranch,
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, pr.Links.HTML.Href)
	fmt.Fprintln(opts.IO.ErrOut, createdSummary(opts.IO, pr, opts.Draft, head, effectiveBase))
	return nil
}

// configuredBase returns the base branch configured for new pull requests:
// git config bh.base (repository-local, then global) or else $BH_BASE. It is
// "" when neither is set.
func configuredBase(ctx context.Context, gitRunner git.Runner, getenv func(string) string) (string, error) {
	base, err := git.GetConfig(ctx, gitRunner, "bh.base")
	if err != nil {
		return "", fmt.Errorf("reading git config bh.base: %w", err)
	}
	if base = strings.TrimSpace(base); base != "" || getenv == nil {
		return base, nil
	}
	return strings.TrimSpace(getenv("BH_BASE")), nil
}

// addDefaultReviewers appends the repository's effective default reviewers
// to ids, leaving out the current user (Bitbucket rejects the author as a
// reviewer) and anyone already present. It is best effort: when the
// defaults or the current user cannot be fetched a warning is printed and ids
// is returned unchanged, so the pull request is still created.
func addDefaultReviewers(ctx context.Context, ios *cmdutil.IOStreams, client *api.Client, repo string, ids []string) []string {
	warn := func(format string, args ...any) {
		fmt.Fprintf(ios.ErrOut, "%s %s (use --no-default-reviewers to skip them)\n",
			ios.ErrColorScheme().WarningIcon(), fmt.Sprintf(format, args...))
	}

	defaults, err := client.EffectiveDefaultReviewers(ctx, repo)
	if err != nil {
		warn("could not fetch default reviewers, continuing without them: %v", err)
		return ids
	}
	if len(defaults) == 0 {
		return ids
	}
	me, err := client.CurrentUser(ctx)
	if err != nil {
		warn("could not determine the current user to leave out of the default reviewers, continuing without them: %v", err)
		return ids
	}

	for _, u := range defaults {
		if matchesUser(u, me.UUID) || matchesUser(u, me.AccountID) {
			continue
		}
		if slices.ContainsFunc(ids, func(id string) bool { return matchesUser(u, id) }) {
			continue
		}
		id := u.UUID
		if id == "" {
			id = u.AccountID
		}
		ids = append(ids, id)
	}
	return ids
}

// createdSummary renders the stderr line printed after a pull request is
// created, e.g. "✓ Created #934 (draft) ft/x → dev · reviewers: Ada, Bob".
// Branches missing from the response fall back to head and base.
func createdSummary(ios *cmdutil.IOStreams, pr *api.PullRequest, draft bool, head, base string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s Created #%d", ios.ErrColorScheme().SuccessIcon(), pr.ID)
	if pr.Draft || draft {
		b.WriteString(" (draft)")
	}
	if name := pr.Source.Branch.Name; name != "" {
		head = name
	}
	if name := pr.Destination.Branch.Name; name != "" {
		base = name
	}
	fmt.Fprintf(&b, " %s", head)
	if base != "" {
		fmt.Fprintf(&b, " → %s", base)
	}
	if names := userNames(pr.Reviewers); len(names) > 0 {
		fmt.Fprintf(&b, " · reviewers: %s", strings.Join(names, ", "))
	}
	return b.String()
}

// userNames returns a display name for each user: the display name, else the
// nickname, else the UUID.
func userNames(users []api.User) []string {
	names := make([]string, 0, len(users))
	for _, u := range users {
		name := u.DisplayName
		if name == "" {
			name = u.Nickname
		}
		if name == "" {
			name = u.UUID
		}
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

// ensurePushed makes sure head is available on the remote before the PR is
// created, prompting or erroring per the auto-push rules.
//
// The branch is pushed to its own upstream remote (branch.<head>.remote), or
// to "origin" when it has none; never to the base repository's remote, which
// for a fork workflow is the upstream repository the user may not be able to
// push to.
func ensurePushed(ctx context.Context, opts *CreateOptions, gitRunner git.Runner, head string) error {
	upRemote, mergeRef, err := git.BranchUpstream(ctx, gitRunner, head)
	if err != nil {
		return err
	}
	if upRemote == "." {
		// Tracking a local branch: nothing on a remote to compare with.
		upRemote = ""
	}
	remote := upRemote
	if remote == "" {
		remote = "origin"
	}

	if opts.Push {
		return pushBranch(ctx, opts.IO, gitRunner, remote, head)
	}

	upstreamRef := ""
	if upRemote != "" {
		upBranch := strings.TrimPrefix(mergeRef, "refs/heads/")
		if upBranch == "" {
			upBranch = head
		}
		upstreamRef = upRemote + "/" + upBranch
	} else {
		exists, err := git.RemoteBranchExists(ctx, gitRunner, remote, head)
		if err != nil {
			return err
		}
		if !exists {
			if !opts.IO.IsStdinTTY() {
				return fmt.Errorf("branch %s has not been pushed to %s; run with --push or push it first", head, remote)
			}
			ok, err := opts.IO.Prompter().Confirm(fmt.Sprintf("Push branch %s to %s?", head, remote), true)
			if err != nil {
				return err
			}
			if !ok {
				return cmdutil.ErrCancel
			}
			return pushBranch(ctx, opts.IO, gitRunner, remote, head)
		}
		// The branch exists on the remote but is not tracked; still compare
		// with the remote-tracking ref so unpushed commits are reported.
		upstreamRef = remote + "/" + head
	}

	// The ahead check is advisory: when the remote-tracking ref is missing
	// (never fetched, or the upstream is a URL rather than a named remote)
	// it is skipped rather than failing the command.
	ahead, err := git.AheadCount(ctx, gitRunner, head, upstreamRef)
	if err == nil && ahead > 0 {
		cs := opts.IO.ErrColorScheme()
		fmt.Fprintf(opts.IO.ErrOut, "%s local branch is %s ahead of %s; run with --push to update it\n",
			cs.WarningIcon(), pluralize(ahead, "commit"), upstreamRef)
	}
	return nil
}

// pushBranch pushes head to remote and sets it as the upstream. On a terminal
// git's progress is shown as usual; otherwise (scripts, agents) the push is
// quiet, git's output is only surfaced in the error when it fails, and a
// single confirmation line is printed to stderr.
func pushBranch(ctx context.Context, ios *cmdutil.IOStreams, gitRunner git.Runner, remote, head string) error {
	if ios.IsStdoutTTY() {
		return git.Push(ctx, gitRunner, remote, head)
	}
	if err := git.PushQuiet(ctx, gitRunner, remote, head); err != nil {
		return err
	}
	fmt.Fprintf(ios.ErrOut, "%s Pushed %s to %s\n", ios.ErrColorScheme().SuccessIcon(), head, remote)
	return nil
}

// pluralize renders "1 commit" / "3 commits".
func pluralize(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// fillFromCommits derives a title and body from the commits in base..head.
func fillFromCommits(ctx context.Context, gitRunner git.Runner, base, head string) (title, body string, err error) {
	commits, err := git.Commits(ctx, gitRunner, base, head)
	if err != nil {
		return "", "", err
	}
	if len(commits) == 0 {
		return "", "", fmt.Errorf("no commits between %s and %s", base, head)
	}
	if len(commits) == 1 {
		return commits[0].Subject, commits[0].Body, nil
	}

	var b strings.Builder
	// git.Commits returns newest first; list oldest first.
	for i := len(commits) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "- %s\n", commits[i].Subject)
	}
	return humanizeBranch(head), strings.TrimRight(b.String(), "\n"), nil
}

// humanizeBranch turns a branch name into a rough title by replacing
// separators with spaces.
func humanizeBranch(name string) string {
	return strings.NewReplacer("-", " ", "_", " ").Replace(name)
}
