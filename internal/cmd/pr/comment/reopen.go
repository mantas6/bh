package comment

import (
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdReopen creates the "pr comment reopen" command.
func NewCmdReopen(f *cmdutil.Factory, runF func(*ResolveOptions) error) *cobra.Command {
	opts := newResolveOptions(f)

	cmd := &cobra.Command{
		Use:   "reopen <number> <comment-id>",
		Short: "Reopen (unresolve) a pull request comment thread",
		Args:  cmdutil.ExactArgs(2, "a pull request and a comment id are required"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Arg = args[0]
			id, err := parseCommentID(args[1])
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}
			opts.CommentID = id
			if runF != nil {
				return runF(opts)
			}
			return resolveRun(opts, true)
		},
	}

	return cmd
}
