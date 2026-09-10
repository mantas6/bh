package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Table renders rows of cells. On a TTY it aligns columns with a tabwriter;
// otherwise it emits raw tab-separated fields for scriptability.
type Table struct {
	w     io.Writer
	tw    *tabwriter.Writer
	isTTY bool
}

// NewTable creates a Table writing to w. When isTTY is true columns are
// padded for readability; otherwise cells are tab-separated without padding.
func NewTable(w io.Writer, isTTY bool) *Table {
	t := &Table{w: w, isTTY: isTTY}
	if isTTY {
		t.tw = tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	}
	return t
}

// AddRow appends a row of cells.
func (t *Table) AddRow(cells ...string) {
	line := strings.Join(cells, "\t")
	if t.isTTY {
		fmt.Fprintln(t.tw, line)
	} else {
		fmt.Fprintln(t.w, line)
	}
}

// Flush writes any buffered output. It must be called once rows are added.
func (t *Table) Flush() error {
	if t.tw != nil {
		return t.tw.Flush()
	}
	return nil
}
