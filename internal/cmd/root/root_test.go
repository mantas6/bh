package root

import (
	"errors"
	"strings"
	"testing"

	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
)

func TestRootVersion(t *testing.T) {
	ios, _, out, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios, Version: "1.2.3"}

	cmd := NewCmdRoot(f)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "1.2.3") {
		t.Fatalf("version output = %q, want to contain %q", got, "1.2.3")
	}
}

func TestRepoFlagScopedToPR(t *testing.T) {
	ios, _, _, _ := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{IOStreams: ios}
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
	ios, _, _, _ := cmdutil.TestIOStreams()
	errStop := errors.New("stop")
	var seen string
	f := &cmdutil.Factory{IOStreams: ios}
	f.BaseRepo = func() (git.Repo, *git.ResolvedRemote, error) {
		seen = f.RepoOverride
		return git.Repo{}, nil, errStop
	}

	cmd := NewCmdRoot(f)
	cmd.SetArgs([]string{"pr", "list", "-R", "ws/repo"})
	if err := cmd.Execute(); !errors.Is(err, errStop) {
		t.Fatalf("Execute() error = %v, want %v", err, errStop)
	}
	if seen != "ws/repo" {
		t.Errorf("RepoOverride = %q, want %q", seen, "ws/repo")
	}
}
