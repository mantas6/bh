// Package browser opens URLs in the user's web browser. The command is
// selected from the BROWSER environment variable when set, otherwise from the
// host operating system. Both the runner and OS/env are injectable for tests.
package browser

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Browser opens URLs. Runner executes the resolved command; GOOS and Env
// override the platform and environment lookups for tests.
type Browser struct {
	// Runner executes name with args. Defaults to os/exec wired to stderr.
	Runner func(name string, args ...string) error
	// GOOS overrides runtime.GOOS when non-empty.
	GOOS string
	// Env overrides os.Getenv when non-nil.
	Env func(string) string
}

// New returns a Browser wired to os/exec and the current process environment.
func New() *Browser {
	return &Browser{
		Runner: defaultRunner,
		GOOS:   runtime.GOOS,
		Env:    os.Getenv,
	}
}

func defaultRunner(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (b *Browser) goos() string {
	if b.GOOS != "" {
		return b.GOOS
	}
	return runtime.GOOS
}

func (b *Browser) env(key string) string {
	if b.Env != nil {
		return b.Env(key)
	}
	return os.Getenv(key)
}

// Command resolves the browser command and arguments for url without running
// it. It honors BROWSER, then falls back to a per-OS opener.
func (b *Browser) Command(url string) (name string, args []string, err error) {
	if browser := strings.TrimSpace(b.env("BROWSER")); browser != "" {
		fields := strings.Fields(browser)
		if len(fields) == 0 {
			return "", nil, errors.New("BROWSER environment variable is empty")
		}
		args = append(args, fields[1:]...)
		args = append(args, url)
		return fields[0], args, nil
	}

	switch b.goos() {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	case "darwin":
		return "open", []string{url}, nil
	default:
		return "xdg-open", []string{url}, nil
	}
}

// Browse opens url in the browser.
func (b *Browser) Browse(url string) error {
	name, args, err := b.Command(url)
	if err != nil {
		return err
	}
	runner := b.Runner
	if runner == nil {
		runner = defaultRunner
	}
	return runner(name, args...)
}
