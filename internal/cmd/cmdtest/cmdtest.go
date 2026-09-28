// Package cmdtest provides helpers and fixtures shared by the command tests
// under internal/cmd: running a cobra command with captured output, wiring
// fake API/git/config dependencies into command options, finding recorded API
// requests, and sample Bitbucket objects.
package cmdtest

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
	"github.com/spf13/cobra"
)

// RunCommand executes cmd with args under t.Context() and returns what cobra
// itself wrote to its output and error streams (usage, help, version).
// Command output written through IOStreams goes to the IOStreams buffers.
func RunCommand(t testing.TB, cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	return RunCommandContext(t.Context(), t, cmd, args...)
}

// RunCommandContext is RunCommand with an explicit context.
func RunCommandContext(ctx context.Context, t testing.TB, cmd *cobra.Command, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	if args == nil {
		// cobra falls back to os.Args when args are nil.
		args = []string{}
	}
	var out, errOut bytes.Buffer
	cmd.SetArgs(args)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err = cmd.ExecuteContext(ctx)
	return out.String(), errOut.String(), err
}

// NewFactory returns a Factory with in-memory IOStreams and no other
// dependencies, for tests that only exercise flag and argument parsing.
func NewFactory() *cmdutil.Factory {
	ios, _, _, _ := cmdutil.TestIOStreams()
	return &cmdutil.Factory{IOStreams: ios}
}

// AssertFlagError fails t unless err is (or wraps) a *cmdutil.FlagError whose
// message contains want. An empty want accepts any FlagError.
func AssertFlagError(t testing.TB, err error, want string) {
	t.Helper()
	var fe *cmdutil.FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("error = %v (%T), want *cmdutil.FlagError", err, err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), want)
	}
}

// ClientFunc returns an APIClient provider bound to srv.
func ClientFunc(srv *apitest.Server) func() (*api.Client, error) {
	return func() (*api.Client, error) {
		return srv.APIClient(), nil
	}
}

// GitFunc returns a Git provider yielding stub.
func GitFunc(stub *gittest.Stub) func() (git.Runner, error) {
	return func() (git.Runner, error) {
		return stub, nil
	}
}

// TestRepo is the base repository used across command tests.
func TestRepo() git.Repo {
	return git.Repo{Host: "bitbucket.org", Workspace: "myws", Name: "myrepo"}
}

// OriginRemote is TestRepo resolved from an SSH "origin" remote.
func OriginRemote() *git.ResolvedRemote {
	return &git.ResolvedRemote{
		Remote: git.Remote{Name: "origin", FetchURL: "git@bitbucket.org:myws/myrepo.git"},
		Repo:   TestRepo(),
	}
}

// BaseRepoFunc returns a BaseRepo resolver yielding TestRepo and rr (which
// may be nil, as when the repository comes from -R without a local remote).
func BaseRepoFunc(rr *git.ResolvedRemote) func(context.Context) (git.Repo, *git.ResolvedRemote, error) {
	return func(context.Context) (git.Repo, *git.ResolvedRemote, error) {
		return TestRepo(), rr, nil
	}
}

// FindRequest returns the most recent request recorded by srv whose method
// matches and whose path ends with pathSuffix, or nil if there is none.
func FindRequest(srv *apitest.Server, method, pathSuffix string) *apitest.Request {
	reqs := srv.Requests()
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Method == method && strings.HasSuffix(reqs[i].Path, pathSuffix) {
			return &reqs[i]
		}
	}
	return nil
}

// RequireRequest is FindRequest that fails t when no request matches.
func RequireRequest(t testing.TB, srv *apitest.Server, method, pathSuffix string) apitest.Request {
	t.Helper()
	req := FindRequest(srv, method, pathSuffix)
	if req == nil {
		t.Fatalf("no recorded %s request with path suffix %q; got %d requests", method, pathSuffix, len(srv.Requests()))
		return apitest.Request{}
	}
	return *req
}

// FakeBrowser records the URLs passed to Browse.
type FakeBrowser struct {
	URLs []string
}

// Browse implements cmdutil.Browser.
func (b *FakeBrowser) Browse(u string) error {
	b.URLs = append(b.URLs, u)
	return nil
}

// URL returns the last URL opened, or "".
func (b *FakeBrowser) URL() string {
	if len(b.URLs) == 0 {
		return ""
	}
	return b.URLs[len(b.URLs)-1]
}

// ClientForFunc returns an APIClientFor provider bound to srv that honours
// the credentials passed by the command.
func ClientForFunc(srv *apitest.Server) func(token, email string) *api.Client {
	return func(token, email string) *api.Client {
		c := srv.APIClient()
		c.Token, c.Email = token, email
		return c
	}
}
