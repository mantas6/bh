package auth

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
)

func TestLoginWithTokenSaves(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada Lovelace", Nickname: "ada"})

	ios, in, _, errOut := cmdutil.TestIOStreams()
	in.WriteString("secret-token\n")

	cfg := cmdtest.NewConfig(nil)
	opts := &LoginOptions{
		IO:           ios,
		Config:       cfg.Load,
		APIClientFor: cmdtest.ClientForFunc(srv),
		WithToken:    true,
	}

	if err := loginRun(t.Context(), opts); err != nil {
		t.Fatalf("loginRun: %v", err)
	}

	if !strings.Contains(errOut.String(), "Logged in to bitbucket.org as Ada Lovelace") {
		t.Errorf("unexpected output: %q", errOut.String())
	}
	if auth := srv.LastRequest(t, "GET", "/user").Header.Get("Authorization"); auth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want the new token", auth)
	}

	hc := cfg.SavedHost(config.DefaultHost)
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
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/user", 401, `{"error":{"message":"invalid credentials"}}`)

	ios, in, _, _ := cmdutil.TestIOStreams()
	in.WriteString("bad-token\n")

	cfg := cmdtest.NewConfig(nil)
	opts := &LoginOptions{
		IO:           ios,
		Config:       cfg.Load,
		APIClientFor: cmdtest.ClientForFunc(srv),
		WithToken:    true,
	}

	err := loginRun(t.Context(), opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid token: invalid credentials") {
		t.Errorf("error = %q", err.Error())
	}
	if n := cfg.Saves(); n != 0 {
		t.Errorf("config saved %d times, want none", n)
	}
}

func TestLoginEmptyTokenOnStdin(t *testing.T) {
	t.Parallel()
	ios, in, _, _ := cmdutil.TestIOStreams()
	in.WriteString("  \n")

	cfg := cmdtest.NewConfig(nil)
	opts := &LoginOptions{IO: ios, Config: cfg.Load, WithToken: true}

	err := loginRun(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "a token must be provided on standard input") {
		t.Fatalf("err = %v", err)
	}
	if cfg.Saves() != 0 {
		t.Error("config should not be saved")
	}
}

func TestLoginNonTTYWithoutToken(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := cmdutil.TestIOStreams()
	ios.SetStdinTTY(false)

	opts := &LoginOptions{
		IO:     ios,
		Config: cmdtest.NewConfig(nil).Load,
	}

	err := loginRun(t.Context(), opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "--with-token is required when not running interactively") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestLoginInteractiveBasic(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Grace"})

	ios, in, _, errOut := cmdutil.TestIOStreams()
	ios.SetStdinTTY(true)
	// Email is read as a plain line from stdin.
	in.WriteString("grace@example.com\n")

	cfg := cmdtest.NewConfig(nil)
	opts := &LoginOptions{
		IO:           ios,
		Config:       cfg.Load,
		APIClientFor: cmdtest.ClientForFunc(srv),
		ReadPassword: func() (string, error) { return "interactive-token", nil },
	}

	if err := loginRun(t.Context(), opts); err != nil {
		t.Fatalf("loginRun: %v", err)
	}

	if !strings.Contains(errOut.String(), tokenInstructions) {
		t.Errorf("instructions not printed: %q", errOut.String())
	}

	hc := cfg.SavedHost(config.DefaultHost)
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
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

	ios, in, _, errOut := cmdutil.TestIOStreams()
	in.WriteString("stored-token\n")

	cfg := cmdtest.NewConfig(map[string]string{config.EnvToken: "env-token"})
	opts := &LoginOptions{
		IO:           ios,
		Config:       cfg.Load,
		APIClientFor: cmdtest.ClientForFunc(srv),
		WithToken:    true,
	}

	if err := loginRun(t.Context(), opts); err != nil {
		t.Fatalf("loginRun: %v", err)
	}
	if !strings.Contains(errOut.String(), "BH_TOKEN environment variable is set") {
		t.Errorf("expected BH_TOKEN warning, got %q", errOut.String())
	}
	if hc := cfg.SavedHost(config.DefaultHost); hc == nil || hc.Token != "stored-token" {
		t.Errorf("saved host = %+v, want the token from stdin", hc)
	}
}

func TestNewCmdLoginFlagParsing(t *testing.T) {
	t.Parallel()
	var captured *LoginOptions
	cmd := NewCmdLogin(cmdtest.NewFactory(), func(o *LoginOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "--with-token", "--email", "user@example.com"); err != nil {
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

func TestNewCmdLoginDefaultReadPassword(t *testing.T) {
	t.Parallel()
	var captured *LoginOptions
	cmd := NewCmdLogin(cmdtest.NewFactory(), func(o *LoginOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured.ReadPassword == nil {
		t.Fatal("ReadPassword should default to a terminal reader")
	}
	// In-memory stdin has no file descriptor to read from without echo.
	if _, err := captured.ReadPassword(); err == nil || !strings.Contains(err.Error(), "not a terminal") {
		t.Errorf("ReadPassword error = %v", err)
	}
}

func TestNewCmdLoginRejectsArgs(t *testing.T) {
	t.Parallel()
	cmd := NewCmdLogin(cmdtest.NewFactory(), func(*LoginOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "token")
	cmdtest.AssertFlagError(t, err, `unknown argument "token"`)
}
