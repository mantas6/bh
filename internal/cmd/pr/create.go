package pr

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// CreateOptions holds the dependencies and flags for `bh pr create`.
type CreateOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	Browser   browser
	Now       func() time.Time

	Title             string
	Body              string
	BodyFile          string
	Base              string
	Head              string
	Draft             bool
	Reviewers         []string
	CloseSourceBranch bool
	Fill              bool
	Push              bool
	Web               bool

	titleSet bool
}

// NewCmdCreate creates the "pr create" command.
func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}
	if f.Browser != nil {
		opts.Browser = f.Browser
	}

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a pull request",
		Long: cmdutil.Heredoc(`
			Create a pull request on Bitbucket.

			The head branch defaults to the current branch. When it has not been
			pushed yet you are prompted to push it (use --push to skip the prompt,
			required when not running interactively). The base branch defaults to
			the repository's main branch.
		`),
		Example: cmdutil.Heredoc(`
			# Interactively create a PR from the current branch
			$ bh pr create

			# Fill the title and body from the branch's commits
			$ bh pr create --fill

			# Create a PR against a specific base with reviewers
			$ bh pr create --base main --reviewer alice,bob --title "Fix bug"

			# Open the create page in the browser instead
			$ bh pr create --web
		`),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.titleSet = cmd.Flags().Changed("title")
			if opts.Body != "" && opts.BodyFile != "" {
				return cmdutil.FlagErrorf("specify only one of --body or --body-file")
			}
			if runF != nil {
				return runF(opts)
			}
			return createRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "Title for the pull request")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "Body for the pull request")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" for stdin)")
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "The branch to merge into")
	cmd.Flags().StringVarP(&opts.Head, "head", "H", "", "The branch that contains commits (defaults to current branch)")
	cmd.Flags().BoolVar(&opts.Draft, "draft", false, "Mark the pull request as a draft")
	cmd.Flags().StringSliceVarP(&opts.Reviewers, "reviewer", "r", nil, "Request reviews from people (comma-separated)")
	cmd.Flags().BoolVar(&opts.CloseSourceBranch, "close-source-branch", false, "Close the source branch when the PR merges")
	cmd.Flags().BoolVarP(&opts.Fill, "fill", "f", false, "Use commit info for the title and body")
	cmd.Flags().BoolVar(&opts.Push, "push", false, "Push the branch to the remote before creating")
	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the create page in the web browser")

	return cmd
}

func createRun(opts *CreateOptions) error {
	ctx := context.Background()

	repo, resolvedRemote, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	gitRunner, err := opts.Git()
	if err != nil {
		return err
	}

	head := opts.Head
	if head == "" {
		head, err = git.CurrentBranch(ctx, gitRunner)
		if err != nil {
			if errors.Is(err, git.ErrNotOnBranch) {
				return errors.New("could not determine the current branch; specify --head")
			}
			return err
		}
	}

	client, err := opts.ApiClient()
	if err != nil {
		return err
	}

	// The effective base is needed for --fill commit ranges and the --web
	// URL. When the user did not pass --base we look up the repo's default
	// branch, but we still omit destination from the API create body so the
	// server applies its own default.
	effectiveBase := opts.Base
	if opts.Base == "" && (opts.Fill || opts.Web) {
		r, err := client.Repository(ctx, repo.FullName())
		if err != nil {
			return err
		}
		if r.MainBranch != nil {
			effectiveBase = r.MainBranch.Name
		}
	}

	if err := ensurePushed(ctx, opts, gitRunner, resolvedRemote, head); err != nil {
		return err
	}

	if opts.Web {
		u := fmt.Sprintf("https://bitbucket.org/%s/pull-requests/new?source=%s",
			repo.FullName(), url.QueryEscape(head))
		if effectiveBase != "" {
			u += "&dest=" + url.QueryEscape(effectiveBase)
		}
		if opts.IO.IsStdoutTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", u)
		}
		return opts.Browser.Browse(u)
	}

	title := opts.Title
	body := opts.Body
	if opts.BodyFile != "" {
		b, err := readBodyFile(opts.IO, opts.BodyFile)
		if err != nil {
			return err
		}
		body = b
	}

	if opts.Fill {
		t, b, err := fillFromCommits(ctx, gitRunner, effectiveBase, head)
		if err != nil {
			return err
		}
		if title == "" {
			title = t
		}
		if body == "" {
			body = b
		}
	}

	if title == "" {
		if opts.IO.IsStdinTTY() {
			fmt.Fprint(opts.IO.ErrOut, "Title: ")
			line, err := readLine(opts.IO.In)
			if err != nil {
				return err
			}
			title = strings.TrimSpace(line)
		}
		if title == "" {
			return errors.New("--title (or --fill) is required when not running interactively")
		}
	}

	var reviewerUUIDs []string
	for _, name := range opts.Reviewers {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		u, err := client.FindMember(ctx, repo.Workspace, name)
		if err != nil {
			return err
		}
		reviewerUUIDs = append(reviewerUUIDs, u.UUID)
	}

	pr, err := client.CreatePullRequest(ctx, repo.FullName(), api.CreatePRInput{
		Title:             title,
		Description:       body,
		SourceBranch:      head,
		DestinationBranch: opts.Base,
		Reviewers:         reviewerUUIDs,
		Draft:             opts.Draft,
		CloseSourceBranch: opts.CloseSourceBranch,
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, pr.Links.HTML.Href)
	return nil
}

// ensurePushed makes sure head is available on the remote before the PR is
// created, prompting or erroring per the auto-push rules.
func ensurePushed(ctx context.Context, opts *CreateOptions, gitRunner git.Runner, resolvedRemote *git.ResolvedRemote, head string) error {
	remote := "origin"
	if resolvedRemote != nil && resolvedRemote.Remote.Name != "" {
		remote = resolvedRemote.Remote.Name
	}

	upRemote, mergeRef, err := git.BranchUpstream(ctx, gitRunner, head)
	if err != nil {
		return err
	}

	if upRemote == "" {
		if git.RemoteBranchExists(ctx, gitRunner, remote, head) {
			return nil
		}
		if opts.Push {
			return git.Push(ctx, gitRunner, remote, head)
		}
		if opts.IO.IsStdinTTY() {
			fmt.Fprintf(opts.IO.ErrOut, "Push branch %s to %s? [Y/n] ", head, remote)
			ans, err := readLine(opts.IO.In)
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(ans)) {
			case "", "y", "yes":
				return git.Push(ctx, gitRunner, remote, head)
			default:
				return cmdutil.ErrCancel
			}
		}
		return fmt.Errorf("branch %s has not been pushed to %s; run with --push or push it first", head, remote)
	}

	if opts.Push {
		return git.Push(ctx, gitRunner, remote, head)
	}

	upBranch := strings.TrimPrefix(mergeRef, "refs/heads/")
	if upBranch == "" {
		upBranch = head
	}
	upstreamRef := upRemote + "/" + upBranch
	ahead, err := git.IsAhead(ctx, gitRunner, head, upstreamRef)
	if err != nil {
		return err
	}
	if ahead > 0 {
		cs := output.NewColorScheme(opts.IO.ColorEnabled())
		fmt.Fprintf(opts.IO.ErrOut, "%s local branch is %d commits ahead of %s\n", cs.WarningIcon(), ahead, upstreamRef)
	}
	return nil
}

// fillFromCommits derives a title and body from the commits in base..head.
func fillFromCommits(ctx context.Context, gitRunner git.Runner, base, head string) (title, body string, err error) {
	commits, err := git.Commits(ctx, gitRunner, base, head)
	if err != nil {
		return "", "", err
	}
	if len(commits) == 0 {
		return "", "", fmt.Errorf("no commits between %s and %s", base, head)
	}
	if len(commits) == 1 {
		return commits[0].Subject, commits[0].Body, nil
	}

	var b strings.Builder
	// git.Commits returns newest first; list oldest first.
	for i := len(commits) - 1; i >= 0; i-- {
		fmt.Fprintf(&b, "- %s\n", commits[i].Subject)
	}
	return humanizeBranch(head), strings.TrimRight(b.String(), "\n"), nil
}

// humanizeBranch turns a branch name into a rough title by replacing
// separators with spaces.
func humanizeBranch(name string) string {
	return strings.NewReplacer("-", " ", "_", " ").Replace(name)
}
