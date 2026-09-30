package sqlite

import (
	"context"
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
