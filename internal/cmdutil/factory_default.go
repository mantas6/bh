package cmdutil

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/mantas6/bh/internal/api"
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

	return &Factory{
		IOStreams:  io,
		Version:    version,
		Executable: exe,
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
			client := api.NewClient(config.DefaultAPIBase, token, cfg.Email(host))
			client.UserAgent = "bh/" + version
			return client, nil
		},
	}
}
