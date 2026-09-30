package postgres

import (
	"context"
	"os"
	"testing"

	"runbooks/stores"
	"runbooks/stores/storetest"
)

// tables are dropped between contract subtests so each starts clean.
var tables = []string{"auth_events", "webauthn_challenges", "invites", "sessions", "credentials", "users"}

func newStore(t *testing.T) stores.Store {
	t.Helper()
	dsn := os.Getenv("STORES_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set STORES_POSTGRES_DSN (or run `mise run test:postgres`)")
	}
	s, err := Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	reset(t, s)
	return s
}

// reset drops every identity table and re-applies the schema.
func reset(t *testing.T, s stores.Store) {
	t.Helper()
	raw := s.(*store)
	ctx := context.Background()
	for _, table := range tables {
		if _, err := raw.db.ExecContext(ctx, "DROP TABLE IF EXISTS "+table+" CASCADE"); err != nil {
			t.Fatalf("drop %s: %v", table, err)
		}
	}
	if err := raw.migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestStore(t *testing.T) {
	storetest.StoreContract(t, newStore)
}

// TestMigrationAddsCredentialFlags covers upgrading a database created before
// the credentials.flags column existed.
func TestMigrationAddsCredentialFlags(t *testing.T) {
	dsn := os.Getenv("STORES_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set STORES_POSTGRES_DSN (or run `mise run test:postgres`)")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	reset(t, s)
	raw := s.(*store)

	if _, err := raw.db.ExecContext(ctx, `ALTER TABLE credentials DROP COLUMN flags`); err != nil {
		t.Fatalf("drop flags: %v", err)
	}
	if hasCredentialFlags(t, raw) {
		t.Fatal("flags column still present after drop")
	}
	if err := raw.migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !hasCredentialFlags(t, raw) {
		t.Error("migrate did not re-add the flags column")
	}
}

func hasCredentialFlags(t *testing.T, s *store) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'credentials' AND column_name = 'flags'`).Scan(&n); err != nil {
		t.Fatalf("information_schema: %v", err)
	}
	return n > 0
}

func TestMigrationIdempotent(t *testing.T) {
	dsn := os.Getenv("STORES_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set STORES_POSTGRES_DSN (or run `mise run test:postgres`)")
	}
	ctx := context.Background()

	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	reset(t, s)
	raw := s.(*store)

	// Re-applying the schema must not error (all IF NOT EXISTS).
	if err := raw.migrate(ctx); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}

	for _, table := range tables {
		var name string
		err := raw.db.QueryRowContext(ctx,
			`SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}
