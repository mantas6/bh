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
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(nonNilSlice(v)); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// nonNilSlice returns v, or an empty slice of the same type when v is a nil
// slice, so it encodes as [] rather than null.
func nonNilSlice(v any) any {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.IsNil() {
		return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	return v
}

// marshalNoEscape is json.Marshal without HTML escaping.
func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
