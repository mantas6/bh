package cmdtest

import (
	"strings"
	"time"

	"github.com/mantas6/bh/internal/api"
)

// FixedNow is a stable "now" for deterministic relative-time output.
func FixedNow() time.Time {
	return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
}

// User returns a user fixture for nickname, e.g. User("bob") has UUID
// "{bob-uuid}" and display name "Bob".
func User(nickname string) api.User {
	return api.User{
		UUID:        "{" + nickname + "-uuid}",
		Nickname:    nickname,
		DisplayName: strings.ToUpper(nickname[:1]) + nickname[1:],
	}
}

// Members returns a workspace member listing page for the given nicknames.
func Members(nicknames ...string) map[string]any {
	members := make([]api.WorkspaceMember, len(nicknames))
	for i, n := range nicknames {
		members[i] = api.WorkspaceMember{User: User(n)}
	}
	return ValuesPage(members)
}

// SamplePR returns an open pull request #123 in myws/myrepo from branch
// "feature" into "main", authored by ada, with bob (approved) and cara
// (changes requested) as reviewers.
func SamplePR() *api.PullRequest {
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
		Reviewers: []api.User{User("bob")},
		Participants: []api.Participant{
			{User: api.User{DisplayName: "Bob", Nickname: "bob"}, Role: "REVIEWER", Approved: true, State: "approved"},
			{User: api.User{DisplayName: "Cara", Nickname: "cara"}, Role: "REVIEWER", State: "changes_requested"},
		},
		CommentCount: 2,
		UpdatedOn:    time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC),
		Links:        api.Links{HTML: api.Link{Href: "https://bitbucket.org/myws/myrepo/pull-requests/123"}},
	}
}

// CommentAt returns a comment fixture by author created minutesAgo before
// FixedNow.
func CommentAt(id int, author, raw string, minutesAgo int) api.Comment {
	return api.Comment{
		ID:        id,
		Content:   api.Content{Raw: raw},
		User:      api.User{Nickname: author, DisplayName: author},
		CreatedOn: FixedNow().Add(-time.Duration(minutesAgo) * time.Minute),
	}
}

// ValuesPage wraps items in a single-page Bitbucket pagination envelope.
func ValuesPage(items any) map[string]any {
	return map[string]any{"values": items}
}
