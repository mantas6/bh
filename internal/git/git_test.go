package git

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"testing"

	"github.com/mantas6/bh/internal/git/gittest"
)

func TestRemotesParsing(t *testing.T) {
	ctx := context.Background()
	out := "origin\tgit@bitbucket.org:ws/repo.git (fetch)\n" +
		"origin\tgit@bitbucket.org:ws/repo.git (push)\n" +
		"upstream\thttps://bitbucket.org/ups/repo.git (fetch)\n" +
		"upstream\thttps://bitbucket.org/ups/repo.git (push)\n"

	s := gittest.New().Register(out, nil, "remote", "-v")
	remotes, err := Remotes(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	want := []Remote{
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
	remotes := parseRemotes(out)
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
	branch, err := CurrentBranch(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
}

func TestCurrentBranchDetached(t *testing.T) {
	ctx := context.Background()

	s := gittest.New().Register("", &GitError{ExitCode: 1}, "symbolic-ref", "--quiet", "--short", "HEAD")
	_, err := CurrentBranch(ctx, s)
	if !errors.Is(err, ErrNotOnBranch) {
		t.Fatalf("err = %v, want ErrNotOnBranch", err)
	}
}

func TestBranchUpstream(t *testing.T) {
	ctx := context.Background()

	s := gittest.New().
		Register("origin\n", nil, "config", "--get", "branch.feature.remote").
		Register("refs/heads/feature\n", nil, "config", "--get", "branch.feature.merge")

	remote, mergeRef, err := BranchUpstream(ctx, s, "feature")
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
		Register("", &GitError{ExitCode: 1}, "config", "--get", "branch.feature.remote").
		Register("", &GitError{ExitCode: 1}, "config", "--get", "branch.feature.merge")

	remote, mergeRef, err := BranchUpstream(ctx, s, "feature")
	if err != nil {
		t.Fatal(err)
	}
	if remote != "" || mergeRef != "" {
		t.Fatalf("expected empty upstream, got remote=%q mergeRef=%q", remote, mergeRef)
	}
}

func TestGetConfigUnset(t *testing.T) {
	ctx := context.Background()
	s := gittest.New().Register("", &GitError{ExitCode: 1}, "config", "--get", "some.key")
	v, err := GetConfig(ctx, s, "some.key")
	if err != nil {
		t.Fatal(err)
	}
	if v != "" {
		t.Fatalf("value = %q, want empty", v)
	}
}

func TestGetConfigError(t *testing.T) {
	ctx := context.Background()
	s := gittest.New().Register("", &GitError{ExitCode: 2, Stderr: "boom"}, "config", "--get", "some.key")
	_, err := GetConfig(ctx, s, "some.key")
	if err == nil {
		t.Fatal("expected error for non-1 exit code")
	}
}

func TestIsAhead(t *testing.T) {
	ctx := context.Background()
	s := gittest.New().Register("3\n", nil, "rev-list", "--count", "origin/main..feature")
	n, err := IsAhead(ctx, s, "feature", "origin/main")
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("n = %d, want 3", n)
	}
}

func TestCommits(t *testing.T) {
	out := "sha1\x00subject one\x00body one\x1e" + "sha2\x00subject two\x00\x1e"
	commits := parseCommits(out)
	want := []Commit{
		{SHA: "sha1", Subject: "subject one", Body: "body one"},
		{SHA: "sha2", Subject: "subject two", Body: ""},
	}
	if !reflect.DeepEqual(commits, want) {
		t.Fatalf("commits = %+v, want %+v", commits, want)
	}
}

func TestGitErrorMessage(t *testing.T) {
	e := &GitError{Args: []string{"push", "origin", "main"}, ExitCode: 1, Stderr: "denied\n"}
	got := e.Error()
	const want = "git push origin main: exit status 1: denied"
	if got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestPushInteractive(t *testing.T) {
	ctx := context.Background()
	s := gittest.New()
	if err := Push(ctx, s, "origin", "feature"); err != nil {
		t.Fatal(err)
	}
	if len(s.Interactive) != 1 {
		t.Fatalf("expected 1 interactive call, got %d", len(s.Interactive))
	}
	want := []string{"push", "-u", "origin", "feature"}
	if !reflect.DeepEqual(s.Interactive[0], want) {
		t.Fatalf("interactive args = %v, want %v", s.Interactive[0], want)
	}
}

// Integration test: exercises the real git binary in a temp repo.
func TestRemotesIntegration(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found in PATH")
	}

	t.Setenv("BH_REPO", "")
	dir := t.TempDir()
	c := &Client{GitPath: gitPath, Dir: dir}
	ctx := context.Background()

	env := []string{"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, gitPath, args...)
		cmd.Dir = dir
		cmd.Env = append(cmd.Environ(), env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("remote", "add", "origin", "git@bitbucket.org:ows/repo.git")
	run("remote", "add", "upstream", "https://bitbucket.org/ups/repo.git")

	remotes, err := Remotes(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 2 {
		t.Fatalf("expected 2 remotes, got %d: %+v", len(remotes), remotes)
	}

	repo, rr, err := ResolveRepo(ctx, c, "")
	if err != nil {
		t.Fatal(err)
	}
	want := Repo{Host: "bitbucket.org", Workspace: "ups", Name: "repo"}
	if repo != want {
		t.Fatalf("ResolveRepo = %+v, want %+v", repo, want)
	}
	if rr == nil || rr.Remote.Name != "upstream" {
		t.Fatalf("resolved remote = %+v, want upstream", rr)
	}
}
