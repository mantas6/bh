// Command bh is a gh-style CLI for Bitbucket Cloud.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/mantas6/bh/internal/cmd/factory"
	"github.com/mantas6/bh/internal/cmd/root"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Ctrl-C or SIGTERM cancels the context, aborting in-flight API requests
	// and git subprocesses.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	f := factory.New(buildVersion())
	rootCmd := root.NewCmdRoot(f)

	// ExecuteContextC returns the command that ran (or failed) so the usage
	// hint for flag errors names the right command.
	cmd, err := rootCmd.ExecuteContextC(ctx)
	if err != nil && ctx.Err() != nil {
		// Child git processes receive the terminal's SIGINT too and may
		// fail before the context error surfaces; report an interrupt.
		err = errors.Join(ctx.Err(), err)
	}
	return root.HandleError(f.IOStreams.ErrOut, cmd, err)
}
