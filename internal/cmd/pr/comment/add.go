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

// AddOptions holds the dependencies and flags for `bh pr comment add`.
type AddOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg      string
	Body     string
	BodyFile string
	Path     string
	Line     int
	Side     string

	lineSet bool
}

// NewCmdAdd creates the "pr comment add" command.
func NewCmdAdd(f *cmdutil.Factory, runF func(*AddOptions) error) *cobra.Command {
	opts := &AddOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "add [<number> | <url> | <branch>]",
		Short: "Add a comment to a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			opts.lineSet = cmd.Flags().Changed("line")
			if opts.Body != "" && opts.BodyFile != "" {
				return cmdutil.FlagErrorf("specify only one of --body or --body-file")
			}
			if opts.lineSet && opts.Path == "" {
				return cmdutil.FlagErrorf("--line requires --path")
			}
			switch opts.Side {
			case "", "new", "old":
			default:
				return cmdutil.FlagErrorf("--side must be \"new\" or \"old\"")
			}
			if runF != nil {
				return runF(opts)
			}
			return addRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Comment body text")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" for stdin)")
	cmd.Flags().StringVar(&opts.Path, "path", "", "Path of the file to comment on (inline)")
	cmd.Flags().IntVar(&opts.Line, "line", 0, "Line number to comment on (requires --path)")
	cmd.Flags().StringVar(&opts.Side, "side", "new", "Side of the diff for --line: new or old")

	return cmd
}

func addRun(opts *AddOptions) error {
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

	in := api.CommentInput{Body: body, Path: opts.Path}
	if opts.Path != "" && opts.lineSet {
		line := opts.Line
		if strings.EqualFold(opts.Side, "old") {
			in.From = &line
		} else {
			in.To = &line
		}
	}

	comment, err := client.CreateComment(ctx, repo.FullName(), pr.ID, in)
	if err != nil {
		return err
	}

	printCreated(opts.IO, comment)
	return nil
}

// resolveBody derives comment body from -b, -F ("-" = stdin), or, if neither
// is set and stdin is not a TTY, the entirety of stdin.
func resolveBody(ios *cmdutil.IOStreams, body, bodyFile string) (string, error) {
	switch {
	case body != "":
		return body, nil
	case bodyFile != "":
		return readBodyFile(ios, bodyFile)
	case !ios.IsStdinTTY():
		return readBodyFile(ios, "-")
	default:
		return "", nil
	}
}

// printCreated prints the created comment's URL, or a success line if the URL
// is unavailable.
func printCreated(ios *cmdutil.IOStreams, c *api.Comment) {
	if c.Links.HTML.Href != "" {
		fmt.Fprintln(ios.Out, c.Links.HTML.Href)
		return
	}
	fmt.Fprintf(ios.Out, "%s Added comment #%d\n", successIcon(ios), c.ID)
}
