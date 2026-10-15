// Package pr implements the `bh pr` command group: list (ls), view, create,
// edit, checkout, merge, approve, review, decline and comment, along with the
// shared PR argument resolver.
package pr

import (
	"github.com/mantas6/bh/internal/cmd/pr/comment"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdPR creates the "pr" command group.
func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command>",
		Short: "Manage pull requests",
	}

	// Repository override for every pr subcommand, consumed by f.BaseRepo.
	cmd.PersistentFlags().StringVarP(&f.RepoOverride, "repo", "R", "", "Select a repository using the `[HOST/]OWNER/REPO` format or a repository URL")

	cmd.AddCommand(NewCmdList(f, nil))
	cmd.AddCommand(NewCmdView(f, nil))
	cmd.AddCommand(NewCmdCreate(f, nil))
	cmd.AddCommand(NewCmdEdit(f, nil))
	cmd.AddCommand(NewCmdCheckout(f, nil))
	cmd.AddCommand(NewCmdMerge(f, nil))
	cmd.AddCommand(NewCmdApprove(f, nil))
	cmd.AddCommand(NewCmdReview(f, nil))
	cmd.AddCommand(NewCmdDecline(f, nil))
	cmd.AddCommand(comment.NewCmdComment(f))

	cmdutil.SetupGroup(cmd, unknownCommandHints)

	return cmd
}

// unknownCommandHints point users (and agents guessing gh-style names) at the
// command to use instead when they type a `bh pr` subcommand that does not
// exist. An entry is only consulted while no real subcommand has that name.
var unknownCommandHints = map[string]string{
	"comments": "To list comments, run: bh pr comment list [<pr>], or bh pr view [<pr>] --comments",
	"diff":     "bh pr diff is not supported yet; run `bh pr checkout <pr>` and use git diff, or open the pull request with `bh pr view <pr> --web`",
	"commits":  "bh pr commits is not supported yet; run `bh pr checkout <pr>` and use git log, or open the pull request with `bh pr view <pr> --web`",
	"files":    "bh pr files is not supported yet; run `bh pr checkout <pr>` and use git diff --name-only, or open the pull request with `bh pr view <pr> --web`",
	"checks":   "bh pr checks is not supported yet; open the pull request with `bh pr view <pr> --web` to see build status",
	"status":   "bh pr status is not supported; use `bh pr list --author @me` or `bh pr view` for the current branch",
}
