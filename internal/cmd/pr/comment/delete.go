package comment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// DeleteOptions holds the dependencies and flags for `bh pr comment delete`.
type DeleteOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg       string
	CommentID int
	Yes       bool
}

// NewCmdDelete creates the "pr comment delete" command.
func NewCmdDelete(f *cmdutil.Factory, runF func(*DeleteOptions) error) *cobra.Command {
	opts := &DeleteOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "delete <number> <comment-id>",
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
		fmt.Fprintf(opts.IO.ErrOut, "Delete comment #%d? [y/N] ", opts.CommentID)
		ans, err := readLine(opts.IO.In)
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(ans)) {
		case "y", "yes":
			// proceed
		default:
			return cmdutil.ErrCancel
		}
	}

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

	if err := client.DeleteComment(ctx, repo.FullName(), pr.ID, opts.CommentID); err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.ErrOut, "%s Deleted comment #%d\n", successIcon(opts.IO), opts.CommentID)
	return nil
}
