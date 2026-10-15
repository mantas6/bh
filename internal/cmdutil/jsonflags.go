package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

// jsonAllFields is the value pflag passes to the --json flag when it is
// given without a value; it also shows in help as `--json[=fields]`.
const jsonAllFields = "fields"

// JSONOutput holds the --json field selection and the --jq expression of a
// command registered with AddJSONFlags. The zero value prints everything.
type JSONOutput struct {
	// Fields are the selected field paths ("id", "source.branch.name");
	// empty means all fields.
	Fields []string
	// JQ is a jq expression applied to the (selected) JSON; empty means none.
	JQ string
}

// AddJSONFlags registers --json[=fields] and --jq/-q on cmd. --json alone
// sets *enabled; --json=a,b.c also selects the comma-separated fields, which
// are validated against the JSON encoding of typ (the printed type, or the
// element type of a printed list). --jq requires --json. what describes
// the output in the help text, e.g. "the pull request".
func AddJSONFlags(cmd *cobra.Command, enabled *bool, out *JSONOutput, typ reflect.Type, what string) {
	v := &jsonFlagValue{enabled: enabled, out: out, typ: typ}
	f := cmd.Flags().VarPF(v, "json", "",
		"Output "+what+" as JSON; --json=a,b.c keeps only the given comma-separated fields")
	f.NoOptDefVal = jsonAllFields
	cmd.Flags().StringVarP(&out.JQ, "jq", "q", "", "Filter JSON output using a jq `expression` (requires --json)")

	prev := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if c.Flags().Changed("jq") {
			if !*enabled {
				return FlagErrorf("--jq requires --json")
			}
			if _, err := compileJQ(out.JQ); err != nil {
				return FlagErrorWrap(err)
			}
		}
		if prev != nil {
			return prev(c, args)
		}
		return nil
	}
}

// Print writes v to ios.Out as JSON, keeping only the selected fields and
// applying the jq expression when set. Without either it is PrintJSON. jq
// results that are strings are printed raw, other values as JSON (indented
// when stdout is a terminal), one per line.
func (o *JSONOutput) Print(ios *IOStreams, v any) error {
	if len(o.Fields) == 0 && o.JQ == "" {
		return PrintJSON(ios.Out, v)
	}
	data, err := marshalNoEscape(nonNilSlice(v))
	if err != nil {
		return err
	}
	doc, err := decodeOrdered(data)
	if err != nil {
		return err
	}
	if len(o.Fields) > 0 {
		paths := make([][]string, len(o.Fields))
		for i, f := range o.Fields {
			paths[i] = strings.Split(f, ".")
		}
		doc = project(doc, paths)
	}
	if o.JQ == "" {
		return PrintJSON(ios.Out, doc)
	}
	return runJQ(ios.Out, o.JQ, plain(doc), ios.IsStdoutTTY())
}

// NormalizeJSONArgs rewrites "--json a,b" into "--json=a,b" for the command
// args select under root, when the next argument looks like a field list of
// that command (see looksLikeJSONFields). pflag never lets a flag with an
// optional value consume the next argument, so this is what makes the
// space-separated form work; anything else (such as `--json 123` or
// `--json my-branch`) is left alone as a positional argument.
func NormalizeJSONArgs(root *cobra.Command, args []string) []string {
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil {
		return args
	}
	f := cmd.Flags().Lookup("json")
	if f == nil {
		return args
	}
	v, ok := f.Value.(*jsonFlagValue)
	if !ok {
		return args
	}
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			out = append(out, args[i:]...)
			break
		}
		if a == "--json" && i+1 < len(args) && looksLikeJSONFields(v.typ, args[i+1]) {
			out = append(out, "--json="+args[i+1])
			i++
			continue
		}
		out = append(out, a)
	}
	return out
}

// looksLikeJSONFields reports whether arg, following a bare --json, is meant
// as its field list: it is a valid one, or it contains a comma or starts with
// a known field name (so a typo is reported as an unknown field rather than
// taken as a pull request argument).
func looksLikeJSONFields(typ reflect.Type, arg string) bool {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return false
	}
	if _, err := parseJSONFields(typ, arg); err == nil {
		return true
	}
	if strings.Contains(arg, ",") {
		return true
	}
	first, _, _ := strings.Cut(arg, ".")
	return slices.Contains(JSONFieldNames(typ), first)
}

type jsonFlagValue struct {
	enabled *bool
	out     *JSONOutput
	typ     reflect.Type
}

func (v *jsonFlagValue) String() string {
	if v.out == nil {
		return ""
	}
	return strings.Join(v.out.Fields, ",")
}

// Type is empty so help shows `--json[=fields]` without a value name.
func (v *jsonFlagValue) Type() string { return "" }

func (v *jsonFlagValue) Set(s string) error {
	*v.enabled = true
	if s == jsonAllFields {
		return nil
	}
	fields, err := parseJSONFields(v.typ, s)
	if err != nil {
		return err
	}
	for _, f := range fields {
		if !slices.Contains(v.out.Fields, f) {
			v.out.Fields = append(v.out.Fields, f)
		}
	}
	return nil
}

// parseJSONFields splits a comma-separated field list and checks every
// dotted path against the JSON encoding of typ.
func parseJSONFields(typ reflect.Type, s string) ([]string, error) {
	var fields []string
	for f := range strings.SplitSeq(s, ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if err := checkJSONPath(typ, f); err != nil {
			return nil, err
		}
		if !slices.Contains(fields, f) {
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("no JSON fields given; available fields: %s", strings.Join(JSONFieldNames(typ), ", "))
	}
	return fields, nil
}

func checkJSONPath(typ reflect.Type, path string) error {
	t := typ
	segs := strings.Split(path, ".")
	for i, seg := range segs {
		t = jsonElem(t)
		if t == nil {
			// An interface or map: any key may be present.
			return nil
		}
		fields := jsonFields(t)
		if fields == nil {
			return fmt.Errorf("unknown JSON field %q: %s has no fields", path, strings.Join(segs[:i], "."))
		}
		ft, ok := fields[seg]
		if !ok {
			where := "available fields"
			if i > 0 {
				where = "fields of " + strings.Join(segs[:i], ".")
			}
			return fmt.Errorf("unknown JSON field %q; %s: %s", path, where, strings.Join(sortedKeys(fields), ", "))
		}
		t = ft
	}
	return nil
}

// JSONFieldNames returns the sorted top-level JSON field names of typ (or of
// its element type for a slice).
func JSONFieldNames(typ reflect.Type) []string {
	t := jsonElem(typ)
	if t == nil {
		return nil
	}
	return sortedKeys(jsonFields(t))
}

var timeType = reflect.TypeFor[time.Time]()

// jsonElem strips pointers and slices from t. It returns nil for interfaces
// and maps, whose keys are not known statically.
func jsonElem(t reflect.Type) reflect.Type {
	for {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Array:
			t = t.Elem()
		case reflect.Interface, reflect.Map:
			return nil
		default:
			return t
		}
	}
}

// jsonFields returns the JSON object fields of struct type t (including
// promoted fields of embedded structs), or nil when t does not encode as an
// object with known fields.
func jsonFields(t reflect.Type) map[string]reflect.Type {
	if t.Kind() != reflect.Struct || t == timeType {
		return nil
	}
	fields := map[string]reflect.Type{}
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if f.Anonymous && name == "" {
			et := f.Type
			if et.Kind() == reflect.Pointer {
				et = et.Elem()
			}
			for k, v := range jsonFields(et) {
				if _, ok := fields[k]; !ok {
					fields[k] = v
				}
			}
			continue
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		fields[name] = f.Type
	}
	return fields
}

func sortedKeys(m map[string]reflect.Type) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// object is a decoded JSON object that remembers its key order.
type object struct {
	keys []string
	vals map[string]any
}

func (o *object) set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

// MarshalJSON encodes o with its keys in their original order.
func (o *object) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, err := marshalNoEscape(k)
		if err != nil {
			return nil, err
		}
		vb, err := marshalNoEscape(o.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// decodeOrdered decodes data into *object, []any, json.Number, string, bool
// and nil values.
func decodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return decodeValue(dec)
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok {
	case json.Delim('{'):
		o := &object{vals: map[string]any{}}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			o.set(kt.(string), v)
		}
		_, err := dec.Token()
		return o, err
	case json.Delim('['):
		arr := []any{}
		for dec.More() {
			v, err := decodeValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.Token()
		return arr, err
	default:
		return tok, nil
	}
}

// project keeps only the given paths of v, applying them to every element
// of an array. Keys missing from an object are left out; a path that runs
// into a scalar (such as null) keeps that scalar.
func project(v any, paths [][]string) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = project(e, paths)
		}
		return out
	case *object:
		var keys []string
		whole := map[string]bool{}
		sub := map[string][][]string{}
		for _, p := range paths {
			k := p[0]
			if _, seen := sub[k]; !seen && !whole[k] {
				keys = append(keys, k)
			}
			if len(p) == 1 {
				whole[k] = true
			} else {
				sub[k] = append(sub[k], p[1:])
			}
		}
		out := &object{vals: map[string]any{}}
		for _, k := range keys {
			val, ok := x.vals[k]
			if !ok {
				continue
			}
			if whole[k] {
				out.set(k, val)
			} else {
				out.set(k, project(val, sub[k]))
			}
		}
		return out
	default:
		return v
	}
}

// plain converts *object values to map[string]any for gojq.
func plain(v any) any {
	switch x := v.(type) {
	case *object:
		m := make(map[string]any, len(x.vals))
		for k, e := range x.vals {
			m[k] = plain(e)
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = plain(e)
		}
		return out
	default:
		return v
	}
}

func compileJQ(expr string) (*gojq.Code, error) {
	q, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq expression: %w", err)
	}
	code, err := gojq.Compile(q, gojq.WithEnvironLoader(os.Environ))
	if err != nil {
		return nil, fmt.Errorf("invalid --jq expression: %w", err)
	}
	return code, nil
}

func runJQ(w io.Writer, expr string, input any, indent bool) error {
	code, err := compileJQ(expr)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	iter := code.Run(input)
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := v.(error); ok {
			var halt *gojq.HaltError
			if errors.As(err, &halt) && halt.Value() == nil {
				break
			}
			// Print what was produced before the error.
			_, _ = w.Write(buf.Bytes())
			return fmt.Errorf("jq: %w", err)
		}
		if s, ok := v.(string); ok {
			buf.WriteString(s)
		} else {
			b, err := gojq.Marshal(v)
			if err != nil {
				return err
			}
			if indent {
				var ib bytes.Buffer
				if err := json.Indent(&ib, b, "", "  "); err == nil {
					b = ib.Bytes()
				}
			}
			buf.Write(b)
		}
		buf.WriteByte('\n')
	}
	_, err = w.Write(buf.Bytes())
	return err
}
