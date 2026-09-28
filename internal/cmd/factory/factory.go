// Package factory wires the concrete cmdutil.Factory used by the bh binary:
// process streams with TTY detection, cached configuration, the git
// executable, the Bitbucket API client and the web browser.
package factory

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/browser"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
	"github.com/mantas6/bh/internal/git"
	"golang.org/x/term"
)

// New builds a Factory wired to the real process streams, with TTY detection
// and a lazily-loaded, cached configuration.
func New(version string) *cmdutil.Factory {
	ios := &cmdutil.IOStreams{
		In:     os.Stdin,
		Out:    os.Stdout,
		ErrOut: os.Stderr,
		Getenv: os.Getenv,
	}
	ios.SetStdinTTY(isTerminal(os.Stdin))
	ios.SetStdoutTTY(isTerminal(os.Stdout))
	ios.SetStderrTTY(isTerminal(os.Stderr))

	f := &cmdutil.Factory{
		IOStreams: ios,
		Version:   version,
		Browser:   browser.New(),
		Config:    sync.OnceValues(config.Load),
	}

	f.Git = func() (git.Runner, error) {
		path, err := exec.LookPath("git")
		if err != nil {
			return nil, fmt.Errorf("git executable not found on PATH: %w", err)
		}
		return &git.Client{
			GitPath: path,
			Stdin:   ios.In,
			Stdout:  ios.Out,
			Stderr:  ios.ErrOut,
		}, nil
	}

	f.APIClientFor = func(token, email string) *api.Client {
		client := api.NewClient(api.DefaultBaseURL, token, email)
		client.UserAgent = "bh/" + version
		return client
	}

	f.APIClient = func() (*api.Client, error) {
		cfg, err := f.Config()
		if err != nil {
			return nil, err
		}
		host := config.DefaultHost
		token, _ := cfg.Token(host)
		if token == "" {
			return nil, cmdutil.NotLoggedInError(host)
		}
		return f.APIClientFor(token, cfg.Email(host)), nil
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

func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
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
