package pr

import (
	"context"
	"fmt"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmd/pr/shared"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

// CheckoutOptions holds the dependencies and flags for `bh pr checkout`.
type CheckoutOptions struct {
	IO        *cmdutil.IOStreams
	APIClient func() (*api.Client, error)
	Git       func() (git.Runner, error)
	BaseRepo  func(context.Context) (git.Repo, *git.ResolvedRemote, error)

	Arg    string
	Branch string
	Detach bool
	Force  bool
}

// NewCmdCheckout creates the "pr checkout" command.
func NewCmdCheckout(f *cmdutil.Factory, runF func(*CheckoutOptions) error) *cobra.Command {
	opts := &CheckoutOptions{
		IO:        f.IOStreams,
		APIClient: f.APIClient,
		Git:       f.Git,
		BaseRepo:  f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:   "checkout {<number> | <url> | <branch>}",
		Short: "Check out a pull request in git",
		Long: cmdutil.Heredoc(`
			Check out a pull request's source branch locally.

			For same-repository pull requests the source branch is fetched and
			checked out with tracking configured. For pull requests from a fork the
			branch is fetched from the fork's clone URL (matching the origin remote's
			protocol) since Bitbucket exposes no pull/* refs.
		`),
		Example: cmdutil.Heredoc(`
			# Check out PR 123
			$ bh pr checkout 123

			# Check out into a named local branch
			$ bh pr checkout 123 --branch review-123

			# Check out from a URL in a detached HEAD
			$ bh pr checkout https://bitbucket.org/ws/repo/pull-requests/123 --detach
		`),
		Args: cmdutil.ExactArgs(1, "a pull request number, URL, or branch is required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Arg = args[0]
			if err := cmdutil.MutuallyExclusive("--branch and --detach are mutually exclusive",
				cmd.Flags().Changed("branch"), opts.Detach); err != nil {
				return err
			}
			if runF != nil {
				return runF(opts)
			}
			return checkoutRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "Local branch name to use (default: the PR's source branch)")
	cmd.Flags().BoolVar(&opts.Detach, "detach", false, "Check out the PR in a detached HEAD state")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "Reset the existing local branch to the latest state of the PR")

	return cmd
}

func checkoutRun(ctx context.Context, opts *CheckoutOptions) error {
	gitRunner, err := opts.Git()
	if err != nil {
		return err
	}

	found, err := shared.NewFinder(opts.BaseRepo, opts.APIClient, opts.Git).Find(ctx, opts.Arg)
	if err != nil {
		return err
	}
	pr := found.PR

	branch := pr.Source.Branch.Name
	if branch == "" {
		return fmt.Errorf("pull request #%d has no source branch", pr.ID)
	}
	localName := opts.Branch
	if localName == "" {
		localName = branch
	}

	if pr.Source.Repository == nil || pr.Source.Repository.FullName == "" {
		return fmt.Errorf("the source repository of pull request #%d is no longer available (was the fork deleted?)", pr.ID)
	}

	// found.Remote is the local remote for the PR's repository (re-resolved
	// for a URL argument naming another repository). Without one the branch
	// is fetched by URL, the same way as for a fork.
	if shared.SameRepoPR(pr, found.Repo) && found.Remote != nil {
		return checkoutSameRepo(ctx, opts, gitRunner, found.Remote.Remote.Name, branch, localName)
	}
	return checkoutFork(ctx, opts, gitRunner, found.Remote, pr, branch, localName)
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
	exists, err := git.HasLocalBranch(ctx, g, localName)
	if err != nil {
		return err
	}
	if exists {
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

// checkoutFork checks out a PR by fetching its source branch from the source
// repository's clone URL: used for forks, and for same-repository PRs when no
// local remote points at the repository.
func checkoutFork(ctx context.Context, opts *CheckoutOptions, g git.Runner, rr *git.ResolvedRemote, pr *api.PullRequest, branch, localName string) error {
	baseURL := ""
	if rr != nil {
		baseURL = rr.Remote.FetchURL
	}
	cloneURL, err := forkCloneURL(pr.Source.Repository, baseURL)
	if err != nil {
		return err
	}
	forkWS := forkWorkspace(pr.Source.Repository)
	forkRef := fmt.Sprintf("refs/remotes/%s/%s", forkWS, branch)

	refspec := fmt.Sprintf("+refs/heads/%s:refs/remotes/%s/%s", branch, forkWS, branch)
	if err := git.Fetch(ctx, g, cloneURL, refspec); err != nil {
		return err
	}

	if opts.Detach {
		return git.CheckoutDetach(ctx, g, forkRef)
	}

	exists, err := git.HasLocalBranch(ctx, g, localName)
	if err != nil {
		return err
	}
	if exists {
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
// protocol (SSH, in any of git's spellings, vs HTTPS), synthesizing a
// Bitbucket Cloud URL when the API did not return clone links.
func forkCloneURL(repo *api.Repository, baseRemoteURL string) (string, error) {
	ssh := git.IsSSHURL(baseRemoteURL)
	protocol := "https"
	if ssh {
		protocol = "ssh"
	}

	for _, cl := range repo.Links.Clone {
		if strings.EqualFold(cl.Name, protocol) && cl.Href != "" {
			return cl.Href, nil
		}
	}

	r, err := git.ParseRepoArg(repo.FullName)
	if err != nil {
		return "", fmt.Errorf("cannot determine the clone URL of %q: %w", repo.FullName, err)
	}
	return r.CloneURL(ssh), nil
}
