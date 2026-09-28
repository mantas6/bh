package cmdutil

import (
	"errors"
	"strings"
	"testing"
)

func TestPrompterPipedMultiplePrompts(t *testing.T) {
	ios, in, _, errOut := TestIOStreams()
	// All answers arrive at once, as with `printf ... | bh ...`.
	in.WriteString("My title\r\n\nn\n  yes  \n")

	title, err := ios.Prompter().Input("Title", "")
	if err != nil || title != "My title" {
		t.Fatalf("Input #1 = %q, %v", title, err)
	}
	email, err := ios.Prompter().Input("Email", "default@example.com")
	if err != nil || email != "default@example.com" {
		t.Fatalf("Input #2 = %q, %v (want default for empty answer)", email, err)
	}
	ok, err := ios.Prompter().Confirm("Push?", true)
	if err != nil || ok {
		t.Fatalf("Confirm #1 = %v, %v, want false", ok, err)
	}
	ok, err = ios.Prompter().Confirm("Delete?", false)
	if err != nil || !ok {
		t.Fatalf("Confirm #2 = %v, %v, want true", ok, err)
	}

	want := "Title: Email (default@example.com): Push? [Y/n] Delete? [y/N] "
	if got := errOut.String(); got != want {
		t.Errorf("prompts = %q, want %q", got, want)
	}
}

func TestPrompterConfirm(t *testing.T) {
	tests := []struct {
		in   string
		def  bool
		want bool
	}{
		{"\n", true, true},
		{"\n", false, false},
		{"", true, true}, // EOF yields the default
		{"", false, false},
		{"y\n", false, true},
		{"YES\n", false, true},
		{"n\n", true, false},
		{"No\n", true, false},
		{"maybe\n", true, false},
	}
	for _, tt := range tests {
		p := NewPrompter(strings.NewReader(tt.in), &strings.Builder{})
		got, err := p.Confirm("Continue?", tt.def)
		if err != nil {
			t.Fatalf("Confirm(%q): %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("Confirm(%q, def=%v) = %v, want %v", tt.in, tt.def, got, tt.want)
		}
	}
}

func TestPrompterInputEOF(t *testing.T) {
	p := NewPrompter(strings.NewReader("no-newline"), &strings.Builder{})
	got, err := p.Input("Name", "")
	if err != nil || got != "no-newline" {
		t.Fatalf("Input = %q, %v", got, err)
	}
	got, err = p.Input("Name", "fallback")
	if err != nil || got != "fallback" {
		t.Fatalf("Input at EOF = %q, %v, want default", got, err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestPrompterReadError(t *testing.T) {
	p := NewPrompter(errReader{}, &strings.Builder{})
	if _, err := p.Confirm("Continue?", true); err == nil {
		t.Error("expected read error")
	}
	if _, err := p.Input("Name", ""); err == nil {
		t.Error("expected read error")
	}
}
