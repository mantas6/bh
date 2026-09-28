package pr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
)

func newListOptions(srv *apitest.Server) *ListOptions {
	return &ListOptions{
		APIClient: cmdtest.ClientFunc(srv),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Now:       cmdtest.FixedNow,
		State:     "open",
	}
}

func TestListNonTTY(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{*cmdtest.SamplePR()}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios

	if err := listRun(t.Context(), opts); err != nil {
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
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{*cmdtest.SamplePR()}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	opts := newListOptions(srv)
	opts.IO = ios

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Showing 1 open pull request in myws/myrepo") {
		t.Errorf("header missing: %q", got)
	}
}

func TestListEmptyTTY(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	opts := newListOptions(srv)
	opts.IO = ios

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if !strings.Contains(out.String(), "No pull requests match your search in myws/myrepo") {
		t.Errorf("empty message missing: %q", out.String())
	}
}

func TestListJSON(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{*cmdtest.SamplePR()}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.JSON = true

	if err := listRun(t.Context(), opts); err != nil {
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
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{UUID: "{me-uuid}", Nickname: "me"})
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{*cmdtest.SamplePR()}))

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.State = "all"
	opts.Author = "@me"
	opts.Search = "fix"

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	listReq := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests")
	states := listReq.Query["state"]
	if len(states) != 4 {
		t.Errorf("state params = %v, want 4", states)
	}
	q := listReq.Query.Get("q")
	if !strings.Contains(q, `author.uuid="{me-uuid}"`) {
		t.Errorf("q missing author: %q", q)
	}
	if !strings.HasSuffix(q, " AND (fix)") {
		t.Errorf("q missing parenthesised search: %q", q)
	}
}

func TestListSearchAloneNotParenthesised(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.Search = `title ~ "x"`

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); q != `title ~ "x"` {
		t.Errorf("q = %q", q)
	}
}

func TestListAuthorEscaped(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.Author = `ada" OR author.nickname="bob`

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	want := `author.nickname="ada\" OR author.nickname=\"bob"`
	if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); q != want {
		t.Errorf("q = %q, want %q", q, want)
	}
}

func TestListJSONEmptyIsArray(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.JSON = true

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "[]" {
		t.Errorf("output = %q, want []", got)
	}
}

func TestListInvalidLimit(t *testing.T) {
	t.Parallel()
	for _, limit := range []string{"0", "-1"} {
		t.Run(limit, func(t *testing.T) {
			t.Parallel()
			f := cmdtest.NewFactory()
			cmd := NewCmdList(f, func(o *ListOptions) error {
				t.Error("runF should not be called")
				return nil
			})
			_, _, err := cmdtest.RunCommand(t, cmd, "-L", limit)
			cmdtest.AssertFlagError(t, err, "invalid value for --limit")
		})
	}
}

func TestListWeb(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	ios, _, _, _ := cmdutil.TestIOStreams()
	fb := &cmdtest.FakeBrowser{}
	opts := newListOptions(srv)
	opts.IO = ios
	opts.Web = true
	opts.Browser = fb

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if fb.URL() != "https://bitbucket.org/myws/myrepo/pull-requests/" {
		t.Errorf("web url = %q", fb.URL())
	}
}

func TestListInvalidState(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &ListOptions{
		IO:       ios,
		BaseRepo: cmdtest.BaseRepoFunc(nil),
		State:    "bogus",
	}
	err := listRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("err = %v", err)
	}
}

func TestListFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *ListOptions
	cmd := NewCmdList(f, func(o *ListOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "--state", "merged", "-L", "5", "--author", "@me", "-s", "wip", "--json"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured == nil {
		t.Fatal("runF not called")
	}
	if captured.State != "merged" || captured.Limit != 5 || captured.Author != "@me" || captured.Search != "wip" || !captured.JSON {
		t.Errorf("parsed = %+v", captured)
	}
}
