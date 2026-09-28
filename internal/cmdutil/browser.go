package cmdutil

import (
	"errors"
	"fmt"
)

// Browser opens URLs in the user's web browser.
type Browser interface {
	Browse(url string) error
}

// OpenInBrowser opens url with b. When standard output is a terminal a
// short "Opening ..." notice is written to ErrOut first.
func OpenInBrowser(ios *IOStreams, b Browser, url string) error {
	if b == nil {
		return errors.New("no web browser available")
	}
	if ios.IsStdoutTTY() {
		fmt.Fprintf(ios.ErrOut, "Opening %s in your browser.\n", url)
	}
	return b.Browse(url)
}
