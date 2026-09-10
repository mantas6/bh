package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
)

func TestCreatePullRequestBodyShape(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests", 201,
		map[string]any{"id": 42, "title": "Feature"})
	c := srv.Client()

	rev := "{reviewer-uuid}"
	pr, err := c.CreatePullRequest(context.Background(), "ws/repo", api.CreatePRInput{
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

	req := lastRequest(t, srv, http.MethodPost, "/repositories/ws/repo/pullrequests")
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatal(err)
	}
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
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests", 201,
		map[string]any{"id": 1})
	c := srv.Client()
	if _, err := c.CreatePullRequest(context.Background(), "ws/repo", api.CreatePRInput{
		Title:        "T",
		SourceBranch: "b",
	}); err != nil {
		t.Fatal(err)
	}
	req := lastRequest(t, srv, http.MethodPost, "/repositories/ws/repo/pullrequests")
	var body map[string]any
	json.Unmarshal(req.Body, &body)
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
	srv := apitest.New(t)
	var gotQuery string
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Write([]byte(`{"values":[{"id":7,"state":"OPEN"}]}`))
	})
	c := srv.Client()
	pr, err := c.PullRequestForBranch(context.Background(), "ws/repo", "feature/x")
	if err != nil {
		t.Fatal(err)
	}
	if pr.ID != 7 {
		t.Errorf("id = %d, want 7", pr.ID)
	}
	if !strings.Contains(gotQuery, `source.branch.name="feature/x"`) || !strings.Contains(gotQuery, `state="OPEN"`) {
		t.Errorf("q = %q", gotQuery)
	}
}

func TestPullRequestForBranchNone(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/ws/repo/pullrequests", 200,
		`{"values":[]}`)
	c := srv.Client()
	_, err := c.PullRequestForBranch(context.Background(), "ws/repo", "nope")
	if err == nil || !strings.Contains(err.Error(), "no open pull request found for branch") {
		t.Fatalf("err = %v", err)
	}
}

func TestListPullRequestsState(t *testing.T) {
	srv := apitest.New(t)
	var states []string
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		states = r.URL.Query()["state"]
		w.Write([]byte(`{"values":[]}`))
	})
	c := srv.Client()
	if _, err := c.ListPullRequests(context.Background(), "ws/repo", api.ListPROptions{
		State: []string{"OPEN", "MERGED"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 || states[0] != "OPEN" || states[1] != "MERGED" {
		t.Errorf("states = %v", states)
	}
}

func TestMergePolling(t *testing.T) {
	srv := apitest.New(t)
	srv.HandleFunc(http.MethodPost, "/repositories/ws/repo/pullrequests/5/merge", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", srv.URL+"/repositories/ws/repo/pullrequests/5/merge/task-status/abc123")
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"task_status":"PENDING"}`))
	})
	var polls int
	srv.HandleFunc(http.MethodGet, "/repositories/ws/repo/pullrequests/5/merge/task-status/abc123", func(w http.ResponseWriter, r *http.Request) {
		polls++
		if polls < 2 {
			w.Write([]byte(`{"task_status":"PENDING"}`))
			return
		}
		w.Write([]byte(`{"task_status":"SUCCESS","merge_result":{"id":5,"state":"MERGED"}}`))
	})
	c := srv.Client()
	pr, err := c.MergePullRequest(context.Background(), "ws/repo", 5, api.MergeInput{Strategy: "squash"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.State != "MERGED" {
		t.Errorf("state = %q, want MERGED", pr.State)
	}
	if polls < 2 {
		t.Errorf("expected polling, got %d polls", polls)
	}

	// Verify merge body shape.
	req := lastRequest(t, srv, http.MethodPost, "/repositories/ws/repo/pullrequests/5/merge")
	var body map[string]any
	json.Unmarshal(req.Body, &body)
	if body["type"] != "pullrequest" || body["merge_strategy"] != "squash" {
		t.Errorf("merge body wrong: %v", body)
	}
}

func TestMergeSync(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/5/merge", 200,
		map[string]any{"id": 5, "state": "MERGED"})
	c := srv.Client()
	pr, err := c.MergePullRequest(context.Background(), "ws/repo", 5, api.MergeInput{})
	if err != nil {
		t.Fatal(err)
	}
	if pr.State != "MERGED" {
		t.Errorf("state = %q", pr.State)
	}
}

func TestApproveDeclineChanges(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/1/approve", 200,
		map[string]any{"approved": true})
	srv.Handle(http.MethodDelete, "/repositories/ws/repo/pullrequests/1/approve", 204, nil)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/1/request-changes", 200,
		map[string]any{"state": "changes_requested"})
	srv.Handle(http.MethodDelete, "/repositories/ws/repo/pullrequests/1/request-changes", 204, nil)
	srv.Handle(http.MethodPost, "/repositories/ws/repo/pullrequests/1/decline", 200,
		map[string]any{"id": 1, "state": "DECLINED"})
	c := srv.Client()
	ctx := context.Background()

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
	srv := apitest.New(t)
	srv.Handle(http.MethodPut, "/repositories/ws/repo/pullrequests/9", 200,
		map[string]any{"id": 9, "title": "New"})
	c := srv.Client()
	pr, err := c.UpdatePullRequest(context.Background(), "ws/repo", 9, map[string]any{"title": "New"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Title != "New" {
		t.Errorf("title = %q", pr.Title)
	}
}

func lastRequest(t *testing.T, srv *apitest.Server, method, path string) apitest.Request {
	t.Helper()
	for i := len(srv.Requests) - 1; i >= 0; i-- {
		r := srv.Requests[i]
		if r.Method == method && r.Path == path {
			return r
		}
	}
	t.Fatalf("no recorded request %s %s", method, path)
	return apitest.Request{}
}
