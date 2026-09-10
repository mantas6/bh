package cmdutil

import (
	"bytes"
	"io"
	"os"
)

// IOStreams holds the input and output streams for a command along with
// terminal-related metadata.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	stdoutTTY bool
	stdinTTY  bool
}

// IsStdoutTTY reports whether standard output is connected to a terminal.
func (s *IOStreams) IsStdoutTTY() bool {
	return s.stdoutTTY
}

// IsStdinTTY reports whether standard input is connected to a terminal.
func (s *IOStreams) IsStdinTTY() bool {
	return s.stdinTTY
}

// SetStdoutTTY overrides the stdout TTY flag. Intended for tests.
func (s *IOStreams) SetStdoutTTY(v bool) {
	s.stdoutTTY = v
}

// SetStdinTTY overrides the stdin TTY flag. Intended for tests.
func (s *IOStreams) SetStdinTTY(v bool) {
	s.stdinTTY = v
}

// ColorEnabled reports whether ANSI color output should be used: stdout must
// be a TTY, NO_COLOR must be unset, and TERM must not be "dumb".
func (s *IOStreams) ColorEnabled() bool {
	if !s.stdoutTTY {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

// TestIOStreams returns an IOStreams backed by in-memory buffers together with
// the buffers themselves for assertions. TTY flags default to false and may be
// toggled via SetStdoutTTY / SetStdinTTY.
func TestIOStreams() (streams *IOStreams, in, out, errOut *bytes.Buffer) {
	in = &bytes.Buffer{}
	out = &bytes.Buffer{}
	errOut = &bytes.Buffer{}
	streams = &IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
	}
	return streams, in, out, errOut
}
