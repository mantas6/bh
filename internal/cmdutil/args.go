package cmdutil

import (
	"github.com/spf13/cobra"
)

// ExactArgs returns a PositionalArgs that requires exactly n arguments,
// returning a FlagError with msg otherwise.
func ExactArgs(n int, msg string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return FlagErrorf("%s", msg)
		}
		return nil
	}
}

// MinimumArgs returns a PositionalArgs that requires at least n arguments,
// returning a FlagError with msg otherwise.
func MinimumArgs(n int, msg string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return FlagErrorf("%s", msg)
		}
		return nil
	}
}

// NoArgsQuoteReminder is a PositionalArgs that rejects any positional argument,
// reminding the user to quote multi-word values.
func NoArgsQuoteReminder(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return nil
	}
	return FlagErrorf("unknown argument %q; please quote all values that have spaces", args[0])
}
