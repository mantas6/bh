package pr

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func TestEditChangedKeysAndReviewers(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("GET", "/workspaces/myws/members", 200, valuesPage([]api.WorkspaceMember{
		{User: api.User{UUID: "{cara-uuid}", Nickname: "cara", DisplayName: "Cara"}},
	}))
	srv.Handle("PUT", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:              ios,
		ApiClient:       func() (*api.Client, error) { return srv.Client(), nil },
		Git:             gitFunc(gittest.New()),
		BaseRepo:        baseRepoFunc(nil),
		Arg:             "123",
		Title:           "New title",
		titleSet:        true,
		AddReviewers:    []string{"cara"},
		RemoveReviewers: []string{"bob"},
	}

	if err := editRun(opts); err != nil {
		t.Fatalf("editRun: %v", err)
	}

	if !strings.Contains(out.String(), "pull-requests/123") {
		t.Errorf("URL not printed: %q", out.String())
	}

	req := findRequest(srv, "PUT", "/pullrequests/123")
	if req == nil {
		t.Fatal("no PUT recorded")
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["title"] != "New title" {
		t.Errorf("title = %v", body["title"])
	}
	if _, ok := body["description"]; ok {
		t.Errorf("description should be absent: %v", body["description"])
	}
	if _, ok := body["draft"]; ok {
		t.Errorf("draft should be absent: %v", body["draft"])
	}
	reviewers, ok := body["reviewers"].([]any)
	if !ok || len(reviewers) != 1 {
		t.Fatalf("reviewers = %v", body["reviewers"])
	}
	// bob removed, cara added.
	if reviewers[0].(map[string]any)["uuid"] != "{cara-uuid}" {
		t.Errorf("reviewer = %v", reviewers[0])
	}
}

func TestEditDraftReady(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("PUT", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(nil),
		Arg:       "123",
		Ready:     true,
	}

	if err := editRun(opts); err != nil {
		t.Fatalf("editRun: %v", err)
	}
	req := findRequest(srv, "PUT", "/pullrequests/123")
	var body map[string]any
	json.Unmarshal(req.Body, &body)
	if body["draft"] != false {
		t.Errorf("draft = %v, want false", body["draft"])
	}
}

func TestEditNoFlags(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:       ios,
		BaseRepo: baseRepoFunc(nil),
		Arg:      "123",
	}
	err := editRun(opts)
	if err == nil || !strings.Contains(err.Error(), "specify at least one flag to edit") {
		t.Fatalf("err = %v", err)
	}
}

func TestEditDraftReadyConflict(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdEdit(f, func(o *EditOptions) error { return nil })
	cmd.SetArgs([]string{"123", "--draft", "--ready"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	err := cmd.Execute()
	var fe *cmdutil.FlagError
	if err == nil || !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}

func TestEditFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *EditOptions
	cmd := NewCmdEdit(f, func(o *EditOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"123", "-t", "T", "-B", "develop", "--add-reviewer", "a,b", "--remove-reviewer", "c"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "123" || captured.Title != "T" || captured.Base != "develop" {
		t.Errorf("parsed = %+v", captured)
	}
	if !captured.titleSet {
		t.Error("titleSet should be true")
	}
	if len(captured.AddReviewers) != 2 || len(captured.RemoveReviewers) != 1 {
		t.Errorf("reviewers add=%v remove=%v", captured.AddReviewers, captured.RemoveReviewers)
	}
}
