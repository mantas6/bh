package pr

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func mergePR() *api.PullRequest {
	pr := samplePR()
	pr.State = "OPEN"
	pr.Source.Branch.Name = "feature"
	pr.Source.Repository = &api.Repository{FullName: "myws/myrepo"}
	pr.Destination.Branch.Name = "main"
	pr.Destination.Repository = &api.Repository{FullName: "myws/myrepo"}
	pr.Destination.Branch.DefaultMergeStrategy = "squash"
	return pr
}

func TestMergeDefaultStrategyTTYConfirmYes(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, mergePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/merge", 200, mergePR())

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	in.WriteString("y\n")

	opts := &MergeOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
	if err := mergeRun(opts); err != nil {
		t.Fatalf("mergeRun: %v", err)
	}

	if !strings.Contains(errOut.String(), "using squash?") {
		t.Errorf("prompt missing default strategy: %q", errOut.String())
	}
	if !strings.Contains(errOut.String(), "Merged pull request #123") {
		t.Errorf("success missing: %q", errOut.String())
	}

	req := findRequest(srv, "POST", "/pullrequests/123/merge")
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	// The server applies the branch default; bh must not pick one itself.
	if v, ok := body["merge_strategy"]; ok {
		t.Errorf("merge_strategy = %v, want omitted", v)
	}
	if body["type"] != "pullrequest" {
		t.Errorf("type = %v", body["type"])
	}
}

func TestMergeYesWithoutStrategyOmitsStrategy(t *testing.T) {
	srv := apitest.New(t)
	pr := mergePR()
	pr.Destination.Branch.DefaultMergeStrategy = ""
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/merge", 200, pr)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &MergeOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
		Yes:       true,
	}
	if err := mergeRun(opts); err != nil {
		t.Fatalf("mergeRun: %v", err)
	}

	req := findRequest(srv, "POST", "/pullrequests/123/merge")
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if v, ok := body["merge_strategy"]; ok {
		t.Errorf("merge_strategy = %v, want omitted (not merge_commit)", v)
	}
}

func TestMergeDeleteBranchWithoutRemoteSkipsLocalCleanup(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, mergePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/merge", 200, mergePR())

	stub := gittest.New()
	stub.FailUnstubbed = true

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &MergeOptions{
		IO:           ios,
		APIClient:    func() (*api.Client, error) { return srv.Client(), nil },
		Git:          gitFunc(stub),
		BaseRepo:     baseRepoFunc(nil),
		Arg:          "123",
		Merge:        true,
		DeleteBranch: true,
	}
	if err := mergeRun(opts); err != nil {
		t.Fatalf("mergeRun: %v", err)
	}
	if len(stub.Calls) != 0 {
		t.Errorf("unexpected git calls: %v", stub.CallStrings())
	}
	if !strings.Contains(errOut.String(), "Skipped deleting local branch feature: no git remote found for myws/myrepo") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestMergeURLRepoUsedForSameRepoCheck(t *testing.T) {
	srv := apitest.New(t)
	pr := mergePR()
	pr.Source.Repository = &api.Repository{FullName: "other/repo"}
	pr.Destination.Repository = &api.Repository{FullName: "other/repo"}
	srv.Handle("GET", "/repositories/other/repo/pullrequests/123", 200, pr)
	srv.Handle("POST", "/repositories/other/repo/pullrequests/123/merge", 200, pr)

	stub := gittest.New()
	stub.Register("origin\tgit@bitbucket.org:other/repo.git (fetch)\norigin\tgit@bitbucket.org:other/repo.git (push)\n",
		nil, "remote", "-v")
	stub.Register("feature", nil, "symbolic-ref", "--quiet", "--short", "HEAD")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &MergeOptions{
		IO:           ios,
		APIClient:    func() (*api.Client, error) { return srv.Client(), nil },
		Git:          gitFunc(stub),
		BaseRepo:     baseRepoFunc(nil),
		Arg:          "https://bitbucket.org/other/repo/pull-requests/123",
		Merge:        true,
		DeleteBranch: true,
	}
	if err := mergeRun(opts); err != nil {
		t.Fatalf("mergeRun: %v", err)
	}
	calls := strings.Join(stub.CallStrings(), "\n")
	if !strings.Contains(calls, "branch -D --end-of-options feature") {
		t.Errorf("local branch not deleted; calls:\n%s", calls)
	}
}

func TestMergeConfirmNoCancels(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, mergePR())

	ios, in, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	in.WriteString("n\n")

	opts := &MergeOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
	err := mergeRun(opts)
	if !errors.Is(err, cmdutil.ErrCancel) {
		t.Fatalf("err = %v, want ErrCancel", err)
	}
}

func TestMergeNonTTYWithoutFlagsErrors(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, mergePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &MergeOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
	err := mergeRun(opts)
	if err == nil || !strings.Contains(err.Error(), "specify a merge strategy") {
		t.Fatalf("err = %v", err)
	}
}

func TestMergeSquashWithMessageBody(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, mergePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/merge", 200, mergePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &MergeOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
		Squash:    true,
		Message:   "custom message",
	}
	if err := mergeRun(opts); err != nil {
		t.Fatalf("mergeRun: %v", err)
	}

	req := findRequest(srv, "POST", "/pullrequests/123/merge")
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("body: %v", err)
	}
	if body["merge_strategy"] != "squash" {
		t.Errorf("merge_strategy = %v", body["merge_strategy"])
	}
	if body["message"] != "custom message" {
		t.Errorf("message = %v", body["message"])
	}
	if body["close_source_branch"] != false {
		t.Errorf("close_source_branch = %v, want false", body["close_source_branch"])
	}
	if body["type"] != "pullrequest" {
		t.Errorf("type = %v", body["type"])
	}
}

func TestMergeDeleteBranchLocalCleanup(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, mergePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/merge", 200, mergePR())

	stub := gittest.New()
	// Current branch equals the source branch.
	stub.Register("feature", nil, "symbolic-ref", "--quiet", "--short", "HEAD")

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &MergeOptions{
		IO:           ios,
		APIClient:    func() (*api.Client, error) { return srv.Client(), nil },
		Git:          gitFunc(stub),
		BaseRepo:     baseRepoFunc(originRemote()),
		Arg:          "123",
		Merge:        true,
		DeleteBranch: true,
	}
	if err := mergeRun(opts); err != nil {
		t.Fatalf("mergeRun: %v", err)
	}

	assertCalls(t, stub.CallStrings(), []string{
		"symbolic-ref --quiet --short HEAD",
		"checkout --end-of-options main",
		"branch -D --end-of-options feature",
	})
	if !strings.Contains(errOut.String(), "Deleted local branch feature") {
		t.Errorf("cleanup message missing: %q", errOut.String())
	}

	req := findRequest(srv, "POST", "/pullrequests/123/merge")
	var body map[string]any
	json.Unmarshal(req.Body, &body)
	if body["close_source_branch"] != true {
		t.Errorf("close_source_branch = %v, want true", body["close_source_branch"])
	}
}

func TestMergeNotOpenErrors(t *testing.T) {
	srv := apitest.New(t)
	pr := mergePR()
	pr.State = "MERGED"
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &MergeOptions{
		IO:        ios,
		APIClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
		Merge:     true,
	}
	err := mergeRun(opts)
	if err == nil || !strings.Contains(err.Error(), "pull request #123 is merged") {
		t.Fatalf("err = %v", err)
	}
}

func TestMergeStrategyConflict(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdMerge(f, func(o *MergeOptions) error { return nil })
	cmd.SetArgs([]string{"123", "--merge", "--squash"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	err := cmd.Execute()
	var fe *cmdutil.FlagError
	if err == nil || !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}

func TestMergeFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *MergeOptions
	cmd := NewCmdMerge(f, func(o *MergeOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"123", "--squash", "-m", "msg", "-d", "-y"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "123" || !captured.Squash || captured.Message != "msg" || !captured.DeleteBranch || !captured.Yes {
		t.Errorf("parsed = %+v", captured)
	}
}
