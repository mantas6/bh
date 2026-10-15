package root

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

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

// HandleError reports err on ios.ErrOut and returns the process exit code.
// cmd is the command that ran or failed (as returned by cobra's ExecuteC) and
// is used for the usage hint on flag errors; it may be nil.
//
//   - nil: ExitOK.
//   - context.Canceled (Ctrl-C / SIGTERM): ExitInterrupt, nothing printed.
//   - cmdutil.ErrSilent: ExitError, nothing printed (already reported).
//   - cmdutil.ErrCancel: ExitCancel, nothing printed.
//   - *cmdutil.FlagError: "bh: <msg>" plus a "--help" hint, ExitError.
//   - anything else, including *git.Error: "bh: <msg>", ExitError.
//
// When stderr is a terminal the message keeps its own line breaks, any hint
// from cmdutil.HintFor (e.g. for 401/403 API errors) follows on its own line
// and the "--help" hint comes after a blank line. Otherwise (piped or
// captured, as by scripts and agents that keep only the last line) everything
// is folded into a single line, the parts joined with "; ":
//
//	bh: <msg>; <hint> (run 'bh pr --help' for usage)
func HandleError(ios *cmdutil.IOStreams, cmd *cobra.Command, err error) int {
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

	hint := cmdutil.HintFor(err)
	var usage string
	var flagErr *cmdutil.FlagError
	if errors.As(err, &flagErr) && cmd != nil {
		usage = cmd.CommandPath()
	}

	if ios.IsStderrTTY() {
		writeMultiLine(ios.ErrOut, err.Error(), hint, usage)
	} else {
		writeSingleLine(ios.ErrOut, err.Error(), hint, usage)
	}
	return ExitError
}

// writeMultiLine prints the terminal layout: the message as is, the hint on
// the next line and, for usage errors, a "--help" pointer after a blank line.
func writeMultiLine(w io.Writer, msg, hint, usage string) {
	fmt.Fprintf(w, "bh: %s\n", msg)
	if hint != "" {
		fmt.Fprintln(w, hint)
	}
	if usage != "" {
		fmt.Fprintf(w, "\nRun '%s --help' for usage.\n", usage)
	}
}

// writeSingleLine prints msg and hint as one line, each of their non-blank
// lines trimmed and joined with "; ", followed by the "--help" pointer in
// parentheses for usage errors.
func writeSingleLine(w io.Writer, msg, hint, usage string) {
	var parts []string
	for _, line := range strings.Split(msg+"\n"+hint, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			parts = append(parts, line)
		}
	}
	line := "bh: " + strings.Join(parts, "; ")
	if usage != "" {
		line += fmt.Sprintf(" (run '%s --help' for usage)", usage)
	}
	fmt.Fprintln(w, line)
}
