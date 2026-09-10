package pr

import (
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

// gitFunc adapts a gittest.Stub into a Factory-style Git provider.
func gitFunc(stub *gittest.Stub) func() (git.Runner, error) {
	return func() (git.Runner, error) {
		return stub, nil
	}
}

// fixedNow is a stable "now" for deterministic relative-time output in tests.
func fixedNow() time.Time {
	return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
}

// samplePR returns a representative pull request fixture. Steps 6-7 (package
// pr) should reuse this and the helpers below. The comment subpackage
// (internal/cmd/pr/comment) cannot import these test symbols and must
// duplicate an equivalent fixture.
func samplePR() *api.PullRequest {
	return &api.PullRequest{
		ID:          123,
		Title:       "Add feature",
		Description: "This does things.",
		State:       "OPEN",
		Author:      &api.User{Nickname: "ada", DisplayName: "Ada Lovelace", UUID: "{ada-uuid}"},
		Source: api.PRRef{
			Branch:     api.Branch{Name: "feature"},
			Repository: &api.Repository{FullName: "myws/myrepo"},
		},
		Destination: api.PRRef{
			Branch:     api.Branch{Name: "main"},
			Repository: &api.Repository{FullName: "myws/myrepo"},
		},
		Reviewers: []api.User{{UUID: "{bob-uuid}", Nickname: "bob", DisplayName: "Bob"}},
		Participants: []api.Participant{
			{User: api.User{DisplayName: "Bob", Nickname: "bob"}, Role: "REVIEWER", Approved: true, State: "approved"},
			{User: api.User{DisplayName: "Cara", Nickname: "cara"}, Role: "REVIEWER", State: "changes_requested"},
		},
		CommentCount: 2,
		UpdatedOn:    time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC),
		Links:        api.Links{HTML: api.Link{Href: "https://bitbucket.org/myws/myrepo/pull-requests/123"}},
	}
}

// testRepo is the base repo used across pr command tests.
func testRepo() git.Repo {
	return git.Repo{Host: "bitbucket.org", Workspace: "myws", Name: "myrepo"}
}

// baseRepoFunc returns a BaseRepo resolver yielding testRepo and the supplied
// resolved remote.
func baseRepoFunc(rr *git.ResolvedRemote) func() (git.Repo, *git.ResolvedRemote, error) {
	return func() (git.Repo, *git.ResolvedRemote, error) {
		return testRepo(), rr, nil
	}
}

// fakeBrowser records the URL passed to Browse.
type fakeBrowser struct{ url string }

func (b *fakeBrowser) Browse(u string) error {
	b.url = u
	return nil
}

// valuesPage wraps items in a Bitbucket pagination envelope.
func valuesPage(items any) map[string]any {
	return map[string]any{"values": items}
}
