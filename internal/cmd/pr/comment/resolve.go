package comment

import (
	"context"
	"fmt"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// ResolveOptions holds the dependencies for `bh pr comment resolve` and
// `bh pr comment reopen`.
type ResolveOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg       string
	CommentID int
}

func newResolveOptions(f *cmdutil.Factory) *ResolveOptions {
	return &ResolveOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}
}

// NewCmdResolve creates the "pr comment resolve" command.
func NewCmdResolve(f *cmdutil.Factory, runF func(*ResolveOptions) error) *cobra.Command {
	opts := newResolveOptions(f)

	cmd := &cobra.Command{
		Use:   "resolve <number> <comment-id>",
		Short: "Resolve a pull request comment thread",
		Args:  cmdutil.ExactArgs(2, "a pull request and a comment id are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Arg = args[0]
			id, err := parseCommentID(args[1])
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.CommentID = id
			if runF != nil {
				return runF(opts)
			}
			return resolveRun(opts, false)
		},
	}

	return cmd
}

func resolveRun(opts *ResolveOptions, reopen bool) error {
	ctx := context.Background()

	repo, _, err := opts.BaseRepo()
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

	pr, repo, err := resolvePR(ctx, client, gitRunner, repo, opts.Arg)
	if err != nil {
		return err
	}

	if reopen {
		if err := client.ReopenComment(ctx, repo.FullName(), pr.ID, opts.CommentID); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.Out, "%s Reopened comment #%d\n", successIcon(opts.IO), opts.CommentID)
		return nil
	}

	if err := client.ResolveComment(ctx, repo.FullName(), pr.ID, opts.CommentID); err != nil {
		return err
	}
	fmt.Fprintf(opts.IO.Out, "%s Resolved comment #%d\n", successIcon(opts.IO), opts.CommentID)
	return nil
}
