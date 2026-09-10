// Command bh is a gh-style CLI for Bitbucket Cloud.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mantas6/bh/internal/cmd/root"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/spf13/cobra"
)

// version is set via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	f := cmdutil.NewFactory(version)
	rootCmd := root.NewCmdRoot(f)

	// ExecuteC returns the command that ran (or failed) so we can build an
	// accurate "--help" hint for flag/usage errors.
	cmd, err := rootCmd.ExecuteC()
	if err != nil {
		return handleError(f, cmd, err)
	}
	return 0
}

func handleError(f *cmdutil.Factory, cmd *cobra.Command, err error) int {
	if err == nil {
		return 0
	}

	// Already printed elsewhere: exit non-zero with no further output.
	if errors.Is(err, cmdutil.ErrSilent) {
		return 1
	}

	// User cancelled an interactive prompt (gh convention: exit code 2).
	if cmdutil.IsUserCancellation(err) {
		return 2
	}

	// Flag/usage errors: print the error and a hint pointing at --help.
	var flagErr *cmdutil.FlagError
	if errors.As(err, &flagErr) {
		fmt.Fprintf(f.IOStreams.ErrOut, "bh: %s\n", err)
		if cmd != nil {
			fmt.Fprintf(f.IOStreams.ErrOut, "\nRun '%s --help' for usage.\n", cmd.CommandPath())
		}
		return 1
	}

	fmt.Fprintf(f.IOStreams.ErrOut, "bh: %s\n", err)
	return 1
}
