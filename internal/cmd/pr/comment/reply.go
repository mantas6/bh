package comment

import (
	"context"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// ReplyOptions holds the dependencies and flags for `bh pr comment reply`.
type ReplyOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)

	Arg       string
	CommentID int
	Body      string
	BodyFile  string
}

// NewCmdReply creates the "pr comment reply" command.
func NewCmdReply(f *cmdutil.Factory, runF func(*ReplyOptions) error) *cobra.Command {
	opts := &ReplyOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "reply {<number> | <url> | <branch>} <comment-id>",
		Short: "Reply to a pull request comment",
		Args:  cmdutil.ExactArgs(2, "a pull request and a comment id are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Arg = args[0]
			id, err := parseCommentID(args[1])
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.CommentID = id
			if err := cmdutil.MutuallyExclusive("specify only one of --body or --body-file",
				cmd.Flags().Changed("body"), cmd.Flags().Changed("body-file")); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return replyRun(opts)
		},
	}

	cmdutil.AddBodyFlags(cmd, &opts.Body, &opts.BodyFile)

	return cmd
}

func replyRun(opts *ReplyOptions) error {
	ctx := context.Background()

	body, err := resolveBody(opts.IO, opts.Body, opts.BodyFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return errBodyRequired
	}

	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

	comment, err := client.CreateComment(ctx, repo.FullName(), pr.ID, api.CommentInput{
		Body:     body,
		ParentID: opts.CommentID,
	})
	if err != nil {
		return err
	}

	printCreated(opts.IO, comment)
	return nil
}
