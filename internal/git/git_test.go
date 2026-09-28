package git_test

import (
	"context"
	"errors"
	"io/fs"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

func TestRemotesParsing(t *testing.T) {
	ctx := context.Background()
	out := "origin\tgit@bitbucket.org:ws/repo.git (fetch)\n" +
		"origin\tgit@bitbucket.org:ws/repo.git (push)\n" +
		"upstream\thttps://bitbucket.org/ups/repo.git (fetch)\n" +
		"upstream\thttps://bitbucket.org/ups/repo.git (push)\n"

	s := gittest.New().Register(out, nil, "remote", "-v")
	remotes, err := git.Remotes(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	want := []git.Remote{
		{Name: "origin", FetchURL: "git@bitbucket.org:ws/repo.git", PushURL: "git@bitbucket.org:ws/repo.git"},
		{Name: "upstream", FetchURL: "https://bitbucket.org/ups/repo.git", PushURL: "https://bitbucket.org/ups/repo.git"},
	}
	if !reflect.DeepEqual(remotes, want) {
		t.Fatalf("Remotes = %+v, want %+v", remotes, want)
	}
}

func TestRemotesParsingDistinctPushURL(t *testing.T) {
	out := "origin\thttps://bitbucket.org/ws/repo.git (fetch)\n" +
		"origin\tgit@bitbucket.org:ws/repo.git (push)\n"
	s := gittest.New().Register(out, nil, "remote", "-v")
	remotes, err := git.Remotes(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 1 {
		t.Fatalf("expected 1 remote, got %d", len(remotes))
	}
	if remotes[0].FetchURL != "https://bitbucket.org/ws/repo.git" {
		t.Fatalf("fetch URL = %q", remotes[0].FetchURL)
	}
	if remotes[0].PushURL != "git@bitbucket.org:ws/repo.git" {
		t.Fatalf("push URL = %q", remotes[0].PushURL)
	}
}

func TestCurrentBranch(t *testing.T) {
	ctx := context.Background()

	s := gittest.New().Register("main\n", nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	branch, err := git.CurrentBranch(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
}

func TestCurrentBranchDetached(t *testing.T) {
	ctx := context.Background()

	s := gittest.New().Register("", gittest.Exit(1), "symbolic-ref", "--quiet", "--short", "HEAD")
	_, err := git.CurrentBranch(ctx, s)
	if !errors.Is(err, git.ErrNotOnBranch) {
		t.Fatalf("err = %v, want ErrNotOnBranch", err)
	}
}

func TestBranchUpstream(t *testing.T) {
	ctx := context.Background()

	s := gittest.New().
		Register("origin\n", nil, "config", "--get", "branch.feature.remote").
		Register("refs/heads/feature\n", nil, "config", "--get", "branch.feature.merge")

	remote, mergeRef, err := git.BranchUpstream(ctx, s, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if remote != "origin" {
		t.Fatalf("remote = %q, want origin", remote)
	}
	if mergeRef != "refs/heads/feature" {
		t.Fatalf("mergeRef = %q, want refs/heads/feature", mergeRef)
	}
}

func TestBranchUpstreamUnset(t *testing.T) {
	ctx := context.Background()

	// git config exits 1 when a key is unset -> empty string, no error.
	s := gittest.New().
		Register("", gittest.Exit(1), "config", "--get", "branch.feature.remote").
		Register("", gittest.Exit(1), "config", "--get", "branch.feature.merge")

	remote, mergeRef, err := git.BranchUpstream(ctx, s, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if remote != "" || mergeRef != "" {
		t.Fatalf("expected empty upstream, got remote=%q mergeRef=%q", remote, mergeRef)
	}
}

func TestGetConfigUnset(t *testing.T) {
	ctx := context.Background()
	s := gittest.New().Register("", gittest.Exit(1), "config", "--get", "some.key")
	v, err := git.GetConfig(ctx, s, "some.key")
	if err != nil {
		t.Fatal(err)
	}
	if v != "" {
		t.Fatalf("value = %q, want empty", v)
	}
}

func TestGetConfigError(t *testing.T) {
	ctx := context.Background()
	s := gittest.New().Register("", &git.Error{ExitCode: 2, Stderr: "boom"}, "config", "--get", "some.key")
	_, err := git.GetConfig(ctx, s, "some.key")
	if err == nil {
		t.Fatal("expected error for non-1 exit code")
	}
}

func TestHasLocalBranch(t *testing.T) {
	ctx := context.Background()
	argv := []string{"rev-parse", "--verify", "--quiet", "refs/heads/feature"}
	notRepo := &git.Error{ExitCode: 128, Stderr: "fatal: not a git repository"}

	tests := []struct {
		name    string
		err     error
		want    bool
		wantErr error
	}{
		{"exists", nil, true, nil},
		{"missing", gittest.Exit(1), false, nil},
		{"git failure", notRepo, false, notRepo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := gittest.New().Register("", tt.err, argv...)
			got, err := git.HasLocalBranch(ctx, s, "feature")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("HasLocalBranch = %v, want %v", got, tt.want)
			}
			if calls := s.CallStrings(); len(calls) != 1 || calls[0] != strings.Join(argv, " ") {
				t.Fatalf("calls = %q, want [%q]", calls, strings.Join(argv, " "))
			}
		})
	}
}

func TestRemoteBranchExists(t *testing.T) {
	ctx := context.Background()
	// The fully qualified ref must be passed so ls-remote does not suffix-match
	// e.g. refs/heads/foo/feature.
	argv := []string{"ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature"}
	authFailed := &git.Error{ExitCode: 128, Stderr: "fatal: Authentication failed"}

	tests := []struct {
		name    string
		err     error
		want    bool
		wantErr error
	}{
		{"exists", nil, true, nil},
		{"missing", gittest.Exit(2), false, nil},
		{"git failure", authFailed, false, authFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := gittest.New()
			s.FailUnstubbed = true
			s.Register("", tt.err, argv...)
			got, err := git.RemoteBranchExists(ctx, s, "origin", "feature")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("RemoteBranchExists = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAheadCount(t *testing.T) {
	ctx := context.Background()
	s := gittest.New().Register("3\n", nil, "rev-list", "--count", "origin/main..feature")
	n, err := git.AheadCount(ctx, s, "feature", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("n = %d, want 3", n)
	}
}

func TestCommits(t *testing.T) {
	out := "sha1\x00subject one\x00body one\x1e\n" + "sha2\x00subject two\x00\x1e\n"
	s := gittest.New()
	s.FailUnstubbed = true
	s.Register(out, nil, "log", "--pretty=format:%H%x00%s%x00%b%x1e", "--end-of-options", "origin/main..feature")

	commits, err := git.Commits(context.Background(), s, "origin/main", "feature")
	if err != nil {
		t.Fatal(err)
	}
	want := []git.Commit{
		{SHA: "sha1", Subject: "subject one", Body: "body one"},
		{SHA: "sha2", Subject: "subject two", Body: ""},
	}
	if !reflect.DeepEqual(commits, want) {
		t.Fatalf("commits = %+v, want %+v", commits, want)
	}
}

func TestCommitsEmptyBase(t *testing.T) {
	s := gittest.New()
	if _, err := git.Commits(context.Background(), s, "", "feature"); err == nil {
		t.Fatal("expected error for empty base")
	}
	if len(s.Calls) != 0 {
		t.Fatalf("git calls = %v, want none", s.CallStrings())
	}
}

func TestErrorMessage(t *testing.T) {
	e := &git.Error{Args: []string{"push", "origin", "main"}, ExitCode: 1, Stderr: "denied\n"}
	got := e.Error()
	const want = "git push origin main: exit status 1: denied"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestInteractiveArgs(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name string
		run  func(git.Runner) error
		want []string
	}{
		{"push", func(r git.Runner) error { return git.Push(ctx, r, "origin", "feature") },
			[]string{"push", "-u", "--end-of-options", "origin", "feature"}},
		{"fetch", func(r git.Runner) error { return git.Fetch(ctx, r, "origin", "") },
			[]string{"fetch", "--end-of-options", "origin"}},
		{"fetch refspec", func(r git.Runner) error { return git.Fetch(ctx, r, "origin", "+refs/heads/a:refs/remotes/origin/a") },
			[]string{"fetch", "--end-of-options", "origin", "+refs/heads/a:refs/remotes/origin/a"}},
		{"checkout", func(r git.Runner) error { return git.Checkout(ctx, r, "main") },
			[]string{"checkout", "--end-of-options", "main"}},
		{"reset hard", func(r git.Runner) error { return git.ResetHard(ctx, r, "origin/main") },
			[]string{"reset", "--hard", "--end-of-options", "origin/main"}},
		{"delete local branch", func(r git.Runner) error { return git.DeleteLocalBranch(ctx, r, "feature") },
			[]string{"branch", "-D", "--end-of-options", "feature"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := gittest.New()
			if err := tt.run(s); err != nil {
				t.Fatal(err)
			}
			if len(s.Interactive) != 1 {
				t.Fatalf("expected 1 interactive call, got %d", len(s.Interactive))
			}
			if !reflect.DeepEqual(s.Interactive[0], tt.want) {
				t.Fatalf("interactive args = %q, want %q", s.Interactive[0], tt.want)
			}
		})
	}
}

// gitClient returns a real git Client isolated from the user's and system
// git configuration, skipping the test when git is not installed.
func gitClient(t *testing.T, dir string) *git.Client {
	t.Helper()
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found in PATH")
	}
	return &git.Client{
		GitPath: gitPath,
		Dir:     dir,
		Env: []string{
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=bh", "GIT_AUTHOR_EMAIL=bh@example.com",
			"GIT_COMMITTER_NAME=bh", "GIT_COMMITTER_EMAIL=bh@example.com",
		},
	}
}

// mustRun runs git through c and fails the test on error.
func mustRun(t *testing.T, c *git.Client, args ...string) string {
	t.Helper()
	out, err := c.Run(context.Background(), args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Integration test: exercises the real git binary in a temp repo.
func TestRemotesIntegration(t *testing.T) {
	c := gitClient(t, t.TempDir())
	ctx := context.Background()

	mustRun(t, c, "init")
	mustRun(t, c, "remote", "add", "origin", "git@bitbucket.org:ows/repo.git")
	mustRun(t, c, "remote", "add", "upstream", "https://bitbucket.org/ups/repo.git")

	remotes, err := git.Remotes(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 2 {
		t.Fatalf("expected 2 remotes, got %d: %+v", len(remotes), remotes)
	}

	repo, rr, err := git.ResolveRepo(ctx, c, "")
	if err != nil {
		t.Fatal(err)
	}
	want := git.Repo{Host: "bitbucket.org", Workspace: "ups", Name: "repo"}
	if repo != want {
		t.Fatalf("ResolveRepo = %+v, want %+v", repo, want)
	}
	if rr == nil || rr.Remote.Name != "upstream" {
		t.Fatalf("resolved remote = %+v, want upstream", rr)
	}
}

// Integration test: branch existence checks against a real repository and a
// local "remote", including the ls-remote suffix-match case.
func TestBranchExistsIntegration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remoteDir := filepath.Join(root, "remote.git")
	c := gitClient(t, filepath.Join(root, "work"))

	mustRun(t, &git.Client{GitPath: c.GitPath, Env: c.Env}, "init", "--bare", remoteDir)
	mustRun(t, &git.Client{GitPath: c.GitPath, Env: c.Env}, "init", "-b", "main", c.Dir)
	mustRun(t, c, "commit", "--allow-empty", "-m", "initial")
	mustRun(t, c, "branch", "foo/feature")
	mustRun(t, c, "remote", "add", "origin", remoteDir)
	mustRun(t, c, "push", "origin", "main", "foo/feature")

	for _, tt := range []struct {
		branch string
		want   bool
	}{
		{"main", true},
		{"foo/feature", true},
		{"feature", false}, // must not suffix-match refs/heads/foo/feature
	} {
		got, err := git.RemoteBranchExists(ctx, c, "origin", tt.branch)
		if err != nil {
			t.Fatalf("RemoteBranchExists(%q): %v", tt.branch, err)
		}
		if got != tt.want {
			t.Errorf("RemoteBranchExists(%q) = %v, want %v", tt.branch, got, tt.want)
		}

		got, err = git.HasLocalBranch(ctx, c, tt.branch)
		if err != nil {
			t.Fatalf("HasLocalBranch(%q): %v", tt.branch, err)
		}
		if got != tt.want {
			t.Errorf("HasLocalBranch(%q) = %v, want %v", tt.branch, got, tt.want)
		}
	}

	if _, err := git.RemoteBranchExists(ctx, c, "nope", "main"); err == nil {
		t.Error("RemoteBranchExists on unknown remote: expected error")
	}
}

func TestClientRunStderrInError(t *testing.T) {
	c := gitClient(t, t.TempDir())
	mustRun(t, c, "init")

	_, err := c.Run(context.Background(), "rev-parse", "--verify", "no-such-ref")
	var ge *git.Error
	if !errors.As(err, &ge) {
		t.Fatalf("err = %v (%T), want *git.Error", err, err)
	}
	if ge.ExitCode == 0 {
		t.Fatalf("ExitCode = 0, want non-zero")
	}
	if !strings.Contains(ge.Stderr, "fatal:") {
		t.Fatalf("Stderr = %q, want git's fatal message", ge.Stderr)
	}
	if !strings.Contains(err.Error(), "git rev-parse --verify no-such-ref") || !strings.Contains(err.Error(), "fatal:") {
		t.Fatalf("Error() = %q, want argv and stderr", err.Error())
	}
}

func TestClientRunTrimsTrailingNewline(t *testing.T) {
	c := gitClient(t, t.TempDir())
	if got := mustRun(t, c, "--version"); strings.HasSuffix(got, "\n") || !strings.HasPrefix(got, "git version") {
		t.Fatalf("Run output = %q", got)
	}
}

func TestClientRunContextCancelled(t *testing.T) {
	c := gitClient(t, t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Run(ctx, "--version")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run err = %v, want context.Canceled", err)
	}
	if err := c.RunInteractive(ctx, "--version"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunInteractive err = %v, want context.Canceled", err)
	}
}

func TestClientRunExecFailureIncludesArgv(t *testing.T) {
	c := &git.Client{GitPath: filepath.Join(t.TempDir(), "no-such-git")}
	_, err := c.Run(context.Background(), "status", "--short")
	if err == nil {
		t.Fatal("expected error")
	}
	var ge *git.Error
	if errors.As(err, &ge) {
		t.Fatalf("err = %v, want a non-*git.Error exec failure", err)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("err = %v, want to wrap fs.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "git status --short") {
		t.Fatalf("Error() = %q, want argv", err.Error())
	}
}
