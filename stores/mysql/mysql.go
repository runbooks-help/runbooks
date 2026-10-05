// SPDX-License-Identifier: FSL-1.1-MIT

// Package mysql is the MySQL implementation of stores.Store.
package mysql

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"

	"runbooks/internal/nullable"
	"runbooks/stores"
)

//go:embed schema/*.sql
var schemaFS embed.FS

// connMaxLifetime is how long a pooled connection is reused before being
// recycled.
const connMaxLifetime = 5 * time.Minute

type store struct {
	db *sql.DB
}

var _ stores.Store = (*store)(nil)

// Open opens the MySQL database at dsn and applies the schema. dsn is a
// go-sql-driver/mysql DSN, e.g.
// "root:root@tcp(127.0.0.1:3307)/runbooks_identity".
func Open(ctx context.Context, dsn string) (stores.Store, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql: open: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetConnMaxLifetime(connMaxLifetime)

	s := &store{db: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close releases the database.
func (s *store) Close() error { return s.db.Close() }

// migrate applies every schema file, one per table, in filename order. Each is
// IF NOT EXISTS, so it is safe and cheap to re-run on every startup; no
// external migration tool.
func (s *store) migrate(ctx context.Context) error {
	entries, err := schemaFS.ReadDir("schema")
	if err != nil {
		return fmt.Errorf("mysql: read schema: %w", err)
	}
	for _, entry := range entries {
		statements, err := schemaFS.ReadFile("schema/" + entry.Name())
		if err != nil {
			return fmt.Errorf("mysql: read %s: %w", entry.Name(), err)
		}
		if _, err := s.db.ExecContext(ctx, string(statements)); err != nil {
			return fmt.Errorf("mysql: apply %s: %w", entry.Name(), err)
		}
	}
	return s.ensureColumns(ctx)
}

// addedColumns are columns a schema file introduced after some databases were
// already created. CREATE TABLE IF NOT EXISTS leaves an existing table alone, so
// each is added explicitly when the live table is missing it; otherwise a query
// that names the column fails on an older database.
var addedColumns = []struct{ table, column, ddl string }{
	{"credentials", "flags", `ALTER TABLE credentials ADD COLUMN flags TINYINT NOT NULL DEFAULT 0`},
	{"auth_events", "detail", `ALTER TABLE auth_events ADD COLUMN detail VARCHAR(512) NULL`},
}

// ensureColumns adds every addedColumns entry the live schema is missing.
func (s *store) ensureColumns(ctx context.Context) error {
	for _, c := range addedColumns {
		exists, err := s.columnExists(ctx, c.table, c.column)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if _, err := s.db.ExecContext(ctx, c.ddl); err != nil {
			return fmt.Errorf("mysql: add %s.%s: %w", c.table, c.column, err)
		}
	}
	return nil
}

// columnExists reports whether the table already has the column. MySQL has no
// ADD COLUMN IF NOT EXISTS, so the information schema is checked first.
func (s *store) columnExists(ctx context.Context, table, column string) (bool, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, column).Scan(&exists); err != nil {
		return false, fmt.Errorf("mysql: check %s.%s: %w", table, column, err)
	}
	return exists > 0, nil
}

// mysqlDuplicateEntry is ER_DUP_ENTRY.
const mysqlDuplicateEntry = 1062

// conflictError wraps a uniqueness violation as a stores.ConflictError.
func conflictError(err error) error {
	var me *mysqldriver.MySQLError
	if errors.As(err, &me) && me.Number == mysqlDuplicateEntry {
		return stores.NewConflictError(err)
	}
	return err
}

const userColumns = `id, email, display_name, role, created_at, disabled_at`

// GetUser returns the user with the given id, or stores.ErrNotFound.
func (s *store) GetUser(ctx context.Context, id string) (stores.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id).Scan)
}

// GetUserByEmail returns the user with the given email, or stores.ErrNotFound.
// An empty email never matches.
func (s *store) GetUserByEmail(ctx context.Context, email string) (stores.User, error) {
	if email == "" {
		return stores.User{}, stores.ErrNotFound
	}
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email = ?`, email).Scan)
}

// ListUsers returns every user, ordered by display name. Empty is OK.
func (s *store) ListUsers(ctx context.Context) ([]stores.User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY display_name, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []stores.User
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// InsertUser adds a user. A duplicate email is a stores.ConflictError.
func (s *store) InsertUser(ctx context.Context, u stores.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, display_name, role, created_at, disabled_at) VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, nullable.Null[string]{V: u.Email, Valid: u.Email != ""}, u.DisplayName,
		string(u.Role), u.CreatedAt.Unix(),
		nullable.Null[int64]{V: u.DisabledAt.Unix(), Valid: !u.DisabledAt.IsZero()})
	return conflictError(err)
}

// UpdateUser updates the mutable fields (email, display name, role, disabled).
// A duplicate email is a stores.ConflictError; an unknown id is not an error.
func (s *store) UpdateUser(ctx context.Context, u stores.User) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email = ?, display_name = ?, role = ?, disabled_at = ? WHERE id = ?`,
		nullable.Null[string]{V: u.Email, Valid: u.Email != ""}, u.DisplayName,
		string(u.Role), nullable.Null[int64]{V: u.DisabledAt.Unix(), Valid: !u.DisabledAt.IsZero()}, u.ID)
	return conflictError(err)
}

// scanUser scans a user row. scan is a *sql.Row's or *sql.Rows' Scan method.
func scanUser(scan func(dest ...any) error) (stores.User, error) {
	var (
		u          stores.User
		email      nullable.Null[string]
		createdAt  int64
		disabledAt nullable.Null[int64]
	)
	if err := scan(&u.ID, &email, &u.DisplayName, &u.Role, &createdAt, &disabledAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stores.User{}, stores.ErrNotFound
		}
		return stores.User{}, err
	}
	u.Email = email.V
	u.CreatedAt = time.Unix(createdAt, 0).UTC()
	if disabledAt.Valid {
		u.DisabledAt = time.Unix(disabledAt.V, 0).UTC()
	}
	return u, nil
}

const credentialColumns = `id, user_id, credential_id, public_key, sign_count, transports, aaguid, flags, label, created_at, last_used_at`

// GetCredential returns the credential with the authenticator's credential id,
// or stores.ErrNotFound.
func (s *store) GetCredential(ctx context.Context, credentialID []byte) (stores.Credential, error) {
	return scanCredential(s.db.QueryRowContext(ctx,
		`SELECT `+credentialColumns+` FROM credentials WHERE credential_id = ?`, credentialID).Scan)
}

// ListCredentials returns a user's credentials, oldest first. Empty is OK.
func (s *store) ListCredentials(ctx context.Context, userID string) ([]stores.Credential, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+credentialColumns+` FROM credentials WHERE user_id = ? ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var creds []stores.Credential
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
// stores.ConflictError.
func (s *store) InsertCredential(ctx context.Context, c stores.Credential) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO credentials (id, user_id, credential_id, public_key, sign_count, transports, aaguid, flags, label, created_at, last_used_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.UserID, c.CredentialID, c.PublicKey, int64(c.SignCount), c.Transports,
		c.AAGUID, int64(c.Flags), c.Label, c.CreatedAt.Unix(),
		nullable.Null[int64]{V: c.LastUsedAt.Unix(), Valid: !c.LastUsedAt.IsZero()})
	return conflictError(err)
}

// UpdateCredential updates sign count, flags, transports, label and last-used
// time. An unknown id is not an error.
func (s *store) UpdateCredential(ctx context.Context, c stores.Credential) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE credentials SET sign_count = ?, flags = ?, transports = ?, label = ?, last_used_at = ? WHERE id = ?`,
		int64(c.SignCount), int64(c.Flags), c.Transports, c.Label,
		nullable.Null[int64]{V: c.LastUsedAt.Unix(), Valid: !c.LastUsedAt.IsZero()}, c.ID)
	return err
}

// DeleteCredential removes a credential by its internal id. An unknown id is
// not an error.
func (s *store) DeleteCredential(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE id = ?`, id)
	return err
}

// scanCredential scans a credential row. scan is a *sql.Row's or *sql.Rows'
// Scan method.
func scanCredential(scan func(dest ...any) error) (stores.Credential, error) {
	var (
		c          stores.Credential
		signCount  int64
		flags      int64
		createdAt  int64
		lastUsedAt nullable.Null[int64]
	)
	if err := scan(&c.ID, &c.UserID, &c.CredentialID, &c.PublicKey, &signCount,
		&c.Transports, &c.AAGUID, &flags, &c.Label, &createdAt, &lastUsedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stores.Credential{}, stores.ErrNotFound
		}
		return stores.Credential{}, err
	}
	c.SignCount = uint32(signCount)
	c.Flags = uint8(flags)
	c.CreatedAt = time.Unix(createdAt, 0).UTC()
	if lastUsedAt.Valid {
		c.LastUsedAt = time.Unix(lastUsedAt.V, 0).UTC()
	}
	return c, nil
}

const sessionColumns = `id, user_id, created_at, expires_at, last_seen_at, user_agent, ip`

// GetSession returns the session with the given hashed id, or stores.ErrNotFound.
func (s *store) GetSession(ctx context.Context, id string) (stores.Session, error) {
	return scanSession(s.db.QueryRowContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE id = ?`, id).Scan)
}

// InsertSession adds a session.
func (s *store) InsertSession(ctx context.Context, sess stores.Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, created_at, expires_at, last_seen_at, user_agent, ip) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.UserID, sess.CreatedAt.Unix(), sess.ExpiresAt.Unix(), sess.LastSeenAt.Unix(),
		nullable.Null[string]{V: sess.UserAgent, Valid: sess.UserAgent != ""},
		nullable.Null[string]{V: sess.IP, Valid: sess.IP != ""})
	return err
}

// UpdateSession updates expiry and last-seen. An unknown id is not an error.
func (s *store) UpdateSession(ctx context.Context, sess stores.Session) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET expires_at = ?, last_seen_at = ? WHERE id = ?`,
		sess.ExpiresAt.Unix(), sess.LastSeenAt.Unix(), sess.ID)
	return err
}

// DeleteSession removes a session by its hashed id. An unknown id is not an error.
func (s *store) DeleteSession(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id)
	return err
}

// DeleteSessionsForUser removes every session for a user. Zero rows is OK.
func (s *store) DeleteSessionsForUser(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// ListSessions returns a user's sessions, most recently seen first. Empty is OK.
func (s *store) ListSessions(ctx context.Context, userID string) ([]stores.Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sessionColumns+` FROM sessions WHERE user_id = ? ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSessions(rows)
}

// scanSession scans a session row. scan is a *sql.Row's or *sql.Rows' Scan method.
func scanSession(scan func(dest ...any) error) (stores.Session, error) {
	var (
		sess       stores.Session
		createdAt  int64
		expiresAt  int64
		lastSeenAt int64
		userAgent  nullable.Null[string]
		ip         nullable.Null[string]
	)
	if err := scan(&sess.ID, &sess.UserID, &createdAt, &expiresAt, &lastSeenAt, &userAgent, &ip); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stores.Session{}, stores.ErrNotFound
		}
		return stores.Session{}, err
	}
	sess.CreatedAt = time.Unix(createdAt, 0).UTC()
	sess.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	sess.LastSeenAt = time.Unix(lastSeenAt, 0).UTC()
	sess.UserAgent = userAgent.V
	sess.IP = ip.V
	return sess, nil
}

// scanSessions drains a session row cursor.
func scanSessions(rows *sql.Rows) ([]stores.Session, error) {
	var out []stores.Session
	for rows.Next() {
		sess, err := scanSession(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

const challengeColumns = `id, kind, data, expires_at`

// GetChallenge returns ceremony state by its cookie id, or stores.ErrNotFound.
func (s *store) GetChallenge(ctx context.Context, id string) (stores.Challenge, error) {
	return scanChallenge(s.db.QueryRowContext(ctx, `SELECT `+challengeColumns+` FROM webauthn_challenges WHERE id = ?`, id).Scan)
}

// InsertChallenge stores ceremony state.
func (s *store) InsertChallenge(ctx context.Context, c stores.Challenge) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO webauthn_challenges (id, kind, data, expires_at) VALUES (?, ?, ?, ?)`,
		c.ID, c.Kind, c.Data, c.ExpiresAt.Unix())
	return err
}

// DeleteChallenge clears ceremony state. An unknown id is not an error.
func (s *store) DeleteChallenge(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM webauthn_challenges WHERE id = ?`, id)
	return err
}

// scanChallenge scans a challenge row. scan is a *sql.Row's or *sql.Rows' Scan method.
func scanChallenge(scan func(dest ...any) error) (stores.Challenge, error) {
	var (
		c         stores.Challenge
		expiresAt int64
	)
	if err := scan(&c.ID, &c.Kind, &c.Data, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stores.Challenge{}, stores.ErrNotFound
		}
		return stores.Challenge{}, err
	}
	c.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	return c, nil
}

const inviteColumns = `id, user_id, role, created_by, created_at, expires_at, used_at`

// GetInvite returns the invite with the given hashed token id, or stores.ErrNotFound.
func (s *store) GetInvite(ctx context.Context, id string) (stores.Invite, error) {
	return scanInvite(s.db.QueryRowContext(ctx, `SELECT `+inviteColumns+` FROM invites WHERE id = ?`, id).Scan)
}

// InsertInvite adds an invite.
func (s *store) InsertInvite(ctx context.Context, i stores.Invite) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO invites (id, user_id, role, created_by, created_at, expires_at, used_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		i.ID, nullable.Null[string]{V: i.UserID, Valid: i.UserID != ""}, string(i.Role), i.CreatedBy,
		i.CreatedAt.Unix(), i.ExpiresAt.Unix(),
		nullable.Null[int64]{V: i.UsedAt.Unix(), Valid: !i.UsedAt.IsZero()})
	return err
}

// UpdateInvite updates the bound user and used time. An unknown id is not an error.
func (s *store) UpdateInvite(ctx context.Context, i stores.Invite) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE invites SET user_id = ?, used_at = ? WHERE id = ?`,
		nullable.Null[string]{V: i.UserID, Valid: i.UserID != ""},
		nullable.Null[int64]{V: i.UsedAt.Unix(), Valid: !i.UsedAt.IsZero()}, i.ID)
	return err
}

func scanInvite(scan func(dest ...any) error) (stores.Invite, error) {
	var (
		inv       stores.Invite
		userID    nullable.Null[string]
		createdAt int64
		expiresAt int64
		usedAt    nullable.Null[int64]
	)
	if err := scan(&inv.ID, &userID, &inv.Role, &inv.CreatedBy, &createdAt, &expiresAt, &usedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stores.Invite{}, stores.ErrNotFound
		}
		return stores.Invite{}, err
	}
	inv.UserID = userID.V
	inv.CreatedAt = time.Unix(createdAt, 0).UTC()
	inv.ExpiresAt = time.Unix(expiresAt, 0).UTC()
	if usedAt.Valid {
		inv.UsedAt = time.Unix(usedAt.V, 0).UTC()
	}
	return inv, nil
}

const apiKeyColumns = `id, user_id, label, created_by, created_at, last_used_at, revoked_at`

// GetAPIKey returns the key with the given hashed id, or stores.ErrNotFound.
func (s *store) GetAPIKey(ctx context.Context, id string) (stores.APIKey, error) {
	return scanAPIKey(s.db.QueryRowContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE id = ?`, id).Scan)
}

// InsertAPIKey adds a key. Only the hash is stored; the raw key never is.
func (s *store) InsertAPIKey(ctx context.Context, k stores.APIKey) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_keys (id, user_id, label, created_by, created_at, last_used_at, revoked_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		k.ID, k.UserID, k.Label, k.CreatedBy, k.CreatedAt.Unix(),
		nullable.Null[int64]{V: k.LastUsedAt.Unix(), Valid: !k.LastUsedAt.IsZero()},
		nullable.Null[int64]{V: k.RevokedAt.Unix(), Valid: !k.RevokedAt.IsZero()})
	return err
}

// UpdateAPIKey touches last-used and revoke. An unknown id is not an error.
func (s *store) UpdateAPIKey(ctx context.Context, k stores.APIKey) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET last_used_at = ?, revoked_at = ? WHERE id = ?`,
		nullable.Null[int64]{V: k.LastUsedAt.Unix(), Valid: !k.LastUsedAt.IsZero()},
		nullable.Null[int64]{V: k.RevokedAt.Unix(), Valid: !k.RevokedAt.IsZero()}, k.ID)
	return err
}

// ListAPIKeys returns a user's keys, oldest first. Empty is OK.
func (s *store) ListAPIKeys(ctx context.Context, userID string) ([]stores.APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+apiKeyColumns+` FROM api_keys WHERE user_id = ? ORDER BY created_at`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []stores.APIKey
	for rows.Next() {
		k, err := scanAPIKey(rows.Scan)
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// scanAPIKey scans a key row. scan is a *sql.Row's or *sql.Rows' Scan method.
func scanAPIKey(scan func(dest ...any) error) (stores.APIKey, error) {
	var (
		k          stores.APIKey
		createdAt  int64
		lastUsedAt nullable.Null[int64]
		revokedAt  nullable.Null[int64]
	)
	if err := scan(&k.ID, &k.UserID, &k.Label, &k.CreatedBy, &createdAt, &lastUsedAt, &revokedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return stores.APIKey{}, stores.ErrNotFound
		}
		return stores.APIKey{}, err
	}
	k.CreatedAt = time.Unix(createdAt, 0).UTC()
	if lastUsedAt.Valid {
		k.LastUsedAt = time.Unix(lastUsedAt.V, 0).UTC()
	}
	if revokedAt.Valid {
		k.RevokedAt = time.Unix(revokedAt.V, 0).UTC()
	}
	return k, nil
}

// InsertAuthEvent appends an audit row. The store assigns the id.
func (s *store) InsertAuthEvent(ctx context.Context, e stores.AuthEvent) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO auth_events (at, actor_user_id, action, target_user_id, detail, ip, user_agent) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.At.Unix(),
		nullable.Null[string]{V: e.ActorUserID, Valid: e.ActorUserID != ""},
		string(e.Action),
		nullable.Null[string]{V: e.TargetUserID, Valid: e.TargetUserID != ""},
		nullable.Null[string]{V: e.Detail, Valid: e.Detail != ""},
		nullable.Null[string]{V: e.IP, Valid: e.IP != ""},
		nullable.Null[string]{V: e.UserAgent, Valid: e.UserAgent != ""})
	return err
}

// ListAuthEvents returns every audit row, oldest first.
func (s *store) ListAuthEvents(ctx context.Context) ([]stores.AuthEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, at, actor_user_id, action, target_user_id, detail, ip, user_agent FROM auth_events ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []stores.AuthEvent
	for rows.Next() {
		e, err := scanAuthEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func scanAuthEvent(scan func(dest ...any) error) (stores.AuthEvent, error) {
	var (
		e         stores.AuthEvent
		at        int64
		actorID   nullable.Null[string]
		targetID  nullable.Null[string]
		detail    nullable.Null[string]
		ip        nullable.Null[string]
		userAgent nullable.Null[string]
	)
	if err := scan(&e.ID, &at, &actorID, &e.Action, &targetID, &detail, &ip, &userAgent); err != nil {
		return stores.AuthEvent{}, err
	}
	e.At = time.Unix(at, 0).UTC()
	e.ActorUserID = actorID.V
	e.TargetUserID = targetID.V
	e.Detail = detail.V
	e.IP = ip.V
	e.UserAgent = userAgent.V
	return e, nil
}
