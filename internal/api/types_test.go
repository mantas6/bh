package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
)

// encode marshals v the way `--json` does: no HTML escaping.
func encode(t *testing.T, v any) string {
	t.Helper()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.String()
}

// decodeMap re-reads encoded JSON as a generic object.
func decodeMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return m
}

// listedPR is a pull request as the list endpoint returns it: the nested
// repositories carry only a few fields and there are no reviewers or
// participants.
const listedPR = `{
	"id": 1901,
	"title": "Fix <wallet> & balance",
	"state": "OPEN",
	"author": {"uuid": "{u1}", "display_name": "Ada", "type": "user",
		"links": {"html": {"href": "https://bitbucket.org/ada/"}}},
	"source": {
		"branch": {"name": "SUPER-4315"},
		"commit": {"hash": "51c907f997e1"},
		"repository": {"uuid": "{r1}", "name": "front", "full_name": "ws/front",
			"links": {"html": {"href": "https://bitbucket.org/ws/front"}}}
	},
	"destination": {
		"branch": {"name": "dev"},
		"commit": {"hash": "21aee068a125"},
		"repository": {"uuid": "{r1}", "name": "front", "full_name": "ws/front"}
	},
	"merge_commit": null,
	"closed_by": null,
	"links": {"html": {"href": "https://bitbucket.org/ws/front/pull-requests/1901"}}
}`

func TestPullRequestJSONOmitsAbsentNestedFields(t *testing.T) {
	t.Parallel()
	var pr api.PullRequest
	if err := json.Unmarshal([]byte(listedPR), &pr); err != nil {
		t.Fatal(err)
	}
	out := encode(t, pr)

	for _, absent := range []string{
		`"slug"`, `"mainbranch"`, `"owner"`, `"workspace"`, `"parent"`,
		`"is_private"`, `"clone"`, `"merge_strategies"`,
		`"default_merge_strategy"`, `"account_id"`, `"nickname"`, `"self"`,
	} {
		if strings.Contains(out, absent) {
			t.Errorf("%s should be omitted when not reported:\n%s", absent, out)
		}
	}
	m := decodeMap(t, out)
	for _, key := range []string{"author", "source", "destination"} {
		if nested := encode(t, m[key]); strings.Contains(nested, `""`) {
			t.Errorf("%s: no nested field should encode as an empty string: %s", key, nested)
		}
	}
	for _, key := range []string{"reviewers", "participants"} {
		arr, ok := m[key].([]any)
		if !ok || len(arr) != 0 {
			t.Errorf("%s = %#v, want []", key, m[key])
		}
	}
	// Top-level fields Bitbucket always returns keep their place, even when
	// null, so existing scripts keep working.
	for _, key := range []string{"merge_commit", "closed_by", "description", "reason", "draft", "comment_count"} {
		if _, ok := m[key]; !ok {
			t.Errorf("top-level %q should still be present:\n%s", key, out)
		}
	}
	src := m["source"].(map[string]any)
	repo := src["repository"].(map[string]any)
	if repo["full_name"] != "ws/front" || repo["uuid"] != "{r1}" {
		t.Errorf("source.repository = %#v", repo)
	}
	if !strings.Contains(out, `"title":"Fix <wallet> & balance"`) {
		t.Errorf("title should not be HTML-escaped:\n%s", out)
	}
}

func TestPullRequestJSONKeepsReportedFields(t *testing.T) {
	t.Parallel()
	private := true
	pr := api.PullRequest{
		ID:        1,
		Reviewers: []api.User{{DisplayName: "Bob"}},
		Participants: []api.Participant{{
			User: api.User{DisplayName: "Bob"}, Role: "REVIEWER", Approved: true,
		}},
		Destination: api.PRRef{
			Branch: api.Branch{Name: "main", DefaultMergeStrategy: "squash", MergeStrategies: []string{"squash"}},
			Repository: &api.Repository{
				FullName: "ws/repo", Slug: "repo", IsPrivate: &private,
				MainBranch: &api.Branch{Name: "main"},
			},
		},
	}
	m := decodeMap(t, encode(t, pr))

	if r := m["reviewers"].([]any); len(r) != 1 {
		t.Errorf("reviewers = %#v", r)
	}
	p := m["participants"].([]any)[0].(map[string]any)
	if p["approved"] != true || p["role"] != "REVIEWER" {
		t.Errorf("participant = %#v", p)
	}
	if _, ok := p["participated_on"]; ok {
		t.Errorf("unset participated_on should be omitted: %#v", p)
	}
	dst := m["destination"].(map[string]any)
	if dst["branch"].(map[string]any)["default_merge_strategy"] != "squash" {
		t.Errorf("destination.branch = %#v", dst["branch"])
	}
	repo := dst["repository"].(map[string]any)
	if repo["slug"] != "repo" || repo["is_private"] != true || repo["mainbranch"].(map[string]any)["name"] != "main" {
		t.Errorf("destination.repository = %#v", repo)
	}
}

func TestPullRequestPointerJSON(t *testing.T) {
	t.Parallel()
	// A *PullRequest (as printed by pr view) encodes the same way.
	m := decodeMap(t, encode(t, &api.PullRequest{ID: 2}))
	if r, ok := m["reviewers"].([]any); !ok || len(r) != 0 {
		t.Errorf("reviewers = %#v, want []", m["reviewers"])
	}
}

func TestRepositoryIsPrivateFalseIsKept(t *testing.T) {
	t.Parallel()
	var r api.Repository
	if err := json.Unmarshal([]byte(`{"full_name":"ws/repo","is_private":false}`), &r); err != nil {
		t.Fatal(err)
	}
	if r.IsPrivate == nil || *r.IsPrivate {
		t.Fatalf("IsPrivate = %v, want pointer to false", r.IsPrivate)
	}
	if out := encode(t, r); !strings.Contains(out, `"is_private":false`) {
		t.Errorf("reported is_private=false should be kept: %s", out)
	}
}

func TestPRCommitJSONParentsNeverNull(t *testing.T) {
	t.Parallel()
	out := encode(t, api.PRCommit{Hash: "abc", Message: "a < b"})
	m := decodeMap(t, out)
	if p, ok := m["parents"].([]any); !ok || len(p) != 0 {
		t.Errorf("parents = %#v, want []", m["parents"])
	}
	if _, ok := m["links"]; ok {
		t.Errorf("empty links should be omitted: %s", out)
	}
	if !strings.Contains(out, `"message":"a < b"`) {
		t.Errorf("message should not be HTML-escaped: %s", out)
	}
}

func TestCommentJSONOmitsUnsetRefs(t *testing.T) {
	t.Parallel()
	var c api.Comment
	body := `{"id":5,"content":{"raw":"hi"},"user":{"display_name":"Ada"},"deleted":false,"pending":false}`
	if err := json.Unmarshal([]byte(body), &c); err != nil {
		t.Fatal(err)
	}
	m := decodeMap(t, encode(t, c))
	for _, key := range []string{"parent", "inline", "resolution", "links"} {
		if _, ok := m[key]; ok {
			t.Errorf("%q should be omitted when unset: %#v", key, m)
		}
	}
	for _, key := range []string{"id", "content", "user", "deleted", "pending", "created_on"} {
		if _, ok := m[key]; !ok {
			t.Errorf("%q should be present: %#v", key, m)
		}
	}

	to := 3
	c.Parent = &api.CommentRef{ID: 1}
	c.Inline = &api.Inline{Path: "a.go", To: &to}
	m = decodeMap(t, encode(t, c))
	inline := m["inline"].(map[string]any)
	if inline["to"] != float64(3) {
		t.Errorf("inline = %#v", inline)
	}
	if _, ok := inline["from"]; ok {
		t.Errorf("unset inline.from should be omitted: %#v", inline)
	}
	if m["parent"].(map[string]any)["id"] != float64(1) {
		t.Errorf("parent = %#v", m["parent"])
	}
}

func TestListPullRequestsRequestsReviewersAndParticipants(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo/pullrequests", 200, `{"values":[]}`)
	if _, err := srv.APIClient().ListPullRequests(t.Context(), "ws/repo", api.ListPROptions{}); err != nil {
		t.Fatal(err)
	}
	req := srv.LastRequest(t, http.MethodGet, "/repositories/ws/repo/pullrequests")
	if got := req.Query.Get("fields"); got != "+values.reviewers,+values.participants" {
		t.Errorf("fields = %q", got)
	}
}
