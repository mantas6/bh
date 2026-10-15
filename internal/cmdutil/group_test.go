package cmdutil

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func newTestGroup(hints map[string]string) (*cobra.Command, *bool) {
	root := &cobra.Command{Use: "bh", SilenceErrors: true, SilenceUsage: true}
	group := &cobra.Command{Use: "pr <command>", Short: "Manage pull requests"}
	ran := new(bool)
	group.AddCommand(
		&cobra.Command{Use: "comment", Short: "Manage comments", Run: func(*cobra.Command, []string) { *ran = true }},
		&cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List", Run: func(*cobra.Command, []string) { *ran = true }},
		&cobra.Command{Use: "view", Short: "View", Run: func(*cobra.Command, []string) { *ran = true }},
	)
	SetupGroup(group, hints)
	root.AddCommand(group)
	return root, ran
}

func runGroup(t *testing.T, root *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root.SetArgs(args)
	root.SetOut(&out)
	root.SetErr(&out)
	err := root.ExecuteContext(t.Context())
	return out.String(), err
}

func TestSetupGroupUnknownCommand(t *testing.T) {
	t.Parallel()
	hints := map[string]string{"comments": "To list comments, run: bh pr comment list", "diff": "bh pr diff is not supported yet"}
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"suggestion and hint", []string{"pr", "comments", "1"},
			"unknown command \"comments\" for \"bh pr\"\nDid you mean \"comment\"?\nTo list comments, run: bh pr comment list"},
		{"hint only", []string{"pr", "diff", "1"}, "unknown command \"diff\" for \"bh pr\"\nbh pr diff is not supported yet"},
		{"suggestion only", []string{"pr", "veiw"}, "unknown command \"veiw\" for \"bh pr\"\nDid you mean \"view\"?"},
		{"prefix suggestion", []string{"pr", "li"}, "unknown command \"li\" for \"bh pr\"\nDid you mean \"list\"?"},
		{"nothing close", []string{"pr", "zzzzzz"}, "unknown command \"zzzzzz\" for \"bh pr\""},
		{"after persistent flag", []string{"pr", "--help=false", "diff"}, "unknown command \"diff\" for \"bh pr\"\nbh pr diff is not supported yet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, ran := newTestGroup(hints)
			out, err := runGroup(t, root, tt.args...)
			var fe *FlagError
			if !errors.As(err, &fe) {
				t.Fatalf("error = %v (%T), want *FlagError", err, err)
			}
			if err.Error() != tt.want {
				t.Errorf("error =\n%s\nwant\n%s", err, tt.want)
			}
			if out != "" {
				t.Errorf("output = %q, want none (help must not be printed)", out)
			}
			if *ran {
				t.Error("a subcommand ran")
			}
		})
	}
}

func TestSetupGroupMultipleSuggestions(t *testing.T) {
	t.Parallel()
	root := &cobra.Command{Use: "bh"}
	group := &cobra.Command{Use: "g <command>"}
	for _, n := range []string{"reopen", "reply", "remove"} {
		group.AddCommand(&cobra.Command{Use: n, Run: func(*cobra.Command, []string) {}})
	}
	SetupGroup(group, nil)
	root.AddCommand(group)
	err := UnknownCommandError(group, "re", "")
	if want := "unknown command \"re\" for \"bh g\"\nDid you mean \"remove\", \"reopen\" or \"reply\"?"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestSetupGroupHelpAndValidCommands(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		wantOut string // substring of output; "" means no output expected
		wantRan bool
	}{
		{"no args prints help", []string{"pr"}, "Manage pull requests", false},
		{"help flag", []string{"pr", "--help"}, "Available Commands:", false},
		{"help subcommand", []string{"pr", "help"}, "Manage pull requests", false},
		{"help for child", []string{"pr", "help", "comment"}, "Manage comments", false},
		{"valid subcommand", []string{"pr", "view", "1"}, "", true},
		{"alias", []string{"pr", "ls"}, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root, ran := newTestGroup(nil)
			out, err := runGroup(t, root, tt.args...)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if *ran != tt.wantRan {
				t.Errorf("ran = %v, want %v", *ran, tt.wantRan)
			}
			if tt.wantOut == "" && out != "" {
				t.Errorf("output = %q, want none", out)
			}
			if !strings.Contains(out, tt.wantOut) {
				t.Errorf("output = %q, want it to contain %q", out, tt.wantOut)
			}
		})
	}
}

func TestSetupGroupUsageLine(t *testing.T) {
	t.Parallel()
	root, _ := newTestGroup(nil)
	out, err := runGroup(t, root, "pr", "--help")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "bh pr <command> [flags]") {
		t.Errorf("usage advertises [flags] on a group command:\n%s", out)
	}
}
