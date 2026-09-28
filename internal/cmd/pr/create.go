package pr

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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

	Title             string
	Body              string
	BodyFile          string
	Base              string
	Head              string
	Draft             bool
	Reviewers         []string
	CloseSourceBranch bool
	Fill              bool
	Push              bool
	Web               bool
}

// NewCmdCreate creates the "pr create" command.
func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Browser:   f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: cmdutil.Heredoc(`
			Create a pull request on Bitbucket.

			The head branch defaults to the current branch. When it has not been
			pushed yet you are prompted to push it (use --push to skip the prompt,
			required when not running interactively). The base branch defaults to
			the repository's main branch.
		`),
		Example: cmdutil.Heredoc(`
			# Interactively create a PR from the current branch
			$ bh pr create

			# Fill the title and body from the branch's commits
			$ bh pr create --fill

			# Create a PR against a specific base with reviewers
			$ bh pr create --base main --reviewer alice,bob --title "Fix bug"

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
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "The branch to merge into")
	cmd.Flags().StringVarP(&opts.Head, "head", "H", "", "The branch that contains commits (defaults to current branch)")
	cmd.Flags().BoolVar(&opts.Draft, "draft", false, "Mark the pull request as a draft")
	cmd.Flags().StringSliceVarP(&opts.Reviewers, "reviewer", "r", nil, "Request reviews from people (comma-separated)")
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

	// The effective base is needed for --fill commit ranges and the --web
	// URL. When the user did not pass --base we look up the repo's default
	// branch, but we still omit destination from the API create body so the
	// server applies its own default.
	effectiveBase := opts.Base
	if opts.Base == "" && (opts.Fill || opts.Web) {
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

	reviewerUUIDs, err := resolveReviewers(ctx, client, repo.Workspace, opts.Reviewers)
	if err != nil {
		return err
	}

	pr, err := client.CreatePullRequest(ctx, repo.FullName(), api.CreatePRInput{
		Title:             title,
		Description:       body,
		SourceBranch:      head,
		DestinationBranch: opts.Base,
		Reviewers:         reviewerUUIDs,
		Draft:             opts.Draft,
		CloseSourceBranch: opts.CloseSourceBranch,
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, pr.Links.HTML.Href)
	return nil
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
		return git.Push(ctx, gitRunner, remote, head)
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
			return git.Push(ctx, gitRunner, remote, head)
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
