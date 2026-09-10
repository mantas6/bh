package browser

import (
	"reflect"
	"testing"
)

func TestCommand(t *testing.T) {
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
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
				t.Errorf("args = %v, want %v", args, tc.wantArgs)
			}
		})
	}
}

func TestBrowseInvokesRunner(t *testing.T) {
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
