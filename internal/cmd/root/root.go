// Package root defines the top-level bh command.
package root

import (
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdRoot creates the root "bh" command.
func NewCmdRoot(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "bh <command> <subcommand> [flags]",
		Short:         "Bitbucket CLI",
		Long:          "Work with Bitbucket Cloud from the command line.",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       f.Version,
	}

	cmd.SetVersionTemplate("bh version {{.Version}}\n")
	cmd.SetOut(f.IOStreams.Out)
	cmd.SetErr(f.IOStreams.ErrOut)
	cmd.SetIn(f.IOStreams.In)

	// Cobra registers a --version flag automatically because Version is set.
	// Subcommand groups are added by later steps.

	return cmd
}
