package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
)

func TestStatusNotLoggedIn(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &StatusOptions{IO: ios, Config: config.Load}

	err := statusRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not logged in to bitbucket.org; run `bh auth login`") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestStatusLoggedInViaBHToken(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "env-token")

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &StatusOptions{
		IO:           ios,
		Config:       config.Load,
		ApiClientFor: clientForServer(srv),
	}

	if err := statusRun(opts); err != nil {
		t.Fatalf("statusRun: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Logged in to bitbucket.org as Ada (BH_TOKEN)") {
		t.Errorf("source not shown: %q", got)
	}
	if !strings.Contains(got, "Auth mode: Bearer") {
		t.Errorf("auth mode not shown: %q", got)
	}
	if !strings.Contains(got, "Token: ********") {
		t.Errorf("token should be masked: %q", got)
	}
}

func TestStatusShowToken(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	cfg, _ := config.Load()
	cfg.SetHost(config.DefaultHost, &config.HostConfig{Token: "stored-tok", Email: "ada@example.com", User: "Ada"})
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &StatusOptions{
		IO:           ios,
		Config:       config.Load,
		ApiClientFor: clientForServer(srv),
		ShowToken:    true,
	}

	if err := statusRun(opts); err != nil {
		t.Fatalf("statusRun: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "(hosts.yml)") {
		t.Errorf("source not shown: %q", got)
	}
	if !strings.Contains(got, "Auth mode: Basic (ada@example.com)") {
		t.Errorf("basic auth mode not shown: %q", got)
	}
	if !strings.Contains(got, "Token: stored-tok") {
		t.Errorf("token should be revealed: %q", got)
	}
}

func TestStatusInvalidToken(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "env-token")

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 401, `{"error":{"message":"token expired"}}`)

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &StatusOptions{
		IO:           ios,
		Config:       config.Load,
		ApiClientFor: clientForServer(srv),
	}

	err := statusRun(opts)
	if !errors.Is(err, cmdutil.ErrSilent) {
		t.Fatalf("expected ErrSilent, got %v", err)
	}
	if !strings.Contains(out.String(), "✗ Token for bitbucket.org is invalid: token expired") {
		t.Errorf("output = %q", out.String())
	}
}
