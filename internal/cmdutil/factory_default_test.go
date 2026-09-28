package cmdutil

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mantas6/bh/internal/git"
)

func TestRepoOverride(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{"none", "", "", ""},
		{"flag only", "flag/repo", "", "flag/repo"},
		{"env only", "", "env/repo", "env/repo"},
		{"flag over env", "flag/repo", "env/repo", "flag/repo"},
		{"blank flag falls back to env", "  ", "env/repo", "env/repo"},
		{"trims env", "", "  env/repo\n", "env/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				if key != "BH_REPO" {
					t.Errorf("unexpected env lookup %q", key)
				}
				return tt.env
			}
			if got := repoOverride(tt.flag, getenv); got != tt.want {
				t.Fatalf("repoOverride(%q, %q) = %q, want %q", tt.flag, tt.env, got, tt.want)
			}
		})
	}
}

func TestBaseRepoOverrideWithoutGit(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("BH_REPO", "")

	f := NewFactory("test")
	f.RepoOverride = "ws/repo"

	repo, rr, err := f.BaseRepo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := (git.Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}); repo != want {
		t.Fatalf("repo = %+v, want %+v", repo, want)
	}
	if rr != nil {
		t.Fatalf("resolved remote = %+v, want nil", rr)
	}

	// Without an override the missing git binary is still an error.
	f.RepoOverride = ""
	if _, _, err := f.BaseRepo(); err == nil {
		t.Fatal("expected error without override and git, got nil")
	}
}

func TestBaseRepoOutsideCheckout(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found in PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	tests := []struct {
		name string
		flag string
		env  string
		want git.Repo
	}{
		{"flag", "flag/repo", "", git.Repo{Host: "bitbucket.org", Workspace: "flag", Name: "repo"}},
		{"env", "", "env/repo", git.Repo{Host: "bitbucket.org", Workspace: "env", Name: "repo"}},
		{"flag over env", "flag/repo", "env/repo", git.Repo{Host: "bitbucket.org", Workspace: "flag", Name: "repo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BH_REPO", tt.env)
			f := NewFactory("test")
			f.RepoOverride = tt.flag

			repo, rr, err := f.BaseRepo()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo != tt.want {
				t.Fatalf("repo = %+v, want %+v", repo, tt.want)
			}
			if rr != nil {
				t.Fatalf("resolved remote = %+v, want nil", rr)
			}
		})
	}
}
