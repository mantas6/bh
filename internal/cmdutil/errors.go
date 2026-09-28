package cmdutil

import (
	"errors"
	"fmt"

	"github.com/mantas6/bh/internal/api"
)

// ErrSilent is returned when an error has already been printed and the top
// level should simply exit non-zero without printing anything further.
var ErrSilent = errors.New("SilentError")

// ErrCancel is returned when the user cancels an interactive prompt.
var ErrCancel = errors.New("CancelError")

// FlagError indicates a problem with the command's flags or arguments. It is
// distinguished so the top level can print usage information.
type FlagError struct {
	err error
}

// FlagErrorf creates a FlagError from a format string.
func FlagErrorf(format string, args ...any) *FlagError {
	return &FlagError{err: fmt.Errorf(format, args...)}
}

// FlagErrorWrap wraps an existing error as a FlagError.
func FlagErrorWrap(err error) *FlagError {
	return &FlagError{err: err}
}

func (e *FlagError) Error() string {
	return e.err.Error()
}

func (e *FlagError) Unwrap() error {
	return e.err
}

// NotLoggedInError returns the error reported by every command that needs
// credentials for host when none are configured. It matches api.ErrNoToken
// with errors.Is.
func NotLoggedInError(host string) error {
	return &notLoggedInError{host: host}
}

type notLoggedInError struct{ host string }

func (e *notLoggedInError) Error() string {
	return fmt.Sprintf("not logged in to %s; run `bh auth login` or set BH_TOKEN", e.host)
}

func (e *notLoggedInError) Is(target error) bool {
	return target == api.ErrNoToken
}

// IsUserCancellation reports whether err represents a user cancellation.
func IsUserCancellation(err error) bool {
	return errors.Is(err, ErrCancel)
}
