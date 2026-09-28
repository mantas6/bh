package pr

import (
	"context"
	"fmt"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// ApproveOptions holds the dependencies and flags for `bh pr approve`.
type ApproveOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)

	Arg  string
	Undo bool
}

// NewCmdApprove creates the "pr approve" command.
func NewCmdApprove(f *cmdutil.Factory, runF func(*ApproveOptions) error) *cobra.Command {
	opts := &ApproveOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "approve [<number> | <url> | <branch>]",
		Short: "Approve a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return approveRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.Undo, "undo", false, "Remove your approval from the pull request")

	return cmd
}

func approveRun(opts *ApproveOptions) error {
	ctx := context.Background()

	found, err := shared.NewFinder(opts.BaseRepo, opts.ApiClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

	if opts.Undo {
		if err := client.UnapprovePullRequest(ctx, repo.FullName(), pr.ID); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Removed approval from pull request #%d\n", shared.SuccessIcon(opts.IO), pr.ID)
		return nil
	}

	if err := client.ApprovePullRequest(ctx, repo.FullName(), pr.ID); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.ErrOut, "%s Approved pull request #%d\n", shared.SuccessIcon(opts.IO), pr.ID)
	return nil
}
