package cmdutil

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/spf13/cobra"
)

func TestNotLoggedInError(t *testing.T) {
	err := NotLoggedInError("bitbucket.org")
	if got, want := err.Error(), "not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, api.ErrNoToken) {
		t.Error("errors.Is(err, api.ErrNoToken) = false")
	}
	if err.Error() != api.ErrNoToken.Error() {
		t.Errorf("hint differs from api.ErrNoToken: %q vs %q", err.Error(), api.ErrNoToken.Error())
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
	if err := runArgs(t, NoArgs); err != nil {
		t.Fatalf("no args: %v", err)
	}
	assertFlagError(t, runArgs(t, NoArgs, "extra"), `unknown argument "extra"; "bh sub" accepts no arguments`)
}

func TestMaxArgs(t *testing.T) {
	for _, args := range [][]string{nil, {"a"}} {
		if err := runArgs(t, MaxArgs(1), args...); err != nil {
			t.Fatalf("MaxArgs(1) with %v: %v", args, err)
		}
	}
	assertFlagError(t, runArgs(t, MaxArgs(1), "a", "b"), `"bh sub" accepts at most 1 argument, received 2`)
	assertFlagError(t, runArgs(t, MaxArgs(2), "a", "b", "c"), "at most 2 arguments, received 3")
}

func TestExactArgs(t *testing.T) {
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
