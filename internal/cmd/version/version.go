// Package version implements the "bh version" command.
package version

import (
	"fmt"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// Template is the cobra version template used for "bh --version". It renders
// the same text as "bh version".
const Template = "bh version {{.Version}}\n"

// NewCmdVersion creates the "version" command.
func NewCmdVersion(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show the bh version",
		Args:  cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := fmt.Fprint(f.IOStreams.Out, Format(f.Version))
			return err
		},
	}
}

// Format returns the version line printed by "bh version" and "bh --version".
func Format(version string) string {
	return fmt.Sprintf("bh version %s\n", version)
}
