package cmdutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/browser"
	"github.com/mantas6/bh/internal/config"
	"github.com/mantas6/bh/internal/git"
	"golang.org/x/term"
)

// NewFactory builds a Factory wired to the real process streams, with TTY
// detection and a lazily-loaded configuration.
func NewFactory(version string) *Factory {
	io := &IOStreams{
		In:        os.Stdin,
		Out:       os.Stdout,
		ErrOut:    os.Stderr,
		stdoutTTY: term.IsTerminal(int(os.Stdout.Fd())),
		stdinTTY:  term.IsTerminal(int(os.Stdin.Fd())),
	}

	exe, err := os.Executable()
	if err != nil {
		exe = "bh"
	}

	f := &Factory{
		IOStreams:  io,
		Version:    version,
		Executable: exe,
		Browser:    browser.New(),
		Config: func() (*config.Config, error) {
			return config.Load()
		},
		Git: func() (git.Runner, error) {
			path, err := exec.LookPath("git")
			if err != nil {
				return nil, fmt.Errorf("git executable not found on PATH: %w", err)
			}
			return &git.Client{
				GitPath: path,
				Stdin:   io.In,
				Stdout:  io.Out,
				Stderr:  io.ErrOut,
			}, nil
		},
		ApiClient: func() (*api.Client, error) {
			cfg, err := config.Load()
			if err != nil {
				return nil, err
			}
			host := config.DefaultHost
			token, _ := cfg.Token(host)
			if token == "" {
				return nil, api.ErrNoToken
			}
			client := api.NewClient(api.DefaultBaseURL, token, cfg.Email(host))
			client.UserAgent = "bh/" + version
			return client, nil
		},
	}

	f.BaseRepo = func() (git.Repo, *git.ResolvedRemote, error) {
		override := repoOverride(f.RepoOverride, os.Getenv)
		gitRunner, err := f.Git()
		if err != nil {
			if override == "" {
				return git.Repo{}, nil, err
			}
			// An explicit repository doesn't need git; only the
			// matching-remote lookup is skipped.
			gitRunner = nil
		}
		return git.ResolveRepo(context.Background(), gitRunner, override)
	}

	return f
}

// repoOverride combines the -R/--repo flag value and the BH_REPO environment
// variable into the single override passed to git.ResolveRepo. The flag wins
// over the environment.
func repoOverride(flag string, getenv func(string) string) string {
	if flag = strings.TrimSpace(flag); flag != "" {
		return flag
	}
	return strings.TrimSpace(getenv("BH_REPO"))
}
