package cmdutil

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"sync"

	"github.com/mantas6/bh/internal/output"
)

// IOStreams holds the input and output streams for a command along with
// terminal-related metadata.
//
// Standard input should be read through Stdin or Prompter rather than In
// directly so that every consumer shares one buffered reader; otherwise a
// prompt could buffer (and lose) input meant for the next prompt.
type IOStreams struct {
	In     io.Reader
	Out    io.Writer
	ErrOut io.Writer

	// Getenv looks up environment variables that affect presentation
	// (NO_COLOR, CLICOLOR, CLICOLOR_FORCE, TERM). Nil means os.Getenv.
	Getenv func(string) string

	stdinTTY  bool
	stdoutTTY bool
	stderrTTY bool

	stdinOnce sync.Once
	stdin     *bufio.Reader
	prompter  *Prompter
}

// IsStdinTTY reports whether standard input is connected to a terminal.
func (s *IOStreams) IsStdinTTY() bool { return s.stdinTTY }

// IsStdoutTTY reports whether standard output is connected to a terminal.
func (s *IOStreams) IsStdoutTTY() bool { return s.stdoutTTY }

// IsStderrTTY reports whether standard error is connected to a terminal.
func (s *IOStreams) IsStderrTTY() bool { return s.stderrTTY }

// SetStdinTTY sets whether standard input is a terminal.
func (s *IOStreams) SetStdinTTY(v bool) { s.stdinTTY = v }

// SetStdoutTTY sets whether standard output is a terminal.
func (s *IOStreams) SetStdoutTTY(v bool) { s.stdoutTTY = v }

// SetStderrTTY sets whether standard error is a terminal.
func (s *IOStreams) SetStderrTTY(v bool) { s.stderrTTY = v }

// StdinFd returns the file descriptor behind In, if In is backed by one (as
// *os.File is). It is used for terminal operations such as reading a
// password without echo.
func (s *IOStreams) StdinFd() (int, bool) {
	f, ok := s.In.(interface{ Fd() uintptr })
	if !ok {
		return 0, false
	}
	return int(f.Fd()), true
}

// Stdin returns the shared buffered reader over In. All reads of standard
// input (prompts, --body-file -, --with-token) should go through it.
func (s *IOStreams) Stdin() io.Reader {
	s.initStdin()
	return s.stdin
}

// Prompter returns the shared Prompter, which reads answers from Stdin and
// writes prompts to ErrOut.
func (s *IOStreams) Prompter() *Prompter {
	s.initStdin()
	return s.prompter
}

func (s *IOStreams) initStdin() {
	s.stdinOnce.Do(func() {
		in := s.In
		if in == nil {
			in = bytes.NewReader(nil)
		}
		s.stdin = bufio.NewReader(in)
		s.prompter = &Prompter{in: s.stdin, out: s.ErrOut}
	})
}

func (s *IOStreams) getenv(key string) string {
	if s.Getenv != nil {
		return s.Getenv(key)
	}
	return os.Getenv(key)
}

// colorEnabled applies the NO_COLOR / CLICOLOR conventions to a stream with
// the given TTY state: CLICOLOR_FORCE (non-empty, not "0") forces colour on;
// otherwise NO_COLOR (non-empty), CLICOLOR=0 or TERM=dumb turn it off, and
// colour is used only on a terminal.
func (s *IOStreams) colorEnabled(isTTY bool) bool {
	if v := s.getenv("CLICOLOR_FORCE"); v != "" && v != "0" {
		return true
	}
	if s.getenv("NO_COLOR") != "" || s.getenv("CLICOLOR") == "0" || s.getenv("TERM") == "dumb" {
		return false
	}
	return isTTY
}

// ColorEnabled reports whether ANSI colour may be written to Out.
func (s *IOStreams) ColorEnabled() bool { return s.colorEnabled(s.stdoutTTY) }

// ErrColorEnabled reports whether ANSI colour may be written to ErrOut.
func (s *IOStreams) ErrColorEnabled() bool { return s.colorEnabled(s.stderrTTY) }

// ColorScheme returns the colour scheme for text written to Out.
func (s *IOStreams) ColorScheme() *output.ColorScheme {
	return output.NewColorScheme(s.ColorEnabled())
}

// ErrColorScheme returns the colour scheme for text written to ErrOut.
func (s *IOStreams) ErrColorScheme() *output.ColorScheme {
	return output.NewColorScheme(s.ErrColorEnabled())
}

// TestIOStreams returns an IOStreams backed by in-memory buffers together with
// the buffers themselves for assertions. TTY flags default to false and may be
// toggled via the SetXxxTTY methods. The environment lookup is isolated from
// the process environment (every variable reads as unset).
func TestIOStreams() (streams *IOStreams, in, out, errOut *bytes.Buffer) {
	in = &bytes.Buffer{}
	out = &bytes.Buffer{}
	errOut = &bytes.Buffer{}
	streams = &IOStreams{
		In:     in,
		Out:    out,
		ErrOut: errOut,
		Getenv: func(string) string { return "" },
	}
	return streams, in, out, errOut
}
