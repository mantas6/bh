// Package gittest provides a fake git.Runner for use in tests. It records
// every call's arguments and returns stubbed responses keyed by the exact
// argv joined with a single space.
//
// A Stub is strict: an invocation without a stubbed response fails the test
// with the offending argv, and every Expect-ed invocation must happen (in the
// order it was expected) by the time the test finishes.
package gittest

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mantas6/bh/internal/git"
)

var _ git.Runner = (*Stub)(nil)

// Exit returns the error a git.Runner reports when git exits with status
// code, for stubbing responses such as "unset config key" (1) or "no
// matching remote ref" (2).
func Exit(code int) error {
	return &git.Error{ExitCode: code}
}

// Response is a stubbed result for a single git invocation.
type Response struct {
	Stdout string
	Err    error
}

// Stub is a fake git.Runner. It is safe for concurrent use.
type Stub struct {
	t testing.TB

	mu          sync.Mutex
	responses   map[string]Response
	expected    []string
	calls       [][]string
	interactive [][]string
}

// New returns a strict Stub bound to t. Unstubbed invocations fail t, and the
// invocations registered with Expect are verified when t finishes.
func New(t testing.TB) *Stub {
	t.Helper()
	s := &Stub{t: t, responses: map[string]Response{}}
	t.Cleanup(s.verify)
	return s
}

// Register stubs a response for the given argv. The invocation is allowed but
// not required; use Expect or ExpectResponse to require it.
func (s *Stub) Register(stdout string, err error, args ...string) *Stub {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[strings.Join(args, " ")] = Response{Stdout: stdout, Err: err}
	return s
}

// Expect stubs an empty successful response for argv and requires that it is
// invoked before the test finishes.
func (s *Stub) Expect(args ...string) *Stub {
	return s.ExpectResponse("", nil, args...)
}

// ExpectResponse stubs a response for argv and requires that it is invoked
// before the test finishes. Expected invocations must happen in the order
// they were expected, though other calls may be interleaved.
func (s *Stub) ExpectResponse(stdout string, err error, args ...string) *Stub {
	s.Register(stdout, err, args...)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expected = append(s.expected, strings.Join(args, " "))
	return s
}

// Run implements git.Runner. Like git.Client, trailing newlines are trimmed
// from the stubbed stdout.
func (s *Stub) Run(_ context.Context, args ...string) (string, error) {
	resp, err := s.record(false, args)
	if err != nil {
		return "", err
	}
	return git.TrimOutput(resp.Stdout), resp.Err
}

// RunInteractive implements git.Runner.
func (s *Stub) RunInteractive(_ context.Context, args ...string) error {
	resp, err := s.record(true, args)
	if err != nil {
		return err
	}
	return resp.Err
}

func (s *Stub) record(interactive bool, args []string) (Response, error) {
	args = slices.Clone(args)
	key := strings.Join(args, " ")

	s.mu.Lock()
	s.calls = append(s.calls, args)
	if interactive {
		s.interactive = append(s.interactive, args)
	}
	resp, ok := s.responses[key]
	s.mu.Unlock()

	if !ok {
		s.t.Errorf("gittest: unexpected call: git %s", key)
		return Response{}, fmt.Errorf("gittest: no stubbed response for %q", "git "+key)
	}
	return resp, nil
}

// verify reports expected invocations that did not happen in order.
func (s *Stub) verify() {
	calls := s.CallStrings()
	i := 0
	for _, want := range s.expectedCalls() {
		j := slices.Index(calls[i:], want)
		if j < 0 {
			s.t.Errorf("gittest: expected call did not happen (in order): git %s\nactual calls:\n  %s",
				want, strings.Join(calls, "\n  "))
			return
		}
		i += j + 1
	}
}

func (s *Stub) expectedCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.expected)
}

// Calls returns a copy of the argv of every invocation (Run and
// RunInteractive), in order.
func (s *Stub) Calls() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneAll(s.calls)
}

// Interactive returns a copy of the argv of every RunInteractive invocation,
// in order.
func (s *Stub) Interactive() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneAll(s.interactive)
}

// CallStrings returns each recorded call joined with a single space, in order.
func (s *Stub) CallStrings() []string {
	return joinAll(s.Calls())
}

// InteractiveStrings returns each recorded RunInteractive call joined with a
// single space, in order.
func (s *Stub) InteractiveStrings() []string {
	return joinAll(s.Interactive())
}

func cloneAll(in [][]string) [][]string {
	out := make([][]string, len(in))
	for i, c := range in {
		out[i] = slices.Clone(c)
	}
	return out
}

func joinAll(in [][]string) []string {
	out := make([]string, len(in))
	for i, c := range in {
		out[i] = strings.Join(c, " ")
	}
	return out
}
