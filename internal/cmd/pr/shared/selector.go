// Package shared holds helpers used by both the `bh pr` commands and the
// `bh pr comment` subcommands: parsing pull request selectors, resolving them
// to a pull request, and small output/input utilities. It is a leaf package so
// that package pr and its comment subpackage can both import it without a
// cycle.
package shared

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/mantas6/bh/internal/git"
)

// PRSelector identifies a pull request as given on the command line. At most
// one of Number or Branch is set; when both are zero the selector refers to
// the pull request for the current git branch.
type PRSelector struct {
	// Number is the pull request id when selected by number, "#number", or
	// URL.
	Number int
	// Branch is the source branch name when selected by branch.
	Branch string
	// Repo is the repository named by a pull request URL, or nil.
	Repo *git.Repo
}

// IsCurrentBranch reports whether the selector is empty, meaning the pull
// request for the currently checked-out branch.
func (s PRSelector) IsCurrentBranch() bool {
	return s.Number == 0 && s.Branch == ""
}

// ParsePRArg parses a pull request argument. It accepts a number ("123"), a
// "#123" form, or a Bitbucket pull request URL; anything else is treated as
// the name of the pull request's source branch. An empty argument yields an
// empty selector (the current branch).
func ParsePRArg(arg string) (PRSelector, error) {
	arg = strings.TrimSpace(arg)
	switch {
	case arg == "":
		return PRSelector{}, nil
	case strings.Contains(arg, "://"):
		return parsePRURL(arg)
	case strings.HasPrefix(arg, "#"):
		n, err := strconv.Atoi(arg[1:])
		if err != nil || n <= 0 {
			return PRSelector{}, fmt.Errorf("invalid pull request number %q", arg)
		}
		return PRSelector{Number: n}, nil
	}

	if n, err := strconv.Atoi(arg); err == nil {
		if n <= 0 {
			return PRSelector{}, fmt.Errorf("invalid pull request number %q", arg)
		}
		return PRSelector{Number: n}, nil
	}

	if strings.HasPrefix(arg, "-") || strings.ContainsAny(arg, " \t\n") {
		return PRSelector{}, fmt.Errorf("invalid pull request argument %q", arg)
	}
	return PRSelector{Branch: arg}, nil
}

func parsePRURL(raw string) (PRSelector, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return PRSelector{}, fmt.Errorf("invalid pull request URL %q: %w", raw, err)
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	idx := -1
	for i, p := range parts {
		if p == "pull-requests" {
			idx = i
			break
		}
	}
	if idx < 2 || idx+1 >= len(parts) {
		return PRSelector{}, fmt.Errorf("invalid pull request URL %q", raw)
	}
	n, err := strconv.Atoi(parts[idx+1])
	if err != nil || n <= 0 {
		return PRSelector{}, fmt.Errorf("invalid pull request URL %q", raw)
	}
	repo, err := git.ParseRepoArg(raw)
	if err != nil {
		return PRSelector{}, err
	}
	return PRSelector{Number: n, Repo: &repo}, nil
}
