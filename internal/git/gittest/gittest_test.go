package gittest_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mantas6/bh/internal/git"
	"github.com/mantas6/bh/internal/git/gittest"
)

// fakeTB captures the failures and cleanups a Stub registers so the tests can
// assert on them without failing themselves.
type fakeTB struct {
	testing.TB

	mu       sync.Mutex
	errs     []string
	cleanups []func()
}

func (f *fakeTB) Helper() {}

func (f *fakeTB) Errorf(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

func (f *fakeTB) Cleanup(fn func()) { f.cleanups = append(f.cleanups, fn) }

// finish runs the registered cleanups and returns the reported failures.
func (f *fakeTB) finish() []string {
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
	return f.errs
}

func TestRegisteredResponse(t *testing.T) {
	t.Parallel()
	ftb := &fakeTB{}
	boom := gittest.Exit(2)
	s := gittest.New(ftb).
		Register("main\n\n", nil, "symbolic-ref", "HEAD").
		Register("", boom, "ls-remote")

	if out, err := s.Run(t.Context(), "symbolic-ref", "HEAD"); out != "main" || err != nil {
		t.Errorf("Run = %q, %v; want trimmed stdout", out, err)
	}
	if err := s.RunInteractive(t.Context(), "ls-remote"); !errors.Is(err, boom) {
		t.Errorf("RunInteractive err = %v, want %v", err, boom)
	}
	if got := s.CallStrings(); !reflect.DeepEqual(got, []string{"symbolic-ref HEAD", "ls-remote"}) {
		t.Errorf("calls = %q", got)
	}
	if got := s.InteractiveStrings(); !reflect.DeepEqual(got, []string{"ls-remote"}) {
		t.Errorf("interactive = %q", got)
	}
	if errs := ftb.finish(); len(errs) != 0 {
		t.Errorf("unexpected failures: %q", errs)
	}
}

func TestUnstubbedCallFails(t *testing.T) {
	t.Parallel()
	ftb := &fakeTB{}
	s := gittest.New(ftb)

	if _, err := s.Run(t.Context(), "push", "origin"); err == nil {
		t.Error("Run: expected an error for an unstubbed call")
	}
	if err := s.RunInteractive(t.Context(), "fetch"); err == nil {
		t.Error("RunInteractive: expected an error for an unstubbed call")
	}
	errs := ftb.finish()
	if len(errs) != 2 || !strings.Contains(errs[0], "git push origin") || !strings.Contains(errs[1], "git fetch") {
		t.Errorf("failures = %q, want both argvs reported", errs)
	}
}

func TestExpectVerifiedInOrder(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		run     []string
		wantErr string
	}{
		{"all in order", []string{"a", "x", "b"}, ""},
		{"missing", []string{"a"}, "expected call did not happen (in order): git b"},
		{"out of order", []string{"b", "a"}, "expected call did not happen (in order): git b"},
		{"none", nil, "git a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ftb := &fakeTB{}
			s := gittest.New(ftb).Expect("a").ExpectResponse("out", nil, "b").Register("", nil, "x")
			for _, c := range tt.run {
				if _, err := s.Run(t.Context(), c); err != nil {
					t.Fatal(err)
				}
			}
			errs := ftb.finish()
			if tt.wantErr == "" {
				if len(errs) != 0 {
					t.Errorf("unexpected failures: %q", errs)
				}
				return
			}
			if len(errs) != 1 || !strings.Contains(errs[0], tt.wantErr) {
				t.Errorf("failures = %q, want %q", errs, tt.wantErr)
			}
		})
	}
}

func TestCallsDoNotAliasArgs(t *testing.T) {
	t.Parallel()
	s := gittest.New(t).Register("", nil, "checkout", "main")
	args := []string{"checkout", "main"}
	if _, err := s.Run(t.Context(), args...); err != nil {
		t.Fatal(err)
	}
	args[1] = "mutated"
	s.Calls()[0][0] = "mutated"
	if got := s.CallStrings(); !reflect.DeepEqual(got, []string{"checkout main"}) {
		t.Errorf("calls = %q, want the original argv", got)
	}
}

func TestConcurrentUse(t *testing.T) {
	t.Parallel()
	s := gittest.New(t).Register("", nil, "status")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, _ = s.Run(t.Context(), "status")
			_ = s.RunInteractive(t.Context(), "status")
			_ = s.CallStrings()
		})
	}
	wg.Wait()
	if n := len(s.Calls()); n != 16 {
		t.Errorf("calls = %d, want 16", n)
	}
}

func TestExit(t *testing.T) {
	t.Parallel()
	var ge *git.Error
	if err := gittest.Exit(1); !errors.As(err, &ge) || ge.ExitCode != 1 {
		t.Errorf("Exit(1) = %#v", err)
	}
}
