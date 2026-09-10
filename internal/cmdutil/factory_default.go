package cmdutil

import (
	"os"

	"github.com/mantas6/bh/internal/config"
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
	}
}
