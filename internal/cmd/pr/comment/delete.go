package comment

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// DeleteOptions holds the dependencies and flags for `bh pr comment delete`.
type DeleteOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)

	Arg       string
	CommentID int
	Yes       bool
}

// NewCmdDelete creates the "pr comment delete" command.
func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "delete {<number> | <url> | <branch>} <comment-id>",
		Short: "Delete a pull request comment",
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
			return deleteRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "Skip the confirmation prompt")

	return cmd
}

func deleteRun(opts *DeleteOptions) error {
	ctx := context.Background()

	if !opts.Yes {
		if !opts.IO.IsStdinTTY() {
			return errors.New("--yes required when not running interactively")
		}
		ok, err := opts.IO.Prompter().Confirm(fmt.Sprintf("Delete comment #%d?", opts.CommentID), false)
		if err != nil {
			return err
		}
		if !ok {
			return cmdutil.ErrCancel
		}
	}

	// Only the PR id is needed, so a numeric argument skips the PR fetch.
	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).FindID(ctx, opts.Arg)
	if err != nil {
		return err
	}

	if err := found.Client.DeleteComment(ctx, found.Repo.FullName(), found.Number, opts.CommentID); err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted comment #%d\n", opts.IO.ErrColorScheme().SuccessIcon(), opts.CommentID)
	return nil
}
