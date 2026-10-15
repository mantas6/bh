package pr

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// shortHashLen is how many characters of a commit hash are shown.
const shortHashLen = 12

// CommitsOptions holds the dependencies and flags for `bh pr commits`.
type CommitsOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg  string
	JSON bool
}

// NewCmdCommits creates the "pr commits" command.
func NewCmdCommits(f *cmdutil.Factory, runF func(*CommitsOptions) error) *cobra.Command {
	opts := &CommitsOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "commits [<number> | <url> | <branch>]",
		Short: "List the commits of a pull request",
		Long: `List the commits of a pull request, newest first, one per line: the short
hash, the author, the date and the first line of the message.

On a terminal the columns are aligned and dates are relative; otherwise the
fields are tab-separated and dates are RFC 3339 timestamps.`,
		Args: cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return commitsRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Output the commits as JSON")

	return cmd
}

func commitsRun(ctx context.Context, opts *CommitsOptions) error {
	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).FindID(ctx, opts.Arg)
	if err != nil {
		return err
	}

	commits, err := found.Client.PullRequestCommits(ctx, found.Repo.FullName(), found.Number)
	if err != nil {
		return err
	}

	if opts.JSON {
		return cmdutil.PrintJSON(opts.IO.Out, commits)
	}

	isTTY := opts.IO.IsStdoutTTY()
	if len(commits) == 0 {
		if isTTY {
			fmt.Fprintf(opts.IO.Out, "No commits in pull request #%d\n", found.Number)
		}
		return nil
	}

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	cs := opts.IO.ColorScheme()

	table := output.NewTable(opts.IO.Out, isTTY)
	for _, c := range commits {
		date := c.Date.Format(time.RFC3339)
		if isTTY {
			date = output.RelativeTime(c.Date, now())
		}
		table.AddRow(
			cs.Yellow(shortHash(c.Hash)),
			commitAuthor(c.Author),
			date,
			commitSubject(c.Message),
		)
	}
	return table.Flush()
}

func shortHash(hash string) string {
	if len(hash) > shortHashLen {
		return hash[:shortHashLen]
	}
	return hash
}

// commitAuthor names a commit author: the linked Bitbucket account when
// there is one, otherwise the name from the raw "Name <email>" git author.
func commitAuthor(a api.CommitAuthor) string {
	if name := shared.UserName(a.User); name != "" {
		return name
	}
	if addr, err := mail.ParseAddress(a.Raw); err == nil && addr.Name != "" {
		return addr.Name
	}
	if name, _, ok := strings.Cut(a.Raw, "<"); ok && strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return strings.TrimSpace(a.Raw)
}

// commitSubject is the first line of a commit message.
func commitSubject(msg string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(msg), "\n")
	return strings.TrimSpace(line)
}
