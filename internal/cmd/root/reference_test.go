package root

import (
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/spf13/cobra"
)

// leafCommands returns the command paths of every runnable, non-group command
// under root except cobra's help and completion commands.
func leafCommands(root *cobra.Command) []string {
	var paths []string
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if !sub.IsAvailableCommand() || sub.Name() == "completion" {
				continue
			}
			if sub.HasAvailableSubCommands() {
				walk(sub)
				continue
			}
			paths = append(paths, sub.CommandPath())
		}
	}
	walk(root)
	return paths
}

func TestReferenceListsEveryCommand(t *testing.T) {
	t.Parallel()
	leaves := leafCommands(NewCmdRoot(cmdtest.NewFactory()))
	// Guard against the walk silently finding nothing.
	for _, want := range []string{"bh pr view", "bh pr comment reply", "bh auth login", "bh version", "bh reference"} {
		found := false
		for _, l := range leaves {
			found = found || l == want
		}
		if !found {
			t.Fatalf("leafCommands = %q, missing %q", leaves, want)
		}
	}

	var outputs []string
	for _, args := range [][]string{{"reference"}, {"help", "reference"}, {"reference", "--help"}} {
		out, _, err := cmdtest.RunCommand(t, NewCmdRoot(cmdtest.NewFactory()), args...)
		if err != nil {
			t.Fatalf("bh %s: error = %v", strings.Join(args, " "), err)
		}
		outputs = append(outputs, out)
	}
	for i, out := range outputs[1:] {
		if out != outputs[0] {
			t.Errorf("output %d differs from `bh reference`:\n%s", i+1, out)
		}
	}

	out := outputs[0]
	if !strings.HasPrefix(out, "# bh reference\n") {
		t.Errorf("reference does not start with its title:\n%s", out)
	}
	for _, leaf := range leaves {
		if !strings.Contains(out, "`"+leaf) {
			t.Errorf("reference is missing %q", leaf)
		}
	}
	for _, want := range []string{
		"## `bh pr <command>`",
		"### `bh pr comment <command>`",
		"#### `bh pr comment reply {<number> | <url> | <branch>} <comment-id> [flags]`",
		"### `bh pr view [<number> | <url> | <branch>] [flags]`",
		"Flags for every `bh pr` command",
		"-R, --repo",
		"Aliases: ls",
		"Aliases: close",
		"--json[=fields]",
		"--jq expression",
		"--needs-reply",
		"--resolve",
		"--name-only",
		"--watch",
		"--no-default-reviewers",
		"--body-file file",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("reference is missing %q", want)
		}
	}
	for _, unwanted := range []string{"help for", "completion", "Help about any command"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("reference contains %q", unwanted)
		}
	}
}

func TestReferenceRejectsArguments(t *testing.T) {
	t.Parallel()
	_, _, err := cmdtest.RunCommand(t, NewCmdRoot(cmdtest.NewFactory()), "reference", "pr")
	cmdtest.AssertFlagError(t, err, "accepts no arguments")
}

// Example blocks are rendered in --help, indented under "Examples:" like the
// usage line and flags.
func TestExamplesRenderInHelp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want []string
	}{
		{nil, []string{"bh pr comment reply 123 881626438", "--resolve", "bh pr edit 123 --base dev", "--body-file - <<'EOF'", "bh help reference"}},
		{[]string{"pr", "list"}, []string{"$ bh pr list --search 4304", "--base dev"}},
		{[]string{"pr", "view"}, []string{"$ bh pr view 123 --comments", "--json=id,title"}},
		{[]string{"pr", "diff"}, []string{"$ bh pr diff 123 --name-only", "--stat"}},
		{[]string{"pr", "checks"}, []string{"$ bh pr checks 123 --watch"}},
		{[]string{"pr", "create"}, []string{"$ bh pr create --fill", "--body-file - <<'EOF'", "git config bh.base dev"}},
		{[]string{"pr", "edit"}, []string{"$ bh pr edit --base dev", "--body-file - <<'EOF'"}},
		{[]string{"pr", "comment", "list"}, []string{"$ bh pr comment list 123 --needs-reply", "--json id,thread_id"}},
		{[]string{"pr", "comment", "add"}, []string{"$ bh pr comment add 123", "#comment-881626438", "bh pr comment resolve 123 881626438"}},
		{[]string{"pr", "comment", "reply"}, []string{"$ bh pr comment list 123", "$ bh pr comment reply 123 881626438", "--resolve", "#comment-881626440"}},
		{[]string{"pr", "comment", "resolve"}, []string{"$ bh pr comment list 123 --unresolved", "$ bh pr comment resolve 123 881626438 881626440"}},
		{[]string{"pr", "comment", "reopen"}, []string{"$ bh pr comment reopen 123 881626438"}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Parallel()
			out, _, err := cmdtest.RunCommand(t, NewCmdRoot(cmdtest.NewFactory()), append(tt.args, "--help")...)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			_, examples, ok := strings.Cut(out, "\nExamples:\n")
			if !ok {
				t.Fatalf("help has no Examples section:\n%s", out)
			}
			examples, _, _ = strings.Cut(examples, "\n\nAvailable Commands:")
			examples, _, _ = strings.Cut(examples, "\n\nFlags:")
			for _, line := range strings.Split(examples, "\n") {
				if line != "" && !strings.HasPrefix(line, "  ") {
					t.Errorf("example line %q is not indented", line)
				}
			}
			for _, want := range tt.want {
				if !strings.Contains(examples, want) {
					t.Errorf("examples = %q, want them to contain %q", examples, want)
				}
			}
		})
	}
}

// Every Example block is written with cmdutil.Examples, so its lines are
// indented in help output.
func TestAllExamplesIndented(t *testing.T) {
	t.Parallel()
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		for _, line := range strings.Split(c.Example, "\n") {
			if line != "" && !strings.HasPrefix(line, "  ") {
				t.Errorf("%s: example line %q is not indented (use cmdutil.Examples)", c.CommandPath(), line)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(NewCmdRoot(cmdtest.NewFactory()))
}
