// Package browser opens URLs in the user's web browser. The command is
// selected from the BROWSER environment variable when set, otherwise from the
// host operating system. Both the runner and OS/env are injectable for tests.
package browser

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ErrEmptyURL is returned when asked to open an empty URL.
var ErrEmptyURL = errors.New("no URL to open")

// Browser opens URLs. Runner executes the resolved command; GOOS and Env
// override the platform and environment lookups for tests.
type Browser struct {
	// Runner executes name with args. When nil the command is run with
	// os/exec, with its stdout and stderr sent to Stderr.
	Runner func(name string, args ...string) error
	// GOOS overrides runtime.GOOS when non-empty.
	GOOS string
	// Env overrides os.Getenv when non-nil.
	Env func(string) string
	// Stderr receives the browser process's stdout and stderr when the
	// default runner is used. Nil discards the output.
	Stderr io.Writer
}

// New returns a Browser wired to os/exec and the current process environment.
// Output from the launched browser command is written to stderr.
func New(stderr io.Writer) *Browser {
	return &Browser{
		GOOS:   runtime.GOOS,
		Env:    os.Getenv,
		Stderr: stderr,
	}
}

func (b *Browser) run(ctx context.Context, name string, args ...string) error {
	if b.Runner != nil {
		return b.Runner(name, args...)
	}
	out := b.Stderr
	if out == nil {
		out = io.Discard
	}
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = out
	cmd.Stderr = out
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
//
// BROWSER is split into words with shell-like quoting (single quotes, double
// quotes and backslash escapes). If any word contains "%s" every occurrence is
// replaced by url; otherwise url is appended as the final argument.
func (b *Browser) Command(url string) (name string, args []string, err error) {
	if strings.TrimSpace(url) == "" {
		return "", nil, ErrEmptyURL
	}

	if browser := strings.TrimSpace(b.env("BROWSER")); browser != "" {
		words, err := splitCommand(browser)
		if err != nil {
			return "", nil, fmt.Errorf("invalid BROWSER environment variable: %w", err)
		}
		if len(words) == 0 || words[0] == "" {
			return "", nil, errors.New("invalid BROWSER environment variable: no command")
		}
		substituted := false
		for i, w := range words[1:] {
			if strings.Contains(w, "%s") {
				words[i+1] = strings.ReplaceAll(w, "%s", url)
				substituted = true
			}
		}
		args = words[1:]
		if !substituted {
			args = append(args, url)
		}
		return words[0], args, nil
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

// Browse opens url in the browser. It is BrowseContext with a background
// context: the opener is not killed when the calling command is interrupted.
func (b *Browser) Browse(url string) error {
	return b.BrowseContext(context.Background(), url)
}

// BrowseContext opens url in the browser. When the default runner is used the
// launched process is killed if ctx is done before it exits.
func (b *Browser) BrowseContext(ctx context.Context, url string) error {
	name, args, err := b.Command(url)
	if err != nil {
		return err
	}
	if err := b.run(ctx, name, args...); err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}
	return nil
}

// splitCommand splits s into words following a small subset of POSIX shell
// quoting rules: whitespace separates words; single quotes preserve every
// character literally; double quotes preserve everything except that a
// backslash escapes '"', '\\', '$' and '`'; outside quotes a backslash
// escapes the next character. Quoted empty strings produce empty words.
func splitCommand(s string) ([]string, error) {
	const (
		plain = iota
		single
		double
	)
	var (
		words  []string
		cur    strings.Builder
		inWord bool
		state  = plain
		// escaped is set after a backslash; the next rune is taken
		// literally (outside quotes) or per the double-quote rule.
		escaped bool
	)
	for _, r := range s {
		switch {
		case escaped:
			if state == double && !strings.ContainsRune("\"\\$`", r) {
				cur.WriteRune('\\')
			}
			cur.WriteRune(r)
			escaped = false
		case state == single:
			if r == '\'' {
				state = plain
			} else {
				cur.WriteRune(r)
			}
		case state == double:
			switch r {
			case '"':
				state = plain
			case '\\':
				escaped = true
			default:
				cur.WriteRune(r)
			}
		case r == '\\':
			escaped, inWord = true, true
		case r == '\'':
			state, inWord = single, true
		case r == '"':
			state, inWord = double, true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	switch {
	case escaped:
		return nil, errors.New("trailing backslash")
	case state == single:
		return nil, errors.New("unterminated single quote")
	case state == double:
		return nil, errors.New("unterminated double quote")
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, nil
}
