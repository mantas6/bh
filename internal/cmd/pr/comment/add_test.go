package comment

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
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

func newAddOpts(srv *apitest.Server, ios *cmdutil.IOStreams) *AddOptions {
	return &AddOptions{
		IO:        ios,
		ApiClient: clientFunc(srv),
		Git:       gitFunc(newGitStub()),
		BaseRepo:  baseRepoFunc(),
		Now:       nowFunc(),
		Arg:       "123",
		Side:      "new",
	}
}

func createBody(t *testing.T, srv *apitest.Server) map[string]any {
	t.Helper()
	req := findRequest(srv, "POST", "/pullrequests/123/comments")
	if req == nil {
		t.Fatal("no POST recorded")
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	return body
}

func TestAddPlainBody(t *testing.T) {
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newAddOpts(srv, ios)
	opts.Body = "Nice work"
	if err := addRun(opts); err != nil {
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
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newAddOpts(srv, ios)
	opts.Body = "inline"
	opts.Path = "main.go"
	opts.Line = 12
	opts.lineSet = true
	opts.Side = "new"
	if err := addRun(opts); err != nil {
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
	srv := apitest.New(t)
	handleCreate(srv)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newAddOpts(srv, ios)
	opts.Body = "inline old"
	opts.Path = "main.go"
	opts.Line = 7
	opts.lineSet = true
	opts.Side = "old"
	if err := addRun(opts); err != nil {
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
	srv := apitest.New(t)
	handleCreate(srv)

	ios, in, _, _ := cmdutil.TestIOStreams()
	in.WriteString("from stdin\n")
	// stdin is not a TTY (default false), so the body is read from it.
	opts := newAddOpts(srv, ios)
	if err := addRun(opts); err != nil {
		t.Fatalf("addRun: %v", err)
	}
	content, _ := createBody(t, srv)["content"].(map[string]any)
	if !strings.Contains(content["raw"].(string), "from stdin") {
		t.Errorf("content.raw = %v", content["raw"])
	}
}

func TestAddMissingBody(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true) // no stdin body available
	opts := &AddOptions{
		IO:       ios,
		BaseRepo: baseRepoFunc(),
		Arg:      "123",
		Side:     "new",
	}
	err := addRun(opts)
	if err == nil || !strings.Contains(err.Error(), "comment body is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestAddLineWithoutPathFlagError(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdAdd(f, func(*AddOptions) error { return nil })
	cmd.SetArgs([]string{"123", "-b", "x", "--line", "5"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	err := cmd.Execute()
	var fe *cmdutil.FlagError
	if err == nil || !strings.Contains(err.Error(), "--line requires --path") {
		t.Fatalf("err = %v", err)
	}
	if !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %T", err)
	}
}

func TestAddSideInvalidFlagError(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdAdd(f, func(*AddOptions) error { return nil })
	cmd.SetArgs([]string{"123", "-b", "x", "--path", "a.go", "--line", "1", "--side", "middle"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	var fe *cmdutil.FlagError
	if err := cmd.Execute(); err == nil || !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}

func TestAddFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *AddOptions
	cmd := NewCmdAdd(f, func(o *AddOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"77", "-b", "hi", "--path", "f.go", "--line", "3", "--side", "old"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "77" || captured.Body != "hi" || captured.Path != "f.go" ||
		captured.Line != 3 || captured.Side != "old" || !captured.lineSet {
		t.Errorf("parsed = %+v", captured)
	}
}
