package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// MergeOptions holds the dependencies and flags for `bh pr merge`.
type MergeOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)

	Arg          string
	Merge        bool
	Squash       bool
	FastForward  bool
	Message      string
	DeleteBranch bool
	Yes          bool
}

// NewCmdMerge creates the "pr merge" command.
func NewCmdMerge(f *cmdutil.Factory, runF func(*MergeOptions) error) *cobra.Command {
	opts := &MergeOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "merge [<number> | <url> | <branch>]",
		Short: "Merge a pull request",
		Long: cmdutil.Heredoc(`
			Merge a pull request on Bitbucket.

			Without a merge-strategy flag the destination branch's default strategy
			is used and, on a terminal, you are asked to confirm. When not running
			interactively either pass a strategy flag or --yes.

			With no argument the pull request for the current branch is merged.
		`),
		Example: cmdutil.Heredoc(`
			# Merge the PR for the current branch, prompting for confirmation
			$ bh pr merge

			# Squash-merge PR 123 and delete its source branch
			$ bh pr merge 123 --squash --delete-branch

			# Merge non-interactively with a merge commit
			$ bh pr merge 123 --merge --yes
		`),
		Args: cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if err := cmdutil.MutuallyExclusive("specify only one of --merge, --squash, or --fast-forward",
				opts.Merge, opts.Squash, opts.FastForward); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return mergeRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Merge, "merge", false, "Merge with a merge commit")
	cmd.Flags().BoolVar(&opts.Squash, "squash", false, "Squash the commits into a single commit")
	cmd.Flags().BoolVar(&opts.FastForward, "fast-forward", false, "Fast-forward the destination branch")
	cmd.Flags().StringVarP(&opts.Message, "message", "m", "", "Merge commit message")
	cmd.Flags().BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Delete the source branch after merge")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip the merge confirmation prompt")

	return cmd
}

// flagStrategy returns the merge strategy selected by a flag, or "" if none.
func flagStrategy(opts *MergeOptions) string {
	switch {
	case opts.Merge:
		return "merge_commit"
	case opts.Squash:
		return "squash"
	case opts.FastForward:
		return "fast_forward"
	default:
		return ""
	}
}

func mergeRun(ctx context.Context, opts *MergeOptions) error {
	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

	if !strings.EqualFold(pr.State, "OPEN") {
		return fmt.Errorf("pull request #%d is %s; only open pull requests can be merged",
			pr.ID, strings.ToLower(pr.State))
	}

	// Without a strategy flag nothing is sent, so Bitbucket applies the
	// destination branch's configured default.
	strategy := flagStrategy(opts)

	if strategy == "" && !opts.Yes {
		if !opts.IO.IsStdinTTY() {
			return fmt.Errorf("specify a merge strategy (--merge, --squash, --fast-forward) or --yes when not running interactively")
		}
		using := "the default merge strategy"
		if s := pr.Destination.Branch.DefaultMergeStrategy; s != "" {
			using = s
		}
		ok, err := opts.IO.Prompter().Confirm(fmt.Sprintf("Merge pull request #%d (%s) into %s using %s?",
			pr.ID, pr.Title, pr.Destination.Branch.Name, using), true)
		if err != nil {
			return err
		}
		if !ok {
			return cmdutil.ErrCancel
		}
	}

	if _, err := client.MergePullRequest(ctx, repo.FullName(), pr.ID, api.MergeInput{
		Message:           opts.Message,
		Strategy:          strategy,
		CloseSourceBranch: opts.DeleteBranch,
	}); err != nil {
		return err
	}

	cs := opts.IO.ErrColorScheme()
	fmt.Fprintf(opts.IO.ErrOut, "%s Merged pull request #%d (%s)\n", cs.SuccessIcon(), pr.ID, pr.Title)

	// The remote source branch is closed by the API; only the local branch
	// is cleaned up, and only when this checkout has a remote for the PR's
	// repo (i.e. the working directory is a clone of it). There is no
	// fallback to "origin".
	if !opts.DeleteBranch || !shared.SameRepoPR(pr, repo) {
		return nil
	}
	if found.Remote == nil {
		fmt.Fprintf(opts.IO.ErrOut, "%s Skipped deleting local branch %s: no git remote found for %s\n",
			cs.WarningIcon(), pr.Source.Branch.Name, repo.FullName())
		return nil
	}
	gitRunner, err := opts.Git()
	if err != nil {
		return err
	}
	// The PR is merged, but a squash merge leaves the local commits
	// unreachable from the destination, so the branch is force-deleted.
	return deleteLocalSourceBranch(ctx, gitRunner, opts.IO, pr.Source.Branch.Name, pr.Destination.Branch.Name, true)
}
