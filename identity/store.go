package identity

import "context"

// Store is the identity persistence contract: identity tables only, not a
// general ORM.
//
// Get returns one row and treats a miss as ErrNotFound; List returns many and
// treats empty as OK; Update and Delete are idempotent. Stores are safe for
// concurrent use.
type Store interface {
	// Users. InsertUser and UpdateUser report a duplicate email as a
	// ConflictError.
	GetUser(ctx context.Context, id string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, error)
	ListUsers(ctx context.Context) ([]User, error)
	InsertUser(ctx context.Context, u User) error
	UpdateUser(ctx context.Context, u User) error

	// Credentials. GetCredential looks up by the authenticator's credential id.
	// InsertCredential reports a duplicate credential id as a ConflictError.
	GetCredential(ctx context.Context, credentialID []byte) (Credential, error)
	ListCredentials(ctx context.Context, userID string) ([]Credential, error)
	InsertCredential(ctx context.Context, c Credential) error
	UpdateCredential(ctx context.Context, c Credential) error
	DeleteCredential(ctx context.Context, id string) error
}
