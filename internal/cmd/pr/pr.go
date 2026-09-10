// Package pr implements the `bh pr` command group: list, view, create and edit
// pull requests, along with the shared PR argument resolver.
package pr

import (
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdPR creates the "pr" command group.
func NewCmdPR(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pr <command>",
		Short: "Manage pull requests",
	}

	cmd.AddCommand(NewCmdList(f, nil))
	cmd.AddCommand(NewCmdView(f, nil))
	cmd.AddCommand(NewCmdCreate(f, nil))
	cmd.AddCommand(NewCmdEdit(f, nil))

	return cmd
}
