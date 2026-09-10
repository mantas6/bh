package git

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// bitbucketHost is the canonical Bitbucket Cloud host.
const bitbucketHost = "bitbucket.org"

// Repo identifies a Bitbucket repository.
type Repo struct {
	Host      string
	Workspace string
	Name      string
}

// FullName returns "workspace/name".
func (r Repo) FullName() string {
	return r.Workspace + "/" + r.Name
}

// String returns "host/workspace/name".
func (r Repo) String() string {
	return r.Host + "/" + r.FullName()
}

// normalizeHost maps alternate Bitbucket hostnames to the canonical host and
// reports whether the host belongs to Bitbucket Cloud.
func normalizeHost(host string) (string, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	// Strip a port if present.
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	switch host {
	case bitbucketHost, "altssh.bitbucket.org":
		return bitbucketHost, true
	}
	return host, false
}

// splitRepoPath splits a URL path into workspace and repo name. It strips a
// leading slash, a trailing slash, and a ".git" suffix, and requires exactly
// two segments.
func splitRepoPath(p string) (ws, name string, ok bool) {
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimSuffix(p, "/")
	p = strings.TrimSuffix(p, ".git")
	if p == "" {
		return "", "", false
	}
	parts := strings.Split(p, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	if parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// ParseRemoteURL parses a git remote URL into a Repo, returning false when the
// URL is not a recognized Bitbucket Cloud remote.
func ParseRemoteURL(raw string) (Repo, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Repo{}, false
	}

	// scp-like syntax: git@host:ws/repo.git
	if !strings.Contains(raw, "://") {
		at := strings.Index(raw, "@")
		colon := strings.Index(raw, ":")
		if colon > 0 && (at < 0 || at < colon) {
			host := raw[:colon]
			if at >= 0 {
				host = raw[at+1 : colon]
			}
			path := raw[colon+1:]
			normHost, ok := normalizeHost(host)
			if !ok {
				return Repo{}, false
			}
			ws, name, ok := splitRepoPath("/" + path)
			if !ok {
				return Repo{}, false
			}
			return Repo{Host: normHost, Workspace: ws, Name: name}, true
		}
		return Repo{}, false
	}

	u, err := url.Parse(raw)
	if err != nil {
		return Repo{}, false
	}
	switch u.Scheme {
	case "http", "https", "ssh", "git":
	default:
		return Repo{}, false
	}
	normHost, ok := normalizeHost(u.Host)
	if !ok {
		return Repo{}, false
	}
	ws, name, ok := splitRepoPath(u.Path)
	if !ok {
		return Repo{}, false
	}
	return Repo{Host: normHost, Workspace: ws, Name: name}, true
}

// ParseRepoArg parses a -R/--repo value: either "ws/repo" or a full Bitbucket
// URL (with an optional trailing path such as /pull-requests/1).
func ParseRepoArg(s string) (Repo, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Repo{}, errors.New("expected the \"[HOST/]OWNER/REPO\" format, got \"\"")
	}

	if strings.Contains(s, "://") {
		u, err := url.Parse(s)
		if err != nil {
			return Repo{}, fmt.Errorf("invalid repository URL %q: %w", s, err)
		}
		normHost, ok := normalizeHost(u.Host)
		if !ok {
			return Repo{}, fmt.Errorf("%q is not a Bitbucket repository", s)
		}
		p := strings.TrimPrefix(u.Path, "/")
		p = strings.TrimSuffix(p, ".git")
		parts := strings.Split(p, "/")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return Repo{}, fmt.Errorf("invalid repository URL %q", s)
		}
		return Repo{Host: normHost, Workspace: parts[0], Name: strings.TrimSuffix(parts[1], ".git")}, nil
	}

	// "ws/repo" shorthand (reject anything with a scheme-less host, extra
	// segments, or missing parts).
	p := strings.TrimSuffix(s, ".git")
	parts := strings.Split(p, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repo{}, fmt.Errorf("expected the \"OWNER/REPO\" format, got %q", s)
	}
	return Repo{Host: bitbucketHost, Workspace: parts[0], Name: parts[1]}, nil
}

// ResolvedRemote pairs a git Remote with the Bitbucket Repo it points to.
type ResolvedRemote struct {
	Remote
	Repo
}

// BitbucketRemotes filters remotes to those pointing at Bitbucket Cloud,
// preserving order.
func BitbucketRemotes(remotes []Remote) []ResolvedRemote {
	var out []ResolvedRemote
	for _, rem := range remotes {
		repo, ok := ParseRemoteURL(rem.FetchURL)
		if !ok {
			repo, ok = ParseRemoteURL(rem.PushURL)
		}
		if !ok {
			continue
		}
		out = append(out, ResolvedRemote{Remote: rem, Repo: repo})
	}
	return out
}

// ResolveRepo determines the base repository using the precedence:
// override (-R) > BH_REPO env > upstream remote > origin remote > first
// Bitbucket remote (in `git remote -v` order). When resolved from a flag or
// env value, the returned *ResolvedRemote is the matching git remote if one
// exists, otherwise nil.
func ResolveRepo(ctx context.Context, r Runner, override string) (Repo, *ResolvedRemote, error) {
	remotes, err := Remotes(ctx, r)
	if err != nil {
		return Repo{}, nil, err
	}
	bbRemotes := BitbucketRemotes(remotes)

	// findMatch returns the remote whose repo equals want, or nil.
	findMatch := func(want Repo) *ResolvedRemote {
		for i := range bbRemotes {
			if bbRemotes[i].Repo == want {
				return &bbRemotes[i]
			}
		}
		return nil
	}

	if override != "" {
		repo, err := ParseRepoArg(override)
		if err != nil {
			return Repo{}, nil, err
		}
		return repo, findMatch(repo), nil
	}

	if env := strings.TrimSpace(os.Getenv("BH_REPO")); env != "" {
		repo, err := ParseRepoArg(env)
		if err != nil {
			return Repo{}, nil, err
		}
		return repo, findMatch(repo), nil
	}

	byName := func(name string) *ResolvedRemote {
		for i := range bbRemotes {
			if bbRemotes[i].Remote.Name == name {
				return &bbRemotes[i]
			}
		}
		return nil
	}

	if rr := byName("upstream"); rr != nil {
		return rr.Repo, rr, nil
	}
	if rr := byName("origin"); rr != nil {
		return rr.Repo, rr, nil
	}
	if len(bbRemotes) > 0 {
		rr := &bbRemotes[0]
		return rr.Repo, rr, nil
	}

	return Repo{}, nil, errors.New("none of the git remotes point to a Bitbucket repository; use `-R ws/repo` or set BH_REPO")
}
