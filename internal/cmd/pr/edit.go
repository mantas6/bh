package pr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// EditOptions holds the dependencies and flags for `bh pr edit`.
type EditOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg             string
	Title           string
	Body            string
	BodyFile        string
	Base            string
	AddReviewers    []string
	RemoveReviewers []string
	Draft           bool
	Ready           bool

	titleSet bool
	bodySet  bool
}

// NewCmdEdit creates the "pr edit" command.
func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "edit [<number> | <url> | <branch>]",
		Short: "Edit a pull request",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			opts.titleSet = cmd.Flags().Changed("title")
			opts.bodySet = cmd.Flags().Changed("body")
			if opts.Draft && opts.Ready {
				return cmdutil.FlagErrorf("--draft and --ready are mutually exclusive")
			}
			if opts.Body != "" && opts.BodyFile != "" {
				return cmdutil.FlagErrorf("specify only one of --body or --body-file")
			}
			if runF != nil {
				return runF(opts)
			}
			return editRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "New title")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "New body")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" for stdin)")
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "Change the base branch")
	cmd.Flags().StringSliceVar(&opts.AddReviewers, "add-reviewer", nil, "Add reviewers (comma-separated)")
	cmd.Flags().StringSliceVar(&opts.RemoveReviewers, "remove-reviewer", nil, "Remove reviewers (comma-separated)")
	cmd.Flags().BoolVar(&opts.Draft, "draft", false, "Mark the pull request as a draft")
	cmd.Flags().BoolVar(&opts.Ready, "ready", false, "Mark the pull request as ready for review")

	return cmd
}

func editRun(opts *EditOptions) error {
	ctx := context.Background()

	hasEdit := opts.titleSet || opts.bodySet || opts.BodyFile != "" || opts.Base != "" ||
		len(opts.AddReviewers) > 0 || len(opts.RemoveReviewers) > 0 || opts.Draft || opts.Ready
	if !hasEdit {
		return cmdutil.FlagErrorf("specify at least one flag to edit")
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

	body := map[string]any{}
	if opts.titleSet {
		body["title"] = opts.Title
	}
	if opts.bodySet {
		body["description"] = opts.Body
	}
	if opts.BodyFile != "" {
		b, err := readBodyFile(opts.IO, opts.BodyFile)
		if err != nil {
			return err
		}
		body["description"] = b
	}
	if opts.Base != "" {
		body["destination"] = map[string]any{"branch": map[string]any{"name": opts.Base}}
	}
	if opts.Draft {
		body["draft"] = true
	}
	if opts.Ready {
		body["draft"] = false
	}

	if len(opts.AddReviewers) > 0 || len(opts.RemoveReviewers) > 0 {
		uuids, err := mergeReviewers(ctx, client, repo.Workspace, pr, opts.AddReviewers, opts.RemoveReviewers)
		if err != nil {
			return err
		}
		rv := make([]map[string]any, 0, len(uuids))
		for _, uuid := range uuids {
			rv = append(rv, map[string]any{"uuid": uuid})
		}
		body["reviewers"] = rv
	}

	updated, err := client.UpdatePullRequest(ctx, repo.FullName(), pr.ID, body)
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, updated.Links.HTML.Href)
	return nil
}

// mergeReviewers computes the reviewer UUID list to send: it starts from the
// PR's current reviewers, drops those matched by remove, and adds resolved
// members from add.
func mergeReviewers(ctx context.Context, client *api.Client, workspace string, pr *api.PullRequest, add, remove []string) ([]string, error) {
	removeUUIDs := map[string]bool{}
	for _, rm := range remove {
		rm = strings.TrimSpace(rm)
		if rm == "" {
			continue
		}
		for _, u := range pr.Reviewers {
			if matchesUser(u, rm) {
				removeUUIDs[u.UUID] = true
			}
		}
	}

	seen := map[string]bool{}
	var uuids []string
	for _, u := range pr.Reviewers {
		if removeUUIDs[u.UUID] || seen[u.UUID] {
			continue
		}
		seen[u.UUID] = true
		uuids = append(uuids, u.UUID)
	}

	for _, name := range add {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		u, err := client.FindMember(ctx, workspace, name)
		if err != nil {
			return nil, err
		}
		if seen[u.UUID] {
			continue
		}
		seen[u.UUID] = true
		uuids = append(uuids, u.UUID)
	}

	return uuids, nil
}

// matchesUser reports whether q identifies u by nickname, display name,
// account id, or uuid (with or without braces).
func matchesUser(u api.User, q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	if strings.EqualFold(u.Nickname, q) || strings.EqualFold(u.DisplayName, q) {
		return true
	}
	if u.AccountID != "" && u.AccountID == q {
		return true
	}
	nq := strings.ToLower(strings.Trim(q, "{}"))
	nu := strings.ToLower(strings.Trim(u.UUID, "{}"))
	return nu != "" && nu == nq
}
