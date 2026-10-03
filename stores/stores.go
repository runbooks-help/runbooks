// Package stores holds the identity persistence contract and an implementation
// per database under subpackages.
//
// The contract is the Store interface — composed of UserStore, CredentialStore,
// SessionStore and ChallengeStore — with the types it exchanges in types.go and
// the errors in errors.go.
package stores

import "context"

// Store is the whole identity persistence contract, composed of the four
// narrower interfaces below so a consumer can depend on just the part it needs.
// One implementation lives in each subpackage (sqlite, mysql, postgres).
type Store interface {
	UserStore
	CredentialStore
	SessionStore
	ChallengeStore
	InviteStore
	AuthEventStore

	// Close releases the database.
	Close() error
}

// UserStore is the user half of the contract. A Get miss is ErrNotFound, List
// treats empty as OK, a duplicate email is a ConflictError, an empty email never
// matches, and Update is idempotent.
type UserStore interface {
	GetUser(ctx context.Context, id string) (User, error)
	GetUserByEmail(ctx context.Context, email string) (User, error)
	ListUsers(ctx context.Context) ([]User, error)
	InsertUser(ctx context.Context, u User) error
	UpdateUser(ctx context.Context, u User) error
}

// CredentialStore is the passkey half of the contract. A Get miss is
// ErrNotFound, List treats empty as OK, and a duplicate credential id is a
// ConflictError.
type CredentialStore interface {
	GetCredential(ctx context.Context, credentialID []byte) (Credential, error)
	ListCredentials(ctx context.Context, userID string) ([]Credential, error)
	InsertCredential(ctx context.Context, c Credential) error
	UpdateCredential(ctx context.Context, c Credential) error
	DeleteCredential(ctx context.Context, id string) error
}

// SessionStore is the session half of the contract. A Get miss is ErrNotFound;
// List treats empty as OK; Update and Delete are idempotent.
type SessionStore interface {
	GetSession(ctx context.Context, id string) (Session, error)
	ListSessions(ctx context.Context, userID string) ([]Session, error)
	InsertSession(ctx context.Context, s Session) error
	UpdateSession(ctx context.Context, s Session) error
	DeleteSession(ctx context.Context, id string) error
	DeleteSessionsForUser(ctx context.Context, userID string) error
}

// ChallengeStore is the WebAuthn ceremony-state half of the contract:
// short-lived state keyed by an opaque cookie id. A Get miss is ErrNotFound.
type ChallengeStore interface {
	GetChallenge(ctx context.Context, id string) (Challenge, error)
	InsertChallenge(ctx context.Context, c Challenge) error
	DeleteChallenge(ctx context.Context, id string) error
}

// InviteStore is the enrolment-invite half of the contract. A Get miss is
// ErrNotFound; Update marks an invite consumed or binds it to a user.
type InviteStore interface {
	GetInvite(ctx context.Context, id string) (Invite, error)
	InsertInvite(ctx context.Context, i Invite) error
	UpdateInvite(ctx context.Context, i Invite) error
}

// AuthEventStore is the append-only audit half of the contract. Insert assigns
// no id; List returns events oldest-first and treats empty as OK.
type AuthEventStore interface {
	InsertAuthEvent(ctx context.Context, e AuthEvent) error
	ListAuthEvents(ctx context.Context) ([]AuthEvent, error)
}
