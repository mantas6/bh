package output

import (
	"bufio"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"
)

// columnGap is the number of spaces between padded columns on a TTY.
const columnGap = 2

// Table renders rows of cells. On a TTY it pads columns so they line up,
// measuring cells by their visible width (ANSI escape sequences are ignored);
// otherwise it emits raw tab-separated fields for scriptability.
//
// Tabs, carriage returns and newlines inside cells are replaced by spaces in
// both modes so a single cell can never break the row/column structure.
type Table struct {
	w     io.Writer
	isTTY bool
	rows  [][]string
}

// NewTable creates a Table writing to w. When isTTY is true columns are
// padded for readability; otherwise cells are tab-separated without padding.
func NewTable(w io.Writer, isTTY bool) *Table {
	return &Table{w: w, isTTY: isTTY}
}

// AddRow appends a row of cells. Nothing is written until Flush.
func (t *Table) AddRow(cells ...string) {
	row := make([]string, len(cells))
	for i, c := range cells {
		row[i] = sanitizeCell(c)
	}
	t.rows = append(t.rows, row)
}

// Flush writes all buffered rows and resets the table. It must be called once
// rows are added.
func (t *Table) Flush() error {
	rows := t.rows
	t.rows = nil

	bw := bufio.NewWriter(t.w)
	if !t.isTTY {
		for _, row := range rows {
			_, _ = bw.WriteString(strings.Join(row, "\t"))
			_ = bw.WriteByte('\n')
		}
		return bw.Flush()
	}

	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], displayWidth(cell))
		}
	}

	for _, row := range rows {
		for i, cell := range row {
			_, _ = bw.WriteString(cell)
			if i == len(row)-1 {
				break
			}
			pad := widths[i] - displayWidth(cell) + columnGap
			_, _ = bw.WriteString(strings.Repeat(" ", pad))
		}
		_ = bw.WriteByte('\n')
	}
	return bw.Flush()
}

var cellReplacer = strings.NewReplacer("\r\n", " ", "\t", " ", "\n", " ", "\r", " ")

// sanitizeCell replaces characters that would break table structure.
func sanitizeCell(s string) string {
	return cellReplacer.Replace(s)
}

// ansiRE matches CSI sequences (e.g. colors, "\x1b[1;32m") and OSC sequences
// (e.g. hyperlinks) terminated by BEL or ST.
var ansiRE = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	if !strings.Contains(s, "\x1b") {
		return s
	}
	return ansiRE.ReplaceAllString(s, "")
}

// displayWidth returns the number of visible characters in s, ignoring ANSI
// escape sequences. Each rune counts as one column.
func displayWidth(s string) int {
	return utf8.RuneCountInString(stripANSI(s))
}
