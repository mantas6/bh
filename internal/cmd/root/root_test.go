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
