package identity

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	"runbooks/internal/nullable"
)

//go:embed schema/sqlite/*.sql
var sqliteSchema embed.FS

// SQLiteStore is the SQLite provider of Store. It is the default provider: a
// single file, no external service, one container.
type SQLiteStore struct {
	db *sql.DB
}

var _ Store = (*SQLiteStore)(nil)

// OpenSQLite opens (creating the file if needed) the SQLite database at dsn and
// applies the identity schema. dsn is a modernc.org/sqlite DSN, e.g.
// "file:./data/runbooks.db".
func OpenSQLite(ctx context.Context, dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("identity: open sqlite: %w", err)
	}
	// SQLite allows a single writer; pinning one connection avoids
	// "database is locked" under concurrent requests.
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, fmt.Errorf("identity: configure sqlite: %w", err)
	}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database.
func (s *SQLiteStore) Close() error { return s.db.Close() }

// migrate applies every schema file, one per table, in filename order. Each is
// IF NOT EXISTS, so it is safe and cheap to re-run on every startup — no
// external migration tool.
func (s *SQLiteStore) migrate(ctx context.Context) error {
	entries, err := sqliteSchema.ReadDir("schema/sqlite")
	if err != nil {
		return fmt.Errorf("identity: read schema: %w", err)
	}
	for _, entry := range entries {
		statements, err := sqliteSchema.ReadFile("schema/sqlite/" + entry.Name())
		if err != nil {
			return fmt.Errorf("identity: read %s: %w", entry.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx, string(statements)); err != nil {
			return fmt.Errorf("identity: apply %s: %w", entry.Name(), err)
		}
	}
	return nil
}

const userColumns = `id, email, display_name, role, created_at, disabled_at`

// GetUser returns the user with the given id, or ErrNotFound.
func (s *SQLiteStore) GetUser(ctx context.Context, id string) (User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id).Scan)
}

// GetUserByEmail returns the user with the given email, or ErrNotFound. An
// empty email never matches.
func (s *SQLiteStore) GetUserByEmail(ctx context.Context, email string) (User, error) {
	if email == "" {
		return User{}, ErrNotFound
	}
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email = ?`, email).Scan)
}

// ListUsers returns every user, ordered by display name. Empty is OK.
func (s *SQLiteStore) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY display_name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// InsertUser adds a user. A duplicate email is a ConflictError.
func (s *SQLiteStore) InsertUser(ctx context.Context, u User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, display_name, role, created_at, disabled_at) VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, nullable.Null[string]{V: u.Email, Valid: u.Email != ""}, u.DisplayName,
		string(u.Role), u.CreatedAt.Unix(),
		nullable.Null[int64]{V: u.DisabledAt.Unix(), Valid: !u.DisabledAt.IsZero()})
	return NewConflictError(err)
}

// UpdateUser updates the mutable fields (email, display name, role, disabled).
// A duplicate email is a ConflictError; an unknown id is not an error.
func (s *SQLiteStore) UpdateUser(ctx context.Context, u User) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email = ?, display_name = ?, role = ?, disabled_at = ? WHERE id = ?`,
		nullable.Null[string]{V: u.Email, Valid: u.Email != ""}, u.DisplayName,
		string(u.Role), nullable.Null[int64]{V: u.DisabledAt.Unix(), Valid: !u.DisabledAt.IsZero()}, u.ID)
	return NewConflictError(err)
}

// scanUser scans a user row. scan is a *sql.Row's or *sql.Rows' Scan method.
func scanUser(scan func(dest ...any) error) (User, error) {
	var (
		u          User
		email      nullable.Null[string]
		createdAt  int64
		disabledAt nullable.Null[int64]
	)
	if err := scan(&u.ID, &email, &u.DisplayName, &u.Role, &createdAt, &disabledAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	u.Email = email.V
	u.CreatedAt = time.Unix(createdAt, 0).UTC()
	if disabledAt.Valid {
		u.DisabledAt = time.Unix(disabledAt.V, 0).UTC()
	}
	return u, nil
}

const credentialColumns = `id, user_id, credential_id, public_key, sign_count, transports, aaguid, label, created_at, last_used_at`

// GetCredential returns the credential with the authenticator's credential id,
// or ErrNotFound.
func (s *SQLiteStore) GetCredential(ctx context.Context, credentialID []byte) (Credential, error) {
	return scanCredential(s.db.QueryRowContext(ctx,
		`SELECT `+credentialColumns+` FROM credentials WHERE credential_id = ?`, credentialID).Scan)
}

// ListCredentials returns a user's credentials, oldest first. Empty is OK.
func (s *SQLiteStore) ListCredentials(ctx context.Context, userID string) ([]Credential, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+credentialColumns+` FROM credentials WHERE user_id = ? ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []Credential
	for rows.Next() {
		c, err := scanCredential(rows.Scan)
		if err != nil {
			return nil, err
		}
		creds = append(creds, c)
	}
	return creds, rows.Err()
}

// InsertCredential adds a credential. A duplicate credential id is a
// ConflictError.
func (s *SQLiteStore) InsertCredential(ctx context.Context, c Credential) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO credentials (id, user_id, credential_id, public_key, sign_count, transports, aaguid, label, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.UserID, c.CredentialID, c.PublicKey, int64(c.SignCount), c.Transports,
		c.AAGUID, c.Label, c.CreatedAt.Unix(),
		nullable.Null[int64]{V: c.LastUsedAt.Unix(), Valid: !c.LastUsedAt.IsZero()})
	return NewConflictError(err)
}

// UpdateCredential updates sign count, transports, label and last-used time. An
// unknown id is not an error.
func (s *SQLiteStore) UpdateCredential(ctx context.Context, c Credential) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE credentials SET sign_count = ?, transports = ?, label = ?, last_used_at = ? WHERE id = ?`,
		int64(c.SignCount), c.Transports, c.Label,
		nullable.Null[int64]{V: c.LastUsedAt.Unix(), Valid: !c.LastUsedAt.IsZero()}, c.ID)
	return err
}

// DeleteCredential removes a credential by its internal id. An unknown id is
// not an error.
func (s *SQLiteStore) DeleteCredential(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return err
}

// scanCredential scans a credential row. scan is a *sql.Row's or *sql.Rows'
// Scan method.
func scanCredential(scan func(dest ...any) error) (Credential, error) {
	var (
		c          Credential
		signCount  int64
		createdAt  int64
		lastUsedAt nullable.Null[int64]
	)
	if err := scan(&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &signCount,
		&c.Transports, &c.AAGUID, &c.Label, &createdAt, &lastUsedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Credential{}, ErrNotFound
		}
		return Credential{}, err
	}
	c.SignCount = uint32(signCount)
	c.CreatedAt = time.Unix(createdAt, 0).UTC()
	if lastUsedAt.Valid {
		c.LastUsedAt = time.Unix(lastUsedAt.V, 0).UTC()
	}
	return c, nil
}
