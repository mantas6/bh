package comment

import (
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdReopen creates the "pr comment reopen" command. It shares
// ResolveOptions and resolveRun with "pr comment resolve", with
// ResolveOptions.Reopen set.
func NewCmdReopen(f *cmdutil.Factory, runF func(*ResolveOptions) error) *cobra.Command {
	return newCmdResolveOrReopen(f, runF, true)
}
