package pr

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func newReviewOpts(t *testing.T, srv *apitest.Server) *ReviewOptions {
	t.Helper()
	ios, _, _, _ := cmdutil.TestIOStreams()
	return &ReviewOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Arg:       "123",
	}
}

func TestReviewApprove(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/approve", 200, nil)

	opts := newReviewOpts(t, srv)
	opts.Approve = true
	if err := reviewRun(t.Context(), opts); err != nil {
		t.Fatalf("reviewRun: %v", err)
	}
	if cmdtest.FindRequest(srv, "POST", "/pullrequests/123/approve") == nil {
		t.Fatal("no approve request")
	}
	if cmdtest.FindRequest(srv, "POST", "/pullrequests/123/comments") != nil {
		t.Fatal("no comment should be posted without a body")
	}
}

func TestReviewApproveWithBody(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/approve", 200, nil)
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/comments", 201, &api.Comment{ID: 1})

	opts := newReviewOpts(t, srv)
	opts.Approve = true
	opts.Body = "looks good"
	if err := reviewRun(t.Context(), opts); err != nil {
		t.Fatalf("reviewRun: %v", err)
	}

	var body map[string]any
	cmdtest.RequireRequest(t, srv, "POST", "/pullrequests/123/comments").DecodeJSON(t, &body)
	content, _ := body["content"].(map[string]any)
	if content["raw"] != "looks good" {
		t.Errorf("comment raw = %v", content["raw"])
	}
}

func TestReviewRequestChanges(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/request-changes", 200, nil)

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &ReviewOptions{
		IO:             ios,
		APIClient:      cmdtest.ClientFunc(srv),
		Git:            cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:       cmdtest.BaseRepoFunc(cmdtest.OriginRemote()),
		Arg:            "123",
		RequestChanges: true,
	}
	if err := reviewRun(t.Context(), opts); err != nil {
		t.Fatalf("reviewRun: %v", err)
	}
	if cmdtest.FindRequest(srv, "POST", "/pullrequests/123/request-changes") == nil {
		t.Fatal("no request-changes request")
	}
	if !strings.Contains(errOut.String(), "Requested changes on pull request #123") {
		t.Errorf("message = %q", errOut.String())
	}
}

func TestReviewComment(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())
	srv.Handle("POST", "/repositories/myws/myrepo/pullrequests/123/comments", 201, &api.Comment{ID: 1})

	opts := newReviewOpts(t, srv)
	opts.Comment = true
	opts.Body = "a note"
	if err := reviewRun(t.Context(), opts); err != nil {
		t.Fatalf("reviewRun: %v", err)
	}
	if cmdtest.FindRequest(srv, "POST", "/pullrequests/123/comments") == nil {
		t.Fatal("no comment request")
	}
}

func TestReviewCommentRequiresBody(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	opts := newReviewOpts(t, srv)
	opts.Comment = true
	err := reviewRun(t.Context(), opts)
	cmdtest.AssertFlagError(t, err, "comment body is required")
}

func TestReviewCommentBlankBodyRequiresBody(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)

	opts := newReviewOpts(t, srv)
	opts.Comment = true
	opts.Body = "  \n"
	err := reviewRun(t.Context(), opts)
	cmdtest.AssertFlagError(t, err, "")
}

func TestReviewConflictingModes(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdReview(f, func(o *ReviewOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "123", "--approve", "--comment")
	cmdtest.AssertFlagError(t, err, "")
}

func TestReviewNoModeErrors(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdReview(f, func(o *ReviewOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "123")
	cmdtest.AssertFlagError(t, err, "")
}

func TestReviewFlagParsing(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()

	var captured *ReviewOptions
	cmd := NewCmdReview(f, func(o *ReviewOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "123", "-r", "-b", "please fix"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.Arg != "123" || !captured.RequestChanges || captured.Body != "please fix" {
		t.Errorf("parsed = %+v", captured)
	}
}
