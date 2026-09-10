package output

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestTableTTYAligns(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, true)
	tbl.AddRow("#1", "short", "ada")
	tbl.AddRow("#22", "longer title", "grace")
	if err := tbl.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	got := buf.String()
	// Padding should insert more than a single space between columns.
	if !strings.Contains(got, "#1 ") || !strings.Contains(got, "short ") {
		t.Errorf("expected padded output, got %q", got)
	}
	if strings.Contains(got, "\t") {
		t.Errorf("TTY output should not contain tabs: %q", got)
	}
}

func TestTableNonTTYTabs(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, false)
	tbl.AddRow("#1", "short", "ada")
	if err := tbl.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	got := buf.String()
	if got != "#1\tshort\tada\n" {
		t.Errorf("non-TTY output = %q", got)
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		t    time.Time
		want string
	}{
		{"seconds", now.Add(-30 * time.Second), "less than a minute ago"},
		{"minutes", now.Add(-5 * time.Minute), "5 minutes ago"},
		{"hours", now.Add(-3 * time.Hour), "about 3 hours ago"},
		{"one day", now.Add(-25 * time.Hour), "1 day ago"},
		{"days", now.Add(-48 * time.Hour), "2 days ago"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativeTime(tc.t, now); got != tc.want {
				t.Errorf("RelativeTime = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestColorSchemeDisabled(t *testing.T) {
	cs := NewColorScheme(false)
	if got := cs.Green("x"); got != "x" {
		t.Errorf("disabled Green = %q", got)
	}
	if got := cs.StateColor("OPEN"); got != "OPEN" {
		t.Errorf("disabled StateColor = %q", got)
	}
}

func TestColorSchemeEnabled(t *testing.T) {
	cs := NewColorScheme(true)
	if got := cs.StateColor("OPEN"); !strings.Contains(got, "\x1b[32m") {
		t.Errorf("OPEN should be green: %q", got)
	}
	if got := cs.StateColor("MERGED"); !strings.Contains(got, "\x1b[35m") {
		t.Errorf("MERGED should be magenta: %q", got)
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := PrintJSON(&buf, map[string]string{"url": "a&b"}); err != nil {
		t.Fatalf("PrintJSON: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, `"url": "a&b"`) {
		t.Errorf("JSON should not HTML-escape: %q", got)
	}
	if !strings.HasSuffix(got, "}\n") {
		t.Errorf("expected trailing newline: %q", got)
	}
}
