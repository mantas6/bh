package pr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mantas6/bh/internal/api"
	"github.com/mantas6/bh/internal/api/apitest"
	"github.com/mantas6/bh/internal/cmd/cmdtest"
	"github.com/mantas6/bh/internal/cmdutil"
	"github.com/mantas6/bh/internal/git/gittest"
	"github.com/mantas6/bh/internal/output"
)

const statusesPath = "/repositories/myws/myrepo/pullrequests/123/statuses"

// sampleStatuses has one stale status for "lint" (superseded by a newer
// failure) and one status without a name.
const sampleStatuses = `{"values":[
	{"key":"build","name":"Build","state":"SUCCESSFUL","url":"https://ci/build","updated_on":"2026-09-10T11:00:00+00:00"},
	{"key":"lint","name":"Lint","state":"SUCCESSFUL","url":"https://ci/lint/1","updated_on":"2026-09-10T09:00:00+00:00"},
	{"key":"lint","name":"Lint","state":"FAILED","url":"https://ci/lint/2","updated_on":"2026-09-10T11:30:00+00:00"},
	{"key":"deploy","state":"STOPPED","url":"https://ci/deploy","updated_on":"2026-09-08T12:00:00+00:00"}
]}`

func newChecksOptions(t *testing.T, srv *apitest.Server, ios *cmdutil.IOStreams) *ChecksOptions {
	return &ChecksOptions{
		IO:        ios,
		APIClient: cmdtest.ClientFunc(srv),
		Git:       cmdtest.GitFunc(gittest.New(t)),
		BaseRepo:  cmdtest.BaseRepoFunc(nil),
		Now:       cmdtest.FixedNow,
		Sleep: func(context.Context, time.Duration) error {
			t.Error("unexpected sleep")
			return nil
		},
		Arg: "123",
	}
}

func noColorTTY(ios *cmdutil.IOStreams) {
	ios.Getenv = func(k string) string {
		if k == "NO_COLOR" {
			return "1"
		}
		return ""
	}
	ios.SetStdoutTTY(true)
}

func TestChecksPlain(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, statusesPath, 200, sampleStatuses)
	ios, _, out, errOut := cmdutil.TestIOStreams()

	err := checksRun(t.Context(), newChecksOptions(t, srv, ios))
	if !errors.Is(err, cmdutil.ErrSilent) {
		t.Fatalf("err = %v, want ErrSilent for a failed check", err)
	}
	want := "Lint\tFAILED\t2026-09-10T11:30:00Z\thttps://ci/lint/2\n" +
		"deploy\tSTOPPED\t2026-09-08T12:00:00Z\thttps://ci/deploy\n" +
		"Build\tSUCCESSFUL\t2026-09-10T11:00:00Z\thttps://ci/build\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errOut.String())
	}
	if req := cmdtest.FindRequest(srv, http.MethodGet, "/pullrequests/123"); req != nil {
		t.Errorf("unexpected pull request fetch")
	}
}

func TestChecksTTY(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, statusesPath, 200, sampleStatuses)
	ios, _, out, _ := cmdutil.TestIOStreams()
	noColorTTY(ios)

	if err := checksRun(t.Context(), newChecksOptions(t, srv, ios)); !errors.Is(err, cmdutil.ErrSilent) {
		t.Fatalf("err = %v, want ErrSilent", err)
	}
	want := "Checks: 1 passed, 1 failed, 1 stopped\n" +
		"\n" +
		"Lint    FAILED      30 minutes ago     https://ci/lint/2\n" +
		"deploy  STOPPED     2 days ago         https://ci/deploy\n" +
		"Build   SUCCESSFUL  about an hour ago  https://ci/build\n"
	if out.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", out.String(), want)
	}
}

func TestChecksAllPassed(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, statusesPath, 200, `{"values":[
		{"key":"build","name":"Build","state":"SUCCESSFUL","url":"https://ci/build","updated_on":"2026-09-10T11:00:00+00:00"},
		{"key":"e2e","name":"E2E","state":"INPROGRESS","url":"https://ci/e2e","updated_on":"2026-09-10T11:55:00+00:00"}
	]}`)
	ios, _, out, _ := cmdutil.TestIOStreams()

	if err := checksRun(t.Context(), newChecksOptions(t, srv, ios)); err != nil {
		t.Fatalf("checksRun: %v", err)
	}
	want := "E2E\tINPROGRESS\t2026-09-10T11:55:00Z\thttps://ci/e2e\n" +
		"Build\tSUCCESSFUL\t2026-09-10T11:00:00Z\thttps://ci/build\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

func TestChecksEmpty(t *testing.T) {
	t.Parallel()
	for _, tty := range []bool{false, true} {
		srv := apitest.New(t)
		srv.Handle(http.MethodGet, statusesPath, 200, `{"values":[]}`)
		ios, _, out, _ := cmdutil.TestIOStreams()
		ios.SetStdoutTTY(tty)
		opts := newChecksOptions(t, srv, ios)

		if err := checksRun(t.Context(), opts); err != nil {
			t.Fatalf("checksRun: %v", err)
		}
		want := ""
		if tty {
			want = "No checks reported on pull request #123\n"
		}
		if out.String() != want {
			t.Errorf("tty=%v: stdout = %q, want %q", tty, out.String(), want)
		}

		opts.JSON = true
		out.Reset()
		if err := checksRun(t.Context(), opts); err != nil {
			t.Fatalf("checksRun: %v", err)
		}
		if strings.TrimSpace(out.String()) != "[]" {
			t.Errorf("tty=%v: json = %q, want []", tty, out.String())
		}
	}
}

func TestChecksJSON(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, statusesPath, 200, sampleStatuses)
	ios, _, out, _ := cmdutil.TestIOStreams()
	ios.SetStdoutTTY(true)
	opts := newChecksOptions(t, srv, ios)
	opts.JSON = true

	if err := checksRun(t.Context(), opts); !errors.Is(err, cmdutil.ErrSilent) {
		t.Fatalf("err = %v, want ErrSilent for a failed check", err)
	}
	var statuses []api.CommitStatus
	if err := json.Unmarshal(out.Bytes(), &statuses); err != nil {
		t.Fatalf("json: %v\n%s", err, out.String())
	}
	if len(statuses) != 3 {
		t.Fatalf("got %d statuses, want 3 (stale lint dropped): %+v", len(statuses), statuses)
	}
	if s := statuses[0]; s.Key != "lint" || s.State != api.StatusFailed || s.URL != "https://ci/lint/2" {
		t.Errorf("first status = %+v", s)
	}
}

// watchServer answers the statuses request with INPROGRESS for the first
// pending polls and with the final state afterwards.
func watchServer(t *testing.T, pending int, final string) (*apitest.Server, *atomic.Int32) {
	srv := apitest.New(t)
	var calls atomic.Int32
	srv.HandleFunc(http.MethodGet, statusesPath, func(w http.ResponseWriter, r *http.Request) {
		state := final
		if int(calls.Add(1)) <= pending {
			state = api.StatusInProgress
		}
		fmt.Fprintf(w, `{"values":[
			{"key":"build","name":"Build","state":"SUCCESSFUL","url":"https://ci/build","updated_on":"2026-09-10T11:00:00+00:00"},
			{"key":"test","name":"Test","state":%q,"url":"https://ci/test","updated_on":"2026-09-10T11:50:00+00:00"}
		]}`, state)
	})
	return srv, &calls
}

func TestChecksWatch(t *testing.T) {
	t.Parallel()
	srv, calls := watchServer(t, 2, api.StatusSuccessful)
	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newChecksOptions(t, srv, ios)
	opts.Watch = true
	var slept []time.Duration
	opts.Sleep = func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	if err := checksRun(t.Context(), opts); err != nil {
		t.Fatalf("checksRun: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("polled %d times, want 3", calls.Load())
	}
	if len(slept) != 2 || slept[0] != 10*time.Second {
		t.Errorf("slept %v, want 2 × 10s", slept)
	}
	// Not a terminal: only the final result is printed.
	want := "Build\tSUCCESSFUL\t2026-09-10T11:00:00Z\thttps://ci/build\n" +
		"Test\tSUCCESSFUL\t2026-09-10T11:50:00Z\thttps://ci/test\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
}

func TestChecksWatchTTYFailure(t *testing.T) {
	t.Parallel()
	srv, _ := watchServer(t, 1, api.StatusFailed)
	ios, _, out, _ := cmdutil.TestIOStreams()
	noColorTTY(ios)
	opts := newChecksOptions(t, srv, ios)
	opts.Watch = true
	opts.Sleep = func(context.Context, time.Duration) error { return nil }

	if err := checksRun(t.Context(), opts); !errors.Is(err, cmdutil.ErrSilent) {
		t.Fatalf("err = %v, want ErrSilent", err)
	}
	want := "Refreshing checks every 10 seconds. Press Ctrl+C to quit.\n" +
		"\n" +
		"Checks: 1 passed, 1 in progress\n" +
		"\n" +
		"Test   INPROGRESS  10 minutes ago     https://ci/test\n" +
		"Build  SUCCESSFUL  about an hour ago  https://ci/build\n" +
		clearScreen +
		"Checks: 1 passed, 1 failed\n" +
		"\n" +
		"Test   FAILED      10 minutes ago     https://ci/test\n" +
		"Build  SUCCESSFUL  about an hour ago  https://ci/build\n"
	if out.String() != want {
		t.Errorf("stdout =\n%q\nwant\n%q", out.String(), want)
	}
}

func TestChecksWatchNothingPending(t *testing.T) {
	t.Parallel()
	srv, calls := watchServer(t, 0, api.StatusSuccessful)
	ios, _, out, _ := cmdutil.TestIOStreams()
	noColorTTY(ios)
	opts := newChecksOptions(t, srv, ios) // Sleep fails the test if called
	opts.Watch = true

	if err := checksRun(t.Context(), opts); err != nil {
		t.Fatalf("checksRun: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("polled %d times, want 1", calls.Load())
	}
	if strings.Contains(out.String(), "Refreshing") || strings.Contains(out.String(), clearScreen) {
		t.Errorf("no redraw expected:\n%q", out.String())
	}
}

func TestChecksWatchInterrupted(t *testing.T) {
	t.Parallel()
	srv, _ := watchServer(t, 100, api.StatusSuccessful)
	ios, _, _, _ := cmdutil.TestIOStreams()
	opts := newChecksOptions(t, srv, ios)
	opts.Watch = true
	opts.Sleep = func(context.Context, time.Duration) error { return context.Canceled }

	if err := checksRun(t.Context(), opts); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestChecksAPIError(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, statusesPath, 404, `{"type":"error","error":{"message":"Pull request not found"}}`)
	ios, _, _, _ := cmdutil.TestIOStreams()

	err := checksRun(t.Context(), newChecksOptions(t, srv, ios))
	if err == nil || errors.Is(err, cmdutil.ErrSilent) || !strings.Contains(err.Error(), "Pull request not found") {
		t.Fatalf("err = %v, want the API error", err)
	}
}

func TestChecksSummary(t *testing.T) {
	t.Parallel()
	cs := output.NewColorScheme(false)
	statuses := []api.CommitStatus{
		{State: "FAILED"}, {State: "SUCCESSFUL"}, {State: "SUCCESSFUL"},
		{State: "INPROGRESS"}, {State: "STOPPED"}, {State: "QUEUED"},
	}
	if got, want := checksSummary(statuses, cs), "2 passed, 1 failed, 1 in progress, 1 stopped, 1 queued"; got != want {
		t.Errorf("checksSummary = %q, want %q", got, want)
	}
	if got := checksSummary(nil, cs); got != "" {
		t.Errorf("checksSummary(nil) = %q, want empty", got)
	}
}

func TestChecksArgs(t *testing.T) {
	t.Parallel()
	var got *ChecksOptions
	cmd := NewCmdChecks(cmdtest.NewFactory(), func(o *ChecksOptions) error {
		got = o
		return nil
	})
	if _, _, err := cmdtest.RunCommand(t, cmd, "#7", "--json", "--watch"); err != nil {
		t.Fatal(err)
	}
	if got.Arg != "#7" || !got.JSON || !got.Watch {
		t.Errorf("opts = %+v", got)
	}
}

func TestChecksJQKeepsExitCode(t *testing.T) {
	t.Parallel()
	srv := apitest.New(t)
	srv.Handle(http.MethodGet, statusesPath, 200, sampleStatuses)
	ios, _, out, _ := cmdutil.TestIOStreams()
	opts := newChecksOptions(t, srv, ios)
	opts.JSON = true
	opts.JSONOutput = cmdutil.JSONOutput{Fields: []string{"key", "state"}, JQ: `.[] | select(.state == "FAILED") | .key`}

	if err := checksRun(t.Context(), opts); !errors.Is(err, cmdutil.ErrSilent) {
		t.Fatalf("err = %v, want ErrSilent for a failed check", err)
	}
	if got, want := out.String(), "lint\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
