package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"runbooks/stores"
)

func TestDisableUser(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	seedUser(t, st, "u1")

	// Disabling ends a live session.
	raw, err := svc.Create(ctx, "u1", "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.DisableUser(ctx, "u1"); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	u, err := st.GetUser(ctx, "u1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Enabled() {
		t.Error("user should be disabled")
	}
	if _, err := svc.Authenticate(ctx, raw); !errors.Is(err, ErrSession) {
		t.Errorf("session after disable = %v, want ErrSession", err)
	}

	// Idempotent for an already-disabled user.
	if err := svc.DisableUser(ctx, "u1"); err != nil {
		t.Errorf("DisableUser (already disabled) = %v, want nil", err)
	}

	// Re-enabling must not resurrect the session: it has to be deleted, not just
	// hidden behind the disabled flag.
	if err := svc.EnableUser(ctx, "u1"); err != nil {
		t.Fatalf("EnableUser: %v", err)
	}
	if _, err := svc.Authenticate(ctx, raw); !errors.Is(err, ErrSession) {
		t.Errorf("old session after re-enable = %v, want ErrSession (session must be deleted)", err)
	}

	// Unknown user.
	if err := svc.DisableUser(ctx, "nope"); !errors.Is(err, ErrUser) {
		t.Errorf("DisableUser(unknown) = %v, want ErrUser", err)
	}
}

func TestDisableLastAdmin(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	if err := st.InsertUser(ctx, stores.User{ID: "admin1", DisplayName: "Admin", Role: stores.RoleAdmin, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("insert admin: %v", err)
	}
	seedUser(t, st, "member1")

	// The only enabled admin cannot be disabled.
	if err := svc.DisableUser(ctx, "admin1"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("DisableUser(last admin) = %v, want ErrLastAdmin", err)
	}

	// A second enabled admin makes it allowed.
	if err := st.InsertUser(ctx, stores.User{ID: "admin2", DisplayName: "Admin Two", Role: stores.RoleAdmin, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("insert second admin: %v", err)
	}
	if err := svc.DisableUser(ctx, "admin1"); err != nil {
		t.Fatalf("DisableUser with a second admin = %v, want nil", err)
	}
}

func TestEnableUser(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	seedUser(t, st, "u1")

	if err := svc.DisableUser(ctx, "u1"); err != nil {
		t.Fatalf("DisableUser: %v", err)
	}
	if err := svc.EnableUser(ctx, "u1"); err != nil {
		t.Fatalf("EnableUser: %v", err)
	}
	u, err := st.GetUser(ctx, "u1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if !u.Enabled() {
		t.Error("user should be enabled")
	}

	if err := svc.EnableUser(ctx, "nope"); !errors.Is(err, ErrUser) {
		t.Errorf("EnableUser(unknown) = %v, want ErrUser", err)
	}
}
