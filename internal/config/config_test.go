package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
	skipOnWindows(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "nested", "cfg")
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
		wantSource TokenSource
	}{
		{"env overrides file", "envtok", "filetok", "envtok", TokenSourceEnv},
		{"file only", "", "filetok", "filetok", TokenSourceFile},
		{"env only", "envtok", "", "envtok", TokenSourceEnv},
		{"none", "", "", "", TokenSourceNone},
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
				t.Fatalf("Token() = (%q, %v), want (%q, %v)", token, source, tt.wantToken, tt.wantSource)
			}
		})
	}
}

func TestTokenSourceString(t *testing.T) {
	tests := map[TokenSource]string{
		TokenSourceNone: "none",
		TokenSourceEnv:  "BH_TOKEN",
		TokenSourceFile: "hosts.yml",
		TokenSource(42): "none",
	}
	for src, want := range tests {
		if got := src.String(); got != want {
			t.Errorf("TokenSource(%d).String() = %q, want %q", int(src), got, want)
		}
	}
}

func TestBHConfigDirOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)
	t.Setenv("XDG_CONFIG_HOME", "/should/not/use")

	got, err := Dir()
	if err != nil || got != dir {
		t.Fatalf("Dir() = (%q, %v), want %q", got, err, dir)
	}
}

func TestXDGConfigHomeFallback(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", "")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	want := filepath.Join(xdg, "bh")
	got, err := Dir()
	if err != nil || got != want {
		t.Fatalf("Dir() = (%q, %v), want %q", got, err, want)
	}
}

func TestRelativeXDGConfigHomeIgnored(t *testing.T) {
	skipOnWindows(t)
	home := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	t.Setenv("HOME", home)

	want := filepath.Join(home, ".config", "bh")
	got, err := Dir()
	if err != nil || got != want {
		t.Fatalf("Dir() = (%q, %v), want %q", got, err, want)
	}
}

func TestDirNoHome(t *testing.T) {
	skipOnWindows(t)
	for _, home := range []string{"", "relative/home"} {
		t.Run(home, func(t *testing.T) {
			t.Setenv("BH_CONFIG_DIR", "")
			t.Setenv("XDG_CONFIG_HOME", "")
			t.Setenv("HOME", home)

			got, err := Dir()
			if err == nil {
				t.Fatalf("Dir() = %q, want error", got)
			}
			if !strings.Contains(err.Error(), "BH_CONFIG_DIR") {
				t.Errorf("error should suggest BH_CONFIG_DIR: %v", err)
			}
			if _, err := Load(); err == nil {
				t.Error("Load() error = nil, want error")
			}
			c := &Config{}
			c.SetHost(DefaultHost, &HostConfig{Token: "x"})
			if err := c.Save(); err == nil {
				t.Error("Save() error = nil, want error")
			}
		})
	}
}

func TestLoadMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)
	path := filepath.Join(dir, hostsFile)
	if err := os.WriteFile(path, []byte("bitbucket.org: [unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error should mention %s: %v", path, err)
	}
}

func TestLoadEmptyFile(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "whitespace": "\n  \n", "null": "null\n"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("BH_CONFIG_DIR", dir)
			if err := os.WriteFile(filepath.Join(dir, hostsFile), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}

			c, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if c.Hosts == nil || len(c.Hosts) != 0 {
				t.Fatalf("Hosts = %v, want empty non-nil map", c.Hosts)
			}
		})
	}
}

func TestLoadUnreadableErrorMentionsPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)
	// A directory where the file should be makes ReadFile fail.
	path := filepath.Join(dir, hostsFile)
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Load() error = %v, want error mentioning %s", err, path)
	}
}

func TestSaveTightensLoosePermissions(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)
	path := filepath.Join(dir, hostsFile)
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}

	c := &Config{}
	c.SetHost(DefaultHost, &HostConfig{Token: "x"})
	if err := c.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("hosts.yml perm = %o, want 600", perm)
	}
}

func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)

	c := &Config{}
	c.SetHost(DefaultHost, &HostConfig{Token: "one"})
	for range 3 {
		if err := c.Save(); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != hostsFile {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("config dir entries = %v, want [%s]", names, hostsFile)
	}
}

func TestSaveFollowsSymlink(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)
	targetDir := t.TempDir()
	target := filepath.Join(targetDir, "real-hosts.yml")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, hostsFile)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	c := &Config{}
	c.SetHost(DefaultHost, &HostConfig{Token: "linked"})
	if err := c.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("hosts.yml symlink was replaced by a regular file")
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if hc := got.Host(DefaultHost); hc == nil || hc.Token != "linked" {
		t.Fatalf("Host = %+v, want token linked", hc)
	}
}

func TestSaveWriteErrorMentionsPath(t *testing.T) {
	skipOnWindows(t)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	c := &Config{}
	c.SetHost(DefaultHost, &HostConfig{Token: "x"})
	err := c.Save()
	path := filepath.Join(dir, hostsFile)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Save() error = %v, want error mentioning %s", err, path)
	}
}

func TestEmailWithEnvToken(t *testing.T) {
	tests := []struct {
		name        string
		envToken    string
		envEmail    string
		storedEmail string
		want        string
	}{
		{"stored email with file token", "", "env@example.com", "stored@example.com", "stored@example.com"},
		{"BH_TOKEN ignores stored email", "envtok", "", "stored@example.com", ""},
		{"BH_TOKEN with BH_EMAIL", "envtok", "env@example.com", "stored@example.com", "env@example.com"},
		{"BH_TOKEN with BH_EMAIL, no stored host", "envtok", "env@example.com", "", "env@example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("BH_TOKEN", tt.envToken)
			t.Setenv("BH_EMAIL", tt.envEmail)

			c := &Config{}
			if tt.storedEmail != "" {
				c.SetHost(DefaultHost, &HostConfig{Token: "filetok", Email: tt.storedEmail})
			}
			if got := c.Email(DefaultHost); got != tt.want {
				t.Fatalf("Email() = %q, want %q", got, tt.want)
			}
		})
	}
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("not meaningful on Windows")
	}
}

func TestEmail(t *testing.T) {
	t.Setenv("BH_CONFIG_DIR", t.TempDir())
	t.Setenv("BH_TOKEN", "")
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
