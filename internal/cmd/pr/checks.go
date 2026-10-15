package pr

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// checksWatchInterval is how often `bh pr checks --watch` polls.
const checksWatchInterval = 10 * time.Second

// clearScreen moves the cursor home and clears the terminal.
const clearScreen = "\x1b[H\x1b[2J"

// ChecksOptions holds the dependencies and flags for `bh pr checks`.
type ChecksOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	Now       func() time.Time
	// Sleep waits for d or until ctx is done; nil uses a timer.
	Sleep func(ctx context.Context, d time.Duration) error

	Arg        string
	JSON       bool
	JSONOutput cmdutil.JSONOutput
	Watch      bool
}

// NewCmdChecks creates the "pr checks" command.
func NewCmdChecks(f *cmdutil.Factory, runF func(*ChecksOptions) error) *cobra.Command {
	opts := &ChecksOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
		Now:       time.Now,
	}

	cmd := &cobra.Command{
		Use:   "checks [<number> | <url> | <branch>]",
		Short: "Show build status checks of a pull request",
		Long: `Show the build statuses (CI checks) reported on a pull request: one line per
check with its name, state (SUCCESSFUL, FAILED, INPROGRESS or STOPPED), when
it was last updated and its URL. Failed checks are listed first. When a build
reported several times, only its latest status is shown.

On a terminal the columns are aligned, a summary line comes first and times
are relative; otherwise the fields are tab-separated and times are RFC 3339
timestamps.

With --watch, poll every 10 seconds until no check is in progress, then
print the final result (on a terminal the screen is redrawn on every poll).

Exits with status 1 when any check failed, also with --json.`,
		Example: cmdutil.Examples(`
			# Checks of the pull request for the current branch
			$ bh pr checks

			# Wait for the builds of pull request 123 to finish (exit 1 if any failed)
			$ bh pr checks 123 --watch

			# URLs of the failed checks
			$ bh pr checks 123 --json -q '.[] | select(.state == "FAILED") | .url'
		`),
		Args: cmdutil.MaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Arg = args[0]
			}
			if runF != nil {
				return runF(opts)
			}
			return checksRun(cmd.Context(), opts)
		},
	}

	cmdutil.AddJSONFlags(cmd, &opts.JSON, &opts.JSONOutput, reflect.TypeFor[api.CommitStatus](), "the checks")
	cmd.Flags().BoolVar(&opts.Watch, "watch", false, "Poll until no check is in progress")

	return cmd
}

func checksRun(ctx context.Context, opts *ChecksOptions) error {
	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).FindID(ctx, opts.Arg)
	if err != nil {
		return err
	}
	repo := found.Repo.FullName()

	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	sleep := sleepContext
	if opts.Sleep != nil {
		sleep = opts.Sleep
	}
	redraw := opts.Watch && opts.IO.IsStdoutTTY() && !opts.JSON

	var statuses []api.CommitStatus
	for polls := 0; ; polls++ {
		all, err := found.Client.PullRequestStatuses(ctx, repo, found.Number)
		if err != nil {
			return err
		}
		statuses = latestStatuses(all)
		if !opts.Watch || countChecks(statuses)[api.StatusInProgress] == 0 {
			if redraw && polls > 0 {
				fmt.Fprint(opts.IO.Out, clearScreen)
			}
			break
		}
		if redraw {
			if polls > 0 {
				fmt.Fprint(opts.IO.Out, clearScreen)
			}
			fmt.Fprintf(opts.IO.Out, "Refreshing checks every %d seconds. Press Ctrl+C to quit.\n\n", int(checksWatchInterval.Seconds()))
			if err := printChecks(opts.IO, statuses, found.Number, now()); err != nil {
				return err
			}
		}
		if err := sleep(ctx, checksWatchInterval); err != nil {
			return err
		}
	}

	if opts.JSON {
		err = opts.JSONOutput.Print(opts.IO, statuses)
	} else {
		err = printChecks(opts.IO, statuses, found.Number, now())
	}
	if err != nil {
		return err
	}
	if countChecks(statuses)[api.StatusFailed] > 0 {
		// The checks are already printed; just exit non-zero like gh.
		return cmdutil.ErrSilent
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// checkStateOrder ranks states for display: failures first, successes last.
var checkStateOrder = map[string]int{
	api.StatusFailed:     0,
	api.StatusInProgress: 1,
	api.StatusStopped:    2,
	api.StatusSuccessful: 4,
}

func checkStateRank(state string) int {
	if r, ok := checkStateOrder[strings.ToUpper(state)]; ok {
		return r
	}
	return 3
}

// latestStatuses keeps the most recently updated status per build key (a
// rerun or a status on an earlier commit of the source branch reports the
// same key again) and sorts the result by state, then name.
func latestStatuses(statuses []api.CommitStatus) []api.CommitStatus {
	out := []api.CommitStatus{}
	index := map[string]int{}
	for _, s := range statuses {
		if s.Key != "" {
			if i, ok := index[s.Key]; ok {
				if s.UpdatedOn.After(out[i].UpdatedOn) {
					out[i] = s
				}
				continue
			}
			index[s.Key] = len(out)
		}
		out = append(out, s)
	}
	slices.SortStableFunc(out, func(a, b api.CommitStatus) int {
		return cmp.Or(
			cmp.Compare(checkStateRank(a.State), checkStateRank(b.State)),
			strings.Compare(strings.ToLower(checkName(a)), strings.ToLower(checkName(b))),
		)
	})
	return out
}

// checkName is the display name of a status, falling back to its key.
func checkName(s api.CommitStatus) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Key
}

// countChecks counts statuses by upper-cased state.
func countChecks(statuses []api.CommitStatus) map[string]int {
	counts := map[string]int{}
	for _, s := range statuses {
		counts[strings.ToUpper(s.State)]++
	}
	return counts
}

// checksSummary renders e.g. "2 passed, 1 failed, 1 in progress", or ""
// when there are no statuses. Colors apply only when cs is enabled.
func checksSummary(statuses []api.CommitStatus, cs *output.ColorScheme) string {
	counts := countChecks(statuses)
	var parts []string
	add := func(state, label string, color func(string) string) {
		if n := counts[state]; n > 0 {
			parts = append(parts, color(fmt.Sprintf("%d %s", n, label)))
		}
		delete(counts, state)
	}
	add(api.StatusSuccessful, "passed", cs.Green)
	add(api.StatusFailed, "failed", cs.Red)
	add(api.StatusInProgress, "in progress", cs.Yellow)
	add(api.StatusStopped, "stopped", cs.Gray)
	// States Bitbucket may add later.
	others := make([]string, 0, len(counts))
	for state := range counts {
		others = append(others, state)
	}
	slices.Sort(others)
	for _, state := range others {
		parts = append(parts, fmt.Sprintf("%d %s", counts[state], strings.ToLower(state)))
	}
	return strings.Join(parts, ", ")
}

func checkStateColor(cs *output.ColorScheme, state string) string {
	switch strings.ToUpper(state) {
	case api.StatusSuccessful:
		return cs.Green(state)
	case api.StatusFailed:
		return cs.Red(state)
	case api.StatusInProgress:
		return cs.Yellow(state)
	default:
		return cs.Gray(state)
	}
}

func printChecks(ios *cmdutil.IOStreams, statuses []api.CommitStatus, number int, now time.Time) error {
	isTTY := ios.IsStdoutTTY()
	if len(statuses) == 0 {
		// Keep piped stdout empty, but tell the reader why.
		fmt.Fprintf(ttyOr(ios, isTTY), "No checks reported on pull request #%d\n", number)
		return nil
	}

	cs := ios.ColorScheme()
	if isTTY {
		fmt.Fprintf(ios.Out, "Checks: %s\n\n", checksSummary(statuses, cs))
	}
	table := output.NewTable(ios.Out, isTTY)
	for _, s := range statuses {
		updated := s.UpdatedOn.Format(time.RFC3339)
		if isTTY {
			updated = output.RelativeTime(s.UpdatedOn, now)
		}
		table.AddRow(checkName(s), checkStateColor(cs, s.State), updated, s.URL)
	}
	return table.Flush()
}

// ttyOr returns stdout when it is a terminal and stderr otherwise, for notes
// such as "no results" that must not end up in piped output.
func ttyOr(ios *cmdutil.IOStreams, isTTY bool) io.Writer {
	if isTTY {
		return ios.Out
	}
	return ios.ErrOut
}
