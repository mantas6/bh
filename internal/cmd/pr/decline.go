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

	// Deleting the branch needs a local remote for the PR's repo; check before
	// declining so the command doesn't half-succeed.
	deleteBranch := opts.DeleteBranch && shared.SameRepoPR(pr, repo)
	if deleteBranch && found.Remote == nil {
		return fmt.Errorf("cannot delete branch %s: no git remote found for %s", pr.Source.Branch.Name, repo.FullName())
	}

	if _, err := client.DeclinePullRequest(ctx, repo.FullName(), pr.ID); err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Declined pull request #%d (%s)\n", opts.IO.ErrColorScheme().SuccessIcon(), pr.ID, pr.Title)

	if deleteBranch {
		gitRunner, err := opts.Git()
		if err != nil {
			return err
		}
		if err := deleteLocalSourceBranch(ctx, gitRunner, opts.IO, pr.Source.Branch.Name, pr.Destination.Branch.Name); err != nil {
			return err
		}
		if err := git.PushDelete(ctx, gitRunner, remoteName(found.Remote), pr.Source.Branch.Name); err != nil {
			return err
		}
	}

	return nil
}
