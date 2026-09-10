package pr

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/output"
)

// browser is the minimal interface the pr commands need to open URLs.
type browser interface {
	Browse(string) error
}

// ParsePRArg parses a pull request argument. It accepts a bare number
// ("123"), a "#123" form, or a Bitbucket pull request URL. When the argument
// is a URL that carries the workspace/repo, that repo is returned too.
func ParsePRArg(arg string) (id int, repo *git.Repo, err error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0, nil, errors.New("no pull request number or URL provided")
	}

	if strings.Contains(arg, "://") {
		return parsePRURL(arg)
	}

	s := strings.TrimPrefix(arg, "#")
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, nil, fmt.Errorf("invalid pull request argument %q", arg)
	}
	return n, nil, nil
}

func parsePRURL(raw string) (int, *git.Repo, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return 0, nil, fmt.Errorf("invalid pull request URL %q: %w", raw, err)
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
		return 0, nil, fmt.Errorf("invalid pull request URL %q", raw)
	}
	n, err := strconv.Atoi(parts[idx+1])
	if err != nil || n <= 0 {
		return 0, nil, fmt.Errorf("invalid pull request URL %q", raw)
	}
	repo, err := git.ParseRepoArg(raw)
	if err != nil {
		return 0, nil, err
	}
	return n, &repo, nil
}

// FindPR resolves a pull request from arg against baseRepo. When arg is empty
// it looks up the PR for the current branch; a detached HEAD is an error. When
// arg is a URL carrying a repo, that repo is used instead of baseRepo. The
// repo the PR was resolved against is returned alongside it.
func FindPR(ctx context.Context, client *api.Client, gitRunner git.Runner, baseRepo git.Repo, arg string) (*api.PullRequest, git.Repo, error) {
	repo := baseRepo

	if strings.TrimSpace(arg) == "" {
		branch, err := git.CurrentBranch(ctx, gitRunner)
		if err != nil {
			if errors.Is(err, git.ErrNotOnBranch) {
				return nil, repo, errors.New("no pull request specified and not on a branch")
			}
			return nil, repo, err
		}
		pr, err := client.PullRequestForBranch(ctx, repo.FullName(), branch)
		if err != nil {
			return nil, repo, err
		}
		return pr, repo, nil
	}

	id, urlRepo, err := ParsePRArg(arg)
	if err != nil {
		return nil, repo, err
	}
	if urlRepo != nil {
		repo = *urlRepo
	}
	pr, err := client.PullRequest(ctx, repo.FullName(), id)
	if err != nil {
		return nil, repo, err
	}
	return pr, repo, nil
}

// prAuthor returns a PR author's nickname, falling back to display name.
func prAuthor(pr *api.PullRequest) string {
	if pr.Author == nil {
		return ""
	}
	if pr.Author.Nickname != "" {
		return pr.Author.Nickname
	}
	return pr.Author.DisplayName
}

// readLine reads a single line from r without the trailing newline.
func readLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// successIcon returns a green check mark, colored only when the stream allows.
func successIcon(ios *cmdutil.IOStreams) string {
	return output.NewColorScheme(ios.ColorEnabled()).Green("✓")
}

// sameRepoPR reports whether the PR's source repository is the base repo.
func sameRepoPR(pr *api.PullRequest, baseRepo git.Repo) bool {
	return pr.Source.Repository != nil &&
		strings.EqualFold(pr.Source.Repository.FullName, baseRepo.FullName())
}

// remoteName returns the resolved remote's name, defaulting to "origin".
func remoteName(rr *git.ResolvedRemote) string {
	if rr != nil && rr.Remote.Name != "" {
		return rr.Remote.Name
	}
	return "origin"
}

// deleteLocalSourceBranch removes the PR's source branch locally after a merge
// or decline. If it is the current branch, HEAD is first switched to dest. A
// success line is written to ErrOut when a branch is actually deleted.
func deleteLocalSourceBranch(ctx context.Context, g git.Runner, ios *cmdutil.IOStreams, source, dest string) error {
	current, err := git.CurrentBranch(ctx, g)
	if err != nil && !errors.Is(err, git.ErrNotOnBranch) {
		return err
	}

	switch {
	case current == source:
		if err := git.Checkout(ctx, g, dest); err != nil {
			return err
		}
		if err := git.DeleteLocalBranch(ctx, g, source); err != nil {
			return err
		}
	case git.HasLocalBranch(ctx, g, source):
		if err := git.DeleteLocalBranch(ctx, g, source); err != nil {
			return err
		}
	default:
		return nil
	}

	fmt.Fprintf(ios.ErrOut, "%s Deleted local branch %s\n", successIcon(ios), source)
	return nil
}

// readBodyFile reads a --body-file value; "-" reads from ios.In.
func readBodyFile(ios *cmdutil.IOStreams, path string) (string, error) {
	if path == "-" {
		b, err := io.ReadAll(ios.In)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
