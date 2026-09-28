package git_test

import (
	"errors"
	"testing"

	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

func TestParseRemoteURL(t *testing.T) {
	want := git.Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}

	tests := []struct {
		name string
		raw  string
		ok   bool
		repo git.Repo
	}{
		{"https", "https://bitbucket.org/ws/repo.git", true, want},
		{"https no .git", "https://bitbucket.org/ws/repo", true, want},
		{"https trailing slash", "https://bitbucket.org/ws/repo/", true, want},
		{"https userinfo", "https://user@bitbucket.org/ws/repo.git", true, want},
		{"scp", "git@bitbucket.org:ws/repo.git", true, want},
		{"scp no .git", "git@bitbucket.org:ws/repo", true, want},
		{"ssh", "ssh://git@bitbucket.org/ws/repo.git", true, want},
		{"ssh altssh port", "ssh://git@altssh.bitbucket.org:443/ws/repo.git", true, want},
		{"altssh https normalizes", "https://altssh.bitbucket.org/ws/repo.git", true, want},
		{"git+ssh", "git+ssh://git@bitbucket.org/ws/repo.git", true, want},
		{"ssh+git", "ssh+git://git@bitbucket.org/ws/repo.git", true, want},

		{"github", "https://github.com/ws/repo.git", false, git.Repo{}},
		{"github scp", "git@github.com:ws/repo.git", false, git.Repo{}},
		{"local path", "/home/user/repo", false, git.Repo{}},
		{"relative path", "../repo", false, git.Repo{}},
		{"empty", "", false, git.Repo{}},
		{"too few segments", "https://bitbucket.org/ws.git", false, git.Repo{}},
		{"too many segments", "https://bitbucket.org/ws/repo/extra.git", false, git.Repo{}},
		{"scp too many", "git@bitbucket.org:ws/repo/extra.git", false, git.Repo{}},
		{"no host", "https://ws/repo.git", false, git.Repo{}},
		{"missing name", "https://bitbucket.org/ws/.git", false, git.Repo{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, ok := git.ParseRemoteURL(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ParseRemoteURL(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			}
			if ok && repo != tt.repo {
				t.Fatalf("ParseRemoteURL(%q) = %+v, want %+v", tt.raw, repo, tt.repo)
			}
		})
	}
}

func TestIsSSHURL(t *testing.T) {
	tests := []struct {
		raw  string
		want bool
	}{
		{"git@bitbucket.org:ws/repo.git", true},
		{"bitbucket.org:ws/repo.git", true},
		{"ssh://git@bitbucket.org/ws/repo.git", true},
		{"git+ssh://git@bitbucket.org/ws/repo.git", true},
		{"ssh+git://git@bitbucket.org/ws/repo.git", true},
		{"SSH://git@bitbucket.org/ws/repo.git", true},
		{"https://bitbucket.org/ws/repo.git", false},
		{"http://bitbucket.org/ws/repo.git", false},
		{"git://bitbucket.org/ws/repo.git", false},
		{"/local/path", false},
		{"./a:b", false},
		{`C:\repo`, false},
		{"", false},
	}
	for _, tt := range tests {
		if got := git.IsSSHURL(tt.raw); got != tt.want {
			t.Errorf("IsSSHURL(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestRepoURLs(t *testing.T) {
	r := git.Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}
	if got := r.WebURL(); got != "https://bitbucket.org/ws/repo" {
		t.Errorf("WebURL = %q", got)
	}
	if got := r.CloneURL(false); got != "https://bitbucket.org/ws/repo.git" {
		t.Errorf("CloneURL(false) = %q", got)
	}
	if got := r.CloneURL(true); got != "git@bitbucket.org:ws/repo.git" {
		t.Errorf("CloneURL(true) = %q", got)
	}
	if got := (git.Repo{Workspace: "ws", Name: "repo"}).WebURL(); got != "https://bitbucket.org/ws/repo" {
		t.Errorf("WebURL without host = %q", got)
	}
}

func TestParseRepoArg(t *testing.T) {
	want := git.Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}

	tests := []struct {
		name    string
		arg     string
		wantErr bool
		repo    git.Repo
	}{
		{"shorthand", "ws/repo", false, want},
		{"shorthand .git", "ws/repo.git", false, want},
		{"url", "https://bitbucket.org/ws/repo", false, want},
		{"url .git", "https://bitbucket.org/ws/repo.git", false, want},
		{"url trailing path", "https://bitbucket.org/ws/repo/pull-requests/1", false, want},
		{"altssh url", "https://altssh.bitbucket.org/ws/repo", false, want},
		{"url trailing slash", "https://bitbucket.org/ws/repo/", false, want},
		{"surrounding space", "  ws/repo  ", false, want},
		{"scp", "git@bitbucket.org:ws/repo", false, want},
		{"scp .git", "git@bitbucket.org:ws/repo.git", false, want},
		{"ssh url", "ssh://git@bitbucket.org/ws/repo.git", false, want},
		{"ssh altssh port", "ssh://git@altssh.bitbucket.org:443/ws/repo.git", false, want},
		{"host shorthand", "bitbucket.org/ws/repo", false, want},
		{"host shorthand .git", "bitbucket.org/ws/repo.git", false, want},
		{"host shorthand uppercase", "BitBucket.org/ws/repo", false, want},

		{"empty", "", true, git.Repo{}},
		{"blank", "   ", true, git.Repo{}},
		{"single segment", "repo", true, git.Repo{}},
		{"too many segments", "a/b/c", true, git.Repo{}},
		{"host shorthand too many", "bitbucket.org/ws/repo/extra", true, git.Repo{}},
		{"non-bitbucket host shorthand", "github.com/ws/repo", true, git.Repo{}},
		{"leading slash", "/ws/repo", true, git.Repo{}},
		{"github url", "https://github.com/ws/repo", true, git.Repo{}},
		{"github scp", "git@github.com:ws/repo", true, git.Repo{}},
		{"scp too many", "git@bitbucket.org:ws/repo/extra", true, git.Repo{}},
		{"ssh url too many", "ssh://git@bitbucket.org/ws/repo/extra", true, git.Repo{}},
		{"url single segment", "https://bitbucket.org/ws", true, git.Repo{}},
		{"missing name", "ws/", true, git.Repo{}},
		{"missing workspace", "/repo", true, git.Repo{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := git.ParseRepoArg(tt.arg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseRepoArg(%q) expected error, got %+v", tt.arg, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseRepoArg(%q) unexpected error: %v", tt.arg, err)
			}
			if repo != tt.repo {
				t.Fatalf("ParseRepoArg(%q) = %+v, want %+v", tt.arg, repo, tt.repo)
			}
		})
	}
}

func remoteVOutput(pairs ...[2]string) string {
	var b []byte
	for _, p := range pairs {
		name, url := p[0], p[1]
		b = append(b, []byte(name+"\t"+url+" (fetch)\n")...)
		b = append(b, []byte(name+"\t"+url+" (push)\n")...)
	}
	return string(b)
}

func TestResolveRepo(t *testing.T) {
	ctx := t.Context()
	origin := git.Repo{Host: "bitbucket.org", Workspace: "ows", Name: "repo"}
	upstream := git.Repo{Host: "bitbucket.org", Workspace: "ups", Name: "repo"}

	t.Run("upstream over origin", func(t *testing.T) {
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
			[2]string{"upstream", "https://bitbucket.org/ups/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := git.ResolveRepo(ctx, s, "")
		if err != nil {
			t.Fatal(err)
		}
		if repo != upstream {
			t.Fatalf("repo = %+v, want %+v", repo, upstream)
		}
		if rr == nil || rr.Remote.Name != "upstream" {
			t.Fatalf("resolved remote = %+v, want upstream", rr)
		}
	})

	t.Run("origin when no upstream", func(t *testing.T) {
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := git.ResolveRepo(ctx, s, "")
		if err != nil {
			t.Fatal(err)
		}
		if repo != origin {
			t.Fatalf("repo = %+v, want %+v", repo, origin)
		}
		if rr == nil || rr.Remote.Name != "origin" {
			t.Fatalf("resolved remote = %+v, want origin", rr)
		}
	})

	t.Run("first bitbucket remote", func(t *testing.T) {
		s := gittest.New().Register(remoteVOutput(
			[2]string{"gh", "git@github.com:x/y.git"},
			[2]string{"bb", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := git.ResolveRepo(ctx, s, "")
		if err != nil {
			t.Fatal(err)
		}
		if repo != origin {
			t.Fatalf("repo = %+v, want %+v", repo, origin)
		}
		if rr == nil || rr.Remote.Name != "bb" {
			t.Fatalf("resolved remote = %+v, want bb", rr)
		}
	})

	t.Run("override flag", func(t *testing.T) {
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := git.ResolveRepo(ctx, s, "flag/repo")
		if err != nil {
			t.Fatal(err)
		}
		if want := (git.Repo{Host: "bitbucket.org", Workspace: "flag", Name: "repo"}); repo != want {
			t.Fatalf("repo = %+v, want %+v", repo, want)
		}
		if rr != nil {
			t.Fatalf("resolved remote = %+v, want nil (no matching remote)", rr)
		}
	})

	t.Run("override matches remote", func(t *testing.T) {
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		_, rr, err := git.ResolveRepo(ctx, s, "ows/repo")
		if err != nil {
			t.Fatal(err)
		}
		if rr == nil || rr.Remote.Name != "origin" {
			t.Fatalf("resolved remote = %+v, want origin", rr)
		}
	})

	t.Run("override matches scp remote via URL form", func(t *testing.T) {
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "https://bitbucket.org/ows/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := git.ResolveRepo(ctx, s, "git@bitbucket.org:ows/repo.git")
		if err != nil {
			t.Fatal(err)
		}
		if repo != origin {
			t.Fatalf("repo = %+v, want %+v", repo, origin)
		}
		if rr == nil || rr.Remote.Name != "origin" {
			t.Fatalf("resolved remote = %+v, want origin", rr)
		}
	})

	t.Run("override outside git checkout", func(t *testing.T) {
		notRepo := &git.Error{Args: []string{"remote", "-v"}, ExitCode: 128, Stderr: "fatal: not a git repository"}
		s := gittest.New().Register("", notRepo, "remote", "-v")

		repo, rr, err := git.ResolveRepo(ctx, s, "ws/repo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := (git.Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}); repo != want {
			t.Fatalf("repo = %+v, want %+v", repo, want)
		}
		if rr != nil {
			t.Fatalf("resolved remote = %+v, want nil", rr)
		}
	})

	t.Run("override with nil runner", func(t *testing.T) {
		repo, rr, err := git.ResolveRepo(ctx, nil, "bitbucket.org/ws/repo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if want := (git.Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}); repo != want {
			t.Fatalf("repo = %+v, want %+v", repo, want)
		}
		if rr != nil {
			t.Fatalf("resolved remote = %+v, want nil", rr)
		}
	})

	t.Run("invalid override does not run git", func(t *testing.T) {
		s := gittest.New()
		s.FailUnstubbed = true

		if _, _, err := git.ResolveRepo(ctx, s, "not-a-repo"); err == nil {
			t.Fatal("expected error, got nil")
		}
		if len(s.Calls) != 0 {
			t.Fatalf("git calls = %v, want none", s.CallStrings())
		}
	})

	t.Run("remotes error without override", func(t *testing.T) {
		notRepo := &git.Error{Args: []string{"remote", "-v"}, ExitCode: 128, Stderr: "fatal: not a git repository"}
		s := gittest.New().Register("", notRepo, "remote", "-v")

		_, _, err := git.ResolveRepo(ctx, s, "")
		if !errors.Is(err, notRepo) {
			t.Fatalf("error = %v, want %v", err, notRepo)
		}
	})

	t.Run("nil runner without override", func(t *testing.T) {
		if _, _, err := git.ResolveRepo(ctx, nil, ""); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("no bitbucket remote", func(t *testing.T) {
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@github.com:x/y.git"},
		), nil, "remote", "-v")

		_, _, err := git.ResolveRepo(ctx, s, "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		const wantMsg = "none of the git remotes point to a Bitbucket repository; use `-R ws/repo` or set BH_REPO"
		if err.Error() != wantMsg {
			t.Fatalf("error = %q, want %q", err.Error(), wantMsg)
		}
	})
}

func TestBitbucketRemotes(t *testing.T) {
	remotes := []git.Remote{
		{Name: "gh", FetchURL: "git@github.com:x/y.git", PushURL: "git@github.com:x/y.git"},
		{Name: "origin", FetchURL: "/local/mirror", PushURL: "git@bitbucket.org:ows/repo.git"},
	}
	got := git.BitbucketRemotes(remotes)
	if len(got) != 1 {
		t.Fatalf("got %d remotes, want 1: %+v", len(got), got)
	}
	want := git.ResolvedRemote{
		Remote: remotes[1],
		Repo:   git.Repo{Host: "bitbucket.org", Workspace: "ows", Name: "repo"},
	}
	if got[0] != want {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
}
