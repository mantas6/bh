// Package config handles reading and writing bh configuration, including
// per-host authentication tokens stored in hosts.yml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/mantas6/bh/internal/api"
)

const (
	// DefaultHost is the default Bitbucket Cloud host, used as the key for
	// stored credentials.
	DefaultHost = api.DefaultHost

	hostsFile = "hosts.yml"

	// EnvToken overrides any stored token when set.
	EnvToken = "BH_TOKEN"
	// EnvEmail is the Atlassian email paired with EnvToken. It is only
	// consulted when the token comes from EnvToken.
	EnvEmail = "BH_EMAIL"
)

// HostConfig holds the stored credentials for a single host.
type HostConfig struct {
	// Token is the API token used to authenticate.
	Token string `yaml:"token"`
	// Email is the Atlassian account email. When set, Basic auth
	// (email:token) is used instead of Bearer.
	Email string `yaml:"email,omitempty"`
	// User is the display name / nickname of the logged-in user.
	User string `yaml:"user,omitempty"`
}

// Config is the top-level configuration, keyed by host name. It is
// serialised as a plain host -> HostConfig mapping.
type Config struct {
	Hosts map[string]*HostConfig

	// getenv looks up the BH_TOKEN/BH_EMAIL overrides; nil means os.Getenv.
	getenv func(string) string
	// save replaces writing hosts.yml in Save when non-nil.
	save func(*Config) error
}

// NewInMemory returns an empty configuration that never touches the file
// system or the process environment, for use in tests. Save calls save
// (which may be nil) instead of writing hosts.yml, and the BH_TOKEN and
// BH_EMAIL overrides are looked up with getenv (nil means unset).
func NewInMemory(getenv func(string) string, save func(*Config) error) *Config {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if save == nil {
		save = func(*Config) error { return nil }
	}
	return &Config{Hosts: map[string]*HostConfig{}, getenv: getenv, save: save}
}

func (c *Config) lookupEnv(key string) string {
	if c.getenv != nil {
		return c.getenv(key)
	}
	return os.Getenv(key)
}

// Dir returns the directory where bh stores its configuration.
// Resolution order: BH_CONFIG_DIR > $XDG_CONFIG_HOME/bh > ~/.config/bh.
// A relative XDG_CONFIG_HOME is ignored, as required by the XDG Base
// Directory specification. An error is returned when no absolute home
// directory can be determined.
func Dir() (string, error) {
	if dir := os.Getenv("BH_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, "bh"), nil
	}
	home, err := os.UserHomeDir()
	if err == nil && !filepath.IsAbs(home) {
		err = fmt.Errorf("home directory %q is not an absolute path", home)
	}
	if err != nil {
		return "", fmt.Errorf("cannot determine config directory (set BH_CONFIG_DIR or XDG_CONFIG_HOME): %w", err)
	}
	return filepath.Join(home, ".config", "bh"), nil
}

// hostsPath returns the path to the hosts file.
func hostsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, hostsFile), nil
}

// Load reads the configuration from disk. A missing or empty hosts file
// yields an empty configuration rather than an error.
func Load() (*Config, error) {
	path, err := hostsPath()
	if err != nil {
		return nil, err
	}

	c := &Config{Hosts: map[string]*HostConfig{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	hosts := map[string]*HostConfig{}
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if hosts != nil {
		c.Hosts = hosts
	}
	return c, nil
}

// Save writes the configuration to disk atomically: the data is written to a
// temporary file in the same directory, synced, and renamed over the hosts
// file. The config directory is created with 0700 permissions and the hosts
// file always ends up with 0600 permissions, even if it previously existed
// with looser ones. If the hosts file is a symlink, its target is replaced.
// A configuration from NewInMemory is handed to its save function instead.
func (c *Config) Save() error {
	if c.save != nil {
		return c.save(c)
	}
	path, err := hostsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	hosts := c.Hosts
	if hosts == nil {
		hosts = map[string]*HostConfig{}
	}
	data, err := yaml.Marshal(hosts)
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}

	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// writeFileAtomic writes data to path via a 0600 temporary file in the same
// directory followed by a rename, so readers never observe a partial file.
func writeFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if err = f.Chmod(0o600); err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		return err
	}

	// Best effort: persist the rename. Not all platforms support syncing a
	// directory, so failures are ignored.
	if d, derr := os.Open(dir); derr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Host returns the configuration for the named host, or nil if absent.
func (c *Config) Host(name string) *HostConfig {
	if c.Hosts == nil {
		return nil
	}
	return c.Hosts[name]
}

// SetHost stores the configuration for the named host.
func (c *Config) SetHost(name string, hc *HostConfig) {
	if c.Hosts == nil {
		c.Hosts = map[string]*HostConfig{}
	}
	c.Hosts[name] = hc
}

// RemoveHost deletes the configuration for the named host, if present.
func (c *Config) RemoveHost(name string) {
	delete(c.Hosts, name)
}

// TokenSource identifies where a resolved token came from.
type TokenSource int

const (
	// TokenSourceNone means no token is available.
	TokenSourceNone TokenSource = iota
	// TokenSourceEnv means the token came from the BH_TOKEN environment
	// variable.
	TokenSourceEnv
	// TokenSourceFile means the token came from the hosts file.
	TokenSourceFile
)

// String returns a short, user-facing name for the source.
func (s TokenSource) String() string {
	switch s {
	case TokenSourceEnv:
		return EnvToken
	case TokenSourceFile:
		return hostsFile
	default:
		return "none"
	}
}

// Token resolves the token for a host. The BH_TOKEN environment variable
// overrides any stored token. The source is TokenSourceNone when no token is
// available.
func (c *Config) Token(host string) (string, TokenSource) {
	if env := c.lookupEnv(EnvToken); env != "" {
		return env, TokenSourceEnv
	}
	if hc := c.Host(host); hc != nil && hc.Token != "" {
		return hc.Token, TokenSourceFile
	}
	return "", TokenSourceNone
}

// Email returns the Atlassian email to pair with the token Token resolves.
// When the token comes from BH_TOKEN, the stored email is ignored and
// BH_EMAIL is used instead (empty means Bearer auth); otherwise the stored
// email is returned.
func (c *Config) Email(host string) string {
	if _, source := c.Token(host); source == TokenSourceEnv {
		return c.lookupEnv(EnvEmail)
	}
	if hc := c.Host(host); hc != nil {
		return hc.Email
	}
	return ""
}
