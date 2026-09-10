// Command bh is a gh-style CLI for Bitbucket Cloud.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/mantas6/bh/internal/cmd/root"
	"github.com/mantas6/bh/internal/cmdutil"
)

// version is set via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	f := cmdutil.NewFactory(version)
	rootCmd := root.NewCmdRoot(f)

	if err := rootCmd.Execute(); err != nil {
		return handleError(f, err)
	}
	return 0
}

func handleError(f *cmdutil.Factory, err error) int {
	if err == nil {
		return 0
	}

	// Already printed elsewhere.
	if errors.Is(err, cmdutil.ErrSilent) {
		return 1
	}

	// User cancelled an interactive prompt.
	if cmdutil.IsUserCancellation(err) {
		return 1
	}

	// Flag/usage errors: print the error and usage.
	var flagErr *cmdutil.FlagError
	if errors.As(err, &flagErr) {
		fmt.Fprintf(f.IOStreams.ErrOut, "bh: %s\n", err)
		return 1
	}

	fmt.Fprintf(f.IOStreams.ErrOut, "bh: %s\n", err)
	return 1
}
