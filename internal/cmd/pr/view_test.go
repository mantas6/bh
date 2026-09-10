package pr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func newViewOptions(srv *apitest.Server) *ViewOptions {
	return &ViewOptions{
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(nil),
		Now:       fixedNow,
		Arg:       "123",
	}
}

func TestViewText(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(srv)
	opts.IO = ios

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}

	got := out.String()
	checks := []string{
		"Add feature #123",
		"OPEN • ada wants to merge feature into main • 2 comments",
		"Reviewers: Bob (approved ✓), Cara (changes requested)",
		"This does things.",
		"View this pull request on Bitbucket: https://bitbucket.org/myws/myrepo/pull-requests/123",
	}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Errorf("output missing %q in:\n%s", c, got)
		}
	}
}

func TestViewForkPrefix(t *testing.T) {
	pr := samplePR()
	pr.Source.Repository = &api.Repository{FullName: "fork/myrepo"}
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(srv)
	opts.IO = ios

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if !strings.Contains(out.String(), "merge fork/myrepo:feature into main") {
		t.Errorf("fork prefix missing: %q", out.String())
	}
}

func TestViewNoDescription(t *testing.T) {
	pr := samplePR()
	pr.Description = ""
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(srv)
	opts.IO = ios

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if !strings.Contains(out.String(), "No description provided") {
		t.Errorf("missing no-description text: %q", out.String())
	}
}

func TestViewJSON(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(srv)
	opts.IO = ios
	opts.JSON = true

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	var pr api.PullRequest
	if err := json.Unmarshal(out.Bytes(), &pr); err != nil {
		t.Fatalf("json: %v", err)
	}
	if pr.ID != 123 {
		t.Errorf("id = %d", pr.ID)
	}
}

func TestViewWeb(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	fb := &fakeBrowser{}
	opts := newViewOptions(srv)
	opts.IO = ios
	opts.Web = true
	opts.Browser = fb

	if err := viewRun(opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if fb.url != "https://bitbucket.org/myws/myrepo/pull-requests/123" {
		t.Errorf("web url = %q", fb.url)
	}
}
