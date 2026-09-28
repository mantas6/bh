package auth

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
)

// envToken is the environment of a user who exported BH_TOKEN.
var envToken = map[string]string{config.EnvToken: "env-token"}

func TestStatusNotLoggedIn(t *testing.T) {
	t.Parallel()
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := &StatusOptions{IO: ios, Config: cmdtest.NewConfig(nil).Load}

	err := statusRun(t.Context(), opts)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestStatusLoggedInViaBHToken(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &StatusOptions{
		IO:           ios,
		Config:       cmdtest.NewConfig(envToken).Load,
		APIClientFor: cmdtest.ClientForFunc(srv),
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
	t.Parallel()
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
			t.Parallel()
			cfg := cmdtest.NewConfig(map[string]string{config.EnvToken: "env-token", config.EnvEmail: tt.envEmail}).
				SetHost(config.DefaultHost, config.HostConfig{Token: "stored-tok", Email: "stored@example.com"})

			srv := apitest.New(t)
			srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

			var gotToken, gotEmail string
			ios, _, out, _ := cmdutil.TestIOStreams()
			opts := &StatusOptions{
				IO:     ios,
				Config: cfg.Load,
				APIClientFor: func(token, email string) *api.Client {
					gotToken, gotEmail = token, email
					return cmdtest.ClientForFunc(srv)(token, email)
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
	t.Parallel()
	cfg := cmdtest.NewConfig(nil).
		SetHost(config.DefaultHost, config.HostConfig{Token: "stored-tok", Email: "ada@example.com", User: "Ada"})

	srv := apitest.New(t)
	srv.Handle("GET", "/user", 200, api.User{DisplayName: "Ada"})

	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := &StatusOptions{
		IO:           ios,
		Config:       cfg.Load,
		APIClientFor: cmdtest.ClientForFunc(srv),
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
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/user", 401, `{"error":{"message":"token expired"}}`)

	ios, _, out, errOut := cmdutil.TestIOStreams()
	opts := &StatusOptions{
		IO:           ios,
		Config:       cmdtest.NewConfig(envToken).Load,
		APIClientFor: cmdtest.ClientForFunc(srv),
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
	t.Parallel()
	for _, status := range []int{403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			srv := apitest.New(t)
			srv.Handle("GET", "/user", status, `{"error":{"message":"boom"}}`)

			ios, _, out, errOut := cmdutil.TestIOStreams()
			opts := &StatusOptions{
				IO:           ios,
				Config:       cmdtest.NewConfig(envToken).Load,
				APIClientFor: cmdtest.ClientForFunc(srv),
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

func TestStatusFlagParsing(t *testing.T) {
	t.Parallel()
	var captured *StatusOptions
	cmd := NewCmdStatus(cmdtest.NewFactory(), func(o *StatusOptions) error {
		captured = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "--show-token"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if captured == nil || !captured.ShowToken {
		t.Errorf("parsed = %+v", captured)
	}
}
