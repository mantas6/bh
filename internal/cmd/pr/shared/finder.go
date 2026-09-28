package shared

import (
	"context"
	"errors"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/git"
)

// Finder resolves a pull request argument to a pull request. Its fields
// mirror the corresponding cmdutil.Factory providers and are only called when
// needed: BaseRepo is skipped when the argument is a URL, and Git is only
// required when the argument is empty (the current branch).
type Finder struct {
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
}

// NewFinder returns a Finder using the given providers.
func NewFinder(
	baseRepo func() (git.Repo, *git.ResolvedRemote, error),
	apiClient func() (*api.Client, error),
	gitFn func() (git.Runner, error),
) *Finder {
	return &Finder{BaseRepo: baseRepo, APIClient: apiClient, Git: gitFn}
}

// FoundPR is the result of Finder.Find.
type FoundPR struct {
	// PR is the resolved pull request.
	PR *api.PullRequest
	// Repo is the repository the pull request was resolved against: the
	// repo named by a URL argument, otherwise the base repository.
	Repo git.Repo
	// Remote is the local git remote pointing at Repo, or nil when none is
	// known.
	Remote *git.ResolvedRemote
	// Client is the API client used for the lookup, for follow-up calls.
	Client *api.Client
}

// Find resolves arg (see ParsePRArg) to a pull request. A URL argument
// selects its own repository; otherwise the base repository is used. A branch
// argument, or the current branch when arg is empty, is looked up as the
// source branch of an open pull request. A detached HEAD with an empty arg is
// an error.
func (f *Finder) Find(ctx context.Context, arg string) (*FoundPR, error) {
	sel, err := ParsePRArg(arg)
	if err != nil {
		return nil, err
	}

	found := &FoundPR{}
	if sel.Repo != nil {
		found.Repo = *sel.Repo
		found.Remote = f.matchingRemote(ctx, found.Repo)
	} else {
		found.Repo, found.Remote, err = f.BaseRepo()
		if err != nil {
			return nil, err
		}
	}

	found.Client, err = f.APIClient()
	if err != nil {
		return nil, err
	}

	branch := sel.Branch
	if sel.IsCurrentBranch() {
		gitRunner, err := f.Git()
		if err != nil {
			return nil, err
		}
		branch, err = git.CurrentBranch(ctx, gitRunner)
		if err != nil {
			if errors.Is(err, git.ErrNotOnBranch) {
				return nil, errors.New("no pull request specified and not on a branch")
			}
			return nil, err
		}
	}

	if branch != "" {
		found.PR, err = found.Client.PullRequestForBranch(ctx, found.Repo.FullName(), branch)
	} else {
		found.PR, err = found.Client.PullRequest(ctx, found.Repo.FullName(), sel.Number)
	}
	if err != nil {
		return nil, err
	}
	return found, nil
}

// matchingRemote returns the local git remote pointing at repo. Git is
// optional here: when it is unavailable, or the working directory is not a
// checkout with such a remote, nil is returned.
func (f *Finder) matchingRemote(ctx context.Context, repo git.Repo) *git.ResolvedRemote {
	if f.Git == nil {
		return nil
	}
	gitRunner, err := f.Git()
	if err != nil || gitRunner == nil {
		return nil
	}
	_, rr, err := git.ResolveRepo(ctx, gitRunner, repo.Host+"/"+repo.FullName())
	if err != nil {
		return nil
	}
	return rr
}
