package cmdtest_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/config"
	"github.com/mantas6/bh/internal/git/gittest"
	"github.com/spf13/cobra"
)

func TestRunCommand(t *testing.T) {
	t.Parallel()
	var gotArgs []string
	var gotCtx context.Context
	cmd := &cobra.Command{
		Use:          "x",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			gotArgs, gotCtx = args, cmd.Context()
			fmt.Fprint(cmd.OutOrStdout(), "out")
			fmt.Fprint(cmd.ErrOrStderr(), "err")
			return errors.New("boom")
		},
	}
	stdout, stderr, err := cmdtest.RunCommand(t, cmd, "a", "b")
	if err == nil || err.Error() != "boom" {
		t.Errorf("err = %v", err)
	}
	if stdout != "out" || stderr != "errError: boom\n" {
		t.Errorf("stdout = %q, stderr = %q", stdout, stderr)
	}
	if len(gotArgs) != 2 || gotArgs[0] != "a" {
		t.Errorf("args = %v", gotArgs)
	}
	if gotCtx == nil {
		t.Error("command context not set")
	}
}

// With no args RunCommand must not fall back to the test binary's os.Args.
func TestRunCommandNoArgs(t *testing.T) {
	t.Parallel()
	var gotArgs []string
	cmd := &cobra.Command{Use: "x", RunE: func(_ *cobra.Command, args []string) error {
		gotArgs = args
		return nil
	}}
	if _, _, err := cmdtest.RunCommand(t, cmd); err != nil {
		t.Fatal(err)
	}
	if len(gotArgs) != 0 {
		t.Errorf("args = %v, want none", gotArgs)
	}
}

func TestRunCommandContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cmd := &cobra.Command{Use: "x", RunE: func(cmd *cobra.Command, _ []string) error {
		return cmd.Context().Err()
	}}
	if _, _, err := cmdtest.RunCommandContext(ctx, t, cmd); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestAssertFlagError(t *testing.T) {
	t.Parallel()
	cmdtest.AssertFlagError(t, fmt.Errorf("wrapped: %w", cmdutil.FlagErrorf("bad --x")), "bad --x")
	cmdtest.AssertFlagError(t, cmdutil.FlagErrorf("anything"), "")
}

func TestFindRequest(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle("GET", "/repositories/myws/myrepo/pullrequests", 200,
		cmdtest.ValuesPage([]api.PullRequest{*cmdtest.SamplePR()}))
	client := srv.APIClient()
	for _, branch := range []string{"first", "last"} {
		if _, err := client.PullRequestForBranch(t.Context(), "myws/myrepo", branch); err != nil {
			t.Fatal(err)
		}
	}

	if got := cmdtest.FindRequest(srv, "GET", "/pullrequests"); got == nil || !strings.Contains(got.Query.Get("q"), `"last"`) {
		t.Errorf("FindRequest = %+v, want the most recent match", got)
	}
	if got := cmdtest.FindRequest(srv, "POST", "/pullrequests"); got != nil {
		t.Errorf("FindRequest(POST) = %+v, want nil", got)
	}
	if got := cmdtest.RequireRequest(t, srv, "GET", "/myrepo/pullrequests"); !strings.Contains(got.Query.Get("q"), `"last"`) {
		t.Errorf("RequireRequest = %+v", got)
	}
}

func TestProviders(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	client, err := cmdtest.ClientFunc(srv)()
	if err != nil || client.BaseURL != srv.URL {
		t.Errorf("ClientFunc = %+v, %v", client, err)
	}
	c := cmdtest.ClientForFunc(srv)("tok", "a@example.com")
	if c.BaseURL != srv.URL || c.Token != "tok" || c.Email != "a@example.com" {
		t.Errorf("ClientForFunc = %+v", c)
	}

	stub := gittest.New(t)
	if r, err := cmdtest.GitFunc(stub)(); err != nil || r != stub {
		t.Errorf("GitFunc = %v, %v", r, err)
	}

	repo, rr, err := cmdtest.BaseRepoFunc(cmdtest.OriginRemote())(t.Context())
	if err != nil || repo != cmdtest.TestRepo() || rr.Remote.Name != "origin" || rr.Repo != repo {
		t.Errorf("BaseRepoFunc = %v, %+v, %v", repo, rr, err)
	}
	if f := cmdtest.NewFactory(); f.IOStreams == nil {
		t.Error("NewFactory: nil IOStreams")
	}
}

func TestFakeBrowser(t *testing.T) {
	t.Parallel()
	var b cmdtest.FakeBrowser
	if b.URL() != "" {
		t.Errorf("URL() = %q before any Browse", b.URL())
	}
	_ = b.Browse("https://a")
	_ = b.Browse("https://b")
	if b.URL() != "https://b" || len(b.URLs) != 2 {
		t.Errorf("URLs = %v", b.URLs)
	}
}

func TestFixtures(t *testing.T) {
	t.Parallel()
	if u := cmdtest.User("bob"); u.UUID != "{bob-uuid}" || u.DisplayName != "Bob" {
		t.Errorf("User = %+v", u)
	}
	pr := cmdtest.SamplePR()
	if pr.ID != 123 || pr.Source.Branch.Name != "feature" || pr.Reviewers[0].UUID != "{bob-uuid}" {
		t.Errorf("SamplePR = %+v", pr)
	}
	if c := cmdtest.CommentAt(1, "ada", "hi", 5); !c.CreatedOn.Equal(cmdtest.FixedNow().Add(-5 * time.Minute)) {
		t.Errorf("CommentAt created = %v", c.CreatedOn)
	}
	if m := cmdtest.Members("a", "b"); len(m["values"].([]api.WorkspaceMember)) != 2 {
		t.Errorf("Members = %v", m)
	}
}

func TestConfig(t *testing.T) {
	t.Parallel()
	cfg := cmdtest.NewConfig(map[string]string{config.EnvToken: "env"}).
		SetHost(config.DefaultHost, config.HostConfig{Token: "stored"})
	loaded, err := cfg.Load()
	if err != nil {
		t.Fatal(err)
	}
	if tok, src := loaded.Token(config.DefaultHost); tok != "env" || src != config.TokenSourceEnv {
		t.Errorf("Token = %q, %v", tok, src)
	}
	if cfg.Saves() != 0 || cfg.SavedHost(config.DefaultHost) != nil {
		t.Error("nothing should be saved yet")
	}

	if err := loaded.Save(); err != nil {
		t.Fatal(err)
	}
	loaded.Host(config.DefaultHost).Token = "mutated after save"
	if hc := cfg.SavedHost(config.DefaultHost); cfg.Saves() != 1 || hc == nil || hc.Token != "stored" {
		t.Errorf("saves = %d, saved host = %+v; want a snapshot of the first save", cfg.Saves(), hc)
	}
}

func TestTempConfigDir(t *testing.T) {
	t.Setenv(config.EnvToken, "leaked")
	dir := cmdtest.TempConfigDir(t)
	got, err := config.Dir()
	if err != nil || got != dir {
		t.Errorf("config.Dir() = %q, %v; want %q", got, err, dir)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if tok, _ := cfg.Token(config.DefaultHost); tok != "" {
		t.Errorf("BH_TOKEN = %q, want cleared", tok)
	}
}
