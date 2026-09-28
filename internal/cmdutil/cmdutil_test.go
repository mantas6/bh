package cmdutil

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/spf13/cobra"
)

func TestNotLoggedInError(t *testing.T) {
	t.Parallel()
	err := NotLoggedInError("bitbucket.org")
	if got, want := err.Error(), "not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, api.ErrNoToken) {
		t.Error("errors.Is(err, api.ErrNoToken) = false")
	}
	if h := HintFor(err); h != "" {
		t.Errorf("HintFor(NotLoggedInError) = %q, want none (message already has it)", h)
	}
}

func TestHintFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want string // substring; "" means no hint
	}{
		{"nil", nil, ""},
		{"plain", errors.New("boom"), ""},
		{"no token", api.ErrNoToken, "bh auth login"},
		{"wrapped no token", fmt.Errorf("x: %w", api.ErrNoToken), "bh auth login"},
		{"401", &api.HTTPError{StatusCode: 401}, "invalid or expired"},
		{"wrapped 401", fmt.Errorf("x: %w", &api.HTTPError{StatusCode: 401}), "bh auth login"},
		{"403", &api.HTTPError{StatusCode: 403}, "lacks permission"},
		{"404", &api.HTTPError{StatusCode: 404}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HintFor(tt.err)
			if tt.want == "" {
				if got != "" {
					t.Errorf("HintFor = %q, want none", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("HintFor = %q, want to contain %q", got, tt.want)
			}
		})
	}
	if HintFor(&api.HTTPError{StatusCode: 401}) == HintFor(&api.HTTPError{StatusCode: 403}) {
		t.Error("401 and 403 hints should differ")
	}
}

func assertFlagError(t *testing.T, err error, wantSubstr string) {
	t.Helper()
	var fe *FlagError
	if !errors.As(err, &fe) {
		t.Fatalf("error = %v (%T), want *FlagError", err, err)
	}
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error = %q, want to contain %q", err.Error(), wantSubstr)
	}
}

func TestMutuallyExclusive(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		conds []bool
		err   bool
	}{
		{"none", nil, false},
		{"all false", []bool{false, false, false}, false},
		{"one", []bool{false, true, false}, false},
		{"two", []bool{true, true, false}, true},
		{"three", []bool{true, true, true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := MutuallyExclusive("pick one", tt.conds...)
			if !tt.err {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertFlagError(t, err, "pick one")
		})
	}
}

// bodyCmd builds a command using AddBodyFlags and the recommended
// Changed-based exclusivity check.
func bodyCmd(body, bodyFile *string) *cobra.Command {
	cmd := &cobra.Command{
		Use: "x",
		RunE: func(cmd *cobra.Command, args []string) error {
			return MutuallyExclusive("specify only one of --body or --body-file",
				cmd.Flags().Changed("body"), cmd.Flags().Changed("body-file"))
		},
	}
	AddBodyFlags(cmd, body, bodyFile)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd
}

func TestAddBodyFlags(t *testing.T) {
	t.Parallel()
	var body, bodyFile string
	cmd := bodyCmd(&body, &bodyFile)
	cmd.SetArgs([]string{"-b", "hello", "-F", "notes.md"})
	assertFlagError(t, cmd.Execute(), "only one of --body or --body-file")

	// An explicitly empty --body still conflicts with --body-file.
	cmd = bodyCmd(&body, &bodyFile)
	cmd.SetArgs([]string{"--body", "", "--body-file", "x"})
	assertFlagError(t, cmd.Execute(), "only one of --body or --body-file")

	cmd = bodyCmd(&body, &bodyFile)
	cmd.SetArgs([]string{"--body-file", "-"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if bodyFile != "-" {
		t.Errorf("bodyFile = %q", bodyFile)
	}
}

func runArgs(t *testing.T, v cobra.PositionalArgs, args ...string) error {
	t.Helper()
	root := &cobra.Command{Use: "bh"}
	sub := &cobra.Command{Use: "sub", Args: v, RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(sub)
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"sub"}, args...))
	return root.Execute()
}

func TestNoArgs(t *testing.T) {
	t.Parallel()
	if err := runArgs(t, NoArgs); err != nil {
		t.Fatalf("no args: %v", err)
	}
	assertFlagError(t, runArgs(t, NoArgs, "extra"), `unknown argument "extra"; "bh sub" accepts no arguments`)
}

func TestMaxArgs(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"a"}} {
		if err := runArgs(t, MaxArgs(1), args...); err != nil {
			t.Fatalf("MaxArgs(1) with %v: %v", args, err)
		}
	}
	assertFlagError(t, runArgs(t, MaxArgs(1), "a", "b"), `"bh sub" accepts at most 1 argument, received 2`)
	assertFlagError(t, runArgs(t, MaxArgs(2), "a", "b", "c"), "at most 2 arguments, received 3")
}

func TestExactArgs(t *testing.T) {
	t.Parallel()
	v := ExactArgs(2, "a pull request and a comment id are required")
	if err := runArgs(t, v, "1", "2"); err != nil {
		t.Fatalf("exact: %v", err)
	}
	assertFlagError(t, runArgs(t, v, "1"), "a pull request and a comment id are required")
	assertFlagError(t, runArgs(t, v, "1", "2", "3"), "at most 2 arguments, received 3")
}

type recordingBrowser struct {
	url string
	err error
}

func (b *recordingBrowser) Browse(u string) error {
	b.url = u
	return b.err
}

func TestOpenInBrowser(t *testing.T) {
	t.Parallel()
	ios, _, _, errOut := TestIOStreams()
	b := &recordingBrowser{}
	if err := OpenInBrowser(ios, b, "https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	if b.url != "https://example.com/a" {
		t.Errorf("url = %q", b.url)
	}
	if errOut.Len() != 0 {
		t.Errorf("non-TTY should print nothing, got %q", errOut.String())
	}

	ios.SetStdoutTTY(true)
	if err := OpenInBrowser(ios, b, "https://example.com/b"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut.String(), "Opening https://example.com/b in your browser.") {
		t.Errorf("notice = %q", errOut.String())
	}

	b.err = errors.New("boom")
	if err := OpenInBrowser(ios, b, "u"); err == nil {
		t.Error("expected browser error")
	}
	if err := OpenInBrowser(ios, nil, "u"); err == nil {
		t.Error("expected error for nil browser")
	}
}

func TestPrintJSON(t *testing.T) {
	t.Parallel()
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

	buf.Reset()
	if err := PrintJSON(&buf, func() {}); err == nil {
		t.Error("expected encoding error")
	}
	if buf.Len() != 0 {
		t.Errorf("nothing should be written on error, got %q", buf.String())
	}
}

func TestHeredoc(t *testing.T) {
	t.Parallel()
	got := Heredoc(`
		# comment
		$ bh pr list

		  indented
	`)
	want := "# comment\n$ bh pr list\n\n  indented"
	if got != want {
		t.Errorf("Heredoc = %q, want %q", got, want)
	}
}

func TestFlagErrorWrap(t *testing.T) {
	t.Parallel()
	base := errors.New("bad flag")
	err := FlagErrorWrap(base)
	if err.Error() != "bad flag" {
		t.Errorf("Error() = %q", err.Error())
	}
	if !errors.Is(err, base) {
		t.Error("FlagErrorWrap should unwrap to the wrapped error")
	}
	assertFlagError(t, fmt.Errorf("parsing: %w", err), "parsing: bad flag")
}

func TestIsUserCancellation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("boom"), false},
		{ErrCancel, true},
		{fmt.Errorf("prompt: %w", ErrCancel), true},
		{ErrSilent, false},
	}
	for _, tt := range tests {
		if got := IsUserCancellation(tt.err); got != tt.want {
			t.Errorf("IsUserCancellation(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestHeredocCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"single line", "hello", "hello"},
		{"no leading newline", "\ta\n\t\tb\n", "a\n\tb"},
		{"tabs", "\n\t\t$ bh pr list\n\t\t$ bh pr view 1\n\t", "$ bh pr list\n$ bh pr view 1"},
		{"blank lines keep no whitespace", "\n    a\n      \n    b\n", "a\n\nb"},
		{"already flush", "\na\n  b\n", "a\n  b"},
		{"trailing newlines trimmed", "\n  a\n\n\n", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Heredoc(tt.in); got != tt.want {
				t.Errorf("Heredoc(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidatorsAcceptWithinBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		v    cobra.PositionalArgs
		args []string
		want string // "" means accepted
	}{
		{"NoArgs none", NoArgs, nil, ""},
		{"NoArgs quote hint", NoArgs, []string{"My", "title"}, "quote values that contain spaces"},
		{"MaxArgs(0) none", MaxArgs(0), nil, ""},
		{"MaxArgs(0) one", MaxArgs(0), []string{"a"}, "at most 0 arguments, received 1"},
		{"ExactArgs(1) missing", ExactArgs(1, "a pull request is required"), nil, "a pull request is required"},
		{"ExactArgs(1) one", ExactArgs(1, "x"), []string{"a"}, ""},
		{"ExactArgs(1) two", ExactArgs(1, "x"), []string{"a", "b"}, "at most 1 argument, received 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := runArgs(t, tt.v, tt.args...)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			assertFlagError(t, err, tt.want)
		})
	}
}
