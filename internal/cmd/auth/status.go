package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
	"github.com/spf13/cobra"
)

// StatusOptions holds the dependencies and flags for `bh auth status`.
type StatusOptions struct {
	IO           *cmdutil.IOStreams
	Config       func() (*config.Config, error)
	APIClientFor func(token, email string) *api.Client

	ShowToken bool
}

// NewCmdStatus creates the "auth status" command.
func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:           f.IOStreams,
		Config:       f.Config,
		APIClientFor: f.APIClientFor,
	}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "View authentication status",
		Args:  cmdutil.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return statusRun(cmd.Context(), opts)
		},
	}

	cmd.Flags().BoolVar(&opts.ShowToken, "show-token", false, "Display the auth token")

	return cmd
}

func statusRun(ctx context.Context, opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host := config.DefaultHost
	token, source := cfg.Token(host)
	if token == "" {
		return cmdutil.NotLoggedInError(host)
	}
	email := cfg.Email(host)

	client := opts.APIClientFor(token, email)
	user, err := client.CurrentUser(ctx)
	if err != nil {
		// Only a 401 means the token itself is bad; anything else (network
		// failure, 403, 5xx) is reported as an ordinary error.
		var he *api.HTTPError
		if !api.IsUnauthorized(err) || !errors.As(err, &he) {
			return fmt.Errorf("could not verify the token for %s: %w", host, err)
		}
		cs := opts.IO.ErrColorScheme()
		fmt.Fprintln(opts.IO.ErrOut, host)
		fmt.Fprintf(opts.IO.ErrOut, "  %s Token for %s (%s) is invalid: %s\n", cs.FailureIcon(), host, source, he.Message)
		return cmdutil.ErrSilent
	}

	out := opts.IO.Out
	cs := opts.IO.ColorScheme()
	fmt.Fprintln(out, host)
	fmt.Fprintf(out, "  %s Logged in to %s as %s (%s)\n", cs.SuccessIcon(), host, displayName(user), source)
	if email != "" {
		fmt.Fprintf(out, "  - Auth mode: Basic (%s)\n", email)
	} else {
		fmt.Fprintln(out, "  - Auth mode: Bearer")
	}
	if hc := cfg.Host(host); source == config.TokenSourceEnv && hc != nil && hc.Email != "" && email == "" {
		fmt.Fprintf(out, "  - Stored email ignored because %s is set; set %s for Basic auth\n", config.EnvToken, config.EnvEmail)
	}

	tokenDisplay := "********"
	if opts.ShowToken {
		tokenDisplay = token
	}
	fmt.Fprintf(out, "  - Token: %s\n", tokenDisplay)

	return nil
}
