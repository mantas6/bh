package pr

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

const sampleDiff = `diff --git a/main.go b/main.go
index 111..222 100644
--- a/main.go
+++ b/main.go
@@ -1,2 +1,2 @@
 package main
--- removed comment
+// added comment
`

const sampleDiffStat = `{"values":[
	{"status":"modified","lines_added":3,"lines_removed":1,"old":{"path":"main.go"},"new":{"path":"main.go"}},
	{"status":"added","lines_added":10,"lines_removed":0,"old":null,"new":{"path":"new.go"}},
	{"status":"removed","lines_added":0,"lines_removed":7,"old":{"path":"gone.go"},"new":null},
	{"status":"renamed","lines_added":0,"lines_removed":0,"old":{"path":"old/name.go"},"new":{"path":"new/name.go"}}
]}`

func newDiffOptions(t *testing.T, srv *apitest.Server, ios *cmdutil.IOStreams) *DiffOptions {
	return &DiffOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
	}
}

func handleDiff(srv *apitest.Server) {
	srv.HandleFunc(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/diff", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/repositories/myws/myrepo/diff/feature..main", http.StatusFound)
	})
	srv.HandleFunc(http.MethodGet, "/repositories/myws/myrepo/diff/feature..main", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(sampleDiff))
	})
}

func TestDiffPlain(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleDiff(srv)
	ios, _, out, _ := cmdutil.TestIOStreams()

	if err := diffRun(t.Context(), newDiffOptions(t, srv, ios)); err != nil {
		t.Fatalf("diffRun: %v", err)
	}
	if out.String() != sampleDiff {
		t.Errorf("stdout = %q, want the diff verbatim", out.String())
	}
	// A number argument must not fetch the pull request itself.
	if req := cmdtest.FindRequest(srv, http.MethodGet, "/pullrequests/123"); req != nil {
		t.Errorf("unexpected pull request fetch")
	}
}

func TestDiffColor(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleDiff(srv)
	ios, _, out, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)

	if err := diffRun(t.Context(), newDiffOptions(t, srv, ios)); err != nil {
		t.Fatalf("diffRun: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"\x1b[1mdiff --git a/main.go b/main.go\x1b[0m\n",
		"\x1b[1m--- a/main.go\x1b[0m\n",
		"\x1b[1m+++ b/main.go\x1b[0m\n",
		"\x1b[36m@@ -1,2 +1,2 @@\x1b[0m\n",
		" package main\n",
		// Inside a hunk, "---" is a removed line, not a header.
		"\x1b[31m--- removed comment\x1b[0m\n",
		"\x1b[32m+// added comment\x1b[0m\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("colored diff missing %q in:\n%q", want, got)
		}
	}
}

func TestDiffNameOnly(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/diffstat", 200, sampleDiffStat)
	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newDiffOptions(t, srv, ios)
	opts.NameOnly = true

	if err := diffRun(t.Context(), opts); err != nil {
		t.Fatalf("diffRun: %v", err)
	}
	want := "main.go\nnew.go\ngone.go\nnew/name.go\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

func TestDiffStat(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/diffstat", 200, sampleDiffStat)
	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newDiffOptions(t, srv, ios)
	opts.Stat = true

	if err := diffRun(t.Context(), opts); err != nil {
		t.Fatalf("diffRun: %v", err)
	}
	want := "main.go\tmodified\t+3\t-1\n" +
		"new.go\tadded\t+10\t-0\n" +
		"gone.go\tremoved\t+0\t-7\n" +
		"old/name.go → new/name.go\trenamed\t+0\t-0\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

func TestDiffStatTTYSummary(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/diffstat", 200, sampleDiffStat)
	ios, _, out, _ := cmdutil.TestIOStreams()
	ios.Getenv = func(k string) string {
		if k == "NO_COLOR" {
			return "1"
		}
		return ""
	}
	ios.SetStdoutTTY(true)
	opts := newDiffOptions(t, srv, ios)
	opts.Stat = true

	if err := diffRun(t.Context(), opts); err != nil {
		t.Fatalf("diffRun: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "main.go                    modified  +3   -1\n") {
		t.Errorf("columns not aligned:\n%s", got)
	}
	if !strings.HasSuffix(got, "4 files changed, 13 insertions(+), 8 deletions(-)\n") {
		t.Errorf("missing summary line:\n%s", got)
	}
}

func TestDiffWeb(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	ios, _, out, _ := cmdutil.TestIOStreams()
	fb := &cmdtest.FakeBrowser{}
	opts := newDiffOptions(t, srv, ios)
	opts.Web = true
	opts.Browser = fb

	if err := diffRun(t.Context(), opts); err != nil {
		t.Fatalf("diffRun: %v", err)
	}
	if fb.URL() != "https://bitbucket.org/myws/myrepo/pull-requests/123/diff" {
		t.Errorf("web url = %q", fb.URL())
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
}

func TestDiffCurrentBranch(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests", 200,
		cmdtest.ValuesPage([]any{cmdtest.SamplePR()}))
	srv.Handle(http.MethodGet, "/repositories/myws/myrepo/pullrequests/123/diffstat", 200, sampleDiffStat)

	stub := gittest.New(t)
	stub.Register("feature", nil, "symbolic-ref", "--quiet", "--short", "HEAD")
	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newDiffOptions(t, srv, ios)
	opts.Git = cmdtest.GitFunc(stub)
	opts.Arg = ""
	opts.NameOnly = true

	if err := diffRun(t.Context(), opts); err != nil {
		t.Fatalf("diffRun: %v", err)
	}
	if !strings.HasPrefix(out.String(), "main.go\n") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestDiffFlagsExclusive(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"--name-only", "--stat"},
		{"--stat", "--web"},
		{"--name-only", "--web"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			cmd := NewCmdDiff(cmdtest.NewFactory(), func(*DiffOptions) error {
				t.Fatal("run should not be called")
				return nil
			})
			_, _, err := cmdtest.RunCommand(t, cmd, args...)
			cmdtest.AssertFlagError(t, err, "specify only one of --name-only, --stat or --web")
		})
	}
}

func TestDiffArgs(t *testing.T) {
	t.Parallel()
	var got *DiffOptions
	cmd := NewCmdDiff(cmdtest.NewFactory(), func(o *DiffOptions) error {
		got = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "42", "--stat"); err != nil {
		t.Fatal(err)
	}
	if got.Arg != "42" || !got.Stat || got.NameOnly || got.Web {
		t.Errorf("opts = %+v", got)
	}
	_, _, err := cmdtest.RunCommand(t, NewCmdDiff(cmdtest.NewFactory(), nil), "1", "2")
	cmdtest.AssertFlagError(t, err, "")
}
