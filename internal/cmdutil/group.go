package cmdutil

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// SetupGroup configures cmd, a command that only groups subcommands, so that
// an unknown subcommand is a usage error instead of silently printing help and
// exiting 0.
//
// Cobra treats a non-runnable command with leftover arguments as a request for
// help, so `bh pr diff 1` would print the `bh pr` help and succeed. SetupGroup
// makes cmd runnable: with no arguments it prints help as before, `help
// [<sub>...]` shows help for the named subcommand, and anything else returns a
// FlagError naming the unknown command with close-match suggestions and, when
// hints has an entry for the typed name, a pointer to what to use instead.
//
// Being runnable makes cobra print UseLine in the usage section as well, so
// [flags] is kept out of it to avoid suggesting `bh pr --some-flag` works.
func SetupGroup(cmd *cobra.Command, hints map[string]string) {
	cmd.DisableFlagsInUseLine = true
	cmd.Args = func(c *cobra.Command, args []string) error {
		if len(args) == 0 || args[0] == "help" {
			return nil
		}
		return UnknownCommandError(c, args[0], hints[args[0]])
	}
	cmd.RunE = func(c *cobra.Command, args []string) error {
		target := c
		if len(args) > 1 {
			if sub, _, err := c.Find(args[1:]); err == nil && sub != nil {
				target = sub
			}
		}
		return target.Help()
	}
}

// UnknownCommandError returns the FlagError reported when name is not a
// subcommand of cmd:
//
//	unknown command "comments" for "bh pr"
//	Did you mean "comment"?
//	<hint>
//
// The suggestion line is omitted when no subcommand is close to name, and the
// hint line when hint is empty.
func UnknownCommandError(cmd *cobra.Command, name, hint string) *FlagError {
	var b strings.Builder
	fmt.Fprintf(&b, "unknown command %q for %q", name, cmd.CommandPath())
	if s := suggestions(cmd, name); s != "" {
		fmt.Fprintf(&b, "\nDid you mean %s?", s)
	}
	if hint != "" {
		b.WriteString("\n" + hint)
	}
	return FlagErrorf("%s", b.String())
}

// suggestions returns the subcommands of cmd close to name, sorted, as a
// quoted, human-readable list (`"a"`, `"a" or "b"`, `"a", "b" or "c"`), or "".
func suggestions(cmd *cobra.Command, name string) string {
	if cmd.DisableSuggestions {
		return ""
	}
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = 2
	}
	names := cmd.SuggestionsFor(name)
	slices.Sort(names)
	names = slices.Compact(names)
	quoted := make([]string, len(names))
	for i, s := range names {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	switch n := len(quoted); n {
	case 0:
		return ""
	case 1:
		return quoted[0]
	default:
		return strings.Join(quoted[:n-1], ", ") + " or " + quoted[n-1]
	}
}
