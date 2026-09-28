package shared

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
)

func TestUserName(t *testing.T) {
	cases := []struct {
		name string
		user *api.User
		want string
	}{
		{"nil", nil, ""},
		{"nickname", &api.User{Nickname: "ada", DisplayName: "Ada Lovelace"}, "ada"},
		{"display name fallback", &api.User{DisplayName: "Ada Lovelace"}, "Ada Lovelace"},
		{"empty", &api.User{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UserName(tc.user); got != tc.want {
				t.Errorf("UserName = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPRAuthor(t *testing.T) {
	if got := PRAuthor(&api.PullRequest{}); got != "" {
		t.Errorf("PRAuthor(no author) = %q", got)
	}
	pr := &api.PullRequest{Author: &api.User{Nickname: "ada"}}
	if got := PRAuthor(pr); got != "ada" {
		t.Errorf("PRAuthor = %q", got)
	}
}

func TestSameRepoPR(t *testing.T) {
	repo := testRepo()
	cases := []struct {
		name string
		src  *api.Repository
		want bool
	}{
		{"same", &api.Repository{FullName: "myws/myrepo"}, true},
		{"case-insensitive", &api.Repository{FullName: "MyWS/MyRepo"}, true},
		{"fork", &api.Repository{FullName: "fork/myrepo"}, false},
		{"deleted fork", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr := &api.PullRequest{Source: api.PRRef{Repository: tc.src}}
			if got := SameRepoPR(pr, repo); got != tc.want {
				t.Errorf("SameRepoPR = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReadBodyFile(t *testing.T) {
	ios, in, _, _ := cmdutil.TestIOStreams()
	in.WriteString("from stdin\n")
	got, err := ReadBodyFile(ios, "-")
	if err != nil || got != "from stdin\n" {
		t.Fatalf("stdin: got %q, %v", got, err)
	}

	path := filepath.Join(t.TempDir(), "body.md")
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = ReadBodyFile(ios, path)
	if err != nil || got != "from file" {
		t.Fatalf("file: got %q, %v", got, err)
	}

	if _, err := ReadBodyFile(ios, filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadLine(t *testing.T) {
	cases := map[string]string{
		"yes\n":      "yes",
		"yes\r\n":    "yes",
		"no-eol":     "no-eol",
		"":           "",
		"one\ntwo\n": "one",
	}
	for in, want := range cases {
		got, err := ReadLine(strings.NewReader(in))
		if err != nil {
			t.Fatalf("ReadLine(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("ReadLine(%q) = %q, want %q", in, got, want)
		}
	}
}
