// Package auth implements the `bh auth` command group: login, logout, status
// and token.
package auth

import (
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdAuth creates the "auth" command group.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth <command>",
		Short: "Authenticate bh with Bitbucket",
	}

	cmd.AddCommand(NewCmdLogin(f, nil))
	cmd.AddCommand(NewCmdLogout(f, nil))
	cmd.AddCommand(NewCmdStatus(f, nil))
	cmd.AddCommand(NewCmdToken(f, nil))

	return cmd
}
