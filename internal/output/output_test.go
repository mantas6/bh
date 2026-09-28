package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestTableTTYAligns(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestColorSchemeDisabled(t *testing.T) {
	t.Parallel()
	cs := NewColorScheme(false)
	if got := cs.Green("x"); got != "x" {
		t.Errorf("disabled Green = %q", got)
	}
	if got := cs.StateColor("OPEN"); got != "OPEN" {
		t.Errorf("disabled StateColor = %q", got)
	}
}

func TestColorSchemeEnabled(t *testing.T) {
	t.Parallel()
	cs := NewColorScheme(true)
	if got := cs.StateColor("OPEN"); !strings.Contains(got, "\x1b[32m") {
		t.Errorf("OPEN should be green: %q", got)
	}
	if got := cs.StateColor("MERGED"); !strings.Contains(got, "\x1b[35m") {
		t.Errorf("MERGED should be magenta: %q", got)
	}
}
