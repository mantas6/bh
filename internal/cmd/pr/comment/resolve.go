package comment

import (
	"context"
	"fmt"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// ResolveOptions holds the dependencies for `bh pr comment resolve` and
// `bh pr comment reopen`.
type ResolveOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)

	Arg       string
	CommentID int
	// Reopen reopens (unresolves) the thread instead of resolving it.
	Reopen bool
}

// newCmdResolveOrReopen builds the resolve and reopen commands, which differ
// only in their name, help text and the Reopen option.
func newCmdResolveOrReopen(f *cmdutil.Factory, runF func(*ResolveOptions) error, reopen bool) *cobra.Command {
	opts := &ResolveOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Reopen:    reopen,
	}

	use, short := "resolve", "Resolve a pull request comment thread"
	if reopen {
		use, short = "reopen", "Reopen (unresolve) a pull request comment thread"
	}

	return &cobra.Command{
		Use:   use + " {<number> | <url> | <branch>} <comment-id>",
		Short: short,
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
			return resolveRun(cmd.Context(), opts)
		},
	}
}

// NewCmdResolve creates the "pr comment resolve" command.
func NewCmdResolve(f *cmdutil.Factory, runF func(*ResolveOptions) error) *cobra.Command {
	return newCmdResolveOrReopen(f, runF, false)
}

func resolveRun(ctx context.Context, opts *ResolveOptions) error {
	// Only the PR id is needed, so a numeric argument skips the PR fetch.
	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).FindID(ctx, opts.Arg)
	if err != nil {
		return err
	}
	repo, client := found.Repo.FullName(), found.Client

	verb := "Resolved"
	if opts.Reopen {
		verb = "Reopened"
		err = client.ReopenComment(ctx, repo, found.Number, opts.CommentID)
	} else {
		err = client.ResolveComment(ctx, repo, found.Number, opts.CommentID)
	}
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s %s comment #%d\n", opts.IO.ErrColorScheme().SuccessIcon(), verb, opts.CommentID)
	return nil
}
