// Package root defines the top-level bh command.
package root

import (
	"github.com/mantas6/bh/internal/cmd/auth"
	"github.com/mantas6/bh/internal/cmd/pr"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdRoot creates the root "bh" command.
func NewCmdRoot(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bh <command> <subcommand> [flags]",
		Short: "Bitbucket CLI",
		Long:  "Work with Bitbucket Cloud pull requests from the command line.",
		Example: cmdutil.Heredoc(`
			$ bh pr list
			$ bh pr view 123
			$ bh pr create --fill
			$ bh auth login
		`),
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       f.Version,
	}

	cmd.SetVersionTemplate("bh version {{.Version}}\n")
	cmd.SetOut(f.IOStreams.Out)
	cmd.SetErr(f.IOStreams.ErrOut)
	cmd.SetIn(f.IOStreams.In)

	// Wrap cobra's flag-parsing errors so the top level can print a usage hint
	// and select the correct exit code.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return cmdutil.FlagErrorWrap(err)
	})

	// Global repository override, consumed by f.BaseRepo.
	cmd.PersistentFlags().StringVarP(&f.RepoOverride, "repo", "R", "", "Select a repository using the `[HOST/]OWNER/REPO` format")

	// Cobra registers a --version flag automatically because Version is set.
	cmd.AddCommand(auth.NewCmdAuth(f))
	cmd.AddCommand(pr.NewCmdPR(f))
	// Additional subcommand groups are added by later steps.

	return cmd
}
