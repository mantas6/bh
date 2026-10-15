package pr

import (
	"slices"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
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

// newStub returns a strict git stub with no bh.base configured.
func newStub(t *testing.T) *gittest.Stub {
	return gittest.New(t).Register("", gittest.Exit(1), "config", "--get", "bh.base")
}

// noDefaultReviewers serves an empty effective default reviewers list.
func noDefaultReviewers(srv *apitest.Server) {
	srv.Handle("GET", "/repositories/myws/myrepo/effective-default-reviewers", 200, cmdtest.ValuesPage([]any{}))
}

// untrackedStub returns a strict git stub for a head branch with no upstream
// tracking configuration.
func untrackedStub(t *testing.T, head string) *gittest.Stub {
	return newStub(t).
		Register("", gittest.Exit(1), "config", "--get", "branch."+head+".remote").
		Register("", gittest.Exit(1), "config", "--get", "branch."+head+".merge")
}

// pushedStub is untrackedStub for a head branch that already exists on origin
// and has no unpushed commits.
func pushedStub(t *testing.T, head string) *gittest.Stub {
	return untrackedStub(t, head).
		Register("", nil, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/"+head).
		Register("0", nil, "rev-list", "--count", "origin/"+head+".."+head)
}

func TestCreateBodyShape(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/workspaces/myws/members", 200, cmdtest.Members("bob"))
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:                ios,
		APIClient:         cmdtest.ClientFunc(srv),
		Git:               cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:          cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:             "My title",
		Body:              "My body",
		Head:              "feature",
		Draft:             true,
		CloseSourceBranch: true,
		Reviewers:         []string{"bob"},
	}

	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if !strings.Contains(out.String(), "https://bitbucket.org/myws/myrepo/pull-requests/123") {
		t.Errorf("URL not printed: %q", out.String())
	}

	var body map[string]any
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
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

func TestCreateReviewerForbiddenExplainsScope(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/workspaces/myws/members", 403, scopeErrorBody)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "My title",
		Head:      "feature",
		Reviewers: []string{"{bob-uuid}", "julius"},
	}
	err := createRun(t.Context(), opts)
	if err == nil || !strings.HasPrefix(err.Error(), `resolving reviewer "julius": listing workspace members: `) ||
		!strings.Contains(err.Error(), "required scopes: read:workspace:bitbucket") {
		t.Fatalf("err = %v", err)
	}
	if hint := cmdutil.HintFor(err); !strings.Contains(hint, "read:workspace:bitbucket token scope") {
		t.Errorf("hint = %q", hint)
	}
	for _, r := range srv.Requests() {
		if r.Method == "POST" {
			t.Error("PR must not be created when reviewer resolution fails")
		}
	}
}

func TestCreateReviewerByUUIDSkipsLookup(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t) // no members route: a lookup would fail the test
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "My title",
		Head:      "feature",
		Reviewers: []string{"{bob-uuid}", "557058:a1b2c3d4-e5f6-7890-abcd-ef1234567890", "{BOB-uuid}"},
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	var body struct {
		Reviewers []map[string]string `json:"reviewers"`
	}
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
	if len(body.Reviewers) != 2 || body.Reviewers[0]["uuid"] != "{bob-uuid}" ||
		body.Reviewers[1]["account_id"] != "557058:a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("reviewers = %v", body.Reviewers)
	}
}

func TestCreateAutoPushTTYYes(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := untrackedStub(t, "feature")
	// Branch not on remote -> push path.
	stub.Register("", gittest.Exit(2), "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, in, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true) // keep git's own push output
	in.WriteString("y\n")

	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
	}

	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if !slices.Equal(stub.InteractiveStrings(), []string{"push -u --end-of-options origin feature"}) {
		t.Errorf("push argv = %v", stub.InteractiveStrings())
	}
}

func TestCreateNonTTYNoPushErrors(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	stub := untrackedStub(t, "feature")
	stub.Register("", gittest.Exit(2), "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
	}

	err := createRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "has not been pushed to origin; run with --push") {
		t.Fatalf("err = %v", err)
	}
	if len(stub.Interactive()) != 0 {
		t.Errorf("should not push: %v", stub.InteractiveStrings())
	}
}

func TestCreateNonTTYWithPush(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := untrackedStub(t, "feature")
	stub.Expect("push", "--quiet", "-u", "--end-of-options", "origin", "feature")

	ios, _, out, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
		Push:      true,
	}

	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if len(stub.Interactive()) != 0 {
		t.Errorf("push must be quiet when stdout is not a terminal: %v", stub.InteractiveStrings())
	}
	if out.String() != "https://bitbucket.org/myws/myrepo/pull-requests/123\n" {
		t.Errorf("stdout = %q, want only the URL", out.String())
	}
	if !strings.Contains(errOut.String(), "✓ Pushed feature to origin\n") {
		t.Errorf("stderr = %q, want a push confirmation", errOut.String())
	}
}

func TestCreateUpstreamAheadWarns(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := newStub(t)
	stub.Register("origin", nil, "config", "--get", "branch.feature.remote")
	stub.Register("refs/heads/feature", nil, "config", "--get", "branch.feature.merge")
	stub.Register("3", nil, "rev-list", "--count", "origin/feature..feature")

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
	}

	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !strings.Contains(errOut.String(), "! local branch is 3 commits ahead of origin/feature") {
		t.Errorf("warning missing: %q", errOut.String())
	}
	if len(stub.Interactive()) != 0 {
		t.Errorf("should not push when upstream exists: %v", stub.InteractiveStrings())
	}
}

func TestCreateFillSingleCommit(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo", 200, api.Repository{
		FullName:   "myws/myrepo",
		MainBranch: &api.Branch{Name: "main"},
	})
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := pushedStub(t, "feature")
	// git.Commits log for main..feature -> one commit.
	stub.Register("abc\x00Fix the bug\x00Detailed body\x1e", nil,
		"log", "--pretty=format:%H%x00%s%x00%b%x1e", "--end-of-options", "origin/main..feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Head:      "feature",
		Fill:      true,
	}

	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}

	var body map[string]any
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
	if body["title"] != "Fix the bug" {
		t.Errorf("title = %v", body["title"])
	}
	if body["description"] != "Detailed body" {
		t.Errorf("description = %v", body["description"])
	}
}

func TestCreateWeb(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo", 200, api.Repository{
		FullName:   "myws/myrepo",
		MainBranch: &api.Branch{Name: "main"},
	})

	fb := &cmdtest.FakeBrowser{}
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Browser:   fb,
		Head:      "feature",
		Web:       true,
	}

	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	want := "https://bitbucket.org/myws/myrepo/pull-requests/new?source=feature&dest=main"
	if fb.URL() != want {
		t.Errorf("web url = %q, want %q", fb.URL(), want)
	}
}

func TestCreateNonTTYRequiresTitle(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Head:      "feature",
	}

	err := createRun(t.Context(), opts)
	cmdtest.AssertFlagError(t, err, "title is required when not running interactively; use --title or --fill")
}

func TestCreateTTYEmptyTitleErrors(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	ios, in, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	in.WriteString("\n")
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Head:      "feature",
	}

	err := createRun(t.Context(), opts)
	if err == nil || err.Error() != "title is required" {
		t.Fatalf("err = %v", err)
	}
}

// upstreamBaseRemote is a base repo resolved from an "upstream" remote, as in
// a fork workflow where the user pushes to "origin" (their fork).
func upstreamBaseRemote() *git.ResolvedRemote {
	return &git.ResolvedRemote{Remote: git.Remote{Name: "upstream"}, Repo: cmdtest.TestRepo()}
}

func TestCreatePushTargetsBranchUpstreamRemote(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := newStub(t)
	stub.Register("fork", nil, "config", "--get", "branch.feature.remote")
	stub.Register("refs/heads/feature", nil, "config", "--get", "branch.feature.merge")
	stub.Expect("push", "-u", "--end-of-options", "fork", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(upstreamBaseRemote()),
		Title:     "T",
		Head:      "feature",
		Push:      true,
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !slices.Equal(stub.InteractiveStrings(), []string{"push -u --end-of-options fork feature"}) {
		t.Errorf("push argv = %v, want push to fork", stub.InteractiveStrings())
	}
}

func TestCreatePushUntrackedDefaultsToOrigin(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := newStub(t)
	stub.Register("", gittest.Exit(1), "config", "--get", "branch.feature.remote")
	stub.Register("", gittest.Exit(1), "config", "--get", "branch.feature.merge")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(upstreamBaseRemote()),
		Title:     "T",
		Head:      "feature",
		Push:      true,
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !slices.Equal(stub.InteractiveStrings(), []string{"push -u --end-of-options origin feature"}) {
		t.Errorf("push argv = %v, want push to origin (not upstream)", stub.InteractiveStrings())
	}
}

func TestCreatePushWhenRemoteBranchExistsUntracked(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := untrackedStub(t, "feature")
	// The branch already exists on origin, but --push still pushes.
	stub.Register("", nil, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
		Push:      true,
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !slices.Equal(stub.InteractiveStrings(), []string{"push -u --end-of-options origin feature"}) {
		t.Errorf("push argv = %v, want a push", stub.InteractiveStrings())
	}
}

func TestCreateRemoteBranchExistsUntrackedWarnsAhead(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := untrackedStub(t, "feature")
	stub.Register("", nil, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature")
	stub.Register("1", nil, "rev-list", "--count", "origin/feature..feature")

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if !strings.Contains(errOut.String(), "local branch is 1 commit ahead of origin/feature") {
		t.Errorf("warning missing or not singular: %q", errOut.String())
	}
	if len(stub.Interactive()) != 0 {
		t.Errorf("should not push without --push: %v", stub.InteractiveStrings())
	}
}

func TestCreateFillUnknownBaseErrors(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo", 200, api.Repository{FullName: "myws/myrepo"})

	stub := newStub(t)
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Head:      "feature",
		Fill:      true,
	}
	err := createRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "has no main branch configured; specify --base") {
		t.Fatalf("err = %v", err)
	}
	if len(stub.Interactive()) != 0 {
		t.Errorf("should not push before failing: %v", stub.InteractiveStrings())
	}
}

func TestCreateFillExplicitBaseUsesRemoteRef(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := untrackedStub(t, "my-feature")
	stub.Register("a\x00First\x00\x1eb\x00Second\x00\x1e", nil,
		"log", "--pretty=format:%H%x00%s%x00%b%x1e", "--end-of-options", "upstream/develop..my-feature")
	stub.Register("", nil, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/my-feature")
	stub.Register("0", nil, "rev-list", "--count", "origin/my-feature..my-feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(upstreamBaseRemote()),
		Base:      "develop",
		Head:      "my-feature",
		Fill:      true,
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	var body map[string]any
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
	if body["title"] != "my feature" {
		t.Errorf("title = %v", body["title"])
	}
	if body["description"] != "- Second\n- First" {
		t.Errorf("description = %q", body["description"])
	}
}

func TestCreateFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *CreateOptions
	cmd := NewCmdCreate(f, func(o *CreateOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "-t", "Hi", "-b", "Body", "-B", "main", "-H", "feature", "--draft", "-r", "bob,cara", "--close-source-branch", "--push"); err != nil {
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
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdCreate(f, func(o *CreateOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "-b", "x", "-F", "file")
	cmdtest.AssertFlagError(t, err, "")
}

func TestCreateEmptyBodyWithBodyFileConflict(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdCreate(f, func(o *CreateOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "--body", "", "-F", "file")
	cmdtest.AssertFlagError(t, err, "")
}

func TestCreateRejectsPositionalArgs(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdCreate(f, func(o *CreateOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "My", "title")
	cmdtest.AssertFlagError(t, err, "")
}

// Both answers are available on stdin at once; the push confirmation must not
// swallow the title line.
func TestCreatePipedPromptsShareStdin(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
	noDefaultReviewers(srv)

	stub := untrackedStub(t, "feature")
	stub.Register("", gittest.Exit(2), "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true) // keep git's own push output
	in.WriteString("y\nPiped title\n")

	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Head:      "feature",
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}

	if !strings.Contains(errOut.String(), "Push branch feature to origin? [Y/n] Title: ") {
		t.Errorf("prompts = %q", errOut.String())
	}
	var body map[string]any
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
	if body["title"] != "Piped title" {
		t.Errorf("title = %v", body["title"])
	}
}

func TestCreateAddsDefaultReviewersExceptAuthor(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/effective-default-reviewers", 200, cmdtest.ValuesPage([]map[string]any{
		{"reviewer_type": "repository", "user": cmdtest.User("ada")},
		{"reviewer_type": "project", "user": cmdtest.User("bob")},
		{"reviewer_type": "repository", "user": cmdtest.User("cara")},
	}))
	srv.Handle("GET", "/user", 200, cmdtest.User("ada"))
	created := createdPRResponse()
	created.Draft = true
	created.Source.Branch.Name = "feature"
	created.Destination.Branch.Name = "dev"
	created.Reviewers = []api.User{cmdtest.User("cara"), cmdtest.User("bob")}
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, created)

	ios, _, out, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
		Base:      "dev",
		Draft:     true,
		Reviewers: []string{"{CARA-uuid}"},
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}

	var body struct {
		Reviewers []map[string]string `json:"reviewers"`
	}
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
	var got []string
	for _, r := range body.Reviewers {
		got = append(got, r["uuid"])
	}
	if !slices.Equal(got, []string{"{cara-uuid}", "{bob-uuid}"}) {
		t.Errorf("reviewers = %v, want explicit cara then default bob, without the author", got)
	}
	if out.String() != "https://bitbucket.org/myws/myrepo/pull-requests/123\n" {
		t.Errorf("stdout = %q, want only the URL", out.String())
	}
	if errOut.String() != "✓ Created #123 (draft) feature → dev · reviewers: Cara, Bob\n" {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestCreateNoDefaultReviewers(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t) // no default reviewers route: a lookup would fail the test
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:                 ios,
		APIClient:          cmdtest.ClientFunc(srv),
		Git:                cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:           cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:              "T",
		Head:               "feature",
		NoDefaultReviewers: true,
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	var body map[string]any
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
	if _, ok := body["reviewers"]; ok {
		t.Errorf("reviewers = %v, want none", body["reviewers"])
	}
	if errOut.String() != "✓ Created #123 feature\n" {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestCreateDefaultReviewersForbiddenWarns(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/effective-default-reviewers", 403, scopeErrorBody)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	ios, _, out, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun must not fail when default reviewers are forbidden: %v", err)
	}
	if !strings.Contains(out.String(), "pull-requests/123") {
		t.Errorf("URL not printed: %q", out.String())
	}
	if !strings.Contains(errOut.String(), "! could not fetch default reviewers, continuing without them: ") ||
		!strings.Contains(errOut.String(), "required scopes: read:workspace:bitbucket") ||
		!strings.Contains(errOut.String(), "(use --no-default-reviewers to skip them)") {
		t.Errorf("warning = %q", errOut.String())
	}
}

func TestCreateDefaultReviewersUnknownUserSkipsThem(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/effective-default-reviewers", 200, cmdtest.ValuesPage([]map[string]any{
		{"user": cmdtest.User("bob")},
	}))
	srv.Handle("GET", "/user", 403, scopeErrorBody)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(pushedStub(t, "feature")),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	var body map[string]any
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
	if _, ok := body["reviewers"]; ok {
		t.Errorf("reviewers = %v, want none", body["reviewers"])
	}
	if !strings.Contains(errOut.String(), "could not determine the current user") {
		t.Errorf("warning = %q", errOut.String())
	}
}

func TestCreateDefaultBase(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		config string // git config bh.base; "" means unset
		env    string // BH_BASE
		flag   string // --base
		want   string // destination sent; "" means omitted
	}{
		{name: "nothing configured uses the server default"},
		{name: "git config", config: "dev", want: "dev"},
		{name: "env", env: "staging", want: "staging"},
		{name: "git config wins over env", config: "dev", env: "staging", want: "dev"},
		{name: "flag wins", config: "dev", env: "staging", flag: "main", want: "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())
			noDefaultReviewers(srv)

			stub := gittest.New(t).
				Register("", gittest.Exit(1), "config", "--get", "branch.feature.remote").
				Register("", gittest.Exit(1), "config", "--get", "branch.feature.merge").
				Register("", nil, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature").
				Register("0", nil, "rev-list", "--count", "origin/feature..feature")
			if tt.flag == "" {
				// With --base the config must not be consulted (the strict
				// stub fails on an unregistered call).
				var err error
				if tt.config == "" {
					err = gittest.Exit(1)
				}
				stub.Register(tt.config, err, "config", "--get", "bh.base")
			}

			ios, _, _, errOut := cmdutil.TestIOStreams()
			opts := &CreateOptions{
				IO:        ios,
				APIClient: cmdtest.ClientFunc(srv),
				Git:       cmdtest.GitFunc(stub),
				BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
				Getenv: func(k string) string {
					if k == "BH_BASE" {
						return tt.env
					}
					return ""
				},
				Title: "T",
				Head:  "feature",
				Base:  tt.flag,
			}
			if err := createRun(t.Context(), opts); err != nil {
				t.Fatalf("createRun: %v", err)
			}
			var body struct {
				Destination *struct {
					Branch struct{ Name string } `json:"branch"`
				} `json:"destination"`
			}
			cmdtest.RequireRequest(t, srv, "POST", "/pullrequests").DecodeJSON(t, &body)
			got := ""
			if body.Destination != nil {
				got = body.Destination.Branch.Name
			}
			if got != tt.want {
				t.Errorf("destination = %q, want %q", got, tt.want)
			}
			if tt.want != "" && !strings.Contains(errOut.String(), "feature → "+tt.want) {
				t.Errorf("summary = %q", errOut.String())
			}
		})
	}
}

func TestCreateWebUsesConfiguredBase(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t) // no repository route: the main branch must not be looked up

	stub := untrackedStub(t, "feature").
		Register("", nil, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature").
		Register("0", nil, "rev-list", "--count", "origin/feature..feature")
	stub.Register("dev", nil, "config", "--get", "bh.base")

	fb := &cmdtest.FakeBrowser{}
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Browser:   fb,
		Head:      "feature",
		Web:       true,
	}
	if err := createRun(t.Context(), opts); err != nil {
		t.Fatalf("createRun: %v", err)
	}
	want := "https://bitbucket.org/myws/myrepo/pull-requests/new?source=feature&dest=dev"
	if fb.URL() != want {
		t.Errorf("web url = %q, want %q", fb.URL(), want)
	}
}

func TestCreateQuietPushFailureShowsGitOutput(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t) // nothing may be created

	stub := untrackedStub(t, "feature")
	stub.Register("", &git.Error{
		Args:     []string{"push", "--quiet", "-u", "--end-of-options", "origin", "feature"},
		ExitCode: 1,
		Stderr:   "remote: Permission denied\nfatal: Could not read from remote repository.\n",
	}, "push", "--quiet", "-u", "--end-of-options", "origin", "feature")

	ios, _, out, errOut := cmdutil.TestIOStreams()
	opts := &CreateOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(stub),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Title:     "T",
		Head:      "feature",
		Push:      true,
	}
	err := createRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "remote: Permission denied") {
		t.Fatalf("err = %v, want git's output", err)
	}
	if out.Len() != 0 || strings.Contains(errOut.String(), "Pushed") {
		t.Errorf("stdout = %q, stderr = %q", out.String(), errOut.String())
	}
}

func TestCreateNoDefaultReviewersFlag(t *testing.T) {
	t.Parallel()
	var captured *CreateOptions
	cmd := NewCmdCreate(cmdtest.NewFactory(), func(o *CreateOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "--no-default-reviewers"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !captured.NoDefaultReviewers {
		t.Error("--no-default-reviewers not parsed")
	}
	if captured.Getenv == nil {
		t.Error("Getenv must default to the process environment")
	}
}
