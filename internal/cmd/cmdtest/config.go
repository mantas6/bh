package cmdtest

import (
	"sync"
	"testing"

	"github.com/mantas6/bh/internal/config"
)

// Config is an in-memory configuration for command tests. It never touches
// the file system or the process environment, so tests using it may run in
// parallel.
type Config struct {
	cfg *config.Config

	mu    sync.Mutex
	saves int
	saved map[string]config.HostConfig
}

// NewConfig returns an empty in-memory configuration whose BH_TOKEN and
// BH_EMAIL overrides are read from env (nil means both are unset).
func NewConfig(env map[string]string) *Config {
	c := &Config{}
	c.cfg = config.NewInMemory(func(key string) string { return env[key] }, c.record)
	return c
}

// SetHost stores credentials for host, as if previously saved to hosts.yml.
func (c *Config) SetHost(host string, hc config.HostConfig) *Config {
	c.cfg.SetHost(host, &hc)
	return c
}

// Load returns the configuration; use it as an Options.Config provider.
func (c *Config) Load() (*config.Config, error) {
	return c.cfg, nil
}

// Saves reports how many times the configuration was saved.
func (c *Config) Saves() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saves
}

// SavedHost returns the credentials for host as of the last save, or nil if
// the configuration was never saved or host was absent at that point.
func (c *Config) SavedHost(host string) *config.HostConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	hc, ok := c.saved[host]
	if !ok {
		return nil
	}
	return &hc
}

func (c *Config) record(cfg *config.Config) error {
	snapshot := make(map[string]config.HostConfig, len(cfg.Hosts))
	for host, hc := range cfg.Hosts {
		if hc != nil {
			snapshot[host] = *hc
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saves++
	c.saved = snapshot
	return nil
}

// TempConfigDir points BH_CONFIG_DIR at a fresh temporary directory and
// clears BH_TOKEN and BH_EMAIL, for tests that exercise the real file-backed
// configuration. Such tests cannot run in parallel.
func TempConfigDir(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BH_CONFIG_DIR", dir)
	t.Setenv(config.EnvToken, "")
	t.Setenv(config.EnvEmail, "")
	return dir
}
