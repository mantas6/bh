package auth

import (
	"fmt"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
	"github.com/spf13/cobra"
)

// TokenOptions holds the dependencies for `bh auth token`.
type TokenOptions struct {
	IO     *cmdutil.IOStreams
	Config func() (*config.Config, error)
}

// NewCmdToken creates the "auth token" command.
func NewCmdToken(f *cmdutil.Factory, runF func(*TokenOptions) error) *cobra.Command {
	opts := &TokenOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "token",
		Short: "Print the auth token bh is configured to use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return tokenRun(opts)
		},
	}

	return cmd
}

func tokenRun(opts *TokenOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host := config.DefaultHost
	token, _ := cfg.Token(host)
	if token == "" {
		return fmt.Errorf("no token found for %s", host)
	}

	fmt.Fprintln(opts.IO.Out, token)
	return nil
}
