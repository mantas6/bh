package root

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// Process exit codes returned by HandleError.
const (
	ExitOK = 0
	// ExitError is used for every failure without a more specific code,
	// including flag/usage errors and failed git invocations.
	ExitError = 1
	// ExitCancel means the user cancelled an interactive prompt (the gh
	// convention).
	ExitCancel = 2
	// ExitInterrupt means the command was interrupted by SIGINT/SIGTERM
	// (128 + SIGINT, as shells report it).
	ExitInterrupt = 130
)

// HandleError reports err on errOut and returns the process exit code. cmd is
// the command that ran or failed (as returned by cobra's ExecuteC) and is
// used for the usage hint on flag errors; it may be nil.
//
//   - nil: ExitOK.
//   - context.Canceled (Ctrl-C / SIGTERM): ExitInterrupt, nothing printed.
//   - cmdutil.ErrSilent: ExitError, nothing printed (already reported).
//   - cmdutil.ErrCancel: ExitCancel, nothing printed.
//   - *cmdutil.FlagError: "bh: <msg>" plus a "--help" hint, ExitError.
//   - anything else, including *git.Error: "bh: <msg>", ExitError.
//
// Any hint from cmdutil.HintFor (e.g. for 401/403 API errors) is printed on
// the line after the message.
func HandleError(errOut io.Writer, cmd *cobra.Command, err error) int {
	switch {
	case err == nil:
		return ExitOK
	case errors.Is(err, context.Canceled):
		return ExitInterrupt
	case errors.Is(err, cmdutil.ErrSilent):
		return ExitError
	case cmdutil.IsUserCancellation(err):
		return ExitCancel
	}

	fmt.Fprintf(errOut, "bh: %s\n", err)
	if hint := cmdutil.HintFor(err); hint != "" {
		fmt.Fprintln(errOut, hint)
	}

	var flagErr *cmdutil.FlagError
	if errors.As(err, &flagErr) && cmd != nil {
		fmt.Fprintf(errOut, "\nRun '%s --help' for usage.\n", cmd.CommandPath())
	}
	return ExitError
}
