// Package root defines the top-level bh command.
package root

import (
	"github.com/mantas6/bh/internal/cmd/auth"
	"github.com/mantas6/bh/internal/cmd/pr"
	"github.com/mantas6/bh/internal/cmd/version"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdRoot creates the root "bh" command.
func NewCmdRoot(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bh <command> <subcommand> [flags]",
		Short: "Bitbucket CLI",
		Long: cmdutil.Heredoc(`
			Work with Bitbucket Cloud pull requests from the command line.

			Run ` + "`bh help reference`" + ` for every command and its flags at once.
		`),
		Example: cmdutil.Examples(`
			$ bh pr list --author @me
			$ bh pr view 123 --comments

			# Answer review comments: ids come from the "#id author · time" headers
			$ bh pr comment list 123 --needs-reply
			$ bh pr comment reply 123 881626438 --body "Fixed in 63a5b8b" --resolve

			# Retarget a pull request; set a description from standard input
			$ bh pr edit 123 --base dev
			$ bh pr create --push --title "Add login form" --body-file - <<'EOF'
			Adds the login form.
			EOF

			$ bh help reference
		`),
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       f.Version,
	}

	cmd.SetVersionTemplate(version.Template)
	cmd.SetOut(f.IOStreams.Out)
	cmd.SetErr(f.IOStreams.ErrOut)
	cmd.SetIn(f.IOStreams.In)

	// Wrap cobra's flag-parsing errors so the top level can print a usage hint
	// and select the correct exit code.
	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return cmdutil.FlagErrorWrap(err)
	})

	// Cobra registers a --version flag automatically because Version is set.
	cmd.AddCommand(auth.NewCmdAuth(f))
	cmd.AddCommand(pr.NewCmdPR(f))
	cmd.AddCommand(version.NewCmdVersion(f))
	cmd.AddCommand(newCmdReference())

	cmdutil.SetupGroup(cmd, nil)

	return cmd
}
