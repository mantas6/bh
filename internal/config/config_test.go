package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	c, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if c.Hosts == nil {
		t.Fatal("Hosts map is nil, want empty non-nil map")
	}
	if len(c.Hosts) != 0 {
		t.Fatalf("Hosts len = %d, want 0", len(c.Hosts))
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")

	c := &Config{Hosts: map[string]*HostConfig{}}
	c.SetHost(DefaultHost, &HostConfig{
		Token: "tok123",
		Email: "me@example.com",
		User:  "Mantas",
	})
	if err := c.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	hc := got.Host(DefaultHost)
	if hc == nil {
		t.Fatal("Host(DefaultHost) = nil, want config")
	}
	if hc.Token != "tok123" || hc.Email != "me@example.com" || hc.User != "Mantas" {
		t.Fatalf("roundtrip mismatch: %+v", hc)
	}
}

func TestSavePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits not meaningful on Windows")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "cfg")
	t.Setenv("BH_CONFIG_DIR", sub)
	t.Setenv("BH_TOKEN", "")

	c := &Config{Hosts: map[string]*HostConfig{}}
	c.SetHost(DefaultHost, &HostConfig{Token: "x"})
	if err := c.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	fi, err := os.Stat(filepath.Join(sub, hostsFile))
	if err != nil {
		t.Fatalf("Stat error = %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("hosts.yml perm = %o, want 600", perm)
	}

	di, err := os.Stat(sub)
	if err != nil {
		t.Fatalf("Stat dir error = %v", err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Fatalf("config dir perm = %o, want 700", perm)
	}
}

func TestTokenResolution(t *testing.T) {
	tests := []struct {
		name       string
		envToken   string
		fileToken  string
		wantToken  string
		wantSource string
	}{
		{"env overrides file", "envtok", "filetok", "envtok", "BH_TOKEN"},
		{"file only", "", "filetok", "filetok", "hosts.yml"},
		{"env only", "envtok", "", "envtok", "BH_TOKEN"},
		{"none", "", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BH_CONFIG_DIR", t.TempDir())
			t.Setenv("BH_TOKEN", tt.envToken)

			c := &Config{Hosts: map[string]*HostConfig{}}
			if tt.fileToken != "" {
				c.SetHost(DefaultHost, &HostConfig{Token: tt.fileToken})
			}
			token, source := c.Token(DefaultHost)
			if token != tt.wantToken || source != tt.wantSource {
				t.Fatalf("Token() = (%q, %q), want (%q, %q)", token, source, tt.wantToken, tt.wantSource)
			}
		})
	}
}

func TestBHConfigDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)
	t.Setenv("XDG_CONFIG_HOME", "/should/not/use")

	if got := ConfigDir(); got != dir {
		t.Fatalf("ConfigDir() = %q, want %q", got, dir)
	}
}

func TestXDGConfigHomeFallback(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	want := filepath.Join(xdg, "bh")
	if got := ConfigDir(); got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestEmail(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	c := &Config{Hosts: map[string]*HostConfig{}}
	c.SetHost(DefaultHost, &HostConfig{Token: "t", Email: "a@b.c"})

	if got := c.Email(DefaultHost); got != "a@b.c" {
		t.Fatalf("Email() = %q, want a@b.c", got)
	}
	if got := c.Email("missing"); got != "" {
		t.Fatalf("Email(missing) = %q, want empty", got)
	}
}

func TestRemoveHost(t *testing.T) {
	c := &Config{Hosts: map[string]*HostConfig{}}
	c.SetHost(DefaultHost, &HostConfig{Token: "t"})
	c.RemoveHost(DefaultHost)
	if c.Host(DefaultHost) != nil {
		t.Fatal("Host still present after RemoveHost")
	}
}
