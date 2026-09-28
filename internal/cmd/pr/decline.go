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

// DeclineOptions holds the dependencies and flags for `bh pr decline`.
type DeclineOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)

	Arg          string
	DeleteBranch bool
}

// NewCmdDecline creates the "pr decline" command.
func NewCmdDecline(f *cmdutil.Factory, runF func(*DeclineOptions) error) *cobra.Command {
	opts := &DeclineOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:     "decline [<number> | <url> | <branch>]",
		Aliases: []string{"close"},
		Short:   "Decline a pull request",
		Args:    cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return declineRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.DeleteBranch, "delete-branch", "d", false, "Delete the source branch after declining")

	return cmd
}

func declineRun(opts *DeclineOptions) error {
	ctx := context.Background()

	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

	if !strings.EqualFold(pr.State, "OPEN") {
		return fmt.Errorf("pull request #%d is %s; only open pull requests can be declined",
			pr.ID, strings.ToLower(pr.State))
	}

	cs := opts.IO.ErrColorScheme()
	branch := pr.Source.Branch.Name

	// Deleting the branch pushes a deletion to the local git remote for the
	// PR's repo, so one must be known; there is deliberately no fallback to
	// "origin", which could point at an unrelated repository. Check before
	// declining so the command doesn't half-succeed.
	deleteBranch := opts.DeleteBranch && shared.SameRepoPR(pr, repo)
	if deleteBranch && found.Remote == nil {
		return fmt.Errorf("cannot delete branch %s: no git remote found for %s", branch, repo.FullName())
	}

	if _, err := client.DeclinePullRequest(ctx, repo.FullName(), pr.ID); err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Declined pull request #%d (%s)\n", cs.SuccessIcon(), pr.ID, pr.Title)

	if opts.DeleteBranch && !deleteBranch {
		fmt.Fprintf(opts.IO.ErrOut, "%s Skipped deleting branch %s: it is not in %s\n", cs.WarningIcon(), branch, repo.FullName())
		return nil
	}
	if !deleteBranch {
		return nil
	}

	gitRunner, err := opts.Git()
	if err != nil {
		return err
	}
	// Delete the local branch first: `git branch -d` accepts a branch that is
	// fully pushed to its upstream, which only holds while the remote branch
	// still exists.
	if err := deleteLocalSourceBranch(ctx, gitRunner, opts.IO, branch, pr.Destination.Branch.Name, false); err != nil {
		return err
	}
	if err := git.PushDelete(ctx, gitRunner, found.Remote.Remote.Name, branch); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted branch %s from %s\n", cs.SuccessIcon(), branch, found.Remote.Remote.Name)
	return nil
}
