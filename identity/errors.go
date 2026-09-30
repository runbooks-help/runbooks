package identity

import "errors"

// ErrNotFound is returned when a single-row read matches nothing.
var ErrNotFound = errors.New("identity: not found")

// ConflictError reports a uniqueness violation — an email or credential id that
// is already registered. The underlying error is kept for inspection.
type ConflictError struct {
	err error
}

func (e *ConflictError) Error() string {
	return "identity: conflict: " + e.err.Error()
}

// Unwrap returns the underlying error.
func (e *ConflictError) Unwrap() error { return e.err }
