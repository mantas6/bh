package shared

import (
	"io"
	"os"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
)

// UserName returns the name bh displays for a user: the nickname, falling
// back to the display name. A nil user yields "".
func UserName(u *api.User) string {
	if u == nil {
		return ""
	}
	if u.Nickname != "" {
		return u.Nickname
	}
	return u.DisplayName
}

// PRAuthor returns the display name (see UserName) of a pull request's
// author.
func PRAuthor(pr *api.PullRequest) string {
	return UserName(pr.Author)
}

// SameRepoPR reports whether the pull request's source repository is repo,
// i.e. it is not from a fork.
func SameRepoPR(pr *api.PullRequest, repo git.Repo) bool {
	return pr.Source.Repository != nil &&
		strings.EqualFold(pr.Source.Repository.FullName, repo.FullName())
}

// ReadBodyFile reads a --body-file value; "-" reads standard input (through
// the shared ios.Stdin reader).
func ReadBodyFile(ios *cmdutil.IOStreams, path string) (string, error) {
	var (
		b   []byte
		err error
	)
	if path == "-" {
		b, err = io.ReadAll(ios.Stdin())
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}
