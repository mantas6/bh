package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// tokenInstructions describes how to mint an API token with the required
// scopes; printed to ErrOut during interactive login.
const tokenInstructions = "Create an API token at https://id.atlassian.com/manage-profile/security/api-tokens with scopes: read:user:bitbucket, read:repository:bitbucket, read:pullrequest:bitbucket, write:pullrequest:bitbucket"

// LoginOptions holds the dependencies and flags for `bh auth login`.
type LoginOptions struct {
	IO     *cmdutil.IOStreams
	Config func() (*config.Config, error)
	// ApiClientFor builds an API client for the given token/email so tests can
	// point it at a fake server.
	ApiClientFor func(token, email string) *api.Client
	// ReadPassword reads the token without echoing. Injectable for tests.
	ReadPassword func() (string, error)

	WithToken bool
	Email     string
	emailSet  bool
}

// NewCmdLogin creates the "auth login" command. If runF is non-nil it is
// invoked instead of loginRun (used by tests to capture parsed options).
func NewCmdLogin(f *cmdutil.Factory, runF func(*LoginOptions) error) *cobra.Command {
	opts := &LoginOptions{
		IO:     f.IOStreams,
		Config: f.Config,
		ApiClientFor: func(token, email string) *api.Client {
			c := api.NewClient(config.DefaultAPIBase, token, email)
			c.UserAgent = "bh/" + f.Version
			return c
		},
	}

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Bitbucket",
		Long:  "Authenticate bh with a Bitbucket Cloud API token.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.emailSet = cmd.Flags().Changed("email")
			if opts.ReadPassword == nil {
				opts.ReadPassword = func() (string, error) {
					b, err := term.ReadPassword(int(os.Stdin.Fd()))
					fmt.Fprintln(opts.IO.ErrOut)
					return string(b), err
				}
			}
			if runF != nil {
				return runF(opts)
			}
			return loginRun(opts)
		},
	}

	cmd.Flags().BoolVar(&opts.WithToken, "with-token", false, "Read token from standard input")
	cmd.Flags().StringVar(&opts.Email, "email", "", "Atlassian account email (enables Basic auth)")

	return cmd
}

func loginRun(opts *LoginOptions) error {
	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	token := ""
	email := opts.Email

	if opts.WithToken {
		data, err := io.ReadAll(opts.IO.In)
		if err != nil {
			return err
		}
		token = strings.TrimSpace(string(data))
		if token == "" {
			return errors.New("a token must be provided on standard input")
		}
	} else {
		if !opts.IO.IsStdinTTY() {
			return errors.New("--with-token is required when not running interactively")
		}
		fmt.Fprintln(opts.IO.ErrOut, tokenInstructions)
		fmt.Fprint(opts.IO.ErrOut, "Paste your API token: ")
		token, err = opts.ReadPassword()
		if err != nil {
			return err
		}
		token = strings.TrimSpace(token)
		if token == "" {
			return errors.New("a token must be provided")
		}
		if !opts.emailSet {
			fmt.Fprint(opts.IO.ErrOut, "Atlassian account email (leave blank to use Bearer auth): ")
			line, err := readLine(opts.IO.In)
			if err != nil {
				return err
			}
			email = strings.TrimSpace(line)
		}
	}

	client := opts.ApiClientFor(token, email)
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		if api.IsUnauthorized(err) {
			var he *api.HTTPError
			errors.As(err, &he)
			return fmt.Errorf("invalid token: %s", he.Message)
		}
		return err
	}

	name := displayName(user)

	cfg.SetHost(config.DefaultHost, &config.HostConfig{
		Token: token,
		Email: email,
		User:  name,
	})
	if err := cfg.Save(); err != nil {
		return err
	}

	if os.Getenv("BH_TOKEN") != "" {
		fmt.Fprintln(opts.IO.ErrOut, "! The BH_TOKEN environment variable is set and will take precedence over the stored token.")
	}

	fmt.Fprintf(opts.IO.ErrOut, "✓ Logged in to %s as %s\n", config.DefaultHost, name)
	return nil
}

// displayName returns the user's display name, falling back to the nickname.
func displayName(u *api.User) string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Nickname
}

// readLine reads a single line from r (without the trailing newline).
func readLine(r io.Reader) (string, error) {
	line, err := bufio.NewReader(r).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
