package auth

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
)

// clientForServer returns an ApiClientFor that points at the fake server while
// honoring the token/email passed by the command.
func clientForServer(srv *apitest.Server) func(token, email string) *api.Client {
	return func(token, email string) *api.Client {
		c := api.NewClient(srv.URL, token, email)
		c.HTTP = srv.Server.Client()
		return c
	}
}

func TestLoginWithTokenSaves(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada Lovelace", Nickname: "ada"})

	ios, in, _, errOut := cmdutil.TestIOStreams()
	in.WriteString("secret-token\n")

	opts := &LoginOptions{
		IO:           ios,
		Config:       config.Load,
		ApiClientFor: clientForServer(srv),
		WithToken:    true,
	}

	if err := loginRun(opts); err != nil {
		t.Fatalf("loginRun: %v", err)
	}

	if !strings.Contains(errOut.String(), "Logged in to bitbucket.org as Ada Lovelace") {
		t.Errorf("unexpected output: %q", errOut.String())
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	hc := cfg.Host(config.DefaultHost)
	if hc == nil {
		t.Fatal("host not saved")
	}
	if hc.Token != "secret-token" {
		t.Errorf("token = %q", hc.Token)
	}
	if hc.User != "Ada Lovelace" {
		t.Errorf("user = %q", hc.User)
	}
	if hc.Email != "" {
		t.Errorf("email = %q, want empty (Bearer)", hc.Email)
	}
}

func TestLoginInvalidTokenDoesNotSave(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 401, `{"error":{"message":"invalid credentials"}}`)

	ios, in, _, _ := cmdutil.TestIOStreams()
	in.WriteString("bad-token\n")

	opts := &LoginOptions{
		IO:           ios,
		Config:       config.Load,
		ApiClientFor: clientForServer(srv),
		WithToken:    true,
	}

	err := loginRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid token: invalid credentials") {
		t.Errorf("error = %q", err.Error())
	}

	cfg, _ := config.Load()
	if cfg.Host(config.DefaultHost) != nil {
		t.Error("host should not have been saved")
	}
}

func TestLoginNonTTYWithoutToken(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())

	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(false)

	opts := &LoginOptions{
		IO:     ios,
		Config: config.Load,
	}

	err := loginRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--with-token is required when not running interactively") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestLoginInteractiveBasic(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Grace"})

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	// Email is read as a plain line from stdin.
	in.WriteString("grace@example.com\n")

	opts := &LoginOptions{
		IO:           ios,
		Config:       config.Load,
		ApiClientFor: clientForServer(srv),
		ReadPassword: func() (string, error) { return "interactive-token", nil },
	}

	if err := loginRun(opts); err != nil {
		t.Fatalf("loginRun: %v", err)
	}

	if !strings.Contains(errOut.String(), tokenInstructions) {
		t.Errorf("instructions not printed: %q", errOut.String())
	}

	cfg, _ := config.Load()
	hc := cfg.Host(config.DefaultHost)
	if hc == nil {
		t.Fatal("host not saved")
	}
	if hc.Token != "interactive-token" {
		t.Errorf("token = %q", hc.Token)
	}
	if hc.Email != "grace@example.com" {
		t.Errorf("email = %q, want basic auth email", hc.Email)
	}
}

func TestLoginWarnsOnBHToken(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "env-token")

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

	ios, in, _, errOut := cmdutil.TestIOStreams()
	in.WriteString("stored-token\n")

	opts := &LoginOptions{
		IO:           ios,
		Config:       config.Load,
		ApiClientFor: clientForServer(srv),
		WithToken:    true,
	}

	if err := loginRun(opts); err != nil {
		t.Fatalf("loginRun: %v", err)
	}
	if !strings.Contains(errOut.String(), "BH_TOKEN environment variable is set") {
		t.Errorf("expected BH_TOKEN warning, got %q", errOut.String())
	}
}

func TestNewCmdLoginFlagParsing(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}

	var captured *LoginOptions
	cmd := NewCmdLogin(f, func(o *LoginOptions) error {
		captured = o
		return nil
	})
	cmd.SetArgs([]string{"--with-token", "--email", "user@example.com"})
	cmd.SetOut(ios.Out)
	cmd.SetErr(ios.ErrOut)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured == nil {
		t.Fatal("runF not called")
	}
	if !captured.WithToken {
		t.Error("--with-token not parsed")
	}
	if captured.Email != "user@example.com" {
		t.Errorf("email = %q", captured.Email)
	}
	if !captured.emailSet {
		t.Error("emailSet should be true when --email passed")
	}
}
