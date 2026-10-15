package root

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
)

func TestRootVersion(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"--version"}, {"version"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			ios, _, out, _ := cmdutil.TestIOStreams()
			f := &cmdutil.Factory{IOStreams: ios, Version: "1.2.3"}

			cmd := NewCmdRoot(f)
			cmd.SetArgs(args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got, want := out.String(), "bh version 1.2.3\n"; got != want {
				t.Fatalf("version output = %q, want %q", got, want)
			}
		})
	}
}

func TestVersionRejectsArgs(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	f.Version = "1.2.3"
	_, _, err := cmdtest.RunCommand(t, NewCmdRoot(f), "version", "extra")
	cmdtest.AssertFlagError(t, err, "accepts no arguments")
}

// Flag parsing errors surface as FlagErrors so the top level prints usage.
func TestRootWrapsFlagErrors(t *testing.T) {
	t.Parallel()
	_, _, err := cmdtest.RunCommand(t, NewCmdRoot(cmdtest.NewFactory()), "pr", "list", "--nope")
	cmdtest.AssertFlagError(t, err, "unknown flag: --nope")
}

func TestRepoFlagScopedToPR(t *testing.T) {
	t.Parallel()
	f := cmdtest.NewFactory()
	cmd := NewCmdRoot(f)

	if cmd.PersistentFlags().Lookup("repo") != nil {
		t.Error("--repo should not be a root persistent flag")
	}

	for _, path := range [][]string{{"auth", "login"}, {"auth", "status"}} {
		sub, _, err := cmd.Find(path)
		if err != nil {
			t.Fatalf("Find(%v): %v", path, err)
		}
		if sub.Flags().Lookup("repo") != nil || sub.InheritedFlags().Lookup("repo") != nil {
			t.Errorf("%s should not have --repo", sub.CommandPath())
		}
	}

	for _, path := range [][]string{{"pr", "list"}, {"pr", "comment", "list"}} {
		sub, _, err := cmd.Find(path)
		if err != nil {
			t.Fatalf("Find(%v): %v", path, err)
		}
		if sub.InheritedFlags().ShorthandLookup("R") == nil {
			t.Errorf("%s should inherit -R/--repo", sub.CommandPath())
		}
	}
}

func TestRepoFlagFeedsFactory(t *testing.T) {
	t.Parallel()
	errStop := errors.New("stop")
	var seen string
	f := cmdtest.NewFactory()
	f.BaseRepo = func(context.Context) (git.Repo, *git.ResolvedRemote, error) {
		seen = f.RepoOverride
		return git.Repo{}, nil, errStop
	}

	if _, _, err := cmdtest.RunCommand(t, NewCmdRoot(f), "pr", "list", "-R", "ws/repo"); !errors.Is(err, errStop) {
		t.Fatalf("Execute() error = %v, want %v", err, errStop)
	}
	if seen != "ws/repo" {
		t.Errorf("RepoOverride = %q, want %q", seen, "ws/repo")
	}
}

// Unknown subcommands of group commands are usage errors (non-zero exit), not
// a silent help page with exit 0.
func TestUnknownSubcommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"pr", "comments", "1"}, []string{`unknown command "comments" for "bh pr"`, `Did you mean "comment" or "commits"?`, "bh pr comment list"}},
		{[]string{"pr", "chekcs"}, []string{`unknown command "chekcs" for "bh pr"`, `Did you mean "checks"?`}},
		{[]string{"pr", "files", "1"}, []string{`unknown command "files" for "bh pr"`, "bh pr diff [<pr>] --name-only"}},
		{[]string{"pr", "status"}, []string{`unknown command "status" for "bh pr"`, "bh pr list --author @me"}},
		{[]string{"pr", "-R", "ws/repo", "veiw", "1"}, []string{`unknown command "veiw" for "bh pr"`, `Did you mean "view"?`}},
		{[]string{"pr", "comment", "lst", "1"}, []string{`unknown command "lst" for "bh pr comment"`, `Did you mean "list"?`}},
		{[]string{"auth", "stat"}, []string{`unknown command "stat" for "bh auth"`, `Did you mean "status"?`}},
		{[]string{"prr", "list"}, []string{`unknown command "prr" for "bh"`, `Did you mean "pr"?`}},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Parallel()
			ios, _, out, errOut := cmdutil.TestIOStreams()
			f := &cmdutil.Factory{IOStreams: ios}
			cmd := NewCmdRoot(f)
			cmd.SetArgs(tt.args)
			ran, err := cmd.ExecuteContextC(t.Context())
			cmdtest.AssertFlagError(t, err, tt.want[0])
			for _, w := range tt.want[1:] {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error = %q, want it to contain %q", err, w)
				}
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want empty (no help page)", out.String())
			}
			if code := HandleError(ios, ran, err); code == ExitOK {
				t.Errorf("exit code = %d, want non-zero", code)
			}
			// stderr is not a TTY here: everything must be on one line.
			stderr := errOut.String()
			if !strings.HasPrefix(stderr, "bh: "+tt.want[0]) {
				t.Errorf("stderr = %q, want it to start with %q", stderr, "bh: "+tt.want[0])
			}
			if n := strings.Count(stderr, "\n"); n != 1 || !strings.HasSuffix(stderr, " --help' for usage)\n") {
				t.Errorf("stderr = %q, want one line ending with the --help pointer", stderr)
			}
			for _, w := range tt.want[1:] {
				if !strings.Contains(stderr, w) {
					t.Errorf("stderr = %q, want it to contain %q", stderr, w)
				}
			}
		})
	}
}

// Group commands still print their help (exit 0) without arguments, with
// --help, or via `help`.
func TestGroupHelp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"pr"}, "Manage pull requests"},
		{[]string{"pr", "--help"}, "Manage pull requests"},
		{[]string{"pr", "help", "comment"}, "Manage pull request comments"},
		{[]string{"pr", "comment"}, "Manage pull request comments"},
		{[]string{"auth"}, "Authenticate bh with Bitbucket"},
		{[]string{}, "Work with Bitbucket Cloud pull requests"},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			t.Parallel()
			out, _, err := cmdtest.RunCommand(t, NewCmdRoot(cmdtest.NewFactory()), tt.args...)
			if err != nil {
				t.Fatalf("error = %v", err)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("help output = %q, want it to contain %q", out, tt.want)
			}
		})
	}
}

// Real subcommands and aliases still dispatch normally.
func TestValidSubcommandsUnaffected(t *testing.T) {
	t.Parallel()
	errStop := errors.New("stop")
	for _, args := range [][]string{{"pr", "list"}, {"pr", "ls"}, {"pr", "view", "1"}, {"pr", "comment", "list", "1"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			f := cmdtest.NewFactory()
			f.BaseRepo = func(context.Context) (git.Repo, *git.ResolvedRemote, error) {
				return git.Repo{}, nil, errStop
			}
			if _, _, err := cmdtest.RunCommand(t, NewCmdRoot(f), args...); !errors.Is(err, errStop) {
				t.Fatalf("error = %v, want %v (command should have run)", err, errStop)
			}
		})
	}
}

// Every command with --json accepts a space-separated field list, which
// NormalizeJSONArgs (called by main) turns into --json=<fields>.
func TestNormalizeJSONArgsCommands(t *testing.T) {
	t.Parallel()
	tests := []struct {
		args []string
		want []string
	}{
		{[]string{"pr", "view", "5", "--json", "id,source.branch.name"}, []string{"pr", "view", "5", "--json=id,source.branch.name"}},
		{[]string{"pr", "view", "--json", "5"}, []string{"pr", "view", "--json", "5"}},
		{[]string{"pr", "view", "--json", "feature/x"}, []string{"pr", "view", "--json", "feature/x"}},
		{[]string{"pr", "list", "--json", "id,title", "-q", ".[].id"}, []string{"pr", "list", "--json=id,title", "-q", ".[].id"}},
		{[]string{"pr", "commits", "--json", "hash"}, []string{"pr", "commits", "--json=hash"}},
		{[]string{"pr", "checks", "-R", "a/b", "--json", "key,state"}, []string{"pr", "checks", "-R", "a/b", "--json=key,state"}},
		{[]string{"pr", "comment", "list", "1", "--json", "id,resolved,thread_id"}, []string{"pr", "comment", "list", "1", "--json=id,resolved,thread_id"}},
		{[]string{"pr", "create", "--json", "id"}, []string{"pr", "create", "--json", "id"}},
	}
	root := NewCmdRoot(cmdtest.NewFactory())
	for _, tt := range tests {
		if got := cmdutil.NormalizeJSONArgs(root, tt.args); strings.Join(got, " ") != strings.Join(tt.want, " ") {
			t.Errorf("NormalizeJSONArgs(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
}
