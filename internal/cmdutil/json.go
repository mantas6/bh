package cmdutil

import (
	"bytes"
	"encoding/json"
	"io"
)

// PrintJSON writes v to w as indented JSON (two-space indent) without HTML
// escaping, followed by a trailing newline. Nothing is written if encoding
// fails.
func PrintJSON(w io.Writer, v any) error {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}
