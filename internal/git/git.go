// Package git provides a thin wrapper around the git command line together
// with high-level helpers used by bh: remote listing, branch/upstream
// inspection, and repository resolution. All helpers take a Runner so they can
// be exercised with a fake in tests (see internal/git/gittest).
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// ErrNotOnBranch is returned by CurrentBranch when HEAD is detached.
var ErrNotOnBranch = errors.New("not currently on any branch")

// Runner executes git commands. Run captures stdout for parsing;
// RunInteractive passes through the process stdio so progress and prompts are
// visible (used for push/fetch/checkout).
type Runner interface {
	// Run executes git with args, returning captured stdout with trailing
	// newlines trimmed (see TrimOutput). A non-zero exit returns an *Error.
	Run(ctx context.Context, args ...string) (stdout string, err error)
	// RunInteractive executes git with args wired to the caller's stdio. A
	// non-zero exit returns an *Error without Stderr.
	RunInteractive(ctx context.Context, args ...string) error
}

// TrimOutput applies the Runner.Run output contract: trailing newlines (and
// carriage returns) are removed, everything else is preserved. Fake runners
// should use it so they behave like Client.
func TrimOutput(s string) string {
	return strings.TrimRight(s, "\r\n")
}

// Error describes a git invocation that exited non-zero.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	cmd := "git " + strings.Join(e.Args, " ")
	stderr := strings.TrimSpace(e.Stderr)
	if stderr != "" {
		return fmt.Sprintf("%s: exit status %d: %s", cmd, e.ExitCode, stderr)
	}
	return fmt.Sprintf("%s: exit status %d", cmd, e.ExitCode)
}

// Client is the real Runner backed by os/exec.
type Client struct {
	// GitPath is the path (or name) of the git executable.
	GitPath string
	// Dir, when set, is used as the working directory.
	Dir string
	// Env, when non-empty, is appended to the current process environment
	// for every git invocation (e.g. "GIT_CONFIG_NOSYSTEM=1").
	Env []string
	// Stdin, Stdout, Stderr are used for interactive commands.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

func (c *Client) command(ctx context.Context, args []string) *exec.Cmd {
	path := c.GitPath
	if path == "" {
		path = "git"
	}
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = c.Dir
	if len(c.Env) > 0 {
		cmd.Env = append(cmd.Environ(), c.Env...)
	}
	return cmd
}

// Run implements Runner.
func (c *Client) Run(ctx context.Context, args ...string) (string, error) {
	cmd := c.command(ctx, args)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", runError(ctx, args, stderr.String(), err)
	}
	return TrimOutput(stdout.String()), nil
}

// RunInteractive implements Runner.
func (c *Client) RunInteractive(ctx context.Context, args ...string) error {
	cmd := c.command(ctx, args)
	cmd.Stdin = c.Stdin
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr

	if err := cmd.Run(); err != nil {
		return runError(ctx, args, "", err)
	}
	return nil
}

// runError converts an exec error into the error returned by Client. A
// cancelled or expired context takes precedence (the process was killed, so
// its exit status is meaningless); a non-zero exit becomes an *Error; any
// other failure (e.g. git not found) is wrapped with the argv for context.
func runError(ctx context.Context, args []string, stderr string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), ctxErr)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return &Error{Args: args, ExitCode: exitErr.ExitCode(), Stderr: stderr}
	}
	return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
}

// exitCode returns the exit code of an *Error, or -1 when err is not one.
func exitCode(err error) int {
	var ge *Error
	if errors.As(err, &ge) {
		return ge.ExitCode
	}
	return -1
}

// Remote is a git remote with its fetch and push URLs merged.
type Remote struct {
	Name     string
	FetchURL string
	PushURL  string
}

// Remotes parses `git remote -v` into a deduplicated slice, preserving the
// order remotes first appear.
func Remotes(ctx context.Context, r Runner) ([]Remote, error) {
	out, err := r.Run(ctx, "remote", "-v")
	if err != nil {
		return nil, err
	}
	return parseRemotes(out), nil
}

func parseRemotes(out string) []Remote {
	var order []string
	byName := map[string]*Remote{}

	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "<name>\t<url> (fetch|push)"
		tab := strings.IndexAny(line, " \t")
		if tab < 0 {
			continue
		}
		name := strings.TrimSpace(line[:tab])
		rest := strings.TrimSpace(line[tab:])
		sp := strings.LastIndexAny(rest, " \t")
		if sp < 0 {
			continue
		}
		url := strings.TrimSpace(rest[:sp])
		kind := strings.Trim(strings.TrimSpace(rest[sp:]), "()")

		rem, ok := byName[name]
		if !ok {
			rem = &Remote{Name: name}
			byName[name] = rem
			order = append(order, name)
		}
		switch kind {
		case "fetch":
			rem.FetchURL = url
		case "push":
			rem.PushURL = url
		default:
			if rem.FetchURL == "" {
				rem.FetchURL = url
			}
		}
	}

	remotes := make([]Remote, 0, len(order))
	for _, name := range order {
		rem := byName[name]
		if rem.FetchURL == "" {
			rem.FetchURL = rem.PushURL
		}
		if rem.PushURL == "" {
			rem.PushURL = rem.FetchURL
		}
		remotes = append(remotes, *rem)
	}
	return remotes
}

// CurrentBranch returns the checked-out branch name. A detached HEAD returns
// ErrNotOnBranch.
func CurrentBranch(ctx context.Context, r Runner) (string, error) {
	out, err := r.Run(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if exitCode(err) == 1 {
			return "", ErrNotOnBranch
		}
		return "", err
	}
	if out == "" {
		return "", ErrNotOnBranch
	}
	return out, nil
}

// BranchUpstream returns the configured remote and merge ref for a branch.
// Unconfigured values are returned as empty strings without an error.
func BranchUpstream(ctx context.Context, r Runner, branch string) (remote, mergeRef string, err error) {
	remote, err = GetConfig(ctx, r, "branch."+branch+".remote")
	if err != nil {
		return "", "", err
	}
	mergeRef, err = GetConfig(ctx, r, "branch."+branch+".merge")
	if err != nil {
		return "", "", err
	}
	return remote, mergeRef, nil
}

// notFound maps the exit codes git uses for "no such ref" (1 from
// `rev-parse --verify --quiet`, 2 from `ls-remote --exit-code`) to false with
// a nil error; any other failure is returned as-is.
func notFound(err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	switch exitCode(err) {
	case 1, 2:
		return false, nil
	}
	return false, err
}

// HasLocalBranch reports whether a local branch with the given name exists.
func HasLocalBranch(ctx context.Context, r Runner, name string) (bool, error) {
	_, err := r.Run(ctx, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return notFound(err)
}

// RemoteBranchExists reports whether the branch exists on the remote. The
// fully qualified ref is queried so that "feature" does not match
// "refs/heads/foo/feature".
func RemoteBranchExists(ctx context.Context, r Runner, remote, branch string) (bool, error) {
	_, err := r.Run(ctx, "ls-remote", "--exit-code", "--heads", remote, "refs/heads/"+branch)
	return notFound(err)
}

// AheadCount returns the number of commits branch is ahead of upstreamRef.
func AheadCount(ctx context.Context, r Runner, branch, upstreamRef string) (int, error) {
	out, err := r.Run(ctx, "rev-list", "--count", upstreamRef+".."+branch)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("parsing rev-list count %q: %w", out, err)
	}
	return n, nil
}

// Commit is a single commit from a git log range.
type Commit struct {
	SHA     string
	Subject string
	Body    string
}

// Commits returns the commits in base..head, newest first. An empty base is
// an error: "..head" would silently mean "HEAD..head".
func Commits(ctx context.Context, r Runner, base, head string) ([]Commit, error) {
	if base == "" {
		return nil, errors.New("listing commits: base ref is empty")
	}
	out, err := r.Run(ctx, "log", "--pretty=format:%H%x00%s%x00%b%x1e", "--end-of-options", base+".."+head)
	if err != nil {
		return nil, err
	}
	return parseCommits(out), nil
}

func parseCommits(out string) []Commit {
	out = strings.TrimSpace(out)
	if out == "" {
		return nil
	}
	var commits []Commit
	for _, record := range strings.Split(out, "\x1e") {
		record = strings.Trim(record, "\n")
		if record == "" {
			continue
		}
		fields := strings.SplitN(record, "\x00", 3)
		c := Commit{SHA: fields[0]}
		if len(fields) > 1 {
			c.Subject = fields[1]
		}
		if len(fields) > 2 {
			c.Body = strings.TrimSpace(fields[2])
		}
		commits = append(commits, c)
	}
	return commits
}

// Push runs an interactive `git push -u <remote> <branch>`.
func Push(ctx context.Context, r Runner, remote, branch string) error {
	return r.RunInteractive(ctx, "push", "-u", "--end-of-options", remote, branch)
}

// Fetch runs an interactive `git fetch <remote> [refspec]`.
func Fetch(ctx context.Context, r Runner, remote, refspec string) error {
	args := []string{"fetch", "--end-of-options", remote}
	if refspec != "" {
		args = append(args, refspec)
	}
	return r.RunInteractive(ctx, args...)
}

// Checkout runs an interactive `git checkout <ref>`.
func Checkout(ctx context.Context, r Runner, ref string) error {
	return r.RunInteractive(ctx, "checkout", "--end-of-options", ref)
}

// CheckoutNewBranch runs an interactive `git checkout -b <name> [extra...]`,
// where extra typically holds a `--track`/`--no-track <start-point>` pair.
func CheckoutNewBranch(ctx context.Context, r Runner, name string, extra ...string) error {
	args := append([]string{"checkout", "-b", name}, extra...)
	return r.RunInteractive(ctx, args...)
}

// CheckoutDetach runs an interactive `git checkout --detach <ref>`.
func CheckoutDetach(ctx context.Context, r Runner, ref string) error {
	return r.RunInteractive(ctx, "checkout", "--detach", ref)
}

// ResetHard runs an interactive `git reset --hard <ref>`.
func ResetHard(ctx context.Context, r Runner, ref string) error {
	return r.RunInteractive(ctx, "reset", "--hard", "--end-of-options", ref)
}

// MergeFFOnly runs an interactive `git merge --ff-only <ref>`.
func MergeFFOnly(ctx context.Context, r Runner, ref string) error {
	return r.RunInteractive(ctx, "merge", "--ff-only", ref)
}

// DeleteLocalBranch runs an interactive `git branch -d <name>`, which refuses
// to delete a branch that is not fully merged. With force it runs
// `git branch -D <name>` instead, deleting the branch regardless.
func DeleteLocalBranch(ctx context.Context, r Runner, name string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	return r.RunInteractive(ctx, "branch", flag, "--end-of-options", name)
}

// PushDelete runs an interactive `git push <remote> --delete <branch>`.
func PushDelete(ctx context.Context, r Runner, remote, branch string) error {
	return r.RunInteractive(ctx, "push", remote, "--delete", branch)
}

// SetConfig sets a git config value in the current repository.
func SetConfig(ctx context.Context, r Runner, key, value string) error {
	_, err := r.Run(ctx, "config", key, value)
	return err
}

// GetConfig reads a git config value. An unset key (git exit code 1) returns
// "" with a nil error.
func GetConfig(ctx context.Context, r Runner, key string) (string, error) {
	out, err := r.Run(ctx, "config", "--get", key)
	if err != nil {
		if exitCode(err) == 1 {
			return "", nil
		}
		return "", err
	}
	return out, nil
}
