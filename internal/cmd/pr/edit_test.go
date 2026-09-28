package pr

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func TestEditChangedKeysAndReviewers(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("GET", "/workspaces/myws/members", 200, cmdtest.Members("cara"))
	srv.Handle("PUT", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:              ios,
		APIClient:       cmdtest.ClientFunc(srv),
		Git:             cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:        cmdtest.BaseRepoFunc(nil),
		Arg:             "123",
		Title:           "New title",
		titleSet:        true,
		AddReviewers:    []string{"cara"},
		RemoveReviewers: []string{"bob"},
	}

	if err := editRun(t.Context(), opts); err != nil {
		t.Fatalf("editRun: %v", err)
	}

	if !strings.Contains(out.String(), "pull-requests/123") {
		t.Errorf("URL not printed: %q", out.String())
	}

	var body map[string]any
	cmdtest.RequireRequest(t, srv, "PUT", "/pullrequests/123").DecodeJSON(t, &body)
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
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("PUT", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		Ready:     true,
	}

	if err := editRun(t.Context(), opts); err != nil {
		t.Fatalf("editRun: %v", err)
	}
	var body map[string]any
	cmdtest.RequireRequest(t, srv, "PUT", "/pullrequests/123").DecodeJSON(t, &body)
	if body["draft"] != false {
		t.Errorf("draft = %v, want false", body["draft"])
	}
}

func TestEditReviewersResolvedWithOneListing(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("GET", "/workspaces/myws/members", 200, cmdtest.Members("cara", "dan"))
	srv.Handle("PUT", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:           ios,
		APIClient:    cmdtest.ClientFunc(srv),
		Git:          cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:     cmdtest.BaseRepoFunc(nil),
		Arg:          "123",
		AddReviewers: []string{"cara", "dan", "bob-uuid"},
	}
	if err := editRun(t.Context(), opts); err == nil || !strings.Contains(err.Error(), `no workspace member matches "bob-uuid"`) {
		t.Fatalf("err = %v, want unmatched reviewer error", err)
	}

	opts.AddReviewers = []string{"cara", "dan", "CARA"}
	if err := editRun(t.Context(), opts); err != nil {
		t.Fatalf("editRun: %v", err)
	}
	var members int
	for _, r := range srv.Requests() {
		if r.Path == "/workspaces/myws/members" {
			members++
		}
	}
	if members != 2 {
		t.Errorf("member listings = %d, want one per run", members)
	}
	var body struct {
		Reviewers []api.User `json:"reviewers"`
	}
	srv.LastRequest(t, "PUT", "/repositories/myws/myrepo/pullrequests/123").DecodeJSON(t, &body)
	var got []string
	for _, u := range body.Reviewers {
		got = append(got, u.UUID)
	}
	if strings.Join(got, ",") != "{bob-uuid},{cara-uuid},{dan-uuid}" {
		t.Errorf("reviewers = %v", got)
	}
}

func TestEditRemoveAllReviewersSendsEmptyList(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("PUT", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:              ios,
		APIClient:       cmdtest.ClientFunc(srv),
		Git:             cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:        cmdtest.BaseRepoFunc(nil),
		Arg:             "123",
		RemoveReviewers: []string{"bob"},
	}
	if err := editRun(t.Context(), opts); err != nil {
		t.Fatalf("editRun: %v", err)
	}
	if got := string(srv.LastRequest(t, "PUT", "/repositories/myws/myrepo/pullrequests/123").Body); got != `{"reviewers":[]}` {
		t.Errorf("body = %s", got)
	}
}

func TestEditNoFlags(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:       ios,
		BaseRepo: cmdtest.BaseRepoFunc(nil),
		Arg:      "123",
	}
	err := editRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "specify at least one flag to edit") {
		t.Fatalf("err = %v", err)
	}
}

func TestEditDraftReadyConflict(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdEdit(f, func(o *EditOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "123", "--draft", "--ready")
	cmdtest.AssertFlagError(t, err, "")
}

func TestEditFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *EditOptions
	cmd := NewCmdEdit(f, func(o *EditOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "123", "-t", "T", "-B", "develop", "--add-reviewer", "a,b", "--remove-reviewer", "c"); err != nil {
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
