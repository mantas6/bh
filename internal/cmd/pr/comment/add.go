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

// AddOptions holds the dependencies and flags for `bh pr comment add`.
type AddOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)

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
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "add [<number> | <url> | <branch>]",
		Short: "Add a comment to a pull request",
		Long: cmdutil.Heredoc(`
			Add a comment to a pull request.

			The body is taken from --body, --body-file (use "-" for standard input),
			or, when neither is given and standard input is not a terminal, the whole
			of standard input. Pass --path (and optionally --line) to attach the
			comment inline to a file in the diff.
		`),
		Example: cmdutil.Heredoc(`
			# Comment on the PR for the current branch
			$ bh pr comment add --body "Looks good to me"

			# Comment on PR 123 from a file
			$ bh pr comment add 123 --body-file notes.md

			# Add an inline comment on a specific line
			$ bh pr comment add 123 --path main.go --line 42 --body "Rename this"

			# Pipe the body from another command
			$ echo "Nice work" | bh pr comment add 123
		`),
		Args: cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			opts.lineSet = cmd.Flags().Changed("line")
			if err := cmdutil.MutuallyExclusive("specify only one of --body or --body-file",
				cmd.Flags().Changed("body"), cmd.Flags().Changed("body-file")); err != nil {
				return err
			}
			if opts.lineSet && opts.Path == "" {
				return cmdutil.FlagErrorf("--line requires --path")
			}
			if opts.lineSet && opts.Line <= 0 {
				return cmdutil.FlagErrorf("invalid value for --line: %d; line numbers start at 1", opts.Line)
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

	cmdutil.AddBodyFlags(cmd, &opts.Body, &opts.BodyFile)
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
		return errBodyRequired
	}

	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

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

// errBodyRequired is returned by add and reply when no comment body was
// given. It is a FlagError, like the equivalent `bh pr review --comment`
// error, so usage is shown.
var errBodyRequired = cmdutil.FlagErrorf("a comment body is required; use --body, --body-file, or pipe it on standard input")

// resolveBody derives comment body from -b, -F ("-" = stdin), or, if neither
// is set and stdin is not a TTY, the entirety of stdin.
func resolveBody(ios *cmdutil.IOStreams, body, bodyFile string) (string, error) {
	switch {
	case body != "":
		return body, nil
	case bodyFile != "":
		return shared.ReadBodyFile(ios, bodyFile)
	case !ios.IsStdinTTY():
		return shared.ReadBodyFile(ios, "-")
	default:
		return "", nil
	}
}

// printCreated prints the created comment's URL to Out, or, when the URL is
// unavailable, a success line to ErrOut like the other comment commands.
func printCreated(ios *cmdutil.IOStreams, c *api.Comment) {
	if c.Links.HTML.Href != "" {
		fmt.Fprintln(ios.Out, c.Links.HTML.Href)
		return
	}
	fmt.Fprintf(ios.ErrOut, "%s Added comment #%d\n", ios.ErrColorScheme().SuccessIcon(), c.ID)
}
