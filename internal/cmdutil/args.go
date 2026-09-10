package cmdutil

import (
	"strings"

	"github.com/spf13/cobra"
)

// Heredoc trims a leading newline and removes the common leading whitespace
// (the indentation of the least-indented non-blank line) from a raw string
// literal. It is used to write readable Example blocks in source while
// producing left-aligned help output.
func Heredoc(s string) string {
	s = strings.TrimPrefix(s, "\n")
	lines := strings.Split(s, "\n")

	minIndent := -1
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(trimmed)
		if minIndent == -1 || indent < minIndent {
			minIndent = indent
		}
	}
	for i, line := range lines {
		if strings.TrimLeft(line, " \t") == "" {
			lines[i] = ""
			continue
		}
		if minIndent > 0 && len(line) >= minIndent {
			lines[i] = line[minIndent:]
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

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
