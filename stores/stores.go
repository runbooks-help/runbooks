// Package stores holds the identity persistence contract and an implementation
// per database under subpackages.
//
// The contract is the Store interface below; the types it exchanges live in
// types.go and the errors in errors.go.
package stores

import "context"

// Store is the identity persistence contract: identity tables only, not a
// general ORM. One implementation lives in each subpackage (sqlite, mysql,
// postgres).
//
// Get returns one row and treats a miss as ErrNotFound; List returns many and
// treats empty as OK; Update and Delete are idempotent. Implementations are safe
// for concurrent use.
type Store interface {
	// GetUser returns the user with the given id, or ErrNotFound.
	GetUser(ctx context.Context, id string) (User, error)
	// GetUserByEmail returns the user with the given email, or ErrNotFound. An
	// empty email never matches.
	GetUserByEmail(ctx context.Context, email string) (User, error)
	// ListUsers returns every user, ordered by display name. Empty is OK.
	ListUsers(ctx context.Context) ([]User, error)
	// InsertUser adds a user. A duplicate email is a ConflictError.
	InsertUser(ctx context.Context, u User) error
	// UpdateUser updates the mutable fields (email, display name, role,
	// disabled). A duplicate email is a ConflictError; an unknown id is not an
	// error.
	UpdateUser(ctx context.Context, u User) error

	// GetCredential returns the credential with the authenticator's credential
	// id, or ErrNotFound.
	GetCredential(ctx context.Context, credentialID []byte) (Credential, error)
	// ListCredentials returns a user's credentials, oldest first. Empty is OK.
	ListCredentials(ctx context.Context, userID string) ([]Credential, error)
	// InsertCredential adds a credential. A duplicate credential id is a
	// ConflictError.
	InsertCredential(ctx context.Context, c Credential) error
	// UpdateCredential updates sign count, transports, label and last-used time.
	// An unknown id is not an error.
	UpdateCredential(ctx context.Context, c Credential) error
	// DeleteCredential removes a credential by its internal id. An unknown id is
	// not an error.
	DeleteCredential(ctx context.Context, id string) error

	// Close releases the database.
	Close() error
}
