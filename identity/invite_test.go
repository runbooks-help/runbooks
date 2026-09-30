package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"runbooks/stores"
)

func TestInviteLifecycle(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }

	raw, err := svc.CreateInvite(ctx, "admin", stores.RoleMember, "", time.Hour)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	inv, err := svc.Invite(ctx, raw)
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if inv.Role != stores.RoleMember || inv.CreatedBy != "admin" || inv.UserID != "" {
		t.Errorf("invite = %+v", inv)
	}

	// The raw token is not the stored id.
	if inv.ID == raw {
		t.Error("invite stored the raw token; it must be hashed")
	}

	if err := svc.UseInvite(ctx, inv); err != nil {
		t.Fatalf("UseInvite: %v", err)
	}
	if _, err := svc.Invite(ctx, raw); !errors.Is(err, ErrInvite) {
		t.Errorf("used invite err = %v, want ErrInvite", err)
	}

	if _, err := svc.Invite(ctx, "not-a-token"); !errors.Is(err, ErrInvite) {
		t.Errorf("unknown invite err = %v, want ErrInvite", err)
	}

	// Expiry.
	expiring, err := svc.CreateInvite(ctx, "admin", stores.RoleAdmin, "u1", time.Minute)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	svc.now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := svc.Invite(ctx, expiring); !errors.Is(err, ErrInvite) {
		t.Errorf("expired invite err = %v, want ErrInvite", err)
	}

	// Re-enrolment binds an existing user.
	svc.now = func() time.Time { return base }
	re, err := svc.CreateInvite(ctx, "admin", stores.RoleMember, "u1", time.Hour)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	got, err := svc.Invite(ctx, re)
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if got.UserID != "u1" {
		t.Errorf("re-enrolment invite UserID = %q, want u1", got.UserID)
	}
}
