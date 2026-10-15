package cmdutil

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type testInner struct {
	Name string `json:"name"`
	Tag  string `json:"tag,omitempty"`
}

type testEmbedded struct {
	Extra bool `json:"extra"`
}

type testItem struct {
	testEmbedded
	ID      int            `json:"id"`
	Title   string         `json:"title"`
	Inner   testInner      `json:"inner"`
	Opt     *testInner     `json:"opt,omitempty"`
	Ptr     *testInner     `json:"ptr"`
	List    []testInner    `json:"list"`
	Any     any            `json:"any,omitempty"`
	Skipped string         `json:"-"`
	Map     map[string]int `json:"map,omitempty"`
}

func sampleItem() testItem {
	return testItem{
		testEmbedded: testEmbedded{Extra: true},
		ID:           7,
		Title:        "a <b> & c",
		Inner:        testInner{Name: "in", Tag: "t"},
		Ptr:          nil,
		List:         []testInner{{Name: "x"}, {Name: "y", Tag: "z"}},
	}
}

// newJSONCmd returns a command with the JSON flags whose run records that
// it ran.
func newJSONCmd(enabled *bool, out *JSONOutput, ran *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "x",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(*cobra.Command, []string) error {
			*ran = true
			return nil
		},
	}
	AddJSONFlags(cmd, enabled, out, reflect.TypeFor[testItem](), "the item")
	return cmd
}

func runJSONCmd(t *testing.T, args ...string) (bool, JSONOutput, []string, error) {
	t.Helper()
	var enabled, ran bool
	var out JSONOutput
	var gotArgs []string
	cmd := newJSONCmd(&enabled, &out, &ran)
	run := cmd.RunE
	cmd.RunE = func(c *cobra.Command, a []string) error {
		gotArgs = a
		return run(c, a)
	}
	cmd.SetArgs(args)
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	err := cmd.Execute()
	if err == nil && !ran {
		t.Fatal("command did not run")
	}
	return enabled, out, gotArgs, err
}

func TestJSONFlagParsing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		enabled bool
		fields  []string
		jq      string
		rest    []string
		wantErr string
	}{
		{name: "off", args: nil},
		{name: "bare", args: []string{"--json"}, enabled: true},
		{name: "bare keeps positional", args: []string{"--json", "12"}, enabled: true, rest: []string{"12"}},
		{name: "fields", args: []string{"--json=id,title"}, enabled: true, fields: []string{"id", "title"}},
		{name: "spaces and empties", args: []string{"--json= id , ,title,id"}, enabled: true, fields: []string{"id", "title"}},
		{name: "nested", args: []string{"--json=inner.name,list.tag,ptr,opt.name"}, enabled: true, fields: []string{"inner.name", "list.tag", "ptr", "opt.name"}},
		{name: "embedded and any", args: []string{"--json=extra,any.whatever.deep,map.k"}, enabled: true, fields: []string{"extra", "any.whatever.deep", "map.k"}},
		{name: "repeated", args: []string{"--json=id", "--json", "--json=title"}, enabled: true, fields: []string{"id", "title"}},
		{name: "jq", args: []string{"--json", "--jq", ".id"}, enabled: true, jq: ".id"},
		{name: "jq short", args: []string{"--json=id", "-q", ".id"}, enabled: true, fields: []string{"id"}, jq: ".id"},
		{name: "unknown", args: []string{"--json=id,nope"}, wantErr: `unknown JSON field "nope"; available fields: any, extra, id, inner, list, map, opt, ptr, title`},
		{name: "unknown nested", args: []string{"--json=inner.nope"}, wantErr: `unknown JSON field "inner.nope"; fields of inner: name, tag`},
		{name: "scalar has no fields", args: []string{"--json=id.x"}, wantErr: `unknown JSON field "id.x": id has no fields`},
		{name: "skipped field", args: []string{"--json=Skipped"}, wantErr: `unknown JSON field "Skipped"`},
		{name: "empty", args: []string{"--json="}, wantErr: "no JSON fields given; available fields: any, extra"},
		{name: "jq without json", args: []string{"--jq", ".id"}, wantErr: "--jq requires --json"},
		{name: "jq invalid", args: []string{"--json", "--jq", ".["}, wantErr: "invalid --jq expression"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			enabled, out, rest, err := runJSONCmd(t, tt.args...)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if enabled != tt.enabled || !slices.Equal(out.Fields, tt.fields) || out.JQ != tt.jq || !slices.Equal(rest, tt.rest) {
				t.Errorf("enabled=%v fields=%q jq=%q args=%q, want %v %q %q %q",
					enabled, out.Fields, out.JQ, rest, tt.enabled, tt.fields, tt.jq, tt.rest)
			}
		})
	}
}

func TestJSONFlagErrorsAreFlagErrors(t *testing.T) {
	t.Parallel()
	_, _, _, err := runJSONCmd(t, "--jq", ".")
	var fe *FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("error = %v (%T), want *FlagError", err, err)
	}
}

func TestJSONFlagHelp(t *testing.T) {
	t.Parallel()
	var enabled, ran bool
	var out JSONOutput
	usage := newJSONCmd(&enabled, &out, &ran).Flags().FlagUsages()
	for _, want := range []string{"--json[=fields]", "-q, --jq expression"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage missing %q:\n%s", want, usage)
		}
	}
}

func TestJSONOutputPrint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		out    JSONOutput
		v      any
		tty    bool
		want   string
		errMsg string
	}{
		{
			name: "all fields is PrintJSON",
			v:    []testInner(nil),
			want: "[]\n",
		},
		{
			name: "fields keep the requested order",
			out:  JSONOutput{Fields: []string{"title", "id"}},
			v:    sampleItem(),
			want: "{\n  \"title\": \"a <b> & c\",\n  \"id\": 7\n}\n",
		},
		{
			name: "nested paths merge and keep struct order inside whole values",
			out:  JSONOutput{Fields: []string{"inner.tag", "list.name", "inner.name", "ptr.name", "opt.name", "extra"}},
			v:    sampleItem(),
			want: `{
  "inner": {
    "tag": "t",
    "name": "in"
  },
  "list": [
    {
      "name": "x"
    },
    {
      "name": "y"
    }
  ],
  "ptr": null,
  "extra": true
}
`,
		},
		{
			name: "whole nested value",
			out:  JSONOutput{Fields: []string{"list"}},
			v:    sampleItem(),
			want: "{\n  \"list\": [\n    {\n      \"name\": \"x\"\n    },\n    {\n      \"name\": \"y\",\n      \"tag\": \"z\"\n    }\n  ]\n}\n",
		},
		{
			name: "lists select per element",
			out:  JSONOutput{Fields: []string{"id"}},
			v:    []testItem{{ID: 1}, {ID: 2}},
			want: "[\n  {\n    \"id\": 1\n  },\n  {\n    \"id\": 2\n  }\n]\n",
		},
		{
			name: "empty list with fields",
			out:  JSONOutput{Fields: []string{"id"}},
			v:    []testItem(nil),
			want: "[]\n",
		},
		{
			name: "jq strings are raw",
			out:  JSONOutput{JQ: ".title, .id, .inner"},
			v:    sampleItem(),
			want: "a <b> & c\n7\n{\"name\":\"in\",\"tag\":\"t\"}\n",
		},
		{
			name: "jq on a terminal indents",
			out:  JSONOutput{JQ: ".inner"},
			v:    sampleItem(),
			tty:  true,
			want: "{\n  \"name\": \"in\",\n  \"tag\": \"t\"\n}\n",
		},
		{
			name: "jq after field selection",
			out:  JSONOutput{Fields: []string{"id"}, JQ: "[.[] | keys]"},
			v:    []testItem{{ID: 1, Title: "t"}},
			want: "[[\"id\"]]\n",
		},
		{
			name: "jq empty output",
			out:  JSONOutput{JQ: ".[] | select(.id > 5) | .id"},
			v:    []testItem{{ID: 1}, {ID: 9}},
			want: "9\n",
		},
		{
			name:   "jq runtime error keeps earlier output",
			out:    JSONOutput{JQ: ".id, (.id | keys)"},
			v:      testItem{ID: 3},
			want:   "3\n",
			errMsg: "jq: ",
		},
		{
			name: "jq big integers stay exact",
			out:  JSONOutput{JQ: ".n"},
			v:    map[string]int64{"n": 9007199254740993},
			want: "9007199254740993\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ios, _, out, _ := TestIOStreams()
			ios.SetStdoutTTY(tt.tty)
			err := tt.out.Print(ios, tt.v)
			if tt.errMsg != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %v, want it to contain %q", err, tt.errMsg)
				}
			} else if err != nil {
				t.Fatalf("Print: %v", err)
			}
			if out.String() != tt.want {
				t.Errorf("output =\n%s\nwant\n%s", out.String(), tt.want)
			}
		})
	}
}

func TestNormalizeJSONArgs(t *testing.T) {
	t.Parallel()
	var enabled, ran bool
	var out JSONOutput
	root := &cobra.Command{Use: "bh"}
	group := &cobra.Command{Use: "pr"}
	root.AddCommand(group)
	group.AddCommand(newJSONCmd(&enabled, &out, &ran))
	group.AddCommand(&cobra.Command{Use: "plain", Run: func(*cobra.Command, []string) {}})

	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"pr", "x", "--json", "id,title"}, []string{"pr", "x", "--json=id,title"}},
		{[]string{"pr", "x", "5", "--json", "inner.name", "-q", ".x"}, []string{"pr", "x", "5", "--json=inner.name", "-q", ".x"}},
		// Not field lists: left as positional arguments.
		{[]string{"pr", "x", "--json", "5"}, []string{"pr", "x", "--json", "5"}},
		{[]string{"pr", "x", "--json", "feature/x"}, []string{"pr", "x", "--json", "feature/x"}},
		{[]string{"pr", "x", "--json", "--jq", ".id"}, []string{"pr", "x", "--json", "--jq", ".id"}},
		{[]string{"pr", "x", "--json"}, []string{"pr", "x", "--json"}},
		// Typos are still taken as fields so the error names them.
		{[]string{"pr", "x", "--json", "id,nope"}, []string{"pr", "x", "--json=id,nope"}},
		{[]string{"pr", "x", "--json", "inner.nope"}, []string{"pr", "x", "--json=inner.nope"}},
		// After "--" nothing is rewritten.
		{[]string{"pr", "x", "--", "--json", "id"}, []string{"pr", "x", "--", "--json", "id"}},
		// Commands without the JSON flags are untouched.
		{[]string{"pr", "plain", "--json", "id"}, []string{"pr", "plain", "--json", "id"}},
		{[]string{"nope", "--json", "id"}, []string{"nope", "--json", "id"}},
	}
	for _, tt := range tests {
		if got := NormalizeJSONArgs(root, tt.args); !slices.Equal(got, tt.want) {
			t.Errorf("NormalizeJSONArgs(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
}
