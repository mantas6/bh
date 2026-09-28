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

// The validators below return FlagErrors so the top level prints the error
// followed by a "Run '<command> --help' for usage." hint.

// NoArgs is a PositionalArgs that rejects any positional argument.
func NoArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return FlagErrorf("unknown argument %q; %q accepts no arguments (quote values that contain spaces)", args[0], cmd.CommandPath())
}

// MaxArgs returns a PositionalArgs that accepts at most n arguments.
func MaxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) <= n {
			return nil
		}
		return tooManyArgs(cmd, n, args)
	}
}

// ExactArgs returns a PositionalArgs that requires exactly n arguments. When
// arguments are missing the FlagError carries msg.
func ExactArgs(n int, msg string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		switch {
		case len(args) < n:
			return FlagErrorf("%s", msg)
		case len(args) > n:
			return tooManyArgs(cmd, n, args)
		}
		return nil
	}
}

func tooManyArgs(cmd *cobra.Command, n int, args []string) error {
	noun := "arguments"
	if n == 1 {
		noun = "argument"
	}
	return FlagErrorf("too many arguments; %q accepts at most %d %s, received %d (quote values that contain spaces)",
		cmd.CommandPath(), n, noun, len(args))
}
