package root

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git"
	"github.com/spf13/cobra"
)

func TestHandleError(t *testing.T) {
	parent := &cobra.Command{Use: "bh"}
	child := &cobra.Command{Use: "list"}
	parent.AddCommand(child)

	tests := []struct {
		name    string
		cmd     *cobra.Command
		err     error
		code    int
		wantOut string
	}{
		{"nil", child, nil, ExitOK, ""},
		{"plain", child, errors.New("boom"), ExitError, "bh: boom\n"},
		{
			"flag error",
			child,
			cmdutil.FlagErrorf("unknown flag: --nope"),
			ExitError,
			"bh: unknown flag: --nope\n\nRun 'bh list --help' for usage.\n",
		},
		{"flag error without cmd", nil, cmdutil.FlagErrorf("bad"), ExitError, "bh: bad\n"},
		{
			"wrapped flag error",
			child,
			fmt.Errorf("parsing: %w", cmdutil.FlagErrorf("bad")),
			ExitError,
			"bh: parsing: bad\n\nRun 'bh list --help' for usage.\n",
		},
		{"silent", child, cmdutil.ErrSilent, ExitError, ""},
		{"prompt cancel", child, fmt.Errorf("prompt: %w", cmdutil.ErrCancel), ExitCancel, ""},
		{"interrupt", child, context.Canceled, ExitInterrupt, ""},
		{
			"wrapped interrupt",
			child,
			errors.Join(context.Canceled, &git.Error{Args: []string{"fetch"}, ExitCode: 130}),
			ExitInterrupt,
			"",
		},
		{
			"unauthorized",
			child,
			&api.HTTPError{StatusCode: 401, Message: "Unauthorized"},
			ExitError,
			"bh: Unauthorized (HTTP 401)\n" + cmdutil.HintFor(&api.HTTPError{StatusCode: 401}) + "\n",
		},
		{
			"forbidden",
			child,
			fmt.Errorf("approving: %w", &api.HTTPError{StatusCode: 403, Message: "Forbidden"}),
			ExitError,
			"bh: approving: Forbidden (HTTP 403)\n" + cmdutil.HintFor(&api.HTTPError{StatusCode: 403}) + "\n",
		},
		{
			"not logged in has no extra hint",
			child,
			cmdutil.NotLoggedInError("bitbucket.org"),
			ExitError,
			"bh: not logged in to bitbucket.org; run `bh auth login` or set BH_TOKEN\n",
		},
		{
			"git failure",
			child,
			&git.Error{Args: []string{"push", "origin"}, ExitCode: 128, Stderr: "fatal: denied\n"},
			ExitError,
			"bh: git push origin: exit status 128: fatal: denied\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			if got := HandleError(&out, tt.cmd, tt.err); got != tt.code {
				t.Errorf("exit code = %d, want %d", got, tt.code)
			}
			if out.String() != tt.wantOut {
				t.Errorf("output = %q, want %q", out.String(), tt.wantOut)
			}
		})
	}
}

// TestCancelledContextAbortsCommand runs a real command tree under an already
// cancelled context: the API request must not be sent and the error must map
// to the interrupt exit code.
func TestCancelledContextAbortsCommand(t *testing.T) {
	srv := apitest.New(t) // no routes: any request that reaches it fails the test

	ios, _, _, errOut := cmdutil.TestIOStreams()
	f := &cmdutil.Factory{
		IOStreams: ios,
		APIClient: func() (*api.Client, error) { return srv.APIClient(), nil },
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
	if code := HandleError(errOut, ran, err); code != ExitInterrupt {
		t.Errorf("exit code = %d, want %d", code, ExitInterrupt)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
}
