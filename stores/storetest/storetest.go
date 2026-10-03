// SPDX-License-Identifier: FSL-1.1-MIT

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
			Flags:        0x1d, // UP|UV|BE|BS
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
		c.Flags = 0x1c // UP|BE|BS: user verification flag dropped and persisted
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

	t.Run("sessions", func(t *testing.T) {
		s := newStore(t)

		sess := stores.Session{
			ID:         "s1",
			UserID:     "u1",
			CreatedAt:  base,
			ExpiresAt:  base.Add(time.Hour),
			LastSeenAt: base,
			UserAgent:  "curl/8",
			IP:         "127.0.0.1",
		}
		if err := s.InsertSession(ctx, sess); err != nil {
			t.Fatalf("InsertSession: %v", err)
		}
		got, err := s.GetSession(ctx, "s1")
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if diff := cmp.Diff(sess, got); diff != "" {
			t.Errorf("GetSession mismatch (-want +got):\n%s", diff)
		}

		if _, err := s.GetSession(ctx, "missing"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetSession missing err = %v, want ErrNotFound", err)
		}

		sess.LastSeenAt = base.Add(10 * time.Minute)
		sess.ExpiresAt = base.Add(2 * time.Hour)
		if err := s.UpdateSession(ctx, sess); err != nil {
			t.Fatalf("UpdateSession: %v", err)
		}
		got, err = s.GetSession(ctx, "s1")
		if err != nil {
			t.Fatalf("GetSession after update: %v", err)
		}
		if diff := cmp.Diff(sess, got); diff != "" {
			t.Errorf("GetSession after update mismatch (-want +got):\n%s", diff)
		}

		// Sign out everywhere: only the target user's sessions go (s1 already exists).
		if err := s.InsertSession(ctx, stores.Session{ID: "s2", UserID: "u1", CreatedAt: base, ExpiresAt: base.Add(time.Hour), LastSeenAt: base}); err != nil {
			t.Fatalf("InsertSession s2: %v", err)
		}
		if err := s.InsertSession(ctx, stores.Session{ID: "s3", UserID: "u2", CreatedAt: base, ExpiresAt: base.Add(time.Hour), LastSeenAt: base}); err != nil {
			t.Fatalf("InsertSession s3: %v", err)
		}

		// ListSessions is per user and most-recently-seen first; empty is OK.
		list, err := s.ListSessions(ctx, "u1")
		if err != nil {
			t.Fatalf("ListSessions: %v", err)
		}
		if len(list) != 2 || list[0].ID != "s1" || list[1].ID != "s2" {
			t.Errorf("ListSessions(u1) = %+v, want s1 then s2", list)
		}
		if list, err = s.ListSessions(ctx, "u2"); err != nil {
			t.Fatalf("ListSessions(u2): %v", err)
		} else if len(list) != 1 || list[0].ID != "s3" {
			t.Errorf("ListSessions(u2) = %+v, want just s3", list)
		}
		if list, err = s.ListSessions(ctx, "nobody"); err != nil {
			t.Fatalf("ListSessions(nobody): %v", err)
		} else if len(list) != 0 {
			t.Errorf("ListSessions(nobody) = %+v, want none", list)
		}

		if err := s.DeleteSessionsForUser(ctx, "u1"); err != nil {
			t.Fatalf("DeleteSessionsForUser: %v", err)
		}
		if _, err := s.GetSession(ctx, "s1"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("s1 after sign-out err = %v, want ErrNotFound", err)
		}
		if _, err := s.GetSession(ctx, "s3"); err != nil {
			t.Errorf("s3 should survive sign-out, got %v", err)
		}

		if err := s.DeleteSession(ctx, "s3"); err != nil {
			t.Fatalf("DeleteSession: %v", err)
		}
		if _, err := s.GetSession(ctx, "s3"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetSession after delete err = %v, want ErrNotFound", err)
		}
		if err := s.DeleteSession(ctx, "s3"); err != nil {
			t.Errorf("DeleteSession twice err = %v, want nil (idempotent)", err)
		}
	})

	t.Run("challenges", func(t *testing.T) {
		s := newStore(t)

		ch := stores.Challenge{ID: "ch1", Kind: "registration", Data: []byte{1, 2, 3, 4}, ExpiresAt: base.Add(5 * time.Minute)}
		if err := s.InsertChallenge(ctx, ch); err != nil {
			t.Fatalf("InsertChallenge: %v", err)
		}
		got, err := s.GetChallenge(ctx, "ch1")
		if err != nil {
			t.Fatalf("GetChallenge: %v", err)
		}
		if diff := cmp.Diff(ch, got); diff != "" {
			t.Errorf("GetChallenge mismatch (-want +got):\n%s", diff)
		}

		if _, err := s.GetChallenge(ctx, "missing"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetChallenge missing err = %v, want ErrNotFound", err)
		}

		if err := s.DeleteChallenge(ctx, "ch1"); err != nil {
			t.Fatalf("DeleteChallenge: %v", err)
		}
		if _, err := s.GetChallenge(ctx, "ch1"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetChallenge after delete err = %v, want ErrNotFound", err)
		}
		if err := s.DeleteChallenge(ctx, "ch1"); err != nil {
			t.Errorf("DeleteChallenge twice err = %v, want nil (idempotent)", err)
		}
	})

	t.Run("invites", func(t *testing.T) {
		s := newStore(t)

		inv := stores.Invite{ID: "i1", Role: stores.RoleMember, CreatedBy: "u1", CreatedAt: base, ExpiresAt: base.Add(time.Hour)}
		if err := s.InsertInvite(ctx, inv); err != nil {
			t.Fatalf("InsertInvite: %v", err)
		}
		got, err := s.GetInvite(ctx, "i1")
		if err != nil {
			t.Fatalf("GetInvite: %v", err)
		}
		if diff := cmp.Diff(inv, got); diff != "" {
			t.Errorf("GetInvite mismatch (-want +got):\n%s", diff)
		}
		if got.Used() || got.Expired(base) {
			t.Errorf("fresh invite used/expired: %+v", got)
		}

		// A bound (re-enrolment) invite keeps its user through insert.
		re := stores.Invite{ID: "i2", UserID: "u1", Role: stores.RoleMember, CreatedBy: "u1", CreatedAt: base, ExpiresAt: base.Add(time.Hour)}
		if err := s.InsertInvite(ctx, re); err != nil {
			t.Fatalf("InsertInvite bound: %v", err)
		}
		if got, err := s.GetInvite(ctx, "i2"); err != nil {
			t.Fatalf("GetInvite bound: %v", err)
		} else if diff := cmp.Diff(re, got); diff != "" {
			t.Errorf("GetInvite bound mismatch (-want +got):\n%s", diff)
		}

		if _, err := s.GetInvite(ctx, "missing"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetInvite missing err = %v, want ErrNotFound", err)
		}

		// Update binds a user (re-enrolment) and marks the invite used.
		inv.UserID = "u1"
		inv.UsedAt = base.Add(5 * time.Minute)
		if err := s.UpdateInvite(ctx, inv); err != nil {
			t.Fatalf("UpdateInvite: %v", err)
		}
		got, err = s.GetInvite(ctx, "i1")
		if err != nil {
			t.Fatalf("GetInvite after update: %v", err)
		}
		if diff := cmp.Diff(inv, got); diff != "" {
			t.Errorf("GetInvite after update mismatch (-want +got):\n%s", diff)
		}
		if err := s.UpdateInvite(ctx, stores.Invite{ID: "missing"}); err != nil {
			t.Errorf("UpdateInvite missing err = %v, want nil (idempotent)", err)
		}
	})

	t.Run("api keys", func(t *testing.T) {
		s := newStore(t)

		k := stores.APIKey{ID: "k1", UserID: "u1", Label: "on-call bot", CreatedBy: "u1", CreatedAt: base}
		if err := s.InsertAPIKey(ctx, k); err != nil {
			t.Fatalf("InsertAPIKey: %v", err)
		}
		got, err := s.GetAPIKey(ctx, "k1")
		if err != nil {
			t.Fatalf("GetAPIKey: %v", err)
		}
		if diff := cmp.Diff(k, got); diff != "" {
			t.Errorf("GetAPIKey mismatch (-want +got):\n%s", diff)
		}
		if got.Revoked() {
			t.Errorf("fresh key revoked: %+v", got)
		}

		// Listing is per user; another user sees nothing.
		if keys, err := s.ListAPIKeys(ctx, "u1"); err != nil {
			t.Fatalf("ListAPIKeys: %v", err)
		} else if len(keys) != 1 {
			t.Fatalf("ListAPIKeys len = %d, want 1", len(keys))
		}
		if keys, err := s.ListAPIKeys(ctx, "u2"); err != nil {
			t.Fatalf("ListAPIKeys other user: %v", err)
		} else if len(keys) != 0 {
			t.Errorf("ListAPIKeys other user len = %d, want 0", len(keys))
		}

		if _, err := s.GetAPIKey(ctx, "missing"); !errors.Is(err, stores.ErrNotFound) {
			t.Errorf("GetAPIKey missing err = %v, want ErrNotFound", err)
		}

		// Update touches last-used and revokes.
		k.LastUsedAt = base.Add(5 * time.Minute)
		k.RevokedAt = base.Add(6 * time.Minute)
		if err := s.UpdateAPIKey(ctx, k); err != nil {
			t.Fatalf("UpdateAPIKey: %v", err)
		}
		got, err = s.GetAPIKey(ctx, "k1")
		if err != nil {
			t.Fatalf("GetAPIKey after update: %v", err)
		}
		if diff := cmp.Diff(k, got); diff != "" {
			t.Errorf("GetAPIKey after update mismatch (-want +got):\n%s", diff)
		}
		if !got.Revoked() {
			t.Errorf("key should be revoked after update: %+v", got)
		}
		if err := s.UpdateAPIKey(ctx, stores.APIKey{ID: "missing"}); err != nil {
			t.Errorf("UpdateAPIKey missing err = %v, want nil (idempotent)", err)
		}
	})

	t.Run("auth events", func(t *testing.T) {
		s := newStore(t)

		if empty, err := s.ListAuthEvents(ctx); err != nil {
			t.Fatalf("ListAuthEvents empty: %v", err)
		} else if len(empty) != 0 {
			t.Errorf("ListAuthEvents empty len = %d, want 0", len(empty))
		}

		full := stores.AuthEvent{
			At:           base,
			ActorUserID:  "u1",
			Action:       stores.ActionAck,
			TargetUserID: "u1",
			Detail:       "mts-deadlock-recovery",
			IP:           "203.0.113.7",
			UserAgent:    "Mozilla/5.0",
		}
		if err := s.InsertAuthEvent(ctx, full); err != nil {
			t.Fatalf("InsertAuthEvent: %v", err)
		}
		sparse := stores.AuthEvent{At: base.Add(time.Minute), Action: stores.ActionLogout}
		if err := s.InsertAuthEvent(ctx, sparse); err != nil {
			t.Fatalf("InsertAuthEvent sparse: %v", err)
		}

		events, err := s.ListAuthEvents(ctx)
		if err != nil {
			t.Fatalf("ListAuthEvents: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("ListAuthEvents len = %d, want 2", len(events))
		}
		// The store assigns ids on insert; compare everything else, oldest first.
		full.ID = events[0].ID
		sparse.ID = events[1].ID
		if diff := cmp.Diff(full, events[0]); diff != "" {
			t.Errorf("first event mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(sparse, events[1]); diff != "" {
			t.Errorf("second event mismatch (-want +got):\n%s", diff)
		}
	})
}

func isConflict(err error) bool {
	var ce *stores.ConflictError
	return errors.As(err, &ce)
}
