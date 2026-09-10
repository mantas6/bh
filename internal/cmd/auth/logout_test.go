package auth

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
)

func TestLogoutSuccess(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	cfg, _ := config.Load()
	cfg.SetHost(config.DefaultHost, &config.HostConfig{Token: "tok", User: "Ada"})
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	ios, _, _, errOut := cmdutil.TestIOStreams()
	opts := &LogoutOptions{IO: ios, Config: config.Load}

	if err := logoutRun(opts); err != nil {
		t.Fatalf("logoutRun: %v", err)
	}
	if !strings.Contains(errOut.String(), "Logged out of bitbucket.org") {
		t.Errorf("output = %q", errOut.String())
	}

	reloaded, _ := config.Load()
	if reloaded.Host(config.DefaultHost) != nil {
		t.Error("host should have been removed")
	}
}

func TestLogoutNotLoggedIn(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &LogoutOptions{IO: ios, Config: config.Load}

	err := logoutRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not logged in to bitbucket.org") {
		t.Errorf("error = %q", err.Error())
	}
}
