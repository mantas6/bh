package auth

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
)

func TestTokenPrints(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	cfg, _ := config.Load()
	cfg.SetHost(config.DefaultHost, &config.HostConfig{Token: "printed-token"})
	if err := cfg.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &TokenOptions{IO: ios, Config: config.Load}

	if err := tokenRun(opts); err != nil {
		t.Fatalf("tokenRun: %v", err)
	}
	if strings.TrimSpace(out.String()) != "printed-token" {
		t.Errorf("output = %q", out.String())
	}
}

func TestTokenNoToken(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &TokenOptions{IO: ios, Config: config.Load}

	err := tokenRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no token found for bitbucket.org") {
		t.Errorf("error = %q", err.Error())
	}
}
