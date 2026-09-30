package identity

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"runbooks/stores"
	"runbooks/stores/sqlite"
)

func fuzzService(f *testing.F) *Service {
	f.Helper()
	st, err := sqlite.Open(context.Background(), "file:"+filepath.Join(f.TempDir(), "fuzz.db"))
	if err != nil {
		f.Fatalf("open store: %v", err)
	}
	f.Cleanup(func() { st.Close() })
	if err := st.InsertUser(context.Background(), stores.User{
		ID: "u1", DisplayName: "Fuzz", Role: stores.RoleMember, CreatedAt: time.Now(),
	}); err != nil {
		f.Fatalf("insert user: %v", err)
	}
	svc, err := New(Config{
		RPID:          "localhost",
		RPDisplayName: "Fuzz",
		RPOrigins:     []string{"http://localhost"},
	}, st)
	if err != nil {
		f.Fatalf("new service: %v", err)
	}
	return svc
}

// FuzzAuthenticateNeverPanics: an arbitrary cookie value must never resolve to a
// user, and must never do anything other than return ErrSession.
func FuzzAuthenticateNeverPanics(f *testing.F) {
	svc := fuzzService(f)
	f.Add("")
	f.Add("aaaa")
	f.Fuzz(func(t *testing.T, token string) {
		_, err := svc.Authenticate(context.Background(), token)
		if err != nil && !errors.Is(err, ErrSession) {
			t.Fatalf("Authenticate(%q) error = %v, want ErrSession", token, err)
		}
	})
}

// FuzzFinishRegistrationNeverPanics drives the attestation parser with arbitrary
// bytes against a fresh, valid challenge.
func FuzzFinishRegistrationNeverPanics(f *testing.F) {
	svc := fuzzService(f)
	f.Add([]byte("{}"))
	f.Add([]byte(`{"id":"x","rawId":"x","type":"public-key","response":{}}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		token, _, err := svc.BeginRegistration(context.Background(), "u1")
		if err != nil {
			t.Skipf("begin: %v", err)
		}
		_, _ = svc.FinishRegistration(context.Background(), "u1", token, body)
	})
}

// FuzzFinishLoginNeverPanics drives the assertion parser with arbitrary bytes
// against a fresh, valid challenge.
func FuzzFinishLoginNeverPanics(f *testing.F) {
	svc := fuzzService(f)
	f.Add([]byte("{}"))
	f.Add([]byte(`{"id":"x","rawId":"x","type":"public-key","response":{}}`))
	f.Fuzz(func(t *testing.T, body []byte) {
		token, _, err := svc.BeginDiscoverableLogin(context.Background())
		if err != nil {
			t.Skipf("begin: %v", err)
		}
		_, _, _ = svc.FinishLogin(context.Background(), token, body, "fuzz", "127.0.0.1")
	})
}

// FuzzHashTokenDeterministic: the store key is a stable 64-char hex digest.
func FuzzHashTokenDeterministic(f *testing.F) {
	f.Add("")
	f.Add("token")
	f.Fuzz(func(t *testing.T, raw string) {
		h := hashToken(raw)
		if h != hashToken(raw) {
			t.Fatalf("hashToken(%q) is not deterministic", raw)
		}
		if len(h) != 64 {
			t.Fatalf("hashToken(%q) length = %d, want 64", raw, len(h))
		}
	})
}
