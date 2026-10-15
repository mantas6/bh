package cmdutil

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
)

// PrintJSON writes v to w as indented JSON (two-space indent) without HTML
// escaping, followed by a trailing newline. A nil slice is printed as [] so
// an empty list is never null. Nothing is written if encoding fails.
func PrintJSON(w io.Writer, v any) error {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.IsNil() {
		v = reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
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
