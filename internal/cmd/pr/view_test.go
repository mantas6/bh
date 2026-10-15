package pr

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
)

func newViewOptions(t *testing.T, srv *apitest.Server) *ViewOptions {
	return &ViewOptions{
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Arg:       "123",
	}
}

func TestViewText(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}

	got := out.String()
	checks := []string{
		"Add feature #123",
		"OPEN • ada wants to merge feature into main • 1 approval • 2 comments",
		"Reviewers: bob (approved ✓), cara (changes requested)",
		"Approved by: bob\n",
		"Changes requested by: cara\n",
		"This does things.",
		"View this pull request on Bitbucket: https://bitbucket.org/myws/myrepo/pull-requests/123",
	}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Errorf("output missing %q in:\n%s", c, got)
		}
	}
}

func TestViewForkPrefix(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Source.Repository = &api.Repository{FullName: "fork/myrepo"}
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if !strings.Contains(out.String(), "merge fork/myrepo:feature into main") {
		t.Errorf("fork prefix missing: %q", out.String())
	}
}

func TestViewNoDescription(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Description = ""
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if !strings.Contains(out.String(), "No description provided") {
		t.Errorf("missing no-description text: %q", out.String())
	}
}

func TestViewJSON(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios
	opts.JSON = true

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	var pr api.PullRequest
	if err := json.Unmarshal(out.Bytes(), &pr); err != nil {
		t.Fatalf("json: %v", err)
	}
	if pr.ID != 123 {
		t.Errorf("id = %d", pr.ID)
	}
}

func TestViewWeb(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, cmdtest.SamplePR())

	ios, _, _, _ := cmdutil.TestIOStreams()
	fb := &cmdtest.FakeBrowser{}
	opts := newViewOptions(t, srv)
	opts.IO = ios
	opts.Web = true
	opts.Browser = fb

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	if fb.URL() != "https://bitbucket.org/myws/myrepo/pull-requests/123" {
		t.Errorf("web url = %q", fb.URL())
	}
}

// renderView runs `pr view` in text mode against pr and returns stdout.
func renderView(t *testing.T, pr *api.PullRequest) string {
	t.Helper()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios

	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	return out.String()
}

func TestViewApprovalsFromNonReviewers(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Participants = append(pr.Participants,
		api.Participant{User: cmdtest.User("dan"), Role: "PARTICIPANT", Approved: true, State: "approved"},
		api.Participant{User: cmdtest.User("eve"), Role: "PARTICIPANT", State: "changes_requested"},
		api.Participant{User: cmdtest.User("finn"), Role: "PARTICIPANT"},
	)

	got := renderView(t, pr)
	for _, c := range []string{
		"• 2 approvals • 2 comments\n",
		"Reviewers: bob (approved ✓), cara (changes requested)\n",
		"Approved by: bob, dan\n",
		"Changes requested by: cara, eve\n",
	} {
		if !strings.Contains(got, c) {
			t.Errorf("output missing %q in:\n%s", c, got)
		}
	}
	if strings.Contains(got, "finn") {
		t.Errorf("commenting-only participant should not be listed:\n%s", got)
	}
}

func TestViewNoReviewers(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Reviewers = nil
	pr.Participants = nil
	pr.CommentCount = 1

	got := renderView(t, pr)
	for _, c := range []string{
		"OPEN • ada wants to merge feature into main • 0 approvals • 1 comment\n",
		"Reviewers: none\n",
	} {
		if !strings.Contains(got, c) {
			t.Errorf("output missing %q in:\n%s", c, got)
		}
	}
	for _, c := range []string{"Approved by:", "Changes requested by:"} {
		if strings.Contains(got, c) {
			t.Errorf("output should not contain %q:\n%s", c, got)
		}
	}
}

func TestViewApprovalOnlyFromParticipant(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Reviewers = nil
	pr.Participants = []api.Participant{
		{User: cmdtest.User("dan"), Role: "PARTICIPANT", Approved: true, State: "approved"},
	}

	got := renderView(t, pr)
	for _, c := range []string{
		"• 1 approval • 2 comments\n",
		"Reviewers: none\n",
		"Approved by: dan\n",
	} {
		if !strings.Contains(got, c) {
			t.Errorf("output missing %q in:\n%s", c, got)
		}
	}
	if strings.Contains(got, "Changes requested by:") {
		t.Errorf("unexpected changes-requested line:\n%s", got)
	}
}

func TestViewReviewerNotYetParticipant(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Reviewers = []api.User{cmdtest.User("bob"), cmdtest.User("gus")}

	got := renderView(t, pr)
	want := "Reviewers: bob (approved ✓), cara (changes requested), gus (pending)\n"
	if !strings.Contains(got, want) {
		t.Errorf("output missing %q in:\n%s", want, got)
	}
}

func TestViewStateWording(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state string
		want  string
	}{
		{"OPEN", "OPEN • ada wants to merge feature into main •"},
		{"MERGED", "MERGED • ada merged feature into main •"},
		{"DECLINED", "DECLINED • ada declined feature into main •"},
		{"SUPERSEDED", "SUPERSEDED • ada wants to merge feature into main •"},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()
			pr := cmdtest.SamplePR()
			pr.State = tt.state
			got := renderView(t, pr)
			if !strings.Contains(got, tt.want) {
				t.Errorf("output missing %q in:\n%s", tt.want, got)
			}
		})
	}
}
