package comment

import (
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/git/gittest"
)

// newGitStub returns a stub that answers the current-branch lookup with
// "feature", matching the sample PR's source branch.
func newGitStub(t *testing.T) *gittest.Stub {
	return gittest.New(t).Register("feature", nil, "symbolic-ref", "--quiet", "--short", "HEAD")
}

// handlePR registers the by-number PR lookup used when arg == "123".
func handlePR(srv *apitest.Server) {
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
}
