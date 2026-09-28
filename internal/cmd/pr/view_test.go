package pr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func newViewOptions(t *testing.T, srv *apitest.Server) *ViewOptions {
	return &ViewOptions{
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
	}
}

func TestViewText(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}

	got := out.String()
	checks := []string{
		"Add feature #123",
		"OPEN • ada wants to merge feature into main • 2 comments",
		"Reviewers: bob (approved ✓), cara (changes requested)",
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
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Source.Repository = &api.Repository{FullName: "fork/myrepo"}
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if !strings.Contains(out.String(), "merge fork/myrepo:feature into main") {
		t.Errorf("fork prefix missing: %q", out.String())
	}
}

func TestViewNoDescription(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Description = ""
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if !strings.Contains(out.String(), "No description provided") {
		t.Errorf("missing no-description text: %q", out.String())
	}
}

func TestViewJSON(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios
	opts.JSON = true

	if err := viewRun(t.Context(), opts); err != nil {
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
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	fb := &cmdtest.FakeBrowser{}
	opts := newViewOptions(t, srv)
	opts.IO = ios
	opts.Web = true
	opts.Browser = fb

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if fb.URL() != "https://bitbucket.org/myws/myrepo/pull-requests/123" {
		t.Errorf("web url = %q", fb.URL())
	}
}
