package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// MergeOptions holds the dependencies and flags for `bh pr merge`.
type MergeOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)

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
		ApiClient: f.ApiClient,
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
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if mergeStrategyCount(opts) > 1 {
				return cmdutil.FlagErrorf("specify only one of --merge, --squash, or --fast-forward")
			}
			if runF != nil {
				return runF(opts)
			}
			return mergeRun(opts)
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

func mergeStrategyCount(opts *MergeOptions) int {
	n := 0
	for _, b := range []bool{opts.Merge, opts.Squash, opts.FastForward} {
		if b {
			n++
		}
	}
	return n
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

func mergeRun(opts *MergeOptions) error {
	ctx := context.Background()

	baseRepo, resolvedRemote, err := opts.BaseRepo()
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

	pr, repo, err := FindPR(ctx, client, gitRunner, baseRepo, opts.Arg)
	if err != nil {
		return err
	}

	if !strings.EqualFold(pr.State, "OPEN") {
		return fmt.Errorf("pull request #%d is %s; only open pull requests can be merged",
			pr.ID, strings.ToLower(pr.State))
	}

	strategy := flagStrategy(opts)
	if strategy == "" {
		strategy = pr.Destination.Branch.DefaultMergeStrategy
		if strategy == "" {
			strategy = "merge_commit"
		}
	}

	if flagStrategy(opts) == "" && !opts.Yes {
		if !opts.IO.IsStdinTTY() {
			return fmt.Errorf("specify a merge strategy (--merge, --squash, --fast-forward) or --yes when not running interactively")
		}
		fmt.Fprintf(opts.IO.ErrOut, "Merge pull request #%d (%s) into %s using %s? [Y/n] ",
			pr.ID, pr.Title, pr.Destination.Branch.Name, strategy)
		ans, err := readLine(opts.IO.In)
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(ans)) {
		case "", "y", "yes":
			// proceed
		default:
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

	fmt.Fprintf(opts.IO.ErrOut, "%s Merged pull request #%d (%s)\n", successIcon(opts.IO), pr.ID, pr.Title)

	if opts.DeleteBranch && sameRepoPR(pr, baseRepo) {
		_ = resolvedRemote // remote source branch is closed by the API
		if err := deleteLocalSourceBranch(ctx, gitRunner, opts.IO, pr.Source.Branch.Name, pr.Destination.Branch.Name); err != nil {
			return err
		}
	}

	return nil
}
