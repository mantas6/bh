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

func newReviewOpts(t *testing.T, srv *apitest.Server) *ReviewOptions {
	t.Helper()
	ios, _, _, _ := cmdutil.TestIOStreams()
	return &ReviewOptions{
		IO:        ios,
		ApiClient: func() (*api.Client, error) { return srv.Client(), nil },
		Git:       gitFunc(gittest.New()),
		BaseRepo:  baseRepoFunc(originRemote()),
		Arg:       "123",
	}
}

func TestReviewApprove(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/approve", 200, nil)

	opts := newReviewOpts(t, srv)
	opts.Approve = true
	if err := reviewRun(opts); err != nil {
		t.Fatalf("reviewRun: %v", err)
	}
	if findRequest(srv, "POST", "/pullrequests/123/approve") == nil {
		t.Fatal("no approve request")
	}
	if findRequest(srv, "POST", "/pullrequests/123/comments") != nil {
		t.Fatal("no comment should be posted without a body")
	}
}

func TestReviewApproveWithBody(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/approve", 200, nil)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/comments", 201, &api.Comment{ID: 1})

	opts := newReviewOpts(t, srv)
	opts.Approve = true
	opts.Body = "looks good"
	if err := reviewRun(opts); err != nil {
		t.Fatalf("reviewRun: %v", err)
	}

	req := findRequest(srv, "POST", "/pullrequests/123/comments")
	if req == nil {
		t.Fatal("no comment request")
	}
	var body map[string]any
	json.Unmarshal(req.Body, &body)
	content, _ := body["content"].(map[string]any)
	if content["raw"] != "looks good" {
		t.Errorf("comment raw = %v", content["raw"])
	}
}

func TestReviewRequestChanges(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/request-changes", 200, nil)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &ReviewOptions{
		IO:             ios,
		ApiClient:      func() (*api.Client, error) { return srv.Client(), nil },
		Git:            gitFunc(gittest.New()),
		BaseRepo:       baseRepoFunc(originRemote()),
		Arg:            "123",
		RequestChanges: true,
	}
	if err := reviewRun(opts); err != nil {
		t.Fatalf("reviewRun: %v", err)
	}
	if findRequest(srv, "POST", "/pullrequests/123/request-changes") == nil {
		t.Fatal("no request-changes request")
	}
	if !strings.Contains(errOut.String(), "Requested changes on pull request #123") {
		t.Errorf("message = %q", errOut.String())
	}
}

func TestReviewComment(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/comments", 201, &api.Comment{ID: 1})

	opts := newReviewOpts(t, srv)
	opts.Comment = true
	opts.Body = "a note"
	if err := reviewRun(opts); err != nil {
		t.Fatalf("reviewRun: %v", err)
	}
	if findRequest(srv, "POST", "/pullrequests/123/comments") == nil {
		t.Fatal("no comment request")
	}
}

func TestReviewCommentRequiresBody(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())

	opts := newReviewOpts(t, srv)
	opts.Comment = true
	err := reviewRun(opts)
	var fe *cmdutil.FlagError
	if err == nil || !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}

func TestReviewConflictingModes(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdReview(f, func(o *ReviewOptions) error { return nil })
	cmd.SetArgs([]string{"123", "--approve", "--comment"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	err := cmd.Execute()
	var fe *cmdutil.FlagError
	if err == nil || !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}

func TestReviewNoModeErrors(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
	cmd := NewCmdReview(f, func(o *ReviewOptions) error { return nil })
	cmd.SetArgs([]string{"123"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	err := cmd.Execute()
	var fe *cmdutil.FlagError
	if err == nil || !errors.As(err, &fe) {
		t.Fatalf("expected FlagError, got %v", err)
	}
}

func TestReviewFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *ReviewOptions
	cmd := NewCmdReview(f, func(o *ReviewOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"123", "-r", "-b", "please fix"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "123" || !captured.RequestChanges || captured.Body != "please fix" {
		t.Errorf("parsed = %+v", captured)
	}
}
