package pr

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
)

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

	if current == source {
		if err := git.Checkout(ctx, g, dest); err != nil {
			return err
		}
	} else {
		exists, err := git.HasLocalBranch(ctx, g, source)
		if err != nil {
			return err
		}
		if !exists {
			return nil
		}
	}
	if err := git.DeleteLocalBranch(ctx, g, source); err != nil {
		return err
	}

	fmt.Fprintf(ios.ErrOut, "%s Deleted local branch %s\n", ios.ErrColorScheme().SuccessIcon(), source)
	return nil
}
