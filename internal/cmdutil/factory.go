// Package cmdutil provides shared helpers for bh commands: IOStreams and
// prompting, the Factory dependency container, error types, flag and
// argument validators, and small output helpers. The concrete Factory is
// wired in package internal/cmd/factory.
package cmdutil

import (
	"context"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/config"
	"github.com/mantas6/bh/internal/git"
)

// Factory is the dependency container passed to command constructors.
type Factory struct {
	// IOStreams provides the command's input/output streams.
	IOStreams *IOStreams
	// Version is the build version string.
	Version string

	// Config lazily loads the configuration.
	Config func() (*config.Config, error)

	// Git lazily returns a git.Runner backed by the git executable. Commands
	// use it with git.ResolveRepo and the package's high-level helpers.
	Git func() (git.Runner, error)

	// APIClient lazily builds an API client authenticated with the
	// configured credentials. It returns api.ErrNoToken when none are set.
	APIClient func() (*api.Client, error)

	// APIClientFor builds an API client for explicit credentials, e.g. a
	// token that has not been saved yet.
	APIClientFor func(token, email string) *api.Client

	// Browser opens URLs in the user's web browser.
	Browser Browser

	// RepoOverride holds the value of the -R/--repo flag; it feeds BaseRepo.
	// The pr command group binds this to its persistent flag.
	RepoOverride string

	// BaseRepo resolves the base repository using the precedence
	// -R/--repo > BH_REPO > upstream > origin > first Bitbucket remote. The
	// returned *git.ResolvedRemote is the matching git remote, or nil. ctx
	// bounds the git invocations used to list remotes.
	BaseRepo func(context.Context) (git.Repo, *git.ResolvedRemote, error)
}
