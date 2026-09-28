package shared

import (
	"testing"
)

func TestParsePRArg(t *testing.T) {
	cases := []struct {
		name       string
		arg        string
		wantNumber int
		wantBranch string
		wantRepo   string
		wantErr    bool
	}{
		{name: "empty", arg: ""},
		{name: "blank", arg: "   "},
		{name: "number", arg: "123", wantNumber: 123},
		{name: "number padded", arg: " 123 ", wantNumber: 123},
		{name: "hash", arg: "#42", wantNumber: 42},
		{name: "url", arg: "https://bitbucket.org/ws/repo/pull-requests/7", wantNumber: 7, wantRepo: "ws/repo"},
		{name: "url trailing", arg: "https://bitbucket.org/ws/repo/pull-requests/7/diff", wantNumber: 7, wantRepo: "ws/repo"},
		{name: "branch", arg: "feature", wantBranch: "feature"},
		{name: "branch with slash", arg: "feature/login", wantBranch: "feature/login"},
		{name: "branch starting with digits", arg: "123-fix", wantBranch: "123-fix"},
		{name: "zero", arg: "0", wantErr: true},
		{name: "negative", arg: "-3", wantErr: true},
		{name: "hash zero", arg: "#0", wantErr: true},
		{name: "hash word", arg: "#abc", wantErr: true},
		{name: "flag-like", arg: "-feature", wantErr: true},
		{name: "spaces", arg: "my branch", wantErr: true},
		{name: "bad url", arg: "https://bitbucket.org/ws/repo", wantErr: true},
		{name: "url non-numeric id", arg: "https://bitbucket.org/ws/repo/pull-requests/abc", wantErr: true},
		{name: "url other host", arg: "https://github.com/ws/repo/pull-requests/1", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sel, err := ParsePRArg(tc.arg)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %+v", tc.arg, sel)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sel.Number != tc.wantNumber {
				t.Errorf("Number = %d, want %d", sel.Number, tc.wantNumber)
			}
			if sel.Branch != tc.wantBranch {
				t.Errorf("Branch = %q, want %q", sel.Branch, tc.wantBranch)
			}
			switch {
			case tc.wantRepo == "" && sel.Repo != nil:
				t.Errorf("Repo = %v, want nil", sel.Repo)
			case tc.wantRepo != "" && (sel.Repo == nil || sel.Repo.FullName() != tc.wantRepo):
				t.Errorf("Repo = %v, want %s", sel.Repo, tc.wantRepo)
			}
			wantCurrent := tc.wantNumber == 0 && tc.wantBranch == ""
			if sel.IsCurrentBranch() != wantCurrent {
				t.Errorf("IsCurrentBranch = %v, want %v", sel.IsCurrentBranch(), wantCurrent)
			}
		})
	}
}
