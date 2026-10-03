// SPDX-License-Identifier: FSL-1.1-MIT

package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"runbooks/stores"
	"runbooks/stores/storetest"
)

func newStore(t *testing.T) stores.Store {
	t.Helper()
	s, err := Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "stores.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStore(t *testing.T) {
	storetest.StoreContract(t, newStore)
}

func TestOpenCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deeper")
	s, err := Open(context.Background(), "file:"+filepath.Join(dir, "runbooks.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("directory not created: %v", err)
	}
}

func TestFileDir(t *testing.T) {
	cases := map[string]string{
		"file:./data/runbooks.db":              "data",
		"file:./data/runbooks.db?cache=shared": "data",
		"file:/tmp/runbooks.db":                "/tmp",
		"file:runbooks.db":                     "",
		"file::memory:":                        "",
		"runbooks.db":                          "",
	}
	for dsn, want := range cases {
		if got := fileDir(dsn); got != want {
			t.Errorf("fileDir(%q) = %q, want %q", dsn, got, want)
		}
	}
}

// TestMigrationAddsCredentialFlags covers upgrading a database created before
// the credentials.flags column existed.
func TestMigrationAddsCredentialFlags(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, "file:"+filepath.Join(t.TempDir(), "stores.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
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
		`SELECT COUNT(*) FROM pragma_table_info('credentials') WHERE name = 'flags'`).Scan(&n); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	return n > 0
}

// TestMigrationAddsAuthEventDetail covers upgrading a database created before
// the auth_events.detail column existed — without the migration the admin audit
// page 500s because ListAuthEvents names the column.
func TestMigrationAddsAuthEventDetail(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, "file:"+filepath.Join(t.TempDir(), "stores.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	raw := s.(*store)

	if _, err := raw.db.ExecContext(ctx, `ALTER TABLE auth_events DROP COLUMN detail`); err != nil {
		t.Fatalf("drop detail: %v", err)
	}
	if err := raw.migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !hasAuthEventDetail(t, raw) {
		t.Fatal("migrate did not re-add the detail column")
	}
	if _, err := s.ListAuthEvents(ctx); err != nil {
		t.Fatalf("ListAuthEvents after migrate: %v", err)
	}
}

func hasAuthEventDetail(t *testing.T, s *store) bool {
	t.Helper()
	var n int
	if err := s.db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM pragma_table_info('auth_events') WHERE name = 'detail'`).Scan(&n); err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	return n > 0
}

func TestMigrationIdempotent(t *testing.T) {
	ctx := context.Background()
	dsn := "file:" + filepath.Join(t.TempDir(), "stores.db")

	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Re-opening must re-apply the schema without error (all IF NOT EXISTS).
	s, err = Open(ctx, dsn)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer s.Close()

	raw := s.(*store)
	for _, table := range []string{"users", "credentials", "sessions", "invites", "webauthn_challenges", "auth_events"} {
		var name string
		err := raw.db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Errorf("table %q missing: %v", table, err)
		}
	}
}
