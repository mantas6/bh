package output

import (
	"bytes"
	"testing"
)

func TestTableTTYExactPadding(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tbl := NewTable(&buf, true)
	tbl.AddRow("#1", "short", "ada")
	tbl.AddRow("#22", "longer title", "grace")
	if err := tbl.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	want := "" +
		"#1   short         ada\n" +
		"#22  longer title  grace\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%q\nwant\n%q", got, want)
	}
}

func TestTableTTYANSIAware(t *testing.T) {
	t.Parallel()
	cs := NewColorScheme(true)
	var buf bytes.Buffer
	tbl := NewTable(&buf, true)
	tbl.AddRow(cs.Green("OPEN"), "a")
	tbl.AddRow("MERGED", "b")
	if err := tbl.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	want := "" +
		"\x1b[32mOPEN\x1b[0m    a\n" +
		"MERGED  b\n"
	if got := buf.String(); got != want {
		t.Errorf("output =\n%q\nwant\n%q", got, want)
	}
}

func TestTableTTYUnicodeWidth(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tbl := NewTable(&buf, true)
	tbl.AddRow("✓ ok", "x")
	tbl.AddRow("fail", "y")
	if err := tbl.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	want := "✓ ok  x\nfail  y\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestTableTTYRaggedRows(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	tbl := NewTable(&buf, true)
	tbl.AddRow("a", "bb", "c")
	tbl.AddRow("aaa")
	if err := tbl.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	want := "a    bb  c\naaa\n"
	if got := buf.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestTableSanitizesCells(t *testing.T) {
	t.Parallel()
	for _, tty := range []bool{false, true} {
		var buf bytes.Buffer
		tbl := NewTable(&buf, tty)
		tbl.AddRow("a\tb", "line1\nline2", "cr\r\nlf", "x\ry")
		if err := tbl.Flush(); err != nil {
			t.Fatalf("flush: %v", err)
		}
		var want string
		if tty {
			want = "a b  line1 line2  cr lf  x y\n"
		} else {
			want = "a b\tline1 line2\tcr lf\tx y\n"
		}
		if got := buf.String(); got != want {
			t.Errorf("tty=%v output = %q, want %q", tty, got, want)
		}
	}
}

func TestDisplayWidth(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"\x1b[32mabc\x1b[0m", 3},
		{"\x1b[1;31mx\x1b[0m", 1},
		{"\x1b]8;;https://x\x1b\\link\x1b]8;;\x1b\\", 4},
		{"✓", 1},
	}
	for _, tc := range cases {
		if got := displayWidth(tc.in); got != tc.want {
			t.Errorf("displayWidth(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
