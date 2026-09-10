// Package cmdutil provides shared plumbing for bh commands: IOStreams, the
// Factory dependency container, error types, and argument validators.
package cmdutil

import (
	"github.com/mantas6/bh/internal/config"
)

// Factory is the dependency container passed to command constructors. Fields
// are added by later steps (e.g. HTTP/API clients, git, browser); keep it
// simple and extensible.
type Factory struct {
	// IOStreams provides the command's input/output streams.
	IOStreams *IOStreams
	// Version is the build version string.
	Version string
	// Executable is the resolved path to the running binary.
	Executable string

	// Config lazily loads the configuration.
	Config func() (*config.Config, error)
}
