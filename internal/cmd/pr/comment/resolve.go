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

	Arg        string
	CommentIDs []int
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

	use, short, verb := "resolve", "Resolve pull request comment threads", "Resolve"
	if reopen {
		use, short, verb = "reopen", "Reopen (unresolve) pull request comment threads", "Reopen"
	}

	return &cobra.Command{
		Use:   use + " {<number> | <url> | <branch>} <comment-id>...",
		Short: short,
		Long: verb + ` the threads of one or more top-level comments, by the ids shown by
` + "`bh pr comment list`" + `. Each id is reported on stderr; when any of them fails
the others are still attempted and the command exits non-zero.`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 2 {
				return cmdutil.FlagErrorf("a pull request and at least one comment id are required")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Arg = args[0]
			opts.CommentIDs = nil
			for _, a := range args[1:] {
				id, err := parseCommentID(a)
				if err != nil {
					return cmdutil.FlagErrorWrap(err)
				}
				opts.CommentIDs = append(opts.CommentIDs, id)
			}
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
	cs := opts.IO.ErrColorScheme()

	verb, action := "Resolved", "resolve"
	if opts.Reopen {
		verb, action = "Reopened", "reopen"
	}

	failed := 0
	for _, id := range opts.CommentIDs {
		if opts.Reopen {
			err = client.ReopenComment(ctx, repo, found.Number, id)
		} else {
			err = client.ResolveComment(ctx, repo, found.Number, id)
		}
		if err != nil {
			// A single id keeps the plain error; an interrupt stops the batch.
			if len(opts.CommentIDs) == 1 || ctx.Err() != nil {
				return err
			}
			fmt.Fprintf(opts.IO.ErrOut, "%s Failed to %s comment #%d: %v\n", cs.FailureIcon(), action, id, err)
			failed++
			continue
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s %s comment #%d\n", cs.SuccessIcon(), verb, id)
	}
	if failed > 0 {
		return cmdutil.ErrSilent
	}
	return nil
}
