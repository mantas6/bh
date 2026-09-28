package pr

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
)

// deleteLocalSourceBranch removes the PR's source branch locally after a merge
// or decline. If it is the current branch, HEAD is first switched to dest. A
// success line is written to ErrOut when a branch is actually deleted.
//
// With force the branch is deleted with `git branch -D`. Otherwise
// `git branch -d` is tried first; when git refuses (typically because the
// branch is not fully merged) the user is asked whether to force-delete it
// when running interactively, and otherwise a warning is printed and the
// branch is kept.
func deleteLocalSourceBranch(ctx context.Context, g git.Runner, ios *cmdutil.IOStreams, source, dest string, force bool) error {
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

	cs := ios.ErrColorScheme()
	if err := git.DeleteLocalBranch(ctx, g, source, force); err != nil {
		var gitErr *git.Error
		if force || !errors.As(err, &gitErr) {
			return err
		}

		forceDelete := false
		if ios.IsStdinTTY() {
			forceDelete, err = ios.Prompter().Confirm(
				fmt.Sprintf("Local branch %s is not fully merged. Delete it anyway?", source), false)
			if err != nil {
				return err
			}
		}
		if !forceDelete {
			fmt.Fprintf(ios.ErrOut, "%s Kept local branch %s because it is not fully merged; delete it with `git branch -D %s`\n",
				cs.WarningIcon(), source, source)
			return nil
		}
		if err := git.DeleteLocalBranch(ctx, g, source, true); err != nil {
			return err
		}
	}

	fmt.Fprintf(ios.ErrOut, "%s Deleted local branch %s\n", cs.SuccessIcon(), source)
	return nil
}
