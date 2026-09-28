package cmdutil

import "github.com/spf13/cobra"

// MutuallyExclusive returns a FlagError with msg when more than one of
// conditions is true. For string flags pass cmd.Flags().Changed(name) rather
// than comparing the value to "", so an explicit empty value still counts.
func MutuallyExclusive(msg string, conditions ...bool) error {
	n := 0
	for _, c := range conditions {
		if c {
			n++
		}
	}
	if n > 1 {
		return FlagErrorf("%s", msg)
	}
	return nil
}

// AddBodyFlags registers the standard -b/--body and -F/--body-file flags.
// Callers should reject using both with MutuallyExclusive and read a
// --body-file of "-" from standard input.
func AddBodyFlags(cmd *cobra.Command, body, bodyFile *string) {
	cmd.Flags().StringVarP(body, "body", "b", "", "Body text")
	cmd.Flags().StringVarP(bodyFile, "body-file", "F", "", "Read body text from `file` (use \"-\" for stdin)")
}
