package comment

import (
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

// gitFunc adapts a gittest.Stub into a Factory-style Git provider.
func gitFunc(stub *gittest.Stub) func() (git.Runner, error) {
	return func() (git.Runner, error) {
		return stub, nil
	}
}

// newGitStub returns a stub that answers the current-branch lookup with
// "feature", matching the sample PR's source branch.
func newGitStub() *gittest.Stub {
	return gittest.New().Register("feature", nil, "symbolic-ref", "--quiet", "--short", "HEAD")
}

// fixedNow is a stable "now" for deterministic relative-time output in tests.
func fixedNow() time.Time {
	return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
}

// nowFunc returns a Now provider yielding fixedNow.
func nowFunc() func() time.Time {
	return fixedNow
}

// testRepo is the base repo used across comment command tests.
func testRepo() git.Repo {
	return git.Repo{Host: "bitbucket.org", Workspace: "myws", Name: "myrepo"}
}

// baseRepoFunc returns a BaseRepo resolver yielding testRepo and no remote.
func baseRepoFunc() func() (git.Repo, *git.ResolvedRemote, error) {
	return func() (git.Repo, *git.ResolvedRemote, error) {
		return testRepo(), nil, nil
	}
}

// clientFunc returns an ApiClient provider bound to srv.
func clientFunc(srv *apitest.Server) func() (*api.Client, error) {
	return func() (*api.Client, error) {
		return srv.Client(), nil
	}
}

// samplePR returns a representative pull request fixture. The comment
// subpackage cannot import package pr's test symbols, so it duplicates an
// equivalent fixture here.
func samplePR() *api.PullRequest {
	return &api.PullRequest{
		ID:     123,
		Title:  "Add feature",
		State:  "OPEN",
		Author: &api.User{Nickname: "ada", DisplayName: "Ada Lovelace"},
		Source: api.PRRef{
			Branch:     api.Branch{Name: "feature"},
			Repository: &api.Repository{FullName: "myws/myrepo"},
		},
		Destination: api.PRRef{
			Branch:     api.Branch{Name: "main"},
			Repository: &api.Repository{FullName: "myws/myrepo"},
		},
		Links: api.Links{HTML: api.Link{Href: "https://bitbucket.org/myws/myrepo/pull-requests/123"}},
	}
}

// handlePR registers the by-number PR lookup used when arg == "123".
func handlePR(srv *apitest.Server) {
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
}

// intPtr returns a pointer to n.
func intPtr(n int) *int { return &n }

// commentAt builds a Comment fixture with a fixed created time offset.
func commentAt(id int, author, raw string, minutesAgo int) api.Comment {
	return api.Comment{
		ID:        id,
		Content:   api.Content{Raw: raw},
		User:      api.User{Nickname: author, DisplayName: author},
		CreatedOn: fixedNow().Add(-time.Duration(minutesAgo) * time.Minute),
	}
}

// findRequest returns the first recorded request whose method matches and
// whose path ends with suffix, or nil.
func findRequest(srv *apitest.Server, method, suffix string) *apitest.Request {
	for i := range srv.Requests {
		r := &srv.Requests[i]
		if r.Method == method && strings.HasSuffix(r.Path, suffix) {
			return r
		}
	}
	return nil
}
