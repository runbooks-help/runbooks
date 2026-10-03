// SPDX-License-Identifier: FSL-1.1-MIT

package nullable

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
)

var nullLiteral = []byte("null")

// Null defines a nullable type which can box any type (yay!)
type Null[T any] struct {
	V     T
	Valid bool
}

// MarshalJSON for Null
func (n Null[T]) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return nullLiteral, nil
	}
	return json.Marshal(n.V)
}

// UnmarshalJSON for Null
func (n *Null[T]) UnmarshalJSON(b []byte) error {
	if bytes.EqualFold(b, nullLiteral) {
		n.Valid = false
		return nil
	}
	err := json.Unmarshal(b, &n.V)
	n.Valid = err == nil
	return err
}

// Scan implements the Scanner interface from database/sql
func (n *Null[T]) Scan(src any) error {
	t := &sql.Null[T]{
		V:     n.V,
		Valid: n.Valid,
	}
	if err := t.Scan(src); err != nil {
		return err
	}

	n.V = t.V
	n.Valid = t.Valid

	return nil
}

// Value returns the database/sql driver value for Null
func (n Null[T]) Value() (driver.Value, error) {
	// An invalid Null is always SQL NULL, even when T implements driver.Valuer.
	// Without this guard, a Valuer like ulid.ULID would marshal its zero value
	// (e.g. 16 zero bytes for a binary column) instead of NULL.
	if !n.Valid {
		return nil, nil
	}
	if valuer, ok := any(n.V).(driver.Valuer); ok {
		return valuer.Value()
	}
	return sql.Null[T]{
		V:     n.V,
		Valid: n.Valid,
	}.Value()
}
