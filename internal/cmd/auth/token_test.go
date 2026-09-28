package auth

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
)

func TestTokenPrints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"stored", nil, "printed-token"},
		{"BH_TOKEN wins", map[string]string{config.EnvToken: "env-token"}, "env-token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := cmdtest.NewConfig(tt.env).SetHost(config.DefaultHost, config.HostConfig{Token: "printed-token"})

			ios, _, out, _ := cmdutil.TestIOStreams()
			opts := &TokenOptions{IO: ios, Config: cfg.Load}

			if err := tokenRun(opts); err != nil {
				t.Fatalf("tokenRun: %v", err)
			}
			if got := strings.TrimSpace(out.String()); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTokenNoToken(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &TokenOptions{IO: ios, Config: cmdtest.NewConfig(nil).Load}

	err := tokenRun(opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestTokenRejectsArgs(t *testing.T) {
	t.Parallel()
	cmd := NewCmdToken(cmdtest.NewFactory(), func(*TokenOptions) error { return nil })
	_, _, err := cmdtest.RunCommand(t, cmd, "extra")
	cmdtest.AssertFlagError(t, err, "accepts no arguments")
}
