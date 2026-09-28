package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
)

func TestCreatePullRequestBodyShape(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests", 201,
		map[string]any{"id": 42, "title": "Feature"})
	c := srv.APIClient()

	rev := "{reviewer-uuid}"
	pr, err := c.CreatePullRequest(t.Context(), "ws/repo", api.CreatePRInput{
		Title:             "Feature",
		Description:       "Body",
		SourceBranch:      "feat",
		SourceRepo:        "fork/repo",
		DestinationBranch: "main",
		Reviewers:         []string{rev},
		Draft:             true,
		CloseSourceBranch: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if pr.ID != 42 {
		t.Errorf("id = %d, want 42", pr.ID)
	}

	req := srv.LastRequest(t, http.MethodPost, "/repositories/ws/repo/pullrequests")
	var body map[string]any
	req.DecodeJSON(t, &body)
	if body["title"] != "Feature" || body["description"] != "Body" {
		t.Errorf("title/description wrong: %v", body)
	}
	if body["draft"] != true || body["close_source_branch"] != true {
		t.Errorf("flags wrong: %v", body)
	}
	src := body["source"].(map[string]any)
	if src["branch"].(map[string]any)["name"] != "feat" {
		t.Errorf("source branch wrong: %v", src)
	}
	if src["repository"].(map[string]any)["full_name"] != "fork/repo" {
		t.Errorf("source repo wrong: %v", src)
	}
	dest := body["destination"].(map[string]any)
	if dest["branch"].(map[string]any)["name"] != "main" {
		t.Errorf("destination wrong: %v", dest)
	}
	revs := body["reviewers"].([]any)
	if len(revs) != 1 || revs[0].(map[string]any)["uuid"] != rev {
		t.Errorf("reviewers wrong: %v", revs)
	}
}

func TestCreatePullRequestOmitsDestination(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests", 201,
		map[string]any{"id": 1})
	c := srv.APIClient()
	if _, err := c.CreatePullRequest(t.Context(), "ws/repo", api.CreatePRInput{
		Title:        "T",
		SourceBranch: "b",
	}); err != nil {
		t.Fatal(err)
	}
	req := srv.LastRequest(t, http.MethodPost, "/repositories/ws/repo/pullrequests")
	var body map[string]any
	req.DecodeJSON(t, &body)
	if _, ok := body["destination"]; ok {
		t.Errorf("destination should be omitted, got %v", body["destination"])
	}
	if _, ok := body["reviewers"]; ok {
		t.Errorf("reviewers should be omitted when nil")
	}
	if _, ok := body["source"].(map[string]any)["repository"]; ok {
		t.Errorf("source.repository should be omitted")
	}
}

func TestPullRequestForBranch(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	var gotQuery, gotSort string
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		gotSort = r.URL.Query().Get("sort")
		w.Write([]byte(`{"values":[{"id":7,"state":"OPEN"}]}`))
	})
	c := srv.APIClient()
	pr, err := c.PullRequestForBranch(t.Context(), "ws/repo", "feature/x")
	if err != nil {
		t.Fatal(err)
	}
	if pr.ID != 7 {
		t.Errorf("id = %d, want 7", pr.ID)
	}
	if !strings.Contains(gotQuery, `source.branch.name="feature/x"`) || !strings.Contains(gotQuery, `state="OPEN"`) {
		t.Errorf("q = %q", gotQuery)
	}
	if gotSort != "-updated_on" {
		t.Errorf("sort = %q, want -updated_on", gotSort)
	}
}

func TestPullRequestForBranchEscapesQuery(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	var gotQuery string
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Write([]byte(`{"values":[{"id":7,"state":"OPEN"}]}`))
	})
	c := srv.APIClient()
	if _, err := c.PullRequestForBranch(t.Context(), "ws/repo", `x" OR state="MERGED`); err != nil {
		t.Fatal(err)
	}
	want := `source.branch.name="x\" OR state=\"MERGED" AND state="OPEN"`
	if gotQuery != want {
		t.Errorf("q = %q, want %q", gotQuery, want)
	}
}

func TestQuoteBBQL(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"plain", `"plain"`},
		{"", `""`},
		{`a"b`, `"a\"b"`},
		{`a\b`, `"a\\b"`},
		{`\"`, `"\\\""`},
	}
	for _, tt := range tests {
		if got := api.QuoteBBQL(tt.in); got != tt.want {
			t.Errorf("QuoteBBQL(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestListPullRequestsEmptyIsNonNil(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo/pullrequests", 200, `{"values":[]}`)
	c := srv.APIClient()
	prs, err := c.ListPullRequests(t.Context(), "ws/repo", api.ListPROptions{})
	if err != nil {
		t.Fatal(err)
	}
	if prs == nil || len(prs) != 0 {
		t.Errorf("prs = %#v, want non-nil empty slice", prs)
	}
}

func TestPullRequestForBranchNone(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo/pullrequests", 200,
		`{"values":[]}`)
	c := srv.APIClient()
	_, err := c.PullRequestForBranch(t.Context(), "ws/repo", "nope")
	if err == nil || !strings.Contains(err.Error(), "no open pull request found for branch") {
		t.Fatalf("err = %v", err)
	}
}

func TestListPullRequestsState(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	var states []string
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		states = r.URL.Query()["state"]
		w.Write([]byte(`{"values":[]}`))
	})
	c := srv.APIClient()
	if _, err := c.ListPullRequests(t.Context(), "ws/repo", api.ListPROptions{
		State: []string{"OPEN", "MERGED"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0] != "OPEN" || states[1] != "MERGED" {
		t.Errorf("states = %v", states)
	}
}

// mergeServer answers POST .../5/merge with 202 and the given Location, and
// serves statuses (one per poll, the last repeating) on statusPath.
func mergeServer(t *testing.T, location, statusPath string, statuses ...string) (*apitest.Server, *atomic.Int32) {
	t.Helper()
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodPost, "/repositories/ws/repo/pullrequests/5/merge", func(w http.ResponseWriter, r *http.Request) {
		if location != "" {
			w.Header().Set("Location", strings.ReplaceAll(location, "SRV", srv.URL))
		}
		w.WriteHeader(http.StatusAccepted)
		fmt.Fprint(w, `{"task_status":"PENDING","task_id":"abc123"}`)
	})
	var polls atomic.Int32
	srv.HandleFunc(http.MethodGet, statusPath, func(w http.ResponseWriter, r *http.Request) {
		n := int(polls.Add(1))
		fmt.Fprint(w, statuses[min(n, len(statuses))-1])
	})
	return srv, &polls
}

const (
	statusPath = "/repositories/ws/repo/pullrequests/5/merge/task-status/abc123"
	pending    = `{"task_status":"PENDING"}`
	succeeded  = `{"task_status":"SUCCESS","merge_result":{"id":5,"state":"MERGED"}}`
)

func TestMergePolling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		location   string
		statusPath string
	}{
		{"absolute Location", "SRV" + statusPath, statusPath},
		{"relative Location", "task-status/abc123", "/repositories/ws/repo/pullrequests/5/task-status/abc123"},
		{"root-relative Location", "/elsewhere/tasks/abc123?x=1", "/elsewhere/tasks/abc123"},
		{"no Location falls back to task_id", "", statusPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, polls := mergeServer(t, tt.location, tt.statusPath, pending, succeeded)
			c := srv.APIClient()
			var sleeps []time.Duration
			c.PollInterval = 3 * time.Second
			c.Sleep = func(ctx context.Context, d time.Duration) error {
				sleeps = append(sleeps, d)
				return nil
			}
			pr, err := c.MergePullRequest(t.Context(), "ws/repo", 5, api.MergeInput{Strategy: "squash"})
			if err != nil {
				t.Fatal(err)
			}
			if pr.State != "MERGED" {
				t.Errorf("state = %q, want MERGED", pr.State)
			}
			if polls.Load() != 2 {
				t.Errorf("polls = %d, want 2", polls.Load())
			}
			if len(sleeps) != 2 || sleeps[0] != 3*time.Second {
				t.Errorf("sleeps = %v, want 2 x 3s", sleeps)
			}

			req := srv.LastRequest(t, http.MethodPost, "/repositories/ws/repo/pullrequests/5/merge")
			var body map[string]any
			req.DecodeJSON(t, &body)
			if body["type"] != "pullrequest" || body["merge_strategy"] != "squash" {
				t.Errorf("merge body wrong: %v", body)
			}
		})
	}
}

func TestMergeFailedStatus(t *testing.T) {
	t.Parallel()
	srv, _ := mergeServer(t, "SRV"+statusPath, statusPath, pending, `{"task_status":"FAILED"}`)
	_, err := srv.APIClient().MergePullRequest(t.Context(), "ws/repo", 5, api.MergeInput{})
	if err == nil || !strings.Contains(err.Error(), `merge failed with task status "FAILED"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestMergePollingGivesUp(t *testing.T) {
	t.Parallel()
	srv, polls := mergeServer(t, "SRV"+statusPath, statusPath, pending)
	c := srv.APIClient()
	c.MaxPollAttempts = 3
	_, err := c.MergePullRequest(t.Context(), "ws/repo", 5, api.MergeInput{})
	if err == nil || !strings.Contains(err.Error(), "did not complete after 3 polls") {
		t.Fatalf("err = %v", err)
	}
	if polls.Load() != 3 {
		t.Errorf("polls = %d, want 3", polls.Load())
	}
}

func TestMergePollingCancelled(t *testing.T) {
	t.Parallel()
	srv, polls := mergeServer(t, "SRV"+statusPath, statusPath, pending)
	c := srv.APIClient()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	c.Sleep = func(ctx context.Context, d time.Duration) error {
		if polls.Load() == 1 {
			cancel()
		}
		return ctx.Err()
	}
	_, err := c.MergePullRequest(ctx, "ws/repo", 5, api.MergeInput{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if polls.Load() != 1 {
		t.Errorf("polls = %d, want 1", polls.Load())
	}
}

func TestMergeAcceptedWithoutLocation(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/5/merge", http.StatusAccepted, `{}`)
	_, err := srv.APIClient().MergePullRequest(t.Context(), "ws/repo", 5, api.MergeInput{})
	if err == nil || !strings.Contains(err.Error(), "no task status location") {
		t.Fatalf("err = %v", err)
	}
}

func TestMergeSync(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/5/merge", 200,
		map[string]any{"id": 5, "state": "MERGED"})
	c := srv.APIClient()
	pr, err := c.MergePullRequest(t.Context(), "ws/repo", 5, api.MergeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if pr.State != "MERGED" {
		t.Errorf("state = %q", pr.State)
	}
}

func TestApproveDeclineChanges(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/1/approve", 200,
		map[string]any{"approved": true})
	srv.Handle(http.MethodDelete, "/repositories/ws/repo/pullrequests/1/approve", 204, nil)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/1/request-changes", 200,
		map[string]any{"state": "changes_requested"})
	srv.Handle(http.MethodDelete, "/repositories/ws/repo/pullrequests/1/request-changes", 204, nil)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/1/decline", 200,
		map[string]any{"id": 1, "state": "DECLINED"})
	c := srv.APIClient()
	ctx := t.Context()

	if err := c.ApprovePullRequest(ctx, "ws/repo", 1); err != nil {
		t.Errorf("approve: %v", err)
	}
	if err := c.UnapprovePullRequest(ctx, "ws/repo", 1); err != nil {
		t.Errorf("unapprove: %v", err)
	}
	if err := c.RequestChanges(ctx, "ws/repo", 1); err != nil {
		t.Errorf("request-changes: %v", err)
	}
	if err := c.UnrequestChanges(ctx, "ws/repo", 1); err != nil {
		t.Errorf("unrequest-changes: %v", err)
	}
	pr, err := c.DeclinePullRequest(ctx, "ws/repo", 1)
	if err != nil {
		t.Fatalf("decline: %v", err)
	}
	if pr.State != "DECLINED" {
		t.Errorf("decline state = %q", pr.State)
	}
}

func TestUpdatePullRequest(t *testing.T) {
	t.Parallel()
	title, desc, base, draft := "New", "", "develop", false
	tests := []struct {
		name string
		in   api.UpdatePRInput
		want string
	}{
		{"title only", api.UpdatePRInput{Title: &title}, `{"title":"New"}`},
		{"empty description is sent", api.UpdatePRInput{Description: &desc}, `{"description":""}`},
		{"destination", api.UpdatePRInput{DestinationBranch: &base}, `{"destination":{"branch":{"name":"develop"}}}`},
		{"draft false is sent", api.UpdatePRInput{Draft: &draft}, `{"draft":false}`},
		{"reviewers", api.UpdatePRInput{Reviewers: []string{"{a}", "{b}"}}, `{"reviewers":[{"uuid":"{a}"},{"uuid":"{b}"}]}`},
		{"empty reviewers clears", api.UpdatePRInput{Reviewers: []string{}}, `{"reviewers":[]}`},
		{"nothing", api.UpdatePRInput{}, `{}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle(http.MethodPut, "/repositories/ws/repo/pullrequests/9", 200,
				map[string]any{"id": 9, "title": "New"})
			pr, err := srv.APIClient().UpdatePullRequest(t.Context(), "ws/repo", 9, tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if pr.Title != "New" {
				t.Errorf("title = %q", pr.Title)
			}
			if got := string(srv.LastRequest(t, http.MethodPut, "/repositories/ws/repo/pullrequests/9").Body); got != tt.want {
				t.Errorf("body = %s, want %s", got, tt.want)
			}
		})
	}
}
