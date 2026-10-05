// SPDX-License-Identifier: FSL-1.1-MIT

package stores

import "errors"

// ErrNotFound is returned when a single-row read matches nothing.
var ErrNotFound = errors.New("stores: not found")

// ConflictError reports a uniqueness violation: an email or credential id that
// is already registered. The underlying error is kept for inspection.
type ConflictError struct {
	err error
}

// NewConflictError wraps err as a ConflictError.
func NewConflictError(err error) error {
	return &ConflictError{err: err}
}

func (e *ConflictError) Error() string {
	return "stores: conflict: " + e.err.Error()
}

// Unwrap returns the underlying error.
func (e *ConflictError) Unwrap() error { return e.err }
