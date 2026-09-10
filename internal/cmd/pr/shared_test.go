package pr

import (
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

func TestParsePRArg(t *testing.T) {
	cases := []struct {
		name    string
		arg     string
		wantID  int
		wantWS  string
		wantErr bool
	}{
		{"number", "123", 123, "", false},
		{"hash", "#42", 42, "", false},
		{"url", "https://bitbucket.org/ws/repo/pull-requests/7", 7, "ws", false},
		{"url trailing", "https://bitbucket.org/ws/repo/pull-requests/7/diff", 7, "ws", false},
		{"empty", "", 0, "", true},
		{"words", "abc", 0, "", true},
		{"zero", "0", 0, "", true},
		{"bad url", "https://bitbucket.org/ws/repo", 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, repo, err := ParsePRArg(tc.arg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.arg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tc.wantID {
				t.Errorf("id = %d, want %d", id, tc.wantID)
			}
			if tc.wantWS == "" {
				if repo != nil {
					t.Errorf("repo = %v, want nil", repo)
				}
			} else {
				if repo == nil || repo.Workspace != tc.wantWS {
					t.Errorf("repo = %v, want workspace %q", repo, tc.wantWS)
				}
			}
		})
	}
}

func TestFindPRByNumber(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests/123", 200, samplePR())

	pr, repo, err := FindPR(t.Context(), srv.Client(), gittest.New(), testRepo(), "123")
	if err != nil {
		t.Fatalf("FindPR: %v", err)
	}
	if pr.ID != 123 {
		t.Errorf("id = %d", pr.ID)
	}
	if repo.FullName() != "myws/myrepo" {
		t.Errorf("repo = %s", repo.FullName())
	}
}

func TestFindPRCurrentBranch(t *testing.T) {
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200, valuesPage([]api.PullRequest{*samplePR()}))

	stub := gittest.New()
	stub.Register("feature", nil, "symbolic-ref", "--quiet", "--short", "HEAD")

	pr, _, err := FindPR(t.Context(), srv.Client(), stub, testRepo(), "")
	if err != nil {
		t.Fatalf("FindPR: %v", err)
	}
	if pr.ID != 123 {
		t.Errorf("id = %d", pr.ID)
	}
}

func TestFindPRDetachedHead(t *testing.T) {
	srv := apitest.New(t)

	stub := gittest.New()
	stub.Register("", &git.GitError{ExitCode: 1}, "symbolic-ref", "--quiet", "--short", "HEAD")

	_, _, err := FindPR(t.Context(), srv.Client(), stub, testRepo(), "")
	if err == nil || err.Error() != "no pull request specified and not on a branch" {
		t.Fatalf("err = %v", err)
	}
}
