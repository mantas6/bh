package git

import (
	"context"
	"testing"

	"github.com/mantas6/bh/internal/git/gittest"
)

func TestParseRemoteURL(t *testing.T) {
	want := Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}

	tests := []struct {
		name string
		raw  string
		ok   bool
		repo Repo
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

		{"github", "https://github.com/ws/repo.git", false, Repo{}},
		{"github scp", "git@github.com:ws/repo.git", false, Repo{}},
		{"local path", "/home/user/repo", false, Repo{}},
		{"relative path", "../repo", false, Repo{}},
		{"empty", "", false, Repo{}},
		{"too few segments", "https://bitbucket.org/ws.git", false, Repo{}},
		{"too many segments", "https://bitbucket.org/ws/repo/extra.git", false, Repo{}},
		{"scp too many", "git@bitbucket.org:ws/repo/extra.git", false, Repo{}},
		{"no host", "https://ws/repo.git", false, Repo{}},
		{"missing name", "https://bitbucket.org/ws/.git", false, Repo{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, ok := ParseRemoteURL(tt.raw)
			if ok != tt.ok {
				t.Fatalf("ParseRemoteURL(%q) ok = %v, want %v", tt.raw, ok, tt.ok)
			}
			if ok && repo != tt.repo {
				t.Fatalf("ParseRemoteURL(%q) = %+v, want %+v", tt.raw, repo, tt.repo)
			}
		})
	}
}

func TestParseRepoArg(t *testing.T) {
	want := Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}

	tests := []struct {
		name    string
		arg     string
		wantErr bool
		repo    Repo
	}{
		{"shorthand", "ws/repo", false, want},
		{"shorthand .git", "ws/repo.git", false, want},
		{"url", "https://bitbucket.org/ws/repo", false, want},
		{"url .git", "https://bitbucket.org/ws/repo.git", false, want},
		{"url trailing path", "https://bitbucket.org/ws/repo/pull-requests/1", false, want},
		{"altssh url", "https://altssh.bitbucket.org/ws/repo", false, want},

		{"empty", "", true, Repo{}},
		{"single segment", "repo", true, Repo{}},
		{"too many segments", "a/b/c", true, Repo{}},
		{"github url", "https://github.com/ws/repo", true, Repo{}},
		{"missing name", "ws/", true, Repo{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, err := ParseRepoArg(tt.arg)
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
	ctx := context.Background()
	origin := Repo{Host: "bitbucket.org", Workspace: "ows", Name: "repo"}
	upstream := Repo{Host: "bitbucket.org", Workspace: "ups", Name: "repo"}

	t.Run("upstream over origin", func(t *testing.T) {
		t.Setenv("BH_REPO", "")
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
			[2]string{"upstream", "https://bitbucket.org/ups/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := ResolveRepo(ctx, s, "")
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
		t.Setenv("BH_REPO", "")
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := ResolveRepo(ctx, s, "")
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
		t.Setenv("BH_REPO", "")
		s := gittest.New().Register(remoteVOutput(
			[2]string{"gh", "git@github.com:x/y.git"},
			[2]string{"bb", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := ResolveRepo(ctx, s, "")
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
		t.Setenv("BH_REPO", "")
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		repo, rr, err := ResolveRepo(ctx, s, "flag/repo")
		if err != nil {
			t.Fatal(err)
		}
		if want := (Repo{Host: "bitbucket.org", Workspace: "flag", Name: "repo"}); repo != want {
			t.Fatalf("repo = %+v, want %+v", repo, want)
		}
		if rr != nil {
			t.Fatalf("resolved remote = %+v, want nil (no matching remote)", rr)
		}
	})

	t.Run("override matches remote", func(t *testing.T) {
		t.Setenv("BH_REPO", "")
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		_, rr, err := ResolveRepo(ctx, s, "ows/repo")
		if err != nil {
			t.Fatal(err)
		}
		if rr == nil || rr.Remote.Name != "origin" {
			t.Fatalf("resolved remote = %+v, want origin", rr)
		}
	})

	t.Run("env over remotes", func(t *testing.T) {
		t.Setenv("BH_REPO", "envws/repo")
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@bitbucket.org:ows/repo.git"},
		), nil, "remote", "-v")

		repo, _, err := ResolveRepo(ctx, s, "")
		if err != nil {
			t.Fatal(err)
		}
		if want := (Repo{Host: "bitbucket.org", Workspace: "envws", Name: "repo"}); repo != want {
			t.Fatalf("repo = %+v, want %+v", repo, want)
		}
	})

	t.Run("flag over env", func(t *testing.T) {
		t.Setenv("BH_REPO", "envws/repo")
		s := gittest.New().Register("", nil, "remote", "-v")

		repo, _, err := ResolveRepo(ctx, s, "flagws/repo")
		if err != nil {
			t.Fatal(err)
		}
		if want := (Repo{Host: "bitbucket.org", Workspace: "flagws", Name: "repo"}); repo != want {
			t.Fatalf("repo = %+v, want %+v", repo, want)
		}
	})

	t.Run("no bitbucket remote", func(t *testing.T) {
		t.Setenv("BH_REPO", "")
		s := gittest.New().Register(remoteVOutput(
			[2]string{"origin", "git@github.com:x/y.git"},
		), nil, "remote", "-v")

		_, _, err := ResolveRepo(ctx, s, "")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		const wantMsg = "none of the git remotes point to a Bitbucket repository; use `-R ws/repo` or set BH_REPO"
		if err.Error() != wantMsg {
			t.Fatalf("error = %q, want %q", err.Error(), wantMsg)
		}
	})
}
