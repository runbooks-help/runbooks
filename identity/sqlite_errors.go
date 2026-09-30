package identity

import (
	"errors"

	"modernc.org/sqlite"
)

// sqliteConstraint is SQLITE_CONSTRAINT. SQLite reports extended codes (e.g.
// 2067 SQLITE_CONSTRAINT_UNIQUE), so only the low byte is compared.
const sqliteConstraint = 19

// NewConflictError wraps err as a ConflictError when it is a SQLite uniqueness
// violation, and returns it unchanged otherwise.
func NewConflictError(err error) error {
	var se *sqlite.Error
	if !errors.As(err, &se) || se.Code()&0xff != sqliteConstraint {
		return err
	}
	return &ConflictError{err: err}
}
