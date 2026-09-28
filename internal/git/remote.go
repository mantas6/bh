package git

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/mantas6/bh/internal/api"
)

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
	case api.DefaultHost, "altssh.bitbucket.org":
		return api.DefaultHost, true
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

// ParseRepoArg parses a -R/--repo value. Accepted forms are "OWNER/REPO",
// "HOST/OWNER/REPO", any git remote URL accepted by ParseRemoteURL (https,
// ssh, git and scp-like "git@HOST:OWNER/REPO"), and a Bitbucket web URL with
// an optional trailing path such as /pull-requests/1.
func ParseRepoArg(s string) (Repo, error) {
	s = strings.TrimSpace(s)
	formatErr := func() error {
		return fmt.Errorf("expected the \"[HOST/]OWNER/REPO\" format or a repository URL, got %q", s)
	}
	if s == "" {
		return Repo{}, formatErr()
	}

	if repo, ok := ParseRemoteURL(s); ok {
		return repo, nil
	}

	if strings.Contains(s, "://") {
		// Not a plain remote URL; accept a Bitbucket web URL with extra
		// trailing path segments (e.g. a pull request link).
		u, err := url.Parse(s)
		if err != nil {
			return Repo{}, fmt.Errorf("invalid repository URL %q: %w", s, err)
		}
		normHost, ok := normalizeHost(u.Host)
		if !ok {
			return Repo{}, fmt.Errorf("%q is not a Bitbucket repository", s)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return Repo{}, fmt.Errorf("invalid repository URL %q", s)
		}
		parts := strings.SplitN(strings.TrimPrefix(u.Path, "/"), "/", 3)
		if len(parts) < 2 {
			return Repo{}, fmt.Errorf("invalid repository URL %q", s)
		}
		ws, name, ok := splitRepoPath(parts[0] + "/" + parts[1])
		if !ok {
			return Repo{}, fmt.Errorf("invalid repository URL %q", s)
		}
		return Repo{Host: normHost, Workspace: ws, Name: name}, nil
	}

	if strings.Contains(s, ":") {
		// scp-like syntax that ParseRemoteURL rejected.
		return Repo{}, fmt.Errorf("%q is not a Bitbucket repository", s)
	}

	// "[HOST/]OWNER/REPO" shorthand.
	host := api.DefaultHost
	p := s
	if parts := strings.Split(s, "/"); len(parts) == 3 {
		if parts[0] == "" {
			return Repo{}, formatErr()
		}
		normHost, ok := normalizeHost(parts[0])
		if !ok {
			return Repo{}, fmt.Errorf("%q is not a Bitbucket host", parts[0])
		}
		host = normHost
		p = parts[1] + "/" + parts[2]
	} else if strings.HasPrefix(s, "/") {
		return Repo{}, formatErr()
	}
	ws, name, ok := splitRepoPath(p)
	if !ok {
		return Repo{}, formatErr()
	}
	return Repo{Host: host, Workspace: ws, Name: name}, nil
}

// ResolvedRemote pairs a git Remote with the Bitbucket Repo it points to.
type ResolvedRemote struct {
	Remote Remote
	Repo   Repo
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
// override > upstream remote > origin remote > first Bitbucket remote (in
// `git remote -v` order). The override (e.g. the -R flag or BH_REPO) is parsed
// before git is consulted, so it works outside a git checkout; r may be nil in
// that case. When resolved from the override, the returned *ResolvedRemote is
// the matching git remote if one can be found, otherwise nil.
func ResolveRepo(ctx context.Context, r Runner, override string) (Repo, *ResolvedRemote, error) {
	if override = strings.TrimSpace(override); override != "" {
		repo, err := ParseRepoArg(override)
		if err != nil {
			return Repo{}, nil, err
		}
		return repo, matchingRemote(ctx, r, repo), nil
	}

	if r == nil {
		return Repo{}, nil, errors.New("no git runner available to resolve the repository; use `-R ws/repo` or set BH_REPO")
	}
	remotes, err := Remotes(ctx, r)
	if err != nil {
		return Repo{}, nil, err
	}
	bbRemotes := BitbucketRemotes(remotes)

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

// matchingRemote returns the Bitbucket remote pointing at want, or nil. Any
// failure to list remotes (no runner, not a git checkout, git missing) is
// treated as "no match".
func matchingRemote(ctx context.Context, r Runner, want Repo) *ResolvedRemote {
	if r == nil {
		return nil
	}
	remotes, err := Remotes(ctx, r)
	if err != nil {
		return nil
	}
	for _, rr := range BitbucketRemotes(remotes) {
		if rr.Repo == want {
			return &rr
		}
	}
	return nil
}
