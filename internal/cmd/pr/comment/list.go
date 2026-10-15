package comment

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// ListOptions holds the dependencies and flags for `bh pr comment list`.
type ListOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time

	Arg        string
	JSON       bool
	JSONOutput cmdutil.JSONOutput
	Unresolved bool
	Limit      int
	// Since keeps threads with a comment newer than an RFC 3339 time, a
	// duration ago (2h, 3d) or a comment id.
	Since string
	// Author keeps threads started by a user (nickname, display name, UUID,
	// account id, or @me).
	Author string
	// NeedsReply keeps unresolved threads whose last comment is not by the
	// current user.
	NeedsReply bool
	// TopLevel drops replies.
	TopLevel bool
}

// NewCmdList creates the "pr comment list" command.
func NewCmdList(f *cmdutil.Factory, runF func(*ListOptions) error) *cobra.Command {
	opts := &ListOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "list [<number> | <url> | <branch>]",
		Short: "List comments on a pull request",
		Long: `List the comment threads of a pull request, oldest first.

Each comment is a header line ` + "`#id author · time[ · path:line][ [resolved]]`" + `
followed by its indented body; replies are nested under their parent. Times
are relative on a terminal and RFC 3339 timestamps otherwise.

The filters select whole threads, which are always shown complete:
  --unresolved   threads that are not resolved
  --needs-reply  unresolved threads whose last comment is not yours
  --since        threads with a comment newer than an RFC 3339 time
                 (2026-09-10T12:00:00Z, 2026-09-10), a duration ago
                 (90m, 2h, 3d, 1w) or a comment id (newer than #id)
  --author       threads started by a user (nickname, display name,
                 {uuid}, account id, or @me)
--top-level then leaves out the replies, and --limit keeps the newest threads.`,
		Args: cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if opts.Limit < 0 {
				return cmdutil.FlagErrorf("invalid value for --limit: %d", opts.Limit)
			}
			if opts.Since != "" {
				if _, err := parseSince(opts.Since, time.Now()); err != nil {
					return err
				}
			}
			if runF != nil {
				return runF(opts)
			}
			return listRun(cmd.Context(), opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.JSON, &opts.JSONOutput, reflect.TypeFor[shared.CommentJSON](), "comments")
	cmd.Flags().BoolVar(&opts.Unresolved, "unresolved", false, "Show only unresolved comment threads")
	cmd.Flags().BoolVar(&opts.NeedsReply, "needs-reply", false, "Show only unresolved threads whose last comment is not yours")
	cmd.Flags().StringVar(&opts.Since, "since", "", "Show only threads with comments newer than a `time`, duration (2h, 3d) or comment id")
	cmd.Flags().StringVar(&opts.Author, "author", "", "Show only threads started by a `user` (nickname or @me)")
	cmd.Flags().BoolVar(&opts.TopLevel, "top-level", false, "Show only top-level comments, without replies")
	cmd.Flags().IntVarP(&opts.Limit, "limit", "L", 0, "Maximum number of comment threads to show, newest kept (0 = all)")

	return cmd
}

func listRun(ctx context.Context, opts *ListOptions) error {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}

	var since *sinceFilter
	if opts.Since != "" {
		s, err := parseSince(opts.Since, now())
		if err != nil {
			return err
		}
		since = &s
	}

	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr, repo, client := found.PR, found.Repo, found.Client

	// The current user, needed by --needs-reply and --author @me.
	var me *api.User
	if opts.NeedsReply || opts.Author == "@me" {
		if me, err = client.CurrentUser(ctx); err != nil {
			return fmt.Errorf("looking up the current user: %w", err)
		}
	}

	// Fetch every comment: the limit counts threads, which can only be
	// assembled once replies are known.
	comments, err := client.ListComments(ctx, repo.FullName(), pr.ID, 0)
	if err != nil {
		return err
	}

	threads := shared.BuildThreads(comments, opts.Unresolved)
	if opts.NeedsReply {
		threads.Filter(func(thread []*api.Comment) bool {
			return thread[0].Resolution == nil && !shared.SameUser(&shared.LastComment(thread).User, me)
		})
	}
	if since != nil {
		threads.Filter(func(thread []*api.Comment) bool {
			for _, c := range thread {
				if since.match(c) {
					return true
				}
			}
			return false
		})
	}
	if opts.Author != "" {
		threads.Filter(func(thread []*api.Comment) bool {
			if opts.Author == "@me" {
				return shared.SameUser(&thread[0].User, me)
			}
			return matchesUser(&thread[0].User, opts.Author)
		})
	}
	if opts.TopLevel {
		threads.TopLevel()
	}
	threads.KeepNewest(opts.Limit)

	if opts.JSON {
		return opts.JSONOutput.Print(opts.IO, threads.JSON(comments))
	}

	if len(threads.Roots) == 0 {
		if opts.NeedsReply || opts.Since != "" || opts.Author != "" {
			fmt.Fprintf(opts.IO.Out, "No matching comments on pull request #%d\n", pr.ID)
		} else {
			fmt.Fprintf(opts.IO.Out, "No comments on pull request #%d\n", pr.ID)
		}
		return nil
	}

	return shared.WriteThreads(opts.IO.Out, opts.IO.ColorScheme(), shared.CommentTime(opts.IO.IsStdoutTTY(), now()), threads)
}

// sinceFilter matches comments newer than a time or a comment id.
type sinceFilter struct {
	after   time.Time
	afterID int
}

func (f sinceFilter) match(c *api.Comment) bool {
	if f.afterID > 0 {
		// Bitbucket comment ids increase over time.
		return c.ID > f.afterID
	}
	return c.CreatedOn.After(f.after)
}

var daysOrWeeks = regexp.MustCompile(`^(\d+)([dw])$`)

// parseSince parses a --since value: a comment id (123 or #123), a duration
// before now (Go syntax such as 90m or 2h30m, or whole days/weeks: 3d, 1w),
// an RFC 3339 time, or a local date (2006-01-02).
func parseSince(s string, now time.Time) (sinceFilter, error) {
	v := strings.TrimSpace(s)
	if id, err := strconv.Atoi(strings.TrimPrefix(v, "#")); err == nil && id > 0 {
		return sinceFilter{afterID: id}, nil
	}
	if m := daysOrWeeks.FindStringSubmatch(v); m != nil {
		n, _ := strconv.Atoi(m[1])
		days := n
		if m[2] == "w" {
			days *= 7
		}
		return sinceFilter{after: now.AddDate(0, 0, -days)}, nil
	}
	if d, err := time.ParseDuration(v); err == nil && d >= 0 {
		return sinceFilter{after: now.Add(-d)}, nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return sinceFilter{after: t}, nil
	}
	if t, err := time.ParseInLocation(time.DateOnly, v, time.Local); err == nil {
		return sinceFilter{after: t}, nil
	}
	return sinceFilter{}, cmdutil.FlagErrorf(
		"invalid value for --since: %q; use an RFC 3339 time (2026-09-10T12:00:00Z), a date (2026-09-10), a duration (2h, 3d) or a comment id", s)
}

// matchesUser reports whether u is the user named by an --author value: a
// nickname or display name (case-insensitive), a {uuid} or an account id.
func matchesUser(u *api.User, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	return strings.EqualFold(u.Nickname, name) || strings.EqualFold(u.DisplayName, name) ||
		(u.UUID != "" && u.UUID == name) || (u.AccountID != "" && u.AccountID == name)
}
