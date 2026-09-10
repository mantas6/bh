package pr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
)

func newListOptions(srv *apitest.Server) *ListOptions {
	return &ListOptions{
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		BaseRepo:  baseRepoFunc(nil),
		Now:       fixedNow,
		State:     "open",
	}
}

func TestListNonTTY(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, valuesPage([]api.PullRequest{*samplePR()}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "#123\tAdd feature\tfeature\tada\tOPEN\tabout 3 hours ago") {
		t.Errorf("row missing/mismatched: %q", got)
	}
	if strings.Contains(got, "Showing") {
		t.Errorf("non-TTY should not print header: %q", got)
	}
}

func TestListTTYHeader(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, valuesPage([]api.PullRequest{*samplePR()}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	opts := newListOptions(srv)
	opts.IO = ios

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Showing 1 open pull request in myws/myrepo") {
		t.Errorf("header missing: %q", got)
	}
}

func TestListEmptyTTY(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, valuesPage([]api.PullRequest{}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	opts := newListOptions(srv)
	opts.IO = ios

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "No pull requests match your search in myws/myrepo") {
		t.Errorf("empty message missing: %q", out.String())
	}
}

func TestListJSON(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, valuesPage([]api.PullRequest{*samplePR()}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.JSON = true

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	var prs []api.PullRequest
	if err := json.Unmarshal(out.Bytes(), &prs); err != nil {
		t.Fatalf("json: %v (%s)", err, out.String())
	}
	if len(prs) != 1 || prs[0].ID != 123 {
		t.Errorf("prs = %+v", prs)
	}
}

func TestListStateAllAuthorMe(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{UUID: "{me-uuid}", Nickname: "me"})
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, valuesPage([]api.PullRequest{*samplePR()}))

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.State = "all"
	opts.Author = "@me"
	opts.Search = "fix"

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	var listReq *apitest.Request
	for i := range srv.Requests {
		if strings.HasSuffix(srv.Requests[i].Path, "/pullrequests") {
			listReq = &srv.Requests[i]
		}
	}
	if listReq == nil {
		t.Fatal("no list request recorded")
	}
	states := listReq.Query["state"]
	if len(states) != 4 {
		t.Errorf("state params = %v, want 4", states)
	}
	q := listReq.Query.Get("q")
	if !strings.Contains(q, `author.uuid="{me-uuid}"`) {
		t.Errorf("q missing author: %q", q)
	}
	if !strings.Contains(q, " AND fix") {
		t.Errorf("q missing search combine: %q", q)
	}
}

func TestListWeb(t *testing.T) {
	srv := apitest.New(t)

	ios, _, _, _ := cmdutil.TestIOStreams()
	fb := &fakeBrowser{}
	opts := newListOptions(srv)
	opts.IO = ios
	opts.Web = true
	opts.Browser = fb

	if err := listRun(opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if fb.url != "https://bitbucket.org/myws/myrepo/pull-requests/" {
		t.Errorf("web url = %q", fb.url)
	}
}

func TestListInvalidState(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &ListOptions{
		IO:       ios,
		BaseRepo: baseRepoFunc(nil),
		State:    "bogus",
	}
	err := listRun(opts)
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("err = %v", err)
	}
}

func TestListFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *ListOptions
	cmd := NewCmdList(f, func(o *ListOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"--state", "merged", "-L", "5", "--author", "@me", "-s", "wip", "--json"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured == nil {
		t.Fatal("runF not called")
	}
	if captured.State != "merged" || captured.Limit != 5 || captured.Author != "@me" || captured.Search != "wip" || !captured.JSON {
		t.Errorf("parsed = %+v", captured)
	}
}
