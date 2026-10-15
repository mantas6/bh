package root

import (
	"fmt"
	"io"
	"strings"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// newCmdReference creates `bh reference`, which prints every command with its
// usage line and flags in one markdown-ish stream (modelled on `gh
// reference`). Its help is the reference itself, so `bh help reference` and
// `bh reference --help` print the same text.
func newCmdReference() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reference",
		Short: "Print a reference of every command and its flags",
		Long: cmdutil.Heredoc(`
			Print every bh command with its usage line, aliases and flags in a
			single markdown-formatted stream. Also available as ` + "`bh help reference`" + `.
		`),
		Args:                  cmdutil.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(c *cobra.Command, args []string) error {
			return writeReference(c.OutOrStdout(), c.Root())
		},
	}
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		_ = writeReference(c.OutOrStdout(), c.Root())
	})
	return cmd
}

// writeReference writes the reference for the command tree under root.
func writeReference(w io.Writer, root *cobra.Command) error {
	var b strings.Builder
	name := root.Name()
	fmt.Fprintf(&b, "# %s reference\n\n", name)
	fmt.Fprintf(&b, "Run `%s <command> --help` for the full description and examples of a command.\n", name)
	for _, c := range root.Commands() {
		writeReferenceCommand(&b, c, 2)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeReferenceCommand writes the section for cmd, with a heading of the
// given depth, followed by the sections of its subcommands one level deeper.
// Groups get their own section only for their short description and the flags
// they pass on to every subcommand.
func writeReferenceCommand(b *strings.Builder, cmd *cobra.Command, depth int) {
	if !cmd.IsAvailableCommand() || isCompletionCommand(cmd) {
		return
	}
	usage := cmd.UseLine()
	if cmd.HasAvailableSubCommands() {
		usage = cmd.CommandPath() + " <command>"
	}
	fmt.Fprintf(b, "\n%s `%s`\n\n%s\n", strings.Repeat("#", depth), usage, cmd.Short)
	if len(cmd.Aliases) > 0 {
		fmt.Fprintf(b, "\nAliases: %s\n", strings.Join(cmd.Aliases, ", "))
	}

	if cmd.HasAvailableSubCommands() {
		if flags := referenceFlags(cmd.PersistentFlags()); flags != "" {
			fmt.Fprintf(b, "\nFlags for every `%s` command:\n\n```\n%s```\n", cmd.CommandPath(), flags)
		}
		for _, c := range cmd.Commands() {
			writeReferenceCommand(b, c, depth+1)
		}
		return
	}
	if flags := referenceFlags(cmd.NonInheritedFlags()); flags != "" {
		fmt.Fprintf(b, "\n```\n%s```\n", flags)
	}
}

// isCompletionCommand reports whether cmd is cobra's generated `completion`
// command, left out of the reference as it is not about Bitbucket.
func isCompletionCommand(cmd *cobra.Command) bool {
	return cmd.Name() == "completion" && cmd.HasParent() && !cmd.Parent().HasParent()
}

// referenceFlags returns the usage lines of fs without --help (present on
// every command), or "" when nothing is left.
func referenceFlags(fs *pflag.FlagSet) string {
	out := pflag.NewFlagSet("reference", pflag.ContinueOnError)
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Name != "help" && !f.Hidden {
			out.AddFlag(f)
		}
	})
	return out.FlagUsages()
}
