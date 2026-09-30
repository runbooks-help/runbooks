// Package storetest provides the contract every stores.Store implementation must
// satisfy. Each store package runs it against a clean database.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"runbooks/stores"
)

// StoreContract runs the shared behaviour. newStore returns a store backed by a
// clean database.
func StoreContract(t *testing.T, newStore func(t *testing.T) stores.Store) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	t.Run("users", func(t *testing.T) {
		s := newStore(t)

		u := stores.User{ID: "u1", Email: "ada@example.com", DisplayName: "Ada", Role: stores.RoleAdmin, CreatedAt: base}
		if err := s.InsertUser(ctx, u); err != nil {
			t.Fatalf("InsertUser: %v", err)
		}

		got, err := s.GetUser(ctx, "u1")
		if err != nil {
			t.Fatalf("GetUser: %v", err)
		}
		if diff := cmp.Diff(u, got); diff != "" {
			t.Errorf("GetUser mismatch (-want +got):\n%s", diff)
		}
		if !got.Enabled() || !got.IsAdmin() {
			t.Errorf("fresh user should be enabled admin, got enabled=%v admin=%v", got.Enabled(), got.IsAdmin())
		}

		byEmail, err := s.GetUserByEmail(ctx, "ada@example.com")
		if err != nil {
			t.Fatalf("GetUserByEmail: %v", err)
		}
		if byEmail.ID != "u1" {
			t.Errorf("GetUserByEmail id = %q, want u1", byEmail.ID)
		}

		users, err := s.ListUsers(ctx)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}
		if len(users) != 1 {
			t.Fatalf("ListUsers len = %d, want 1", len(users))
		}

		u.Role = stores.RoleMember
		u.DisabledAt = base.Add(time.Hour)
		if err := s.UpdateUser(ctx, u); err != nil {
			t.Fatalf("UpdateUser: %v", err)
		}
		got, err = s.GetUser(ctx, "u1")
		if err != nil {
			t.Fatalf("GetUser after update: %v", err)
		}
		if diff := cmp.Diff(u, got); diff != "" {
			t.Errorf("GetUser after update mismatch (-want +got):\n%s", diff)
		}

		if _, err := s.GetUser(ctx, "missing"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetUser missing err = %v, want ErrNotFound", err)
		}
		if _, err := s.GetUserByEmail(ctx, "missing@example.com"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetUserByEmail missing err = %v, want ErrNotFound", err)
		}
		if _, err := s.GetUserByEmail(ctx, ""); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetUserByEmail empty err = %v, want ErrNotFound", err)
		}
		if err := s.UpdateUser(ctx, stores.User{ID: "missing", Role: stores.RoleMember, CreatedAt: base}); err != nil {
			t.Errorf("UpdateUser missing err = %v, want nil (idempotent)", err)
		}

		dup := stores.User{ID: "u2", Email: "ada@example.com", DisplayName: "Impostor", Role: stores.RoleMember, CreatedAt: base}
		if err := s.InsertUser(ctx, dup); !isConflict(err) {
			t.Errorf("InsertUser duplicate email err = %v, want ConflictError", err)
		}

		// Email is optional: several users may have none.
		for _, id := range []string{"u3", "u4"} {
			if err := s.InsertUser(ctx, stores.User{ID: id, DisplayName: id, Role: stores.RoleMember, CreatedAt: base}); err != nil {
				t.Fatalf("InsertUser %s without email: %v", id, err)
			}
		}
	})

	t.Run("credentials", func(t *testing.T) {
		s := newStore(t)
		credID := []byte{1, 2, 3, 4}

		c := stores.Credential{
			ID:           "c1",
			UserID:       "u1",
			CredentialID: credID,
			PublicKey:    []byte{9, 9, 9},
			Transports:   []string{"usb", "nfc"},
			AAGUID:       []byte{7, 7},
			Label:        "YubiKey 5",
			CreatedAt:    base,
		}
		if err := s.InsertCredential(ctx, c); err != nil {
			t.Fatalf("InsertCredential: %v", err)
		}

		got, err := s.GetCredential(ctx, credID)
		if err != nil {
			t.Fatalf("GetCredential: %v", err)
		}
		if diff := cmp.Diff(c, got); diff != "" {
			t.Errorf("GetCredential mismatch (-want +got):\n%s", diff)
		}

		creds, err := s.ListCredentials(ctx, "u1")
		if err != nil {
			t.Fatalf("ListCredentials: %v", err)
		}
		if len(creds) != 1 {
			t.Fatalf("ListCredentials len = %d, want 1", len(creds))
		}
		if other, err := s.ListCredentials(ctx, "nobody"); err != nil || len(other) != 0 {
			t.Errorf("ListCredentials unknown user = %v, %v; want empty, nil", other, err)
		}

		c.SignCount = 5
		c.LastUsedAt = base.Add(2 * time.Hour)
		if err := s.UpdateCredential(ctx, c); err != nil {
			t.Fatalf("UpdateCredential: %v", err)
		}
		got, err = s.GetCredential(ctx, credID)
		if err != nil {
			t.Fatalf("GetCredential after update: %v", err)
		}
		if diff := cmp.Diff(c, got); diff != "" {
			t.Errorf("GetCredential after update mismatch (-want +got):\n%s", diff)
		}

		dup := stores.Credential{ID: "c2", UserID: "u1", CredentialID: credID, PublicKey: []byte{1}, CreatedAt: base}
		if err := s.InsertCredential(ctx, dup); !isConflict(err) {
			t.Errorf("InsertCredential duplicate err = %v, want ConflictError", err)
		}

		if err := s.DeleteCredential(ctx, "c1"); err != nil {
			t.Fatalf("DeleteCredential: %v", err)
		}
		if _, err := s.GetCredential(ctx, credID); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetCredential after delete err = %v, want ErrNotFound", err)
		}
		if err := s.DeleteCredential(ctx, "c1"); err != nil {
			t.Errorf("DeleteCredential twice err = %v, want nil (idempotent)", err)
		}
		if err := s.UpdateCredential(ctx, stores.Credential{ID: "c1", CreatedAt: base}); err != nil {
			t.Errorf("UpdateCredential missing err = %v, want nil (idempotent)", err)
		}
	})
}

func isConflict(err error) bool {
	var ce *stores.ConflictError
	return errors.As(err, &ce)
}
