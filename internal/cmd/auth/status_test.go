package auth

import (
	"errors"
	"net/http"
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

	err := statusRun(t.Context(), opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestStatusLoggedInViaBHToken(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "env-token")
	t.Setenv("BH_EMAIL", "")

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &StatusOptions{
		IO:           ios,
		Config:       config.Load,
		APIClientFor: clientForServer(srv),
	}

	if err := statusRun(t.Context(), opts); err != nil {
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

func TestStatusBHTokenEmail(t *testing.T) {
	tests := []struct {
		name       string
		envEmail   string
		wantMode   string
		wantIgnore bool
	}{
		{"BH_EMAIL set", "env@example.com", "Auth mode: Basic (env@example.com)", false},
		{"BH_EMAIL unset", "", "Auth mode: Bearer", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BH_CONFIG_DIR", t.TempDir())
			t.Setenv("BH_TOKEN", "")

			cfg, _ := config.Load()
			cfg.SetHost(config.DefaultHost, &config.HostConfig{Token: "stored-tok", Email: "stored@example.com"})
			if err := cfg.Save(); err != nil {
				t.Fatalf("save: %v", err)
			}

			t.Setenv("BH_TOKEN", "env-token")
			t.Setenv("BH_EMAIL", tt.envEmail)

			srv := apitest.New(t)
			srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

			var gotToken, gotEmail string
			ios, _, out, _ := cmdutil.TestIOStreams()
			opts := &StatusOptions{
				IO:     ios,
				Config: config.Load,
				APIClientFor: func(token, email string) *api.Client {
					gotToken, gotEmail = token, email
					return clientForServer(srv)(token, email)
				},
			}

			if err := statusRun(t.Context(), opts); err != nil {
				t.Fatalf("statusRun: %v", err)
			}
			if gotToken != "env-token" || gotEmail != tt.envEmail {
				t.Errorf("client credentials = %q/%q, want env-token/%q", gotToken, gotEmail, tt.envEmail)
			}

			got := out.String()
			if !strings.Contains(got, "(BH_TOKEN)") {
				t.Errorf("source not shown: %q", got)
			}
			if !strings.Contains(got, tt.wantMode) {
				t.Errorf("want %q in %q", tt.wantMode, got)
			}
			if strings.Contains(got, "stored@example.com") {
				t.Errorf("stored email should be ignored: %q", got)
			}
			if ignored := strings.Contains(got, "Stored email ignored"); ignored != tt.wantIgnore {
				t.Errorf("ignored note = %v, want %v: %q", ignored, tt.wantIgnore, got)
			}
		})
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
		APIClientFor: clientForServer(srv),
		ShowToken:    true,
	}

	if err := statusRun(t.Context(), opts); err != nil {
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

	ios, _, out, errOut := cmdutil.TestIOStreams()
	opts := &StatusOptions{
		IO:           ios,
		Config:       config.Load,
		APIClientFor: clientForServer(srv),
	}

	err := statusRun(t.Context(), opts)
	if !errors.Is(err, cmdutil.ErrSilent) {
		t.Fatalf("expected ErrSilent, got %v", err)
	}
	if !strings.Contains(errOut.String(), "✗ Token for bitbucket.org (BH_TOKEN) is invalid: token expired") {
		t.Errorf("stderr = %q", errOut.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
}

func TestStatusNonAuthErrorIsNotInvalidToken(t *testing.T) {
	for _, status := range []int{403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Setenv("BH_CONFIG_DIR", t.TempDir())
			t.Setenv("BH_TOKEN", "env-token")

			srv := apitest.New(t)
			srv.Handle("GET", "/user", status, `{"error":{"message":"boom"}}`)

			ios, _, out, errOut := cmdutil.TestIOStreams()
			opts := &StatusOptions{
				IO:           ios,
				Config:       config.Load,
				APIClientFor: clientForServer(srv),
			}

			err := statusRun(t.Context(), opts)
			if err == nil || errors.Is(err, cmdutil.ErrSilent) {
				t.Fatalf("expected a reported error, got %v", err)
			}
			if !strings.Contains(err.Error(), "could not verify the token for bitbucket.org") || !strings.Contains(err.Error(), "boom") {
				t.Errorf("err = %v", err)
			}
			if strings.Contains(out.String()+errOut.String(), "invalid") {
				t.Errorf("should not claim the token is invalid: out=%q err=%q", out.String(), errOut.String())
			}
		})
	}
}
