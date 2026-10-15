package pr

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// EditOptions holds the dependencies and flags for `bh pr edit`.
type EditOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)

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
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "edit [<number> | <url> | <branch>]",
		Short: "Edit a pull request",
		Long: cmdutil.Heredoc(`
			Edit the title, body, base branch, reviewers or draft state of a pull
			request.

			Reviewers are given by nickname, display name, {uuid} or account ID.
			Names passed to --add-reviewer are resolved by listing the workspace
			members, which needs the read:workspace:bitbucket token scope; a
			{uuid} or account ID is used as is, without that lookup.

			The pull request URL is the only line printed on standard output; a
			summary of what changed goes to standard error.
		`),
		Args: cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			opts.titleSet = cmd.Flags().Changed("title")
			opts.bodySet = cmd.Flags().Changed("body")
			if err := cmdutil.MutuallyExclusive("--draft and --ready are mutually exclusive",
				opts.Draft, opts.Ready); err != nil {
				return err
			}
			if err := cmdutil.MutuallyExclusive("specify only one of --body or --body-file",
				opts.bodySet, cmd.Flags().Changed("body-file")); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return editRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "New title")
	cmdutil.AddBodyFlags(cmd, &opts.Body, &opts.BodyFile)
	cmd.Flags().StringVarP(&opts.Base, "base", "B", "", "Change the base branch")
	cmd.Flags().StringSliceVar(&opts.AddReviewers, "add-reviewer", nil, "Add reviewers by name, {uuid} or account ID (comma-separated)")
	cmd.Flags().StringSliceVar(&opts.RemoveReviewers, "remove-reviewer", nil, "Remove reviewers (comma-separated)")
	cmd.Flags().BoolVar(&opts.Draft, "draft", false, "Mark the pull request as a draft")
	cmd.Flags().BoolVar(&opts.Ready, "ready", false, "Mark the pull request as ready for review")

	return cmd
}

func editRun(ctx context.Context, opts *EditOptions) error {
	hasEdit := opts.titleSet || opts.bodySet || opts.BodyFile != "" || opts.Base != "" ||
		len(opts.AddReviewers) > 0 || len(opts.RemoveReviewers) > 0 || opts.Draft || opts.Ready
	if !hasEdit {
		return cmdutil.FlagErrorf("specify at least one flag to edit")
	}

	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

	// changed names the edited fields for the summary line.
	var in api.UpdatePRInput
	var changed []string
	if opts.titleSet {
		in.Title = &opts.Title
		changed = append(changed, "title")
	}
	if opts.bodySet {
		in.Description = &opts.Body
	}
	if opts.BodyFile != "" {
		b, err := shared.ReadBodyFile(opts.IO, opts.BodyFile)
		if err != nil {
			return err
		}
		in.Description = &b
	}
	if in.Description != nil {
		changed = append(changed, "body")
	}
	if opts.Base != "" {
		in.DestinationBranch = &opts.Base
		changed = append(changed, "base")
	}

	if len(opts.AddReviewers) > 0 || len(opts.RemoveReviewers) > 0 {
		uuids, err := mergeReviewers(ctx, client, repo.Workspace, pr, opts.AddReviewers, opts.RemoveReviewers)
		if err != nil {
			return err
		}
		in.Reviewers = uuids
		changed = append(changed, "reviewers")
	}
	if opts.Draft || opts.Ready {
		draft := opts.Draft
		in.Draft = &draft
		if draft {
			changed = append(changed, "draft")
		} else {
			changed = append(changed, "ready")
		}
	}

	updated, err := client.UpdatePullRequest(ctx, repo.FullName(), pr.ID, in)
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, updated.Links.HTML.Href)
	fmt.Fprintf(opts.IO.ErrOut, "%s Updated #%d: %s\n", opts.IO.ErrColorScheme().SuccessIcon(), pr.ID, strings.Join(changed, ", "))
	return nil
}

// reviewerScopeHint explains why reviewer resolution can fail with a 403;
// %s is the missing scope.
const reviewerScopeHint = "Resolving reviewer names lists the workspace members, which needs the %s token scope. " +
	"Add it to the API token and run: bh auth login, or pass reviewers as {uuid} or account ID to skip the lookup."

var (
	bareUUIDRe  = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	accountIDRe = regexp.MustCompile(`^(\d+:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}|[0-9a-f]{24})$`)
)

// directReviewerID returns the reviewer id to send for q when q already
// identifies a user, so no member lookup is needed: a braced "{uuid}" or bare
// UUID (sent as "{uuid}") or an Atlassian account ID ("557058:<uuid>" or 24
// hex digits).
func directReviewerID(q string) (string, bool) {
	switch {
	case len(q) > 2 && strings.HasPrefix(q, "{") && strings.HasSuffix(q, "}"):
		return strings.ToLower(q), true
	case bareUUIDRe.MatchString(q):
		return "{" + strings.ToLower(q) + "}", true
	case accountIDRe.MatchString(q):
		return q, true
	}
	return "", false
}

// resolveReviewers maps reviewers to ids to send (in order, without
// duplicates). UUIDs and account IDs are used as given; names are resolved
// with a single workspace member listing, made only when needed.
func resolveReviewers(ctx context.Context, client *api.Client, workspace string, names []string) ([]string, error) {
	var lookup []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if _, direct := directReviewerID(name); name != "" && !direct && !slices.Contains(lookup, name) {
			lookup = append(lookup, name)
		}
	}

	var found map[string]api.User
	if len(lookup) > 0 {
		members, err := client.WorkspaceMembers(ctx, workspace, 0)
		if err != nil {
			return nil, reviewerLookupError(lookup, err)
		}
		if found, err = api.MatchMembers(members, lookup); err != nil {
			return nil, err
		}
	}

	var ids []string
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id, ok := directReviewerID(name)
		if !ok {
			var u api.User
			if u, ok = found[name]; !ok {
				continue
			}
			id = u.UUID
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

// reviewerLookupError wraps a failed workspace member listing with the
// reviewers being resolved and, for a 403, a hint naming the scope needed.
func reviewerLookupError(names []string, err error) error {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = strconv.Quote(n)
	}
	what := "reviewer "
	if len(names) > 1 {
		what = "reviewers "
	}
	err = fmt.Errorf("resolving %s%s: listing workspace members: %w", what, strings.Join(quoted, ", "), err)
	if !api.IsForbidden(err) {
		return err
	}
	scope := "read:workspace:bitbucket"
	if missing := cmdutil.MissingScopes(err); len(missing) > 0 {
		scope = strings.Join(missing, ", ")
	}
	return cmdutil.WithHint(err, fmt.Sprintf(reviewerScopeHint, scope))
}

// mergeReviewers computes the reviewer id list to send: it starts from the
// PR's current reviewer UUIDs, drops those matched by remove, and adds the
// reviewers resolved from add that are not already present. The result is never nil, so an emptied list is sent as
// such rather than leaving reviewers unchanged.
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

	added, err := resolveReviewers(ctx, client, workspace, add)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	ids := []string{}
	var kept []api.User
	for _, u := range pr.Reviewers {
		if removeUUIDs[u.UUID] || seen[u.UUID] {
			continue
		}
		seen[u.UUID] = true
		ids = append(ids, u.UUID)
		kept = append(kept, u)
	}
	for _, id := range added {
		// An added account ID may name a reviewer already listed by UUID.
		if seen[id] || slices.ContainsFunc(kept, func(u api.User) bool { return matchesUser(u, id) }) {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids, nil
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
