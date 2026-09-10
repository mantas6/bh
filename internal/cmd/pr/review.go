package pr

import (
	"context"
	"fmt"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// ReviewOptions holds the dependencies and flags for `bh pr review`.
type ReviewOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)

	Arg            string
	Approve        bool
	RequestChanges bool
	Comment        bool
	Body           string
	BodyFile       string
}

// NewCmdReview creates the "pr review" command.
func NewCmdReview(f *cmdutil.Factory, runF func(*ReviewOptions) error) *cobra.Command {
	opts := &ReviewOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "review [<number> | <url> | <branch>]",
		Short: "Add a review to a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			modes := 0
			for _, b := range []bool{opts.Approve, opts.RequestChanges, opts.Comment} {
				if b {
					modes++
				}
			}
			if modes != 1 {
				return cmdutil.FlagErrorf("specify exactly one of --approve, --request-changes, or --comment")
			}
			if opts.Body != "" && opts.BodyFile != "" {
				return cmdutil.FlagErrorf("specify only one of --body or --body-file")
			}
			if runF != nil {
				return runF(opts)
			}
			return reviewRun(opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.Approve, "approve", "a", false, "Approve the pull request")
	cmd.Flags().BoolVarP(&opts.RequestChanges, "request-changes", "r", false, "Request changes on the pull request")
	cmd.Flags().BoolVarP(&opts.Comment, "comment", "c", false, "Comment on the pull request")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "The body of the review")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" for stdin)")

	return cmd
}

func reviewRun(opts *ReviewOptions) error {
	ctx := context.Background()

	body := opts.Body
	if opts.BodyFile != "" {
		b, err := readBodyFile(opts.IO, opts.BodyFile)
		if err != nil {
			return err
		}
		body = b
	}

	if opts.Comment && body == "" {
		return cmdutil.FlagErrorf("a body is required when commenting; use --body or --body-file")
	}

	baseRepo, _, err := opts.BaseRepo()
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

	pr, repo, err := FindPR(ctx, client, gitRunner, baseRepo, opts.Arg)
	if err != nil {
		return err
	}

	fullName := repo.FullName()

	switch {
	case opts.Approve:
		if err := client.ApprovePullRequest(ctx, fullName, pr.ID); err != nil {
			return err
		}
		if body != "" {
			if _, err := client.CreateComment(ctx, fullName, pr.ID, api.CommentInput{Body: body}); err != nil {
				return err
			}
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Approved pull request #%d\n", successIcon(opts.IO), pr.ID)
	case opts.RequestChanges:
		if err := client.RequestChanges(ctx, fullName, pr.ID); err != nil {
			return err
		}
		if body != "" {
			if _, err := client.CreateComment(ctx, fullName, pr.ID, api.CommentInput{Body: body}); err != nil {
				return err
			}
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Requested changes on pull request #%d\n", successIcon(opts.IO), pr.ID)
	case opts.Comment:
		if _, err := client.CreateComment(ctx, fullName, pr.ID, api.CommentInput{Body: body}); err != nil {
			return err
		}
		fmt.Fprintf(opts.IO.ErrOut, "%s Commented on pull request #%d\n", successIcon(opts.IO), pr.ID)
	}

	return nil
}
