package cmdutil

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Prompter asks simple line-based questions. Obtain the shared instance from
// IOStreams.Prompter: all prompts read from one buffered reader, so several
// answers piped on standard input are consumed one line per prompt.
type Prompter struct {
	in  *bufio.Reader
	out io.Writer
}

// NewPrompter returns a Prompter reading answers from in and writing prompts
// to out. Commands should prefer IOStreams.Prompter.
func NewPrompter(in io.Reader, out io.Writer) *Prompter {
	br, ok := in.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(in)
	}
	return &Prompter{in: br, out: out}
}

// Input writes "prompt: " (or "prompt (def): " when def is non-empty) and
// returns the trimmed answer, or def when the answer is empty. End of input
// counts as an empty answer.
func (p *Prompter) Input(prompt, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "%s (%s): ", prompt, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", prompt)
	}
	ans, err := p.readLine()
	if err != nil {
		return "", err
	}
	if ans == "" {
		return def, nil
	}
	return ans, nil
}

// Confirm writes "prompt [Y/n] " (or "[y/N]" when def is false) and returns
// whether the answer is affirmative. An empty answer, or end of input, yields
// def; "y"/"yes" and "n"/"no" (any case) are accepted; anything else is
// treated as no.
func (p *Prompter) Confirm(prompt string, def bool) (bool, error) {
	hint := "[y/N]"
	if def {
		hint = "[Y/n]"
	}
	fmt.Fprintf(p.out, "%s %s ", prompt, hint)
	ans, err := p.readLine()
	if err != nil {
		return false, err
	}
	switch strings.ToLower(ans) {
	case "":
		return def, nil
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// readLine reads one line, trimmed of surrounding whitespace.
func (p *Prompter) readLine() (string, error) {
	line, err := p.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
