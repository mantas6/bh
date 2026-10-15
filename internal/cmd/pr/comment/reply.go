package comment

import (
	"context"
	"fmt"
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
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)

	Arg       string
	CommentID int
	Body      string
	BodyFile  string
	// Resolve resolves the comment's thread once the reply is posted.
	Resolve bool
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
		Long: `Reply to a pull request comment and print the reply's URL.

<comment-id> is the id shown by ` + "`bh pr comment list`" + ` (any comment of a
thread). With --resolve the thread is resolved after the reply is posted; if
posting the reply fails nothing is resolved.`,
		Example: cmdutil.Examples(`
			# Find the comment id: the "#881626438" in a "#id author · time" header
			$ bh pr comment list 123 --unresolved

			# Reply to it, then resolve its thread
			$ bh pr comment reply 123 881626438 --body "Fixed in 63a5b8b" --resolve

			# Or take the id from the end of the URL printed by comment add or reply
			# (https://bitbucket.org/ws/repo/pull-requests/123/_/diff#comment-881626440)
			$ bh pr comment reply 123 881626440 --body-file - <<'EOF'
			Good catch, see the other call site too.
			EOF
		`),
		Args: cmdutil.ExactArgs(2, "a pull request and a comment id are required"),
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
			return replyRun(cmd.Context(), opts)
		},
	}

	cmdutil.AddBodyFlags(cmd, &opts.Body, &opts.BodyFile)
	cmd.Flags().BoolVar(&opts.Resolve, "resolve", false, "Resolve the comment thread after replying")

	return cmd
}

func replyRun(ctx context.Context, opts *ReplyOptions) error {
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

	if !opts.Resolve {
		return nil
	}
	return resolveThread(ctx, opts.IO, client, repo.FullName(), pr.ID, opts.CommentID)
}

// maxThreadDepth bounds the walk up a thread's parent links.
const maxThreadDepth = 100

// resolveThread resolves the thread containing commentID, whose resolution
// Bitbucket records on the top-level comment, and reports it on stderr.
func resolveThread(ctx context.Context, ios *cmdutil.IOStreams, client *api.Client, repo string, prID, commentID int) error {
	root, err := client.GetComment(ctx, repo, prID, commentID)
	for i := 0; err == nil && root.Parent != nil && i < maxThreadDepth; i++ {
		root, err = client.GetComment(ctx, repo, prID, root.Parent.ID)
	}
	if err == nil && root.Resolution == nil {
		err = client.ResolveComment(ctx, repo, prID, root.ID)
	}
	if err != nil {
		return fmt.Errorf("reply posted, but resolving the thread of comment #%d failed: %w", commentID, err)
	}
	fmt.Fprintf(ios.ErrOut, "%s Resolved comment #%d\n", ios.ErrColorScheme().SuccessIcon(), root.ID)
	return nil
}
