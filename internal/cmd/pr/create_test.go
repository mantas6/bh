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

// untrackedStub returns a strict git stub for a head branch with no upstream
// tracking configuration.
func untrackedStub(t *testing.T, head string) *gittest.Stub {
	return gittest.New(t).
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

func TestCreateAutoPushTTYYes(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	stub := untrackedStub(t, "feature")
	// Branch not on remote -> push path.
	stub.Register("", gittest.Exit(2), "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, in, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
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

	stub := untrackedStub(t, "feature")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
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
		t.Errorf("push argv = %v", stub.InteractiveStrings())
	}
}

func TestCreateUpstreamAheadWarns(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests", 201, createdPRResponse())

	stub := gittest.New(t)
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

	stub := gittest.New(t)
	stub.Register("fork", nil, "config", "--get", "branch.feature.remote")
	stub.Register("refs/heads/feature", nil, "config", "--get", "branch.feature.merge")
	stub.Expect("push", "-u", "--end-of-options", "fork", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
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

	stub := gittest.New(t)
	stub.Register("", gittest.Exit(1), "config", "--get", "branch.feature.remote")
	stub.Register("", gittest.Exit(1), "config", "--get", "branch.feature.merge")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
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

	stub := untrackedStub(t, "feature")
	// The branch already exists on origin, but --push still pushes.
	stub.Register("", nil, "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, _, _, _ := cmdutil.TestIOStreams()
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

	stub := gittest.New(t)
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

	stub := untrackedStub(t, "feature")
	stub.Register("", gittest.Exit(2), "ls-remote", "--exit-code", "--heads", "origin", "refs/heads/feature")
	stub.Expect("push", "-u", "--end-of-options", "origin", "feature")

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
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
