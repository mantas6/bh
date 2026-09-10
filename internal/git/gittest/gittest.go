// Package gittest provides a fake git.Runner for use in tests. It records
// every call's arguments and returns stubbed responses keyed by the exact
// argv joined with a single space.
package gittest

import (
	"context"
	"fmt"
	"strings"
)

// Response is a stubbed result for a single git invocation.
type Response struct {
	Stdout string
	Err    error
}

// Stub is a fake git.Runner. Responses maps a joined-argv key (args joined
// with " ") to the response returned by Run/RunInteractive. Calls records the
// argv of every invocation in order.
type Stub struct {
	// Responses is keyed by strings.Join(args, " ").
	Responses map[string]Response
	// Calls records the args of every call, in order.
	Calls [][]string
	// Interactive records the args of RunInteractive calls, in order.
	Interactive [][]string
	// FailUnstubbed, when true, makes Run return an error for unknown argv
	// instead of an empty successful response.
	FailUnstubbed bool
}

// New returns an empty Stub ready for use.
func New() *Stub {
	return &Stub{Responses: map[string]Response{}}
}

// Register stubs a response for the given argv.
func (s *Stub) Register(stdout string, err error, args ...string) *Stub {
	if s.Responses == nil {
		s.Responses = map[string]Response{}
	}
	s.Responses[strings.Join(args, " ")] = Response{Stdout: stdout, Err: err}
	return s
}

// Expect is an alias for Register that stubs an empty successful response,
// convenient for asserting a specific argv was invoked.
func (s *Stub) Expect(args ...string) *Stub {
	return s.Register("", nil, args...)
}

// Run implements git.Runner.
func (s *Stub) Run(_ context.Context, args ...string) (string, error) {
	s.Calls = append(s.Calls, args)
	key := strings.Join(args, " ")
	if resp, ok := lookup(s.Responses, key); ok {
		return resp.Stdout, resp.Err
	}
	if s.FailUnstubbed {
		return "", fmt.Errorf("gittest: no stubbed response for %q", "git "+key)
	}
	return "", nil
}

// RunInteractive implements git.Runner.
func (s *Stub) RunInteractive(_ context.Context, args ...string) error {
	s.Calls = append(s.Calls, args)
	s.Interactive = append(s.Interactive, args)
	key := strings.Join(args, " ")
	if resp, ok := lookup(s.Responses, key); ok {
		return resp.Err
	}
	if s.FailUnstubbed {
		return fmt.Errorf("gittest: no stubbed response for %q", "git "+key)
	}
	return nil
}

func lookup(m map[string]Response, key string) (Response, bool) {
	if m == nil {
		return Response{}, false
	}
	resp, ok := m[key]
	return resp, ok
}

// CallStrings returns each recorded call joined with a single space, in order.
func (s *Stub) CallStrings() []string {
	out := make([]string, len(s.Calls))
	for i, c := range s.Calls {
		out[i] = strings.Join(c, " ")
	}
	return out
}
