package comment

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
)

// createdComment is the response returned by the CreateComment endpoint.
func createdComment() *api.Comment {
	return &api.Comment{
		ID:    99,
		Links: api.Links{HTML: api.Link{Href: "https://bitbucket.org/myws/myrepo/pull-requests/123/_/diff#comment-99"}},
	}
}

func handleCreate(srv *apitest.Server) {
	handlePR(srv)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/comments", 201, createdComment())
}

func newAddOpts(t *testing.T, srv *apitest.Server, ios *cmdutil.IOStreams) *AddOptions {
	return &AddOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(newGitStub(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
		Side:      "new",
	}
}

func createBody(t *testing.T, srv *apitest.Server) map[string]any {
	t.Helper()
	var body map[string]any
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests/123/comments").DecodeJSON(t, &body)
	return body
}

func TestAddPlainBody(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newAddOpts(t, srv, ios)
	opts.Body = "Nice work"
	if err := addRun(t.Context(), opts); err != nil {
		t.Fatalf("addRun: %v", err)
	}

	body := createBody(t, srv)
	content, _ := body["content"].(map[string]any)
	if content["raw"] != "Nice work" {
		t.Errorf("content.raw = %v", content["raw"])
	}
	if _, ok := body["inline"]; ok {
		t.Errorf("inline should be absent: %v", body["inline"])
	}
	if !strings.Contains(out.String(), "comment-99") {
		t.Errorf("expected URL, got %q", out.String())
	}
}

func TestAddInlineNewSide(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newAddOpts(t, srv, ios)
	opts.Body = "inline"
	opts.Path = "main.go"
	opts.Line = 12
	opts.lineSet = true
	opts.Side = "new"
	if err := addRun(t.Context(), opts); err != nil {
		t.Fatalf("addRun: %v", err)
	}

	inline, _ := createBody(t, srv)["inline"].(map[string]any)
	if inline == nil || inline["path"] != "main.go" {
		t.Fatalf("inline = %v", inline)
	}
	if inline["to"] != float64(12) {
		t.Errorf("inline.to = %v, want 12", inline["to"])
	}
	if _, ok := inline["from"]; ok {
		t.Errorf("inline.from should be absent: %v", inline["from"])
	}
}

func TestAddInlineOldSide(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newAddOpts(t, srv, ios)
	opts.Body = "inline old"
	opts.Path = "main.go"
	opts.Line = 7
	opts.lineSet = true
	opts.Side = "old"
	if err := addRun(t.Context(), opts); err != nil {
		t.Fatalf("addRun: %v", err)
	}

	inline, _ := createBody(t, srv)["inline"].(map[string]any)
	if inline == nil || inline["from"] != float64(7) {
		t.Fatalf("inline.from = %v, want 7", inline)
	}
	if _, ok := inline["to"]; ok {
		t.Errorf("inline.to should be absent: %v", inline["to"])
	}
}

func TestAddBodyFromStdin(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	handleCreate(srv)

	ios, in, _, _ := cmdutil.TestIOStreams()
	in.WriteString("from stdin\n")
	// stdin is not a TTY (default false), so the body is read from it.
	opts := newAddOpts(t, srv, ios)
	if err := addRun(t.Context(), opts); err != nil {
		t.Fatalf("addRun: %v", err)
	}
	content, _ := createBody(t, srv)["content"].(map[string]any)
	if !strings.Contains(content["raw"].(string), "from stdin") {
		t.Errorf("content.raw = %v", content["raw"])
	}
}

func TestAddMissingBody(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true) // no stdin body available
	opts := &AddOptions{
		IO:       ios,
		BaseRepo: cmdtest.BaseRepoFunc(nil),
		Arg:      "123",
		Side:     "new",
	}
	err := addRun(t.Context(), opts)
	cmdtest.AssertFlagError(t, err, "comment body is required")
}

func TestAddInvalidLineFlagError(t *testing.T) {
	t.Parallel()
	for _, line := range []string{"0", "-3"} {
		t.Run(line, func(t *testing.T) {
			t.Parallel()
			f := cmdtest.NewFactory()
			cmd := NewCmdAdd(f, func(*AddOptions) error {
				t.Error("runF should not be called")
				return nil
			})
			_, _, err := cmdtest.RunCommand(t, cmd, "123", "-b", "x", "--path", "main.go", "--line", line)
			cmdtest.AssertFlagError(t, err, "invalid value for --line")
		})
	}
}

func TestAddLineWithoutPathFlagError(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdAdd(f, func(*AddOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "123", "-b", "x", "--line", "5")
	cmdtest.AssertFlagError(t, err, "--line requires --path")
}

func TestAddSideInvalidFlagError(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdAdd(f, func(*AddOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "123", "-b", "x", "--path", "a.go", "--line", "1", "--side", "middle")
	cmdtest.AssertFlagError(t, err, "")
}

func TestAddFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *AddOptions
	cmd := NewCmdAdd(f, func(o *AddOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "77", "-b", "hi", "--path", "f.go", "--line", "3", "--side", "old"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "77" || captured.Body != "hi" || captured.Path != "f.go" ||
		captured.Line != 3 || captured.Side != "old" || !captured.lineSet {
		t.Errorf("parsed = %+v", captured)
	}
}
