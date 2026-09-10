package pr

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

func createdPRResponse() *api.PullRequest {
	return &api.PullRequest{
		ID:    123,
		Links: api.Links{HTML: api.Link{Href: "https://bitbucket.org/myws/myrepo/pull-requests/123"}},
	}
}

func originRemote() *git.ResolvedRemote {
	return &git.ResolvedRemote{Remote: git.Remote{Name: "origin"}, Repo: testRepo()}
}

// findRequest returns the last recorded request matching method+path suffix.
func findRequest(srv *apitest.Server, method, pathSuffix string) *apitest.Request {
	var found *apitest.Request
	for i := range srv.Requests {
		if srv.Requests[i].Method == method && strings.HasSuffix(srv.Requests[i].Path, pathSuffix) {
			found = &srv.Requests[i]
		}
	}
	return found
}

func TestCreateBodyShape(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/workspaces/myws/members", 200, valuesPage([]api.WorkspaceMember{
		{User: api.User{UUID: "{bob-uuid}", Nickname: "bob", DisplayName: "Bob"}},
	}))
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:                ios,
		ApiClient:         func() (*api.Client, error) { return srv.Client(), nil },
		Git:               gitFunc(gittest.New()),
		BaseRepo:          baseRepoFunc(originRemote()),
		Now:               fixedNow,
		Title:             "My title",
		Body:              "My body",
		Head:              "feature",
		Draft:             true,
		CloseSourceBranch: true,
		Reviewers:         []string{"bob"},
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if !strings.Contains(out.String(), "https://bitbucket.org/myws/myrepo/pull-requests/123") {
		t.Errorf("URL not printed: %q", out.String())
	}

	req := findRequest(srv, "POST", "/pullrequests")
	if req == nil {
		t.Fatal("no create request recorded")
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["title"] != "My title" {
		t.Errorf("title = %v", body["title"])
	}
	if body["draft"] != true {
		t.Errorf("draft = %v", body["draft"])
	}
	if body["close_source_branch"] != true {
		t.Errorf("close_source_branch = %v", body["close_source_branch"])
	}
	if _, ok := body["destination"]; ok {
		t.Errorf("destination should be omitted when base empty: %v", body["destination"])
	}
	reviewers, ok := body["reviewers"].([]any)
	if !ok || len(reviewers) != 1 {
		t.Fatalf("reviewers = %v", body["reviewers"])
	}
	if reviewers[0].(map[string]any)["uuid"] != "{bob-uuid}" {
		t.Errorf("reviewer uuid = %v", reviewers[0])
	}
}

func TestCreateAutoPushTTYYes(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	stub := gittest.New()
	// Branch not on remote -> push path.
	stub.Register("", &git.GitError{ExitCode: 2}, "ls-remote", "--exit-code", "--heads", "origin", "feature")

	ios, in, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	in.WriteString("y\n")

	opts := &CreateOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(stub),
		BaseRepo:  baseRepoFunc(originRemote()),
		Now:       fixedNow,
		Title:     "T",
		Head:      "feature",
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if len(stub.Interactive) != 1 || strings.Join(stub.Interactive[0], " ") != "push -u origin feature" {
		t.Errorf("push argv = %v", stub.Interactive)
	}
}

func TestCreateNonTTYNoPushErrors(t *testing.T) {
	srv := apitest.New(t)

	stub := gittest.New()
	stub.Register("", &git.GitError{ExitCode: 2}, "ls-remote", "--exit-code", "--heads", "origin", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(stub),
		BaseRepo:  baseRepoFunc(originRemote()),
		Now:       fixedNow,
		Title:     "T",
		Head:      "feature",
	}

	err := createRun(opts)
	if err == nil || !strings.Contains(err.Error(), "has not been pushed to origin; run with --push") {
		t.Fatalf("err = %v", err)
	}
	if len(stub.Interactive) != 0 {
		t.Errorf("should not push: %v", stub.Interactive)
	}
}

func TestCreateNonTTYWithPush(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	stub := gittest.New()
	stub.Register("", &git.GitError{ExitCode: 2}, "ls-remote", "--exit-code", "--heads", "origin", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(stub),
		BaseRepo:  baseRepoFunc(originRemote()),
		Now:       fixedNow,
		Title:     "T",
		Head:      "feature",
		Push:      true,
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if len(stub.Interactive) != 1 || strings.Join(stub.Interactive[0], " ") != "push -u origin feature" {
		t.Errorf("push argv = %v", stub.Interactive)
	}
}

func TestCreateUpstreamAheadWarns(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	stub := gittest.New()
	stub.Register("origin", nil, "config", "--get", "branch.feature.remote")
	stub.Register("refs/heads/feature", nil, "config", "--get", "branch.feature.merge")
	stub.Register("3", nil, "rev-list", "--count", "origin/feature..feature")

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(stub),
		BaseRepo:  baseRepoFunc(originRemote()),
		Now:       fixedNow,
		Title:     "T",
		Head:      "feature",
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !strings.Contains(errOut.String(), "! local branch is 3 commits ahead of origin/feature") {
		t.Errorf("warning missing: %q", errOut.String())
	}
	if len(stub.Interactive) != 0 {
		t.Errorf("should not push when upstream exists: %v", stub.Interactive)
	}
}

func TestCreateFillSingleCommit(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo", 200, api.Repository{
		FullName:   "myws/myrepo",
		MainBranch: &api.Branch{Name: "main"},
	})
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	stub := gittest.New()
	// git.Commits log for main..feature -> one commit.
	stub.Register("abc\x00Fix the bug\x00Detailed body\x1e", nil,
		"log", "--pretty=format:%H%x00%s%x00%b%x1e", "main..feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(stub),
		BaseRepo:  baseRepoFunc(originRemote()),
		Now:       fixedNow,
		Head:      "feature",
		Fill:      true,
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}

	req := findRequest(srv, "POST", "/pullrequests")
	var body map[string]any
	json.Unmarshal(req.Body, &body)
	if body["title"] != "Fix the bug" {
		t.Errorf("title = %v", body["title"])
	}
	if body["description"] != "Detailed body" {
		t.Errorf("description = %v", body["description"])
	}
}

func TestCreateWeb(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo", 200, api.Repository{
		FullName:   "myws/myrepo",
		MainBranch: &api.Branch{Name: "main"},
	})

	fb := &fakeBrowser{}
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Browser:   fb,
		Now:       fixedNow,
		Head:      "feature",
		Web:       true,
	}

	if err := createRun(opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	want := "https://bitbucket.org/myws/myrepo/pull-requests/new?source=feature&dest=main"
	if fb.url != want {
		t.Errorf("web url = %q, want %q", fb.url, want)
	}
}

func TestCreateNonTTYRequiresTitle(t *testing.T) {
	srv := apitest.New(t)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Now:       fixedNow,
		Head:      "feature",
	}

	err := createRun(opts)
	if err == nil || !strings.Contains(err.Error(), "--title (or --fill) is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *CreateOptions
	cmd := NewCmdCreate(f, func(o *CreateOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"-t", "Hi", "-b", "Body", "-B", "main", "-H", "feature", "--draft", "-r", "bob,cara", "--close-source-branch", "--push"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Title != "Hi" || captured.Body != "Body" || captured.Base != "main" || captured.Head != "feature" {
		t.Errorf("parsed = %+v", captured)
	}
	if !captured.Draft || !captured.CloseSourceBranch || !captured.Push {
		t.Errorf("bool flags = %+v", captured)
	}
	if len(captured.Reviewers) != 2 || captured.Reviewers[0] != "bob" {
		t.Errorf("reviewers = %v", captured.Reviewers)
	}
}

func TestCreateBodyFileConflict(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdCreate(f, func(o *CreateOptions) error { return nil })
	cmd.SetArgs([]string{"-b", "x", "-F", "file"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	err := cmd.Execute()
	var fe *cmdutil.FlagError
	if err == nil || !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}
