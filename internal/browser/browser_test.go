package browser

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		goos     string
		browser  string
		wantName string
		wantArgs []string
	}{
		{"linux", "linux", "", "xdg-open", []string{"https://x"}},
		{"darwin", "darwin", "", "open", []string{"https://x"}},
		{"windows", "windows", "", "rundll32", []string{"url.dll,FileProtocolHandler", "https://x"}},
		{"browser env", "linux", "firefox", "firefox", []string{"https://x"}},
		{"browser env with args", "linux", "google-chrome --incognito", "google-chrome", []string{"--incognito", "https://x"}},
		{"whitespace only falls back", "linux", "   ", "xdg-open", []string{"https://x"}},
		{"double-quoted path", "linux", `"/opt/My Browser/bin" --new-tab`, "/opt/My Browser/bin", []string{"--new-tab", "https://x"}},
		{"single-quoted path", "darwin", `'/Applications/Google Chrome.app/x' -a`, "/Applications/Google Chrome.app/x", []string{"-a", "https://x"}},
		{"backslash-escaped space", "linux", `/opt/My\ Browser --flag`, "/opt/My Browser", []string{"--flag", "https://x"}},
		{"percent-s substitution", "linux", "firefox --new-window %s", "firefox", []string{"--new-window", "https://x"}},
		{"percent-s inside word", "linux", "w3m --url=%s", "w3m", []string{"--url=https://x"}},
		{"percent-s repeated", "linux", "sh -c 'echo %s; open %s'", "sh", []string{"-c", "echo https://x; open https://x"}},
		{"quoted percent-s", "linux", `browser "%s"`, "browser", []string{"https://x"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := &Browser{
				GOOS: tc.goos,
				Env:  func(string) string { return tc.browser },
			}
			name, args, err := b.Command("https://x")
			if err != nil {
				t.Fatalf("Command: %v", err)
			}
			if name != tc.wantName {
				t.Errorf("name = %q, want %q", name, tc.wantName)
			}
			if !reflect.DeepEqual(args, tc.wantArgs) {
				t.Errorf("args = %q, want %q", args, tc.wantArgs)
			}
		})
	}
}

func TestCommandErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		browser string
		url     string
		want    string
	}{
		{"empty url", "", "", "no URL to open"},
		{"blank url", "firefox", "  ", "no URL to open"},
		{"unterminated single", "'firefox", "https://x", "unterminated single quote"},
		{"unterminated double", `"firefox`, "https://x", "unterminated double quote"},
		{"trailing backslash", `firefox\`, "https://x", "trailing backslash"},
		{"empty command", `"" --flag`, "https://x", "no command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := &Browser{GOOS: "linux", Env: func(string) string { return tc.browser }}
			_, _, err := b.Command(tc.url)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestBrowseEmptyURLDoesNotRun(t *testing.T) {
	t.Parallel()
	ran := false
	b := &Browser{
		GOOS:   "linux",
		Env:    func(string) string { return "" },
		Runner: func(string, ...string) error { ran = true; return nil },
	}
	if err := b.Browse(""); !errors.Is(err, ErrEmptyURL) {
		t.Fatalf("err = %v, want ErrEmptyURL", err)
	}
	if ran {
		t.Error("runner should not be invoked for an empty URL")
	}
}

func TestSplitCommand(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"", nil, false},
		{"a b  c", []string{"a", "b", "c"}, false},
		{"\ta\n b\t", []string{"a", "b"}, false},
		{`'a b' "c d"`, []string{"a b", "c d"}, false},
		{`a'b'"c"`, []string{"abc"}, false},
		{`'' x`, []string{"", "x"}, false},
		{`'it\'s'`, nil, true}, // backslash is literal in single quotes -> unterminated
		{`"say \"hi\""`, []string{`say "hi"`}, false},
		{`"a\\b"`, []string{`a\b`}, false},
		{`"a\nb"`, []string{`a\nb`}, false},
		{`a\ b`, []string{"a b"}, false},
		{`a\\b`, []string{`a\b`}, false},
		{`'$HOME' "$HOME"`, []string{"$HOME", "$HOME"}, false},
	}
	for _, tc := range cases {
		got, err := splitCommand(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("splitCommand(%q) expected error, got %q", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("splitCommand(%q): %v", tc.in, err)
			continue
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitCommand(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBrowseInvokesRunner(t *testing.T) {
	t.Parallel()
	var gotName string
	var gotArgs []string
	b := &Browser{
		GOOS: "linux",
		Env:  func(string) string { return "" },
		Runner: func(name string, args ...string) error {
			gotName = name
			gotArgs = args
			return nil
		},
	}
	if err := b.Browse("https://example.com"); err != nil {
		t.Fatalf("Browse: %v", err)
	}
	if gotName != "xdg-open" {
		t.Errorf("name = %q", gotName)
	}
	if !reflect.DeepEqual(gotArgs, []string{"https://example.com"}) {
		t.Errorf("args = %v", gotArgs)
	}
}

func TestBrowseDefaultRunnerWritesToStderr(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	var stderr bytes.Buffer
	b := New(&stderr)
	b.Env = func(key string) string {
		if key == "BROWSER" {
			return `'` + sh + `' -c 'echo "out:$0"; echo "err:$0" >&2' %s`
		}
		return ""
	}
	if err := b.Browse("https://example.com"); err != nil {
		t.Fatalf("Browse: %v", err)
	}
	got := stderr.String()
	for _, want := range []string{"out:https://example.com", "err:https://example.com"} {
		if !strings.Contains(got, want) {
			t.Errorf("stderr = %q, want containing %q", got, want)
		}
	}
}

func TestBrowseDefaultRunnerNilStderr(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	if _, err := exec.LookPath("true"); err != nil {
		t.Skip("true not available")
	}
	b := &Browser{Env: func(string) string { return "true" }}
	if err := b.Browse("https://example.com"); err != nil {
		t.Fatalf("Browse: %v", err)
	}
}

func TestBrowseContextCanceled(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX sleep")
	}
	if _, err := exec.LookPath("sleep"); err != nil {
		t.Skip("sleep not available")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b := &Browser{Env: func(string) string { return "sleep 5 %s" }}
	if err := b.BrowseContext(ctx, "10"); err == nil {
		t.Fatal("expected error for canceled context")
	}
}
