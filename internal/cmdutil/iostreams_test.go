package cmdutil

import (
	"io"
	"strings"
	"testing"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestColorEnabledEnv(t *testing.T) {
	tests := []struct {
		name string
		tty  bool
		env  map[string]string
		want bool
	}{
		{"tty", true, nil, true},
		{"not tty", false, nil, false},
		{"NO_COLOR", true, map[string]string{"NO_COLOR": "1"}, false},
		{"empty NO_COLOR ignored", true, map[string]string{"NO_COLOR": ""}, true},
		{"CLICOLOR=0", true, map[string]string{"CLICOLOR": "0"}, false},
		{"CLICOLOR=1", true, map[string]string{"CLICOLOR": "1"}, true},
		{"TERM=dumb", true, map[string]string{"TERM": "dumb"}, false},
		{"CLICOLOR_FORCE without tty", false, map[string]string{"CLICOLOR_FORCE": "1"}, true},
		{"CLICOLOR_FORCE=0", false, map[string]string{"CLICOLOR_FORCE": "0"}, false},
		{"CLICOLOR_FORCE beats NO_COLOR", false, map[string]string{"CLICOLOR_FORCE": "1", "NO_COLOR": "1"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := TestIOStreams()
			ios.Getenv = envMap(tt.env)

			ios.SetStdoutTTY(tt.tty)
			if got := ios.ColorEnabled(); got != tt.want {
				t.Errorf("ColorEnabled() = %v, want %v", got, tt.want)
			}

			ios.SetStdoutTTY(false)
			ios.SetStderrTTY(tt.tty)
			if got := ios.ErrColorEnabled(); got != tt.want {
				t.Errorf("ErrColorEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestColorSchemesFollowTheirStream(t *testing.T) {
	ios, _, _, _ := TestIOStreams()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(false)

	if got := ios.ColorScheme().SuccessIcon(); !strings.Contains(got, "\x1b[") {
		t.Errorf("stdout scheme should be coloured: %q", got)
	}
	// e.g. `bh ... 2>log`: stderr is redirected, so no escape codes.
	if got := ios.ErrColorScheme().SuccessIcon(); got != "✓" {
		t.Errorf("stderr scheme should be plain: %q", got)
	}

	ios.SetStdoutTTY(false)
	ios.SetStderrTTY(true)
	if got := ios.ColorScheme().Green("x"); got != "x" {
		t.Errorf("stdout scheme should be plain: %q", got)
	}
	if got := ios.ErrColorScheme().Green("x"); !strings.Contains(got, "\x1b[32m") {
		t.Errorf("stderr scheme should be coloured: %q", got)
	}
}

func TestTestIOStreamsIgnoresProcessEnv(t *testing.T) {
	t.Setenv("CLICOLOR_FORCE", "1")
	ios, _, _, _ := TestIOStreams()
	if ios.ColorEnabled() {
		t.Error("TestIOStreams should not read the process environment")
	}
}

func TestStdinFd(t *testing.T) {
	ios, _, _, _ := TestIOStreams()
	if _, ok := ios.StdinFd(); ok {
		t.Error("StdinFd should be unavailable for an in-memory reader")
	}
}

func TestStdinSharedWithPrompter(t *testing.T) {
	ios, in, _, _ := TestIOStreams()
	in.WriteString("title\nrest of\nthe body\n")

	title, err := ios.Prompter().Input("Title", "")
	if err != nil || title != "title" {
		t.Fatalf("Input = %q, %v", title, err)
	}
	rest, err := io.ReadAll(ios.Stdin())
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "rest of\nthe body\n" {
		t.Errorf("remaining stdin = %q; input buffered by the prompter was lost", rest)
	}
	if ios.Prompter() != ios.Prompter() {
		t.Error("Prompter() should return a shared instance")
	}
}
