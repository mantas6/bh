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

// scopeErrorBody is Bitbucket's 403 body for a token without the scope the
// member listing needs; its "detail" is an object.
const scopeErrorBody = `{"type":"error","error":{"message":"Your credentials lack one or more required privilege scopes.",` +
	`"detail":{"granted":["read:pullrequest:bitbucket"],"required":["read:workspace:bitbucket"]}}}`

func TestEditAddReviewerForbiddenExplainsScope(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("GET", "/workspaces/myws/members", 403, scopeErrorBody)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:           ios,
		APIClient:    cmdtest.ClientFunc(srv),
		Git:          cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:     cmdtest.BaseRepoFunc(nil),
		Arg:          "123",
		AddReviewers: []string{"Julius Smatavicius", "gytis.putvinskas"},
	}
	err := editRun(t.Context(), opts)
	if !api.IsForbidden(err) {
		t.Fatalf("err = %v, want 403", err)
	}
	want := `resolving reviewers "Julius Smatavicius", "gytis.putvinskas": listing workspace members: ` +
		`Your credentials lack one or more required privilege scopes. (HTTP 403): ` +
		`required scopes: read:workspace:bitbucket; granted: read:pullrequest:bitbucket`
	if err.Error() != want {
		t.Errorf("err = %q\nwant  %q", err, want)
	}
	hint := cmdutil.HintFor(err)
	for _, w := range []string{"read:workspace:bitbucket", "bh auth login", "{uuid} or account ID"} {
		if !strings.Contains(hint, w) {
			t.Errorf("hint = %q, want to contain %q", hint, w)
		}
	}
	for _, r := range srv.Requests() {
		if r.Method == "PUT" {
			t.Error("PR must not be updated when reviewer resolution fails")
		}
	}
}

func TestEditAddReviewerByIDSkipsLookup(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t) // no members route: a lookup would fail the test
	pr := cmdtest.SamplePR()
	pr.Reviewers[0].AccountID = "557058:bbbbbbbb-0000-0000-0000-000000000000"
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)
	srv.Handle("PUT", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		AddReviewers: []string{
			"{Cara-UUID}",
			"5b10ac8d82e05b22cc7d4ef5",
			"DDDDDDDD-1111-2222-3333-444444444444",
			"557058:bbbbbbbb-0000-0000-0000-000000000000", // bob, already a reviewer
			"{bob-uuid}",                                  // bob again, by uuid
		},
	}
	if err := editRun(t.Context(), opts); err != nil {
		t.Fatalf("editRun: %v", err)
	}
	if got := string(srv.LastRequest(t, "PUT", "/repositories/myws/myrepo/pullrequests/123").Body); got != `{"reviewers":[`+
		`{"uuid":"{bob-uuid}"},{"uuid":"{cara-uuid}"},{"account_id":"5b10ac8d82e05b22cc7d4ef5"},`+
		`{"uuid":"{dddddddd-1111-2222-3333-444444444444}"}]}` {
		t.Errorf("body = %s", got)
	}
}

func TestDirectReviewerID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
		ok       bool
	}{
		{"{ABC}", "{abc}", true},
		{"a1b2c3d4-e5f6-7890-abcd-ef1234567890", "{a1b2c3d4-e5f6-7890-abcd-ef1234567890}", true},
		{"557058:a1b2c3d4-e5f6-7890-abcd-ef1234567890", "557058:a1b2c3d4-e5f6-7890-abcd-ef1234567890", true},
		{"5b10ac8d82e05b22cc7d4ef5", "5b10ac8d82e05b22cc7d4ef5", true},
		{"alice", "", false},
		{"Julius Smatavicius", "", false},
		{"bob-uuid", "", false},
		{"{}", "", false},
		{"5b10ac8d82e05b22cc7d4ef", "", false}, // 23 hex digits
	}
	for _, tt := range tests {
		got, ok := directReviewerID(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("directReviewerID(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestEditSummaryLine(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("PUT", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, out, errOut := cmdutil.TestIOStreams()
	opts := &EditOptions{
		IO:              ios,
		APIClient:       cmdtest.ClientFunc(srv),
		Git:             cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:        cmdtest.BaseRepoFunc(nil),
		Arg:             "123",
		Title:           "New",
		titleSet:        true,
		Body:            "",
		bodySet:         true,
		Base:            "dev",
		RemoveReviewers: []string{"bob"},
		Ready:           true,
	}
	if err := editRun(t.Context(), opts); err != nil {
		t.Fatalf("editRun: %v", err)
	}
	if out.String() != "https://bitbucket.org/myws/myrepo/pull-requests/123\n" {
		t.Errorf("stdout = %q, want only the URL", out.String())
	}
	if errOut.String() != "✓ Updated #123: title, body, base, reviewers, ready\n" {
		t.Errorf("stderr = %q", errOut.String())
	}
}
