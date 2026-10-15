package root

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

func TestHandleError(t *testing.T) {
	t.Parallel()
	parent := &cobra.Command{Use: "bh"}
	child := &cobra.Command{Use: "list", Run: func(*cobra.Command, []string) {}}
	parent.AddCommand(child)

	unauthorizedHint := cmdutil.HintFor(&api.HTTPError{StatusCode: 401})
	forbiddenHint := cmdutil.HintFor(&api.HTTPError{StatusCode: 403})

	tests := []struct {
		name string
		cmd  *cobra.Command
		err  error
		code int
		// tty is the expected stderr when it is a terminal, plain when it
		// is not (piped or captured).
		tty, plain string
	}{
		{"nil", child, nil, ExitOK, "", ""},
		{"plain", child, errors.New("boom"), ExitError, "bh: boom\n", "bh: boom\n"},
		{
			"flag error",
			child,
			cmdutil.FlagErrorf("unknown flag: --nope"),
			ExitError,
			"bh: unknown flag: --nope\n\nRun 'bh list --help' for usage.\n",
			"bh: unknown flag: --nope (run 'bh list --help' for usage)\n",
		},
		{"flag error without cmd", nil, cmdutil.FlagErrorf("bad"), ExitError, "bh: bad\n", "bh: bad\n"},
		{
			"wrapped flag error",
			child,
			fmt.Errorf("parsing: %w", cmdutil.FlagErrorf("bad")),
			ExitError,
			"bh: parsing: bad\n\nRun 'bh list --help' for usage.\n",
			"bh: parsing: bad (run 'bh list --help' for usage)\n",
		},
		{
			"multi-line flag error with hint",
			parent,
			cmdutil.UnknownCommandError(parent, "lst", "To list things, run: bh list"),
			ExitError,
			"bh: unknown command \"lst\" for \"bh\"\nDid you mean \"list\"?\nTo list things, run: bh list\n\nRun 'bh --help' for usage.\n",
			"bh: unknown command \"lst\" for \"bh\"; Did you mean \"list\"?; To list things, run: bh list (run 'bh --help' for usage)\n",
		},
		{"silent", child, cmdutil.ErrSilent, ExitError, "", ""},
		{"prompt cancel", child, fmt.Errorf("prompt: %w", cmdutil.ErrCancel), ExitCancel, "", ""},
		{"interrupt", child, context.Canceled, ExitInterrupt, "", ""},
		{
			"wrapped interrupt",
			child,
			errors.Join(context.Canceled, &git.Error{Args: []string{"fetch"}, ExitCode: 130}),
			ExitInterrupt,
			"",
			"",
		},
		{
			"unauthorized",
			child,
			&api.HTTPError{StatusCode: 401, Message: "Unauthorized"},
			ExitError,
			"bh: Unauthorized (HTTP 401)\n" + unauthorizedHint + "\n",
			"bh: Unauthorized (HTTP 401); " + unauthorizedHint + "\n",
		},
		{
			"forbidden",
			child,
			fmt.Errorf("approving: %w", &api.HTTPError{StatusCode: 403, Message: "Forbidden"}),
			ExitError,
			"bh: approving: Forbidden (HTTP 403)\n" + forbiddenHint + "\n",
			"bh: approving: Forbidden (HTTP 403); " + forbiddenHint + "\n",
		},
		{
			"custom hint",
			child,
			cmdutil.WithHint(errors.New("resolving reviewer \"x\": boom"), "Check the name."),
			ExitError,
			"bh: resolving reviewer \"x\": boom\nCheck the name.\n",
			"bh: resolving reviewer \"x\": boom; Check the name.\n",
		},
		{
			"not logged in has no extra hint",
			child,
			cmdutil.NotLoggedInError("bitbucket.org"),
			ExitError,
			"bh: not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN\n",
			"bh: not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN\n",
		},
		{
			"git failure",
			child,
			&git.Error{Args: []string{"push", "origin"}, ExitCode: 128, Stderr: "fatal: denied\n"},
			ExitError,
			"bh: git push origin: exit status 128: fatal: denied\n",
			"bh: git push origin: exit status 128: fatal: denied\n",
		},
		{
			"multi-line git failure",
			child,
			&git.Error{Args: []string{"push"}, ExitCode: 1, Stderr: "error: failed to push\n\nhint: pull first\n"},
			ExitError,
			"bh: git push: exit status 1: error: failed to push\n\nhint: pull first\n",
			"bh: git push: exit status 1: error: failed to push; hint: pull first\n",
		},
	}
	for _, tt := range tests {
		for _, tty := range []bool{true, false} {
			want, mode := tt.plain, "non-tty"
			if tty {
				want, mode = tt.tty, "tty"
			}
			t.Run(tt.name+"/"+mode, func(t *testing.T) {
				t.Parallel()
				ios, _, _, errOut := cmdutil.TestIOStreams()
				ios.SetStderrTTY(tty)
				if got := HandleError(ios, tt.cmd, tt.err); got != tt.code {
					t.Errorf("exit code = %d, want %d", got, tt.code)
				}
				if errOut.String() != want {
					t.Errorf("output = %q, want %q", errOut.String(), want)
				}
			})
		}
	}
}

// TestCancelledContextAbortsCommand runs a real command tree under an already
// cancelled context: the API request must not be sent and the error must map
// to the interrupt exit code.
func TestCancelledContextAbortsCommand(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t) // no routes: any request that reaches it fails the test

	ios, _, _, errOut := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{
		IOStreams: ios,
		APIClient: cmdtest.ClientFunc(srv),
		BaseRepo: func(ctx context.Context) (git.Repo, *git.ResolvedRemote, error) {
			return git.Repo{Host: "bitbucket.org", Workspace: "ws", Name: "repo"}, nil, nil
		},
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cmd := NewCmdRoot(f)
	cmd.SetArgs([]string{"pr", "view", "123"})
	ran, err := cmd.ExecuteContextC(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if len(srv.Requests()) != 0 {
		t.Errorf("requests = %v, want none", srv.Requests())
	}
	if code := HandleError(ios, ran, err); code != ExitInterrupt {
		t.Errorf("exit code = %d, want %d", code, ExitInterrupt)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}
