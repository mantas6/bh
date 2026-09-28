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
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
}

// NewFinder returns a Finder using the given providers.
func NewFinder(
	baseRepo func(context.Context) (git.Repo, *git.ResolvedRemote, error),
	apiClient func() (*api.Client, error),
	gitFn func() (git.Runner, error),
) *Finder {
	return &Finder{BaseRepo: baseRepo, APIClient: apiClient, Git: gitFn}
}

// FoundPR is the result of Finder.Find.
type FoundPR struct {
	// PR is the resolved pull request. It is nil when returned by FindID for
	// a numeric argument, since the pull request was not fetched.
	PR *api.PullRequest
	// Number is the pull request id. It is always set.
	Number int
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
	return f.find(ctx, arg, true)
}

// FindID is like Find for commands that only need the pull request id: when
// arg names the pull request by number or URL, the pull request itself is
// not fetched and FoundPR.PR is nil. Branch arguments (and the current
// branch) still require a lookup, in which case PR is set.
func (f *Finder) FindID(ctx context.Context, arg string) (*FoundPR, error) {
	return f.find(ctx, arg, false)
}

func (f *Finder) find(ctx context.Context, arg string, fetchByNumber bool) (*FoundPR, error) {
	sel, err := ParsePRArg(arg)
	if err != nil {
		return nil, err
	}

	found := &FoundPR{}
	if sel.Repo != nil {
		found.Repo = *sel.Repo
		found.Remote = f.matchingRemote(ctx, found.Repo)
	} else {
		found.Repo, found.Remote, err = f.BaseRepo(ctx)
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

	switch {
	case branch != "":
		found.PR, err = found.Client.PullRequestForBranch(ctx, found.Repo.FullName(), branch)
	case fetchByNumber:
		found.PR, err = found.Client.PullRequest(ctx, found.Repo.FullName(), sel.Number)
	default:
		found.Number = sel.Number
		return found, nil
	}
	if err != nil {
		return nil, err
	}
	found.Number = found.PR.ID
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
