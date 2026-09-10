package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// CheckoutOptions holds the dependencies and flags for `bh pr checkout`.
type CheckoutOptions struct {
	IO        *cmdutil.IOStreams
	ApiClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func() (git.Repo, *git.ResolvedRemote, error)

	Arg    string
	Branch string
	Detach bool
	Force  bool
}

// NewCmdCheckout creates the "pr checkout" command.
func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{
		IO:        f.IOStreams,
		ApiClient: f.ApiClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "checkout {<number> | <url>}",
		Short: "Check out a pull request in git",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Arg = args[0]
			if opts.Branch != "" && opts.Detach {
				return cmdutil.FlagErrorf("--branch and --detach are mutually exclusive")
			}
			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Local branch name to use (default: the PR's source branch)")
	cmd.Flags().BoolVar(&opts.Detach, "detach", false, "Check out the PR in a detached HEAD state")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Reset the existing local branch to the latest state of the PR")

	return cmd
}

func checkoutRun(opts *CheckoutOptions) error {
	ctx := context.Background()

	baseRepo, resolvedRemote, err := opts.BaseRepo()
	if err != nil {
		return err
	}
	client, err := opts.ApiClient()
	if err != nil {
		return err
	}
	gitRunner, err := opts.Git()
	if err != nil {
		return err
	}

	pr, baseRepo, err := FindPR(ctx, client, gitRunner, baseRepo, opts.Arg)
	if err != nil {
		return err
	}

	branch := pr.Source.Branch.Name
	if branch == "" {
		return fmt.Errorf("pull request #%d has no source branch", pr.ID)
	}
	localName := opts.Branch
	if localName == "" {
		localName = branch
	}

	if sameRepoPR(pr, baseRepo) {
		return checkoutSameRepo(ctx, opts, gitRunner, remoteName(resolvedRemote), branch, localName)
	}
	return checkoutFork(ctx, opts, gitRunner, resolvedRemote, pr, branch, localName)
}

// checkoutSameRepo checks out a PR whose source branch lives in the base repo.
func checkoutSameRepo(ctx context.Context, opts *CheckoutOptions, g git.Runner, remote, branch, localName string) error {
	if opts.Detach {
		if err := git.Fetch(ctx, g, remote, branch); err != nil {
			return err
		}
		return git.CheckoutDetach(ctx, g, "FETCH_HEAD")
	}

	refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", branch, remote, branch)
	if err := git.Fetch(ctx, g, remote, refspec); err != nil {
		return err
	}

	remoteRef := remote + "/" + branch
	if git.HasLocalBranch(ctx, g, localName) {
		if err := git.Checkout(ctx, g, localName); err != nil {
			return err
		}
		if opts.Force {
			return git.ResetHard(ctx, g, remoteRef)
		}
		return git.MergeFFOnly(ctx, g, remoteRef)
	}
	return git.CheckoutNewBranch(ctx, g, localName, "--track", remoteRef)
}

// checkoutFork checks out a PR whose source branch lives in a fork.
func checkoutFork(ctx context.Context, opts *CheckoutOptions, g git.Runner, rr *git.ResolvedRemote, pr *api.PullRequest, branch, localName string) error {
	baseURL := ""
	if rr != nil {
		baseURL = rr.Remote.FetchURL
	}
	cloneURL := forkCloneURL(pr.Source.Repository, baseURL)
	forkWS := forkWorkspace(pr.Source.Repository)
	forkRef := fmt.Sprintf("refs/remotes/%s/%s", forkWS, branch)

	refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", branch, forkWS, branch)
	if err := git.Fetch(ctx, g, cloneURL, refspec); err != nil {
		return err
	}

	if opts.Detach {
		return git.CheckoutDetach(ctx, g, forkRef)
	}

	if git.HasLocalBranch(ctx, g, localName) {
		if err := git.Checkout(ctx, g, localName); err != nil {
			return err
		}
		if opts.Force {
			if err := git.ResetHard(ctx, g, forkRef); err != nil {
				return err
			}
		}
	} else {
		if err := git.CheckoutNewBranch(ctx, g, localName, "--no-track", forkRef); err != nil {
			return err
		}
	}

	if err := git.SetConfig(ctx, g, "branch."+localName+".remote", cloneURL); err != nil {
		return err
	}
	return git.SetConfig(ctx, g, "branch."+localName+".merge", "refs/heads/"+branch)
}

// forkWorkspace returns the workspace slug of a fork repository.
func forkWorkspace(repo *api.Repository) string {
	if repo == nil {
		return ""
	}
	if ws := strings.SplitN(repo.FullName, "/", 2)[0]; ws != "" {
		return ws
	}
	return repo.FullName
}

// forkCloneURL selects the fork's clone URL matching the base remote's
// protocol (ssh vs https), synthesizing a Bitbucket Cloud URL when the API did
// not return clone links.
func forkCloneURL(repo *api.Repository, baseRemoteURL string) string {
	protocol := "https"
	if strings.HasPrefix(baseRemoteURL, "git@") || strings.HasPrefix(baseRemoteURL, "ssh://") {
		protocol = "ssh"
	}

	if repo != nil {
		for _, cl := range repo.Links.Clone {
			if strings.EqualFold(cl.Name, protocol) && cl.Href != "" {
				return cl.Href
			}
		}
	}

	full := ""
	if repo != nil {
		full = repo.FullName
	}
	if protocol == "ssh" {
		return fmt.Sprintf("git@bitbucket.org:%s.git", full)
	}
	return fmt.Sprintf("https://bitbucket.org/%s.git", full)
}
