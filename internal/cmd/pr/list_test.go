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
	if !strings.Contains(got, "#123\tAdd feature\tfeature → main\tada\tOPEN\t2026-09-10T09:00:00Z\n") {
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
	if !strings.Contains(got, "feature → main") || !strings.Contains(got, "about 3 hours ago") {
		t.Errorf("TTY row should show branches and relative time: %q", got)
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

func TestListEmptyPiped(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

	ios, _, out, errOut := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if out.String() != "" {
		t.Errorf("stdout should stay empty when piped: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "No pull requests match your search in myws/myrepo") {
		t.Errorf("stderr note missing: %q", errOut.String())
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

func TestListJSONHygiene(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	// The list endpoint's shape: no reviewers/participants, and nested
	// repositories with only a few fields.
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, `{"values":[{
		"id": 7, "title": "x", "state": "OPEN",
		"source": {"branch": {"name": "ft"}, "repository": {"full_name": "myws/myrepo"}},
		"destination": {"branch": {"name": "dev"}, "repository": {"full_name": "myws/myrepo"}}
	}]}`)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.JSON = true
	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}

	got := out.String()
	for _, want := range []string{`"reviewers": []`, `"participants": []`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s:\n%s", want, got)
		}
	}
	for _, absent := range []string{`"slug"`, `"mainbranch"`, `"owner"`, `"workspace"`, `"parent"`, `"commit": null`} {
		if strings.Contains(got, absent) {
			t.Errorf("%s should not be printed:\n%s", absent, got)
		}
	}
	req := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests")
	if f := req.Query.Get("fields"); f != "+values.reviewers,+values.participants" {
		t.Errorf("fields = %q", f)
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
	if want := ` AND (title ~ "fix" OR description ~ "fix" OR source.branch.name ~ "fix")`; !strings.HasSuffix(q, want) {
		t.Errorf("q = %q, want suffix %q", q, want)
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

func TestListSearchPlainTextWrapped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, search, want string
	}{
		{"number", "4304", `(title ~ "4304" OR description ~ "4304" OR source.branch.name ~ "4304")`},
		{"words trimmed", "  fix login ", `(title ~ "fix login" OR description ~ "fix login" OR source.branch.name ~ "fix login")`},
		{"lowercase and/or are text", "this or that", `(title ~ "this or that" OR description ~ "this or that" OR source.branch.name ~ "this or that")`},
		{"quote and backslash escaped", `a"b\c`, `(title ~ "a\"b\\c" OR description ~ "a\"b\\c" OR source.branch.name ~ "a\"b\\c")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

			ios, _, _, _ := cmdutil.TestIOStreams()
			opts := newListOptions(srv)
			opts.IO = ios
			opts.Search = tt.search

			if err := listRun(t.Context(), opts); err != nil {
				t.Fatalf("listRun: %v", err)
			}
			if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); q != tt.want {
				t.Errorf("q = %s, want %s", q, tt.want)
			}
		})
	}
}

func TestListSearchPlainTextWithFilters(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.Base = "dev"
	opts.Search = "SUPER-1"

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	want := `destination.branch.name="dev" AND (title ~ "SUPER-1" OR description ~ "SUPER-1" OR source.branch.name ~ "SUPER-1")`
	if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); q != want {
		t.Errorf("q = %s, want %s", q, want)
	}
}

func TestListSearchBBQLPassedThrough(t *testing.T) {
	t.Parallel()
	for _, search := range []string{
		`title ~ "x"`,
		`id=4304`,
		`id != 1`,
		`id > 10`,
		`created_on < 2026-01-01`,
		`state = "OPEN" AND id>1`,
		`title ~ "a" OR title ~ "b"`,
	} {
		t.Run(search, func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

			ios, _, _, _ := cmdutil.TestIOStreams()
			opts := newListOptions(srv)
			opts.IO = ios
			opts.Search = search

			if err := listRun(t.Context(), opts); err != nil {
				t.Fatalf("listRun: %v", err)
			}
			if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); q != search {
				t.Errorf("q = %q, want %q", q, search)
			}
		})
	}
}

func TestListSearchBadRequestHint(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 400,
		`{"type":"error","error":{"message":"Invalid filter query expression: Syntax error in input at 'x' (type _ANON_14) line 1 col 1"}}`)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.Search = `title ~ x`

	err := listRun(t.Context(), opts)
	if !api.IsBadRequest(err) {
		t.Fatalf("err = %v, want a 400", err)
	}
	if !strings.Contains(err.Error(), "Invalid filter query expression") {
		t.Errorf("err = %q, want the API message", err)
	}
	hint := cmdutil.HintFor(err)
	if !strings.Contains(hint, "--search takes BBQL") ||
		!strings.Contains(hint, "https://developer.atlassian.com/cloud/bitbucket/rest/intro/#filtering") {
		t.Errorf("hint = %q", hint)
	}
}

func TestListBadRequestWithoutSearchNoHint(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 400, `{"type":"error","error":{"message":"bad"}}`)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios

	err := listRun(t.Context(), opts)
	if !api.IsBadRequest(err) {
		t.Fatalf("err = %v, want a 400", err)
	}
	if hint := cmdutil.HintFor(err); strings.Contains(hint, "--search") {
		t.Errorf("hint = %q, want no --search hint", hint)
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

	if _, _, err := cmdtest.RunCommand(t, cmd, "-B", "dev", "-H", "ft/x"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Base != "dev" || captured.Head != "ft/x" {
		t.Errorf("parsed = %+v", captured)
	}
}

func TestListBaseHeadFilters(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{}))

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.Base = "dev"
	opts.Head = `ft/"x"`
	opts.Search = `title ~ "a" OR title ~ "b"`

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	want := `destination.branch.name="dev" AND source.branch.name="ft/\"x\"" AND (title ~ "a" OR title ~ "b")`
	if q := cmdtest.RequireRequest(t, srv, "GET", "/pullrequests").Query.Get("q"); q != want {
		t.Errorf("q = %q, want %q", q, want)
	}
}

func TestListTruncationNotice(t *testing.T) {
	t.Parallel()
	pr1, pr2 := *cmdtest.SamplePR(), *cmdtest.SamplePR()
	pr2.ID = 124
	tests := []struct {
		name  string
		limit int
		page  map[string]any
		json  bool
		want  string
	}{
		{
			name:  "values left on page",
			limit: 1,
			page:  map[string]any{"values": []api.PullRequest{pr1, pr2}},
			want:  "Showing 1 result; use --limit to see more\n",
		},
		{
			name:  "next link with size",
			limit: 2,
			page:  map[string]any{"values": []api.PullRequest{pr1, pr2}, "next": "https://example.invalid/next", "size": 7},
			want:  "Showing 2 of 7 results; use --limit to see more\n",
		},
		{
			name:  "exactly limit",
			limit: 2,
			page:  map[string]any{"values": []api.PullRequest{pr1, pr2}, "size": 2},
		},
		{
			name:  "under limit",
			limit: 30,
			page:  map[string]any{"values": []api.PullRequest{pr1, pr2}},
		},
		{
			name:  "json stays clean",
			limit: 1,
			page:  map[string]any{"values": []api.PullRequest{pr1, pr2}, "size": 2},
			json:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, tt.page)

			ios, _, out, errOut := cmdutil.TestIOStreams()
			opts := newListOptions(srv)
			opts.IO = ios
			opts.Limit = tt.limit
			opts.JSON = tt.json

			if err := listRun(t.Context(), opts); err != nil {
				t.Fatalf("listRun: %v", err)
			}
			if got := errOut.String(); got != tt.want {
				t.Errorf("stderr = %q, want %q", got, tt.want)
			}
			if strings.Contains(out.String(), "--limit") {
				t.Errorf("notice leaked to stdout: %q", out.String())
			}
			if n := len(srv.Requests()); n != 1 {
				t.Errorf("requests = %d, want 1 (no extra page fetched)", n)
			}
		})
	}
}

func TestListJSONFieldsAndJQ(t *testing.T) {
	t.Parallel()
	second := cmdtest.SamplePR()
	second.ID, second.Title = 124, "Second"
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, cmdtest.ValuesPage([]api.PullRequest{*cmdtest.SamplePR(), *second}))

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newListOptions(srv)
	opts.IO = ios
	opts.JSON = true
	opts.JSONOutput = cmdutil.JSONOutput{Fields: []string{"id", "title"}, JQ: `.[] | "\(.id) \(.title) \(keys | length)"`}

	if err := listRun(t.Context(), opts); err != nil {
		t.Fatalf("listRun: %v", err)
	}
	if got, want := out.String(), "123 Add feature 2\n124 Second 2\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
