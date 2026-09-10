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
	// Run executes git with args, returning captured stdout with the trailing
	// newline trimmed. A non-zero exit returns a *GitError.
	Run(ctx context.Context, args ...string) (stdout string, err error)
	// RunInteractive executes git with args wired to the caller's stdio.
	RunInteractive(ctx context.Context, args ...string) error
}

// GitError describes a git invocation that exited non-zero.
type GitError struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *GitError) Error() string {
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
	// Stdin, Stdout, Stderr are used for interactive commands.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}

func (c *Client) gitPath() string {
	if c.GitPath == "" {
		return "git"
	}
	return c.GitPath
}

// Run implements Runner.
func (c *Client) Run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.gitPath(), args...)
	cmd.Dir = c.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", &GitError{
				Args:     args,
				ExitCode: exitErr.ExitCode(),
				Stderr:   stderr.String(),
			}
		}
		return "", err
	}
	return strings.TrimRight(stdout.String(), "\n"), nil
}

// RunInteractive implements Runner.
func (c *Client) RunInteractive(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, c.gitPath(), args...)
	cmd.Dir = c.Dir
	cmd.Stdin = c.Stdin
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return &GitError{Args: args, ExitCode: exitErr.ExitCode()}
		}
		return err
	}
	return nil
}

// exitCode returns the exit code of a *GitError, or -1 when err is not one.
func exitCode(err error) int {
	var ge *GitError
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
	branch := strings.TrimSpace(out)
	if branch == "" {
		return "", ErrNotOnBranch
	}
	return branch, nil
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

// HasLocalBranch reports whether a local branch with the given name exists.
func HasLocalBranch(ctx context.Context, r Runner, name string) bool {
	_, err := r.Run(ctx, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// RemoteBranchExists reports whether the branch exists on the remote.
func RemoteBranchExists(ctx context.Context, r Runner, remote, branch string) bool {
	_, err := r.Run(ctx, "ls-remote", "--exit-code", "--heads", remote, branch)
	return err == nil
}

// IsAhead returns the number of commits branch is ahead of upstreamRef.
func IsAhead(ctx context.Context, r Runner, branch, upstreamRef string) (int, error) {
	out, err := r.Run(ctx, "rev-list", "--count", upstreamRef+".."+branch)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(out))
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

// Commits returns the commits in base..head, newest first.
func Commits(ctx context.Context, r Runner, base, head string) ([]Commit, error) {
	out, err := r.Run(ctx, "log", "--pretty=format:%H%x00%s%x00%b%x1e", base+".."+head)
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
	return r.RunInteractive(ctx, "push", "-u", remote, branch)
}

// Fetch runs an interactive `git fetch <remote> [refspec]`.
func Fetch(ctx context.Context, r Runner, remote, refspec string) error {
	args := []string{"fetch", remote}
	if refspec != "" {
		args = append(args, refspec)
	}
	return r.RunInteractive(ctx, args...)
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
	return strings.TrimSpace(out), nil
}
