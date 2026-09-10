package cmdutil

import (
	"errors"
	"fmt"
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

// IsUserCancellation reports whether err represents a user cancellation.
func IsUserCancellation(err error) bool {
	return errors.Is(err, ErrCancel)
}
