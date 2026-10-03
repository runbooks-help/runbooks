package identity

import (
	"context"
	"errors"
	"testing"

	"runbooks/stores"
)

func TestCredentialRenameRemove(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	seedUser(t, st, "u1")
	first, _, _ := register(t, svc, "u1")
	second, _, _ := register(t, svc, "u1")

	if err := svc.RenameCredential(ctx, "u1", second.ID, "  Laptop  "); err != nil {
		t.Fatalf("RenameCredential: %v", err)
	}
	creds, err := svc.Credentials(ctx, "u1")
	if err != nil {
		t.Fatalf("Credentials: %v", err)
	}
	if got := labelOf(creds, second.ID); got != "Laptop" {
		t.Errorf("renamed label = %q, want %q", got, "Laptop")
	}

	if err := svc.RemoveCredential(ctx, "u1", first.ID); err != nil {
		t.Fatalf("RemoveCredential: %v", err)
	}
	if creds, err = svc.Credentials(ctx, "u1"); err != nil {
		t.Fatalf("Credentials after remove: %v", err)
	} else if len(creds) != 1 || creds[0].ID != second.ID {
		t.Errorf("after remove = %+v, want just %q", creds, second.ID)
	}

	if err := svc.RemoveCredential(ctx, "u1", second.ID); !errors.Is(err, ErrLastPasskey) {
		t.Errorf("removing the last passkey = %v, want ErrLastPasskey", err)
	}

	// Another user can neither rename nor remove a credential they do not own.
	seedUser(t, st, "u2")
	if err := svc.RenameCredential(ctx, "u2", second.ID, "x"); !errors.Is(err, ErrCredential) {
		t.Errorf("foreign rename = %v, want ErrCredential", err)
	}
	if err := svc.RemoveCredential(ctx, "u2", second.ID); !errors.Is(err, ErrCredential) {
		t.Errorf("foreign remove = %v, want ErrCredential", err)
	}
}

func TestRevokeSessionOwnership(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	seedUser(t, st, "u1")
	seedUser(t, st, "u2")

	raw, err := svc.Create(ctx, "u1", "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	id := SessionID(raw)

	if err := svc.RevokeSession(ctx, "u2", id); !errors.Is(err, ErrSession) {
		t.Errorf("foreign revoke = %v, want ErrSession", err)
	}
	if _, err := svc.Authenticate(ctx, raw); err != nil {
		t.Errorf("session should survive a foreign revoke: %v", err)
	}

	if err := svc.RevokeSession(ctx, "u1", id); err != nil {
		t.Fatalf("owner revoke: %v", err)
	}
	if _, err := svc.Authenticate(ctx, raw); !errors.Is(err, ErrSession) {
		t.Errorf("session after revoke = %v, want ErrSession", err)
	}
}

func TestUpdateProfile(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	seedUser(t, st, "u1")

	u, err := svc.UpdateProfile(ctx, "u1", "  New Name  ", " new@example.com ")
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if u.DisplayName != "New Name" || u.Email != "new@example.com" {
		t.Errorf("UpdateProfile = %q / %q, want trimmed values", u.DisplayName, u.Email)
	}

	if _, err := svc.UpdateProfile(ctx, "missing", "x", ""); !errors.Is(err, ErrUser) {
		t.Errorf("UpdateProfile(missing) = %v, want ErrUser", err)
	}
}

func labelOf(creds []stores.Credential, id string) string {
	for _, c := range creds {
		if c.ID == id {
			return c.Label
		}
	}
	return ""
}
