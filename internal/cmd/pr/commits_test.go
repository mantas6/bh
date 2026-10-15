package pr

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

const sampleCommits = `{"values":[
	{"hash":"aaaaaaaaaaaa1111111111111111111111111111","date":"2026-09-10T11:00:00+00:00",
	 "message":"Fix the thing\n\nBecause it was broken.","author":{"raw":"Ada Lovelace <ada@example.com>","user":{"nickname":"ada","display_name":"Ada Lovelace"}}},
	{"hash":"bbbbbbbbbbbb2222222222222222222222222222","date":"2026-09-08T12:00:00+00:00",
	 "message":"Initial work\n","author":{"raw":"Bob Builder <bob@example.com>"}}
]}`

func newCommitsOptions(t *testing.T, srv *apitest.Server, ios *cmdutil.IOStreams) *CommitsOptions {
	return &CommitsOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Now:       cmdtest.FixedNow,
		Arg:       "123",
	}
}

func TestCommitsPlain(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/commits", 200, sampleCommits)
	ios, _, out, _ := cmdutil.TestIOStreams()

	if err := commitsRun(t.Context(), newCommitsOptions(t, srv, ios)); err != nil {
		t.Fatalf("commitsRun: %v", err)
	}
	want := "aaaaaaaaaaaa\tada\t2026-09-10T11:00:00Z\tFix the thing\n" +
		"bbbbbbbbbbbb\tBob Builder\t2026-09-08T12:00:00Z\tInitial work\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if req := cmdtest.FindRequest(srv, http.MethodGet, "/pullrequests/123"); req != nil {
		t.Errorf("unexpected pull request fetch")
	}
}

func TestCommitsTTY(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/commits", 200, sampleCommits)
	ios, _, out, _ := cmdutil.TestIOStreams()
	ios.Getenv = func(k string) string {
		if k == "NO_COLOR" {
			return "1"
		}
		return ""
	}
	ios.SetStdoutTTY(true)

	if err := commitsRun(t.Context(), newCommitsOptions(t, srv, ios)); err != nil {
		t.Fatalf("commitsRun: %v", err)
	}
	want := "aaaaaaaaaaaa  ada          about an hour ago  Fix the thing\n" +
		"bbbbbbbbbbbb  Bob Builder  2 days ago         Initial work\n"
	if out.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", out.String(), want)
	}
}

func TestCommitsJSON(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/commits", 200, sampleCommits)
	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newCommitsOptions(t, srv, ios)
	opts.JSON = true

	if err := commitsRun(t.Context(), opts); err != nil {
		t.Fatalf("commitsRun: %v", err)
	}
	var commits []api.PRCommit
	if err := json.Unmarshal(out.Bytes(), &commits); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(commits) != 2 || commits[0].Hash != "aaaaaaaaaaaa1111111111111111111111111111" || commits[1].Author.Raw != "Bob Builder <bob@example.com>" {
		t.Errorf("commits = %+v", commits)
	}
}

func TestCommitsEmpty(t *testing.T) {
	t.Parallel()
	for _, tty := range []bool{false, true} {
		srv := apitest.New(t)
		srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/commits", 200, `{"values":[]}`)
		ios, _, out, _ := cmdutil.TestIOStreams()
		ios.SetStdoutTTY(tty)
		opts := newCommitsOptions(t, srv, ios)

		if err := commitsRun(t.Context(), opts); err != nil {
			t.Fatalf("commitsRun: %v", err)
		}
		want := ""
		if tty {
			want = "No commits in pull request #123\n"
		}
		if out.String() != want {
			t.Errorf("tty=%v: stdout = %q, want %q", tty, out.String(), want)
		}

		opts.JSON = true
		out.Reset()
		if err := commitsRun(t.Context(), opts); err != nil {
			t.Fatalf("commitsRun: %v", err)
		}
		if strings.TrimSpace(out.String()) != "[]" {
			t.Errorf("tty=%v: json = %q, want []", tty, out.String())
		}
	}
}

func TestCommitAuthor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   api.CommitAuthor
		want string
	}{
		{api.CommitAuthor{Raw: "Ada <ada@example.com>", User: &api.User{Nickname: "ada"}}, "ada"},
		{api.CommitAuthor{Raw: "Ada Lovelace <ada@example.com>"}, "Ada Lovelace"},
		{api.CommitAuthor{Raw: "\"Lovelace, Ada\" <ada@example.com>"}, "Lovelace, Ada"},
		{api.CommitAuthor{Raw: "<ada@example.com>"}, "<ada@example.com>"},
		{api.CommitAuthor{Raw: "ada"}, "ada"},
	}
	for _, tt := range tests {
		if got := commitAuthor(tt.in); got != tt.want {
			t.Errorf("commitAuthor(%+v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestCommitsArgs(t *testing.T) {
	t.Parallel()
	var got *CommitsOptions
	cmd := NewCmdCommits(cmdtest.NewFactory(), func(o *CommitsOptions) error {
		got = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "#7", "--json"); err != nil {
		t.Fatal(err)
	}
	if got.Arg != "#7" || !got.JSON {
		t.Errorf("opts = %+v", got)
	}
}

func TestCommitsJSONFields(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/commits", 200, sampleCommits)
	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newCommitsOptions(t, srv, ios)
	opts.JSON = true
	opts.JSONOutput = cmdutil.JSONOutput{Fields: []string{"hash", "author.raw"}, JQ: `.[1]`}

	if err := commitsRun(t.Context(), opts); err != nil {
		t.Fatalf("commitsRun: %v", err)
	}
	want := `{"author":{"raw":"Bob Builder <bob@example.com>"},"hash":"bbbbbbbbbbbb2222222222222222222222222222"}` + "\n"
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}
