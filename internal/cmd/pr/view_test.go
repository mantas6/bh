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

// viewWithComments runs `pr view` in text mode against pr and comments with
// the given options applied, and returns stdout.
func viewWithComments(t *testing.T, pr *api.PullRequest, comments []api.Comment, setup func(*ViewOptions)) (string, *apitest.Server) {
	t.Helper()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, pr)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123/comments", 200, cmdtest.ValuesPage(comments))

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios
	opts.Now = cmdtest.FixedNow
	if setup != nil {
		setup(opts)
	}
	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	return out.String(), srv
}

func viewComments() []api.Comment {
	root := cmdtest.CommentAt(1, "ada", "Rename this", 120)
	root.Inline = &api.Inline{Path: "main.go", To: new(42)}
	root.Resolution = &api.Resolution{Type: "resolved"}
	reply := cmdtest.CommentAt(2, "bob", "Done", 30)
	reply.Parent = &api.CommentRef{ID: 1}
	open := cmdtest.CommentAt(3, "cara", "Why?", 5)
	return []api.Comment{root, reply, open}
}

func TestViewComments(t *testing.T) {
	t.Parallel()
	got, _ := viewWithComments(t, cmdtest.SamplePR(), viewComments(), func(o *ViewOptions) {
		o.Comments = true
	})

	want := "This does things.\n" +
		"\n" +
		"Comments:\n" +
		"#1 ada · about 2 hours ago · main.go:42 [resolved]\n" +
		"  Rename this\n" +
		"\n" +
		"  #2 bob · 30 minutes ago\n" +
		"    Done\n" +
		"\n" +
		"#3 cara · 5 minutes ago\n" +
		"  Why?\n" +
		"\n" +
		"View this pull request on Bitbucket: https://bitbucket.org/myws/myrepo/pull-requests/123\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("output does not end with the comment threads:\n got:\n%s\nwant suffix:\n%s", got, want)
	}
	if strings.Contains(got, "bh pr comment list") {
		t.Errorf("hint should be omitted with --comments:\n%s", got)
	}
}

func TestViewCommentsUnresolved(t *testing.T) {
	t.Parallel()
	got, _ := viewWithComments(t, cmdtest.SamplePR(), viewComments(), func(o *ViewOptions) {
		o.Comments = true
		o.Unresolved = true
	})
	if !strings.Contains(got, "Unresolved comments:\n#3 cara · 5 minutes ago\n  Why?\n\n") {
		t.Errorf("missing unresolved thread:\n%s", got)
	}
	for _, c := range []string{"#1 ada", "#2 bob", "[resolved]"} {
		if strings.Contains(got, c) {
			t.Errorf("resolved thread should be hidden (found %q):\n%s", c, got)
		}
	}
}

func TestViewCommentsNone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		unresolved bool
		comments   []api.Comment
		want       string
	}{
		{"no comments", false, []api.Comment{}, "No comments\n\nView this pull request"},
		{"all resolved", true, viewComments()[:2], "No unresolved comments\n\nView this pull request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, _ := viewWithComments(t, cmdtest.SamplePR(), tt.comments, func(o *ViewOptions) {
				o.Comments = true
				o.Unresolved = tt.unresolved
			})
			if !strings.Contains(got, tt.want) {
				t.Errorf("output missing %q:\n%s", tt.want, got)
			}
		})
	}
}

func TestViewCommentHint(t *testing.T) {
	t.Parallel()
	got, srv := viewWithComments(t, cmdtest.SamplePR(), nil, nil)

	want := "This does things.\n" +
		"\n" +
		"2 comments · bh pr comment list 123\n" +
		"View this pull request on Bitbucket: https://bitbucket.org/myws/myrepo/pull-requests/123\n"
	if !strings.HasSuffix(got, want) {
		t.Errorf("output does not end with the hint:\n got:\n%s\nwant suffix:\n%s", got, want)
	}
	if req := cmdtest.FindRequest(srv, "GET", "/comments"); req != nil {
		t.Errorf("comments should not be fetched without --comments")
	}
}

func TestViewCommentHintSingularAndAbsent(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.CommentCount = 1
	if got := renderView(t, pr); !strings.Contains(got, "\n\n1 comment · bh pr comment list 123\n") {
		t.Errorf("missing singular hint:\n%s", got)
	}

	pr = cmdtest.SamplePR()
	pr.CommentCount = 0
	if got := renderView(t, pr); strings.Contains(got, "bh pr comment list") {
		t.Errorf("hint should be omitted without comments:\n%s", got)
	}
}

func TestViewCommentHintURLArg(t *testing.T) {
	t.Parallel()
	pr := cmdtest.SamplePR()
	pr.Links.HTML.Href = "https://bitbucket.org/other/repo/pull-requests/123"
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/other/repo/pullrequests/123", 200, pr)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newViewOptions(t, srv)
	opts.IO = ios
	opts.Arg = "https://bitbucket.org/other/repo/pull-requests/123"
	opts.Git = nil // no local checkout of other/repo
	if err := viewRun(t.Context(), opts); err != nil {
		t.Fatalf("viewRun: %v", err)
	}
	want := "2 comments · bh pr comment list https://bitbucket.org/other/repo/pull-requests/123\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("output missing %q:\n%s", want, out.String())
	}
}

func TestViewCommentsFlags(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		args           []string
		wantComments   bool
		wantUnresolved bool
		wantErr        string
	}{
		{"long", []string{"1", "--comments"}, true, false, ""},
		{"short", []string{"1", "-c"}, true, false, ""},
		{"unresolved implies comments", []string{"1", "--unresolved"}, true, true, ""},
		{"both", []string{"1", "--comments", "--unresolved"}, true, true, ""},
		{"json", []string{"1", "--comments", "--json"}, false, false, "--comments cannot be combined with --json"},
		{"json unresolved", []string{"1", "--unresolved", "--json"}, false, false, "--comments cannot be combined with --json"},
		{"web", []string{"1", "-c", "--web"}, false, false, "--comments cannot be combined with --web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var captured *ViewOptions
			cmd := NewCmdView(cmdtest.NewFactory(), func(o *ViewOptions) error {
				captured = o
				return nil
			})
			_, _, err := cmdtest.RunCommand(t, cmd, tt.args...)
			if tt.wantErr != "" {
				cmdtest.AssertFlagError(t, err, tt.wantErr)
				if captured != nil {
					t.Error("runF should not be called")
				}
				return
			}
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if captured.Comments != tt.wantComments || captured.Unresolved != tt.wantUnresolved {
				t.Errorf("Comments=%v Unresolved=%v, want %v %v",
					captured.Comments, captured.Unresolved, tt.wantComments, tt.wantUnresolved)
			}
		})
	}
}
