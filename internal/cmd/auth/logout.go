package auth

import (
	"fmt"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
	"github.com/mantas6/bh/internal/output"
	"github.com/spf13/cobra"
)

// LogoutOptions holds the dependencies for `bh auth logout`.
type LogoutOptions struct {
	IO     *cmdutil.IOStreams
	Config func() (*config.Config, error)
}

// NewCmdLogout creates the "auth logout" command.
func NewCmdLogout(f *cmdutil.Factory, runF func(*LogoutOptions) error) *cobra.Command {
	opts := &LogoutOptions{
		IO:     f.IOStreams,
		Config: f.Config,
	}

	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Log out of Bitbucket",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return logoutRun(opts)
		},
	}

	return cmd
}

func logoutRun(opts *LogoutOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host := config.DefaultHost
	if cfg.Host(host) == nil {
		return fmt.Errorf("not logged in to %s", host)
	}

	cfg.RemoveHost(host)
	if err := cfg.Save(); err != nil {
		return err
	}

	cs := output.NewColorScheme(opts.IO.ColorEnabled())
	fmt.Fprintf(opts.IO.ErrOut, "%s Logged out of %s\n", cs.SuccessIcon(), host)
	return nil
}
