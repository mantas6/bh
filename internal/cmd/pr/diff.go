package pr

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// DiffOptions holds the dependencies and flags for `bh pr diff`.
type DiffOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Browser   cmdutil.Browser

	Arg      string
	NameOnly bool
	Stat     bool
	Web      bool
}

// NewCmdDiff creates the "pr diff" command.
func NewCmdDiff(f *cmdutil.Factory, runF func(*DiffOptions) error) *cobra.Command {
	opts := &DiffOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Browser:   f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "diff [<number> | <url> | <branch>]",
		Short: "View changes in a pull request",
		Long: `View the changes in a pull request as a unified diff.

With --name-only, print only the paths of the changed files, one per line.
With --stat, print one line per changed file: path, status (added, modified,
removed, renamed, ...) and the added/removed line counts. A renamed file is
shown as "old → new". On a terminal the columns are aligned and a summary
line follows; otherwise fields are tab-separated.`,
		Example: `  # Diff of the pull request for the current branch
  bh pr diff

  # Changed files of pull request 123
  bh pr diff 123 --name-only

  # Per-file line counts
  bh pr diff 123 --stat`,
		Args: cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if err := cmdutil.MutuallyExclusive("specify only one of --name-only, --stat or --web",
				opts.NameOnly, opts.Stat, opts.Web); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return diffRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.NameOnly, "name-only", false, "Display only the names of changed files")
	cmd.Flags().BoolVar(&opts.Stat, "stat", false, "Display the status and line counts of each changed file")
	cmd.Flags().BoolVar(&opts.Web, "web", false, "Open the pull request diff in the web browser")

	return cmd
}

func diffRun(ctx context.Context, opts *DiffOptions) error {
	finder := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git)

	if opts.Web {
		found, err := finder.Find(ctx, opts.Arg)
		if err != nil {
			return err
		}
		return cmdutil.OpenInBrowser(opts.IO, opts.Browser, strings.TrimRight(found.PR.Links.HTML.Href, "/")+"/diff")
	}

	found, err := finder.FindID(ctx, opts.Arg)
	if err != nil {
		return err
	}
	repo := found.Repo.FullName()

	if opts.NameOnly || opts.Stat {
		stats, err := found.Client.PullRequestDiffStat(ctx, repo, found.Number)
		if err != nil {
			return err
		}
		if opts.NameOnly {
			for _, s := range stats {
				fmt.Fprintln(opts.IO.Out, diffStatPath(s))
			}
			return nil
		}
		return printDiffStat(opts.IO, stats)
	}

	diff, err := found.Client.PullRequestDiff(ctx, repo, found.Number)
	if err != nil {
		return err
	}
	if opts.IO.ColorEnabled() {
		return writeColoredDiff(opts.IO.Out, opts.IO.ColorScheme(), diff)
	}
	_, err = opts.IO.Out.Write(diff)
	return err
}

// diffStatPath is the path of the file after the change, or before it when
// the file was removed.
func diffStatPath(s api.DiffStat) string {
	if s.New != nil && s.New.Path != "" {
		return s.New.Path
	}
	if s.Old != nil {
		return s.Old.Path
	}
	return ""
}

// diffStatDisplayPath is diffStatPath, with "old → new" for a file whose
// path changed.
func diffStatDisplayPath(s api.DiffStat) string {
	path := diffStatPath(s)
	if s.Old != nil && s.New != nil && s.Old.Path != "" && s.Old.Path != s.New.Path {
		return s.Old.Path + " → " + path
	}
	return path
}

func printDiffStat(ios *cmdutil.IOStreams, stats []api.DiffStat) error {
	isTTY := ios.IsStdoutTTY()
	cs := ios.ColorScheme()
	table := output.NewTable(ios.Out, isTTY)
	added, removed := 0, 0
	for _, s := range stats {
		added += s.LinesAdded
		removed += s.LinesRemoved
		table.AddRow(
			diffStatDisplayPath(s),
			s.Status,
			cs.Green(fmt.Sprintf("+%d", s.LinesAdded)),
			cs.Red(fmt.Sprintf("-%d", s.LinesRemoved)),
		)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	if isTTY {
		fmt.Fprintf(ios.Out, "%s changed, %s(+), %s(-)\n",
			pluralize(len(stats), "file"), pluralize(added, "insertion"), pluralize(removed, "deletion"))
	}
	return nil
}

// writeColoredDiff writes a unified diff with ANSI colors: file headers
// (from "diff --git" up to the first hunk) in bold, hunk headers in cyan,
// added lines in green and removed lines in red.
func writeColoredDiff(w io.Writer, cs *output.ColorScheme, diff []byte) error {
	bw := bufio.NewWriter(w)
	inHeader := false
	for len(diff) > 0 {
		line, rest, found := bytes.Cut(diff, []byte("\n"))
		text := string(line)
		switch {
		case strings.HasPrefix(text, "diff --git "):
			inHeader = true
			text = cs.Bold(text)
		case strings.HasPrefix(text, "@@"):
			inHeader = false
			text = cs.Cyan(text)
		case inHeader:
			text = cs.Bold(text)
		case strings.HasPrefix(text, "+"):
			text = cs.Green(text)
		case strings.HasPrefix(text, "-"):
			text = cs.Red(text)
		}
		_, _ = bw.WriteString(text)
		if found {
			_ = bw.WriteByte('\n')
		}
		diff = rest
	}
	return bw.Flush()
}
