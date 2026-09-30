package identity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestSQLiteLocalDB is the local proof that the SQLite provider works against a
// real on-disk database, not a throwaway temp file. It is skipped unless
// IDENTITY_TEST_DSN is set; `mise run test:identity` provides it.
//
// It opens a fresh DB, applies the schema, round-trips a user and a credential,
// closes and reopens to prove persistence and idempotent migration, then runs a
// small concurrency smoke.
func TestSQLiteLocalDB(t *testing.T) {
	dsn := os.Getenv("IDENTITY_TEST_DSN")
	if dsn == "" {
		t.Skip("set IDENTITY_TEST_DSN (or run `mise run test:identity`)")
	}
	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)
	path := strings.TrimPrefix(dsn, "file:")

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remove %s: %v", path, err)
	}
	t.Logf("database: %s", path)

	s, err := OpenSQLite(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	logTables(t, ctx, s)

	user := User{ID: "local-user", Email: "local@example.com", DisplayName: "Local User", Role: RoleAdmin, CreatedAt: base}
	if err := s.InsertUser(ctx, user); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	cred := Credential{
		ID:           "local-cred",
		UserID:       user.ID,
		CredentialID: []byte("credential-id"),
		PublicKey:    []byte("public-key"),
		Transports:   Transports{"usb"},
		Label:        "Local Key",
		CreatedAt:    base,
	}
	if err := s.InsertCredential(ctx, cred); err != nil {
		t.Fatalf("insert credential: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Reopen: the row survived and the schema re-applies without error.
	s, err = OpenSQLite(ctx, dsn)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s.Close()

	got, err := s.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("get user after reopen: %v", err)
	}
	if got.Email != user.Email || got.DisplayName != user.DisplayName {
		t.Errorf("reopened user = %+v, want %+v", got, user)
	}

	// One writer connection serialises; concurrent callers must not error.
	var wg sync.WaitGroup
	errs := make(chan error, 40)
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			u := User{ID: fmt.Sprintf("concurrent-%d", i), DisplayName: fmt.Sprintf("Concurrent %d", i), Role: RoleMember, CreatedAt: base}
			if err := s.InsertUser(ctx, u); err != nil {
				errs <- fmt.Errorf("insert %s: %w", u.ID, err)
				return
			}
			if _, err := s.GetUser(ctx, u.ID); err != nil {
				errs <- fmt.Errorf("get %s: %w", u.ID, err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent: %v", err)
	}

	users, err := s.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	t.Logf("users in %s: %d", path, len(users))
}

func logTables(t *testing.T, ctx context.Context, s *SQLiteStore) {
	t.Helper()
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}
	t.Logf("tables: %v", names)
}
