// Package config handles reading and writing bh configuration, including
// per-host authentication tokens stored in hosts.yml.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// DefaultHost is the default Bitbucket Cloud host.
	DefaultHost = "bitbucket.org"
	// DefaultAPIBase is the base URL for the Bitbucket Cloud REST 2.0 API.
	DefaultAPIBase = "https://api.bitbucket.org/2.0"

	hostsFile = "hosts.yml"
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

// Config is the top-level configuration, keyed by host name.
type Config struct {
	Hosts map[string]*HostConfig `yaml:"-"`
}

// ConfigDir returns the directory where bh stores its configuration.
// Resolution order: BH_CONFIG_DIR > $XDG_CONFIG_HOME/bh > ~/.config/bh.
func ConfigDir() string {
	if dir := os.Getenv("BH_CONFIG_DIR"); dir != "" {
		return dir
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "bh")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to a relative path; Save will surface any error.
		return filepath.Join(".config", "bh")
	}
	return filepath.Join(home, ".config", "bh")
}

func hostsPath() string {
	return filepath.Join(ConfigDir(), hostsFile)
}

// Load reads the configuration from disk. A missing hosts file yields an
// empty configuration rather than an error.
func Load() (*Config, error) {
	c := &Config{Hosts: map[string]*HostConfig{}}

	data, err := os.ReadFile(hostsPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return c, nil
		}
		return nil, err
	}

	hosts := map[string]*HostConfig{}
	if err := yaml.Unmarshal(data, &hosts); err != nil {
		return nil, err
	}
	if hosts == nil {
		hosts = map[string]*HostConfig{}
	}
	c.Hosts = hosts
	return c, nil
}

// Save writes the configuration to disk, creating the config directory with
// 0700 permissions and the hosts file with 0600 permissions.
func (c *Config) Save() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	hosts := c.Hosts
	if hosts == nil {
		hosts = map[string]*HostConfig{}
	}
	data, err := yaml.Marshal(hosts)
	if err != nil {
		return err
	}

	return os.WriteFile(hostsPath(), data, 0o600)
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

// Token resolves the token for a host. The BH_TOKEN environment variable
// overrides any stored token. The returned source is "BH_TOKEN", "hosts.yml",
// or "" when no token is available.
func (c *Config) Token(host string) (token string, source string) {
	if env := os.Getenv("BH_TOKEN"); env != "" {
		return env, "BH_TOKEN"
	}
	if hc := c.Host(host); hc != nil && hc.Token != "" {
		return hc.Token, "hosts.yml"
	}
	return "", ""
}

// Email returns the stored Atlassian email for a host, if any.
func (c *Config) Email(host string) string {
	if hc := c.Host(host); hc != nil {
		return hc.Email
	}
	return ""
}
