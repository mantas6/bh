package comment

import (
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdComment creates the "pr comment" command group.
func NewCmdComment(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comment",
		Short: "Manage pull request comments",
	}

	cmd.AddCommand(NewCmdList(f, nil))
	cmd.AddCommand(NewCmdAdd(f, nil))
	cmd.AddCommand(NewCmdReply(f, nil))
	cmd.AddCommand(NewCmdDelete(f, nil))
	cmd.AddCommand(NewCmdResolve(f, nil))
	cmd.AddCommand(NewCmdReopen(f, nil))

	return cmd
}
