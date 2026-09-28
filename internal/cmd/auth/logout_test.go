package auth

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
)

func TestLogoutSuccess(t *testing.T) {
	t.Parallel()
	cfg := cmdtest.NewConfig(nil).SetHost(config.DefaultHost, config.HostConfig{Token: "tok", User: "Ada"})

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &LogoutOptions{IO: ios, Config: cfg.Load}

	if err := logoutRun(opts); err != nil {
		t.Fatalf("logoutRun: %v", err)
	}
	if !strings.Contains(errOut.String(), "Logged out of bitbucket.org") {
		t.Errorf("output = %q", errOut.String())
	}
	if cfg.Saves() != 1 {
		t.Fatalf("saves = %d, want 1", cfg.Saves())
	}
	if hc := cfg.SavedHost(config.DefaultHost); hc != nil {
		t.Errorf("saved host = %+v, want it removed", hc)
	}
}

func TestLogoutNotLoggedIn(t *testing.T) {
	t.Parallel()
	cfg := cmdtest.NewConfig(nil)
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &LogoutOptions{IO: ios, Config: cfg.Load}

	err := logoutRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN") {
		t.Errorf("error = %q", err.Error())
	}
	if cfg.Saves() != 0 {
		t.Error("config should not be saved")
	}
}

func TestLogoutWarnsWhenBHTokenSet(t *testing.T) {
	t.Parallel()
	cfg := cmdtest.NewConfig(map[string]string{config.EnvToken: "env-token"}).
		SetHost(config.DefaultHost, config.HostConfig{Token: "tok", User: "Ada"})

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &LogoutOptions{IO: ios, Config: cfg.Load}

	if err := logoutRun(opts); err != nil {
		t.Fatalf("logoutRun: %v", err)
	}
	got := errOut.String()
	if !strings.Contains(got, "Logged out of bitbucket.org") {
		t.Errorf("output = %q", got)
	}
	if !strings.Contains(got, "BH_TOKEN environment variable is set, so bh remains authenticated") {
		t.Errorf("missing BH_TOKEN warning: %q", got)
	}
	if cfg.Saves() != 1 || cfg.SavedHost(config.DefaultHost) != nil {
		t.Errorf("stored credentials should be removed (saves = %d)", cfg.Saves())
	}
}

func TestLogoutOnlyBHTokenErrors(t *testing.T) {
	t.Parallel()
	cfg := cmdtest.NewConfig(map[string]string{config.EnvToken: "env-token"})
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &LogoutOptions{IO: ios, Config: cfg.Load}

	err := logoutRun(opts)
	if err == nil || !strings.Contains(err.Error(), "unset it to log out") {
		t.Fatalf("err = %v", err)
	}
}

func TestLogoutRejectsArgs(t *testing.T) {
	t.Parallel()
	cmd := NewCmdLogout(cmdtest.NewFactory(), func(*LogoutOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "extra")
	cmdtest.AssertFlagError(t, err, "accepts no arguments")
}
