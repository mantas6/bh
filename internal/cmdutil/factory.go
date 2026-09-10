// Package cmdutil provides shared plumbing for bh commands: IOStreams, the
// Factory dependency container, error types, and argument validators.
package cmdutil

import (
	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/browser"
	"github.com/mantas6/bh/internal/config"
	"github.com/mantas6/bh/internal/git"
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

	// Git lazily returns a git.Runner backed by the git executable. Commands
	// use it with git.ResolveRepo and the package's high-level helpers.
	Git func() (git.Runner, error)

	// ApiClient lazily builds an authenticated Bitbucket API client. It
	// returns api.ErrNoToken when no credentials are configured.
	ApiClient func() (*api.Client, error)

	// Browser opens URLs in the user's web browser.
	Browser *browser.Browser

	// RepoOverride holds the value of the global -R/--repo flag; it feeds
	// BaseRepo. The root command binds this to the persistent flag.
	RepoOverride string

	// BaseRepo resolves the base repository using the precedence
	// -R/--repo > BH_REPO > upstream > origin > first Bitbucket remote. The
	// returned *git.ResolvedRemote is the matching git remote, or nil.
	BaseRepo func() (git.Repo, *git.ResolvedRemote, error)
}
