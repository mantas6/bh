package comment

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// ReplyOptions holds the dependencies and flags for `bh pr comment reply`.
type ReplyOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg       string
	CommentID int
	Body      string
	BodyFile  string
}

// NewCmdReply creates the "pr comment reply" command.
func NewCmdReply(f *cmdutil.Factory, runF func(*ReplyOptions) error) *cobra.Command {
	opts := &ReplyOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "reply <number> <comment-id>",
		Short: "Reply to a pull request comment",
		Args:  cmdutil.ExactArgs(2, "a pull request and a comment id are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Arg = args[0]
			id, err := parseCommentID(args[1])
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.CommentID = id
			if opts.Body != "" && opts.BodyFile != "" {
				return cmdutil.FlagErrorf("specify only one of --body or --body-file")
			}
			if runF != nil {
				return runF(opts)
			}
			return replyRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Reply body text")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" for stdin)")

	return cmd
}

func replyRun(opts *ReplyOptions) error {
	ctx := context.Background()

	body, err := resolveBody(opts.IO, opts.Body, opts.BodyFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return errors.New("comment body is required (use -b, -F, or pipe via stdin)")
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
