package comment

import (
	"context"
	"fmt"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// ListOptions holds the dependencies and flags for `bh pr comment list`.
type ListOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg        string
	JSON       bool
	Unresolved bool
	Limit      int
}

// NewCmdList creates the "pr comment list" command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list [<number> | <url> | <branch>]",
		Short: "List comments on a pull request",
		Args:  cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if opts.Limit < 0 {
				return cmdutil.FlagErrorf("invalid value for --limit: %d", opts.Limit)
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output comments as JSON")
	cmd.Flags().BoolVar(&opts.Unresolved, "unresolved", false, "Show only unresolved comment threads")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 0, "Maximum number of comment threads to show (0 = all)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

	// Fetch every comment: the limit counts threads, which can only be
	// assembled once replies are known.
	comments, err := client.ListComments(ctx, repo.FullName(), pr.ID, 0)
	if err != nil {
		return err
	}

	threads := shared.BuildThreads(comments, opts.Unresolved)
	if opts.Limit > 0 && len(threads.Roots) > opts.Limit {
		threads.Roots = threads.Roots[:opts.Limit]
	}

	if opts.JSON {
		return cmdutil.PrintJSON(opts.IO.Out, threads.JSON(comments))
	}

	if len(threads.Roots) == 0 {
		fmt.Fprintf(opts.IO.Out, "No comments on pull request #%d\n", pr.ID)
		return nil
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	return shared.WriteThreads(opts.IO.Out, opts.IO.ColorScheme(), now(), threads)
}
