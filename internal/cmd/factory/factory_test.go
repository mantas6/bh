package factory

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/config"
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

	f := New("test")
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
			f := New("test")
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

func TestConfigIsCached(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())

	f := New("test")
	a, err := f.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	b, err := f.Config()
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if a != b {
		t.Fatal("Config() returned different instances; want a cached config")
	}
}

func TestAPIClientUsesFactoryConfig(t *testing.T) {
	t.Setenv("BH_TOKEN", "")
	// Point the on-disk config somewhere empty: the client must come from
	// f.Config, not from a fresh config.Load.
	t.Setenv("BH_CONFIG_DIR", t.TempDir())

	f := New("1.2.3")
	calls := 0
	f.Config = func() (*config.Config, error) {
		calls++
		cfg := &config.Config{}
		cfg.SetHost(config.DefaultHost, &config.HostConfig{Token: "tok", Email: "ada@example.com"})
		return cfg, nil
	}

	client, err := f.APIClient()
	if err != nil {
		t.Fatalf("APIClient: %v", err)
	}
	if calls != 1 {
		t.Errorf("Config called %d times, want 1", calls)
	}
	if client.Token != "tok" || client.Email != "ada@example.com" {
		t.Errorf("client credentials = %q/%q", client.Token, client.Email)
	}
	if client.UserAgent != "bh/1.2.3" {
		t.Errorf("UserAgent = %q", client.UserAgent)
	}
	if client.BaseURL != api.DefaultBaseURL {
		t.Errorf("BaseURL = %q", client.BaseURL)
	}
}

func TestAPIClientNoToken(t *testing.T) {
	t.Setenv("BH_TOKEN", "")
	t.Setenv("BH_CONFIG_DIR", t.TempDir())

	f := New("test")
	if _, err := f.APIClient(); !errors.Is(err, api.ErrNoToken) {
		t.Fatalf("APIClient error = %v, want api.ErrNoToken", err)
	}
}

func TestAPIClientFor(t *testing.T) {
	f := New("1.2.3")
	c := f.APIClientFor("tok", "")
	if c.Token != "tok" || c.Email != "" {
		t.Errorf("credentials = %q/%q", c.Token, c.Email)
	}
	if c.UserAgent != "bh/1.2.3" {
		t.Errorf("UserAgent = %q", c.UserAgent)
	}
}

func TestNewIOStreams(t *testing.T) {
	f := New("test")
	ios := f.IOStreams
	if ios.In != os.Stdin || ios.Out != os.Stdout || ios.ErrOut != os.Stderr {
		t.Fatal("IOStreams not wired to the process streams")
	}
	if _, ok := ios.StdinFd(); !ok {
		t.Error("StdinFd unavailable for os.Stdin")
	}
}
