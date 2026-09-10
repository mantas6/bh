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
	ApiClientFor func(token, email string) *api.Client

	ShowToken bool
}

// NewCmdStatus creates the "auth status" command.
func NewCmdStatus(f *cmdutil.Factory, runF func(*StatusOptions) error) *cobra.Command {
	opts := &StatusOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		ApiClientFor: func(token, email string) *api.Client {
			c := api.NewClient(config.DefaultAPIBase, token, email)
			c.UserAgent = "bh/" + f.Version
			return c
		},
	}

	cmd := &cobra.Command{
		Use:   "status",
		Short: "View authentication status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return statusRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.ShowToken, "show-token", false, "Display the auth token")

	return cmd
}

func statusRun(opts *StatusOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host := config.DefaultHost
	token, source := cfg.Token(host)
	if token == "" {
		return fmt.Errorf("not logged in to %s; run `bh auth login`", host)
	}
	email := cfg.Email(host)

	out := opts.IO.Out
	fmt.Fprintln(out, host)

	client := opts.ApiClientFor(token, email)
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		msg := err.Error()
		var he *api.HTTPError
		if errors.As(err, &he) {
			msg = he.Message
		}
		fmt.Fprintf(out, "  ✗ Token for %s is invalid: %s\n", host, msg)
		return cmdutil.ErrSilent
	}

	fmt.Fprintf(out, "  ✓ Logged in to %s as %s (%s)\n", host, displayName(user), source)
	if email != "" {
		fmt.Fprintf(out, "  - Auth mode: Basic (%s)\n", email)
	} else {
		fmt.Fprintln(out, "  - Auth mode: Bearer")
	}

	tokenDisplay := "********"
	if opts.ShowToken {
		tokenDisplay = token
	}
	fmt.Fprintf(out, "  - Token: %s\n", tokenDisplay)

	return nil
}
