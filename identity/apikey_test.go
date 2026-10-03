package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMintAPIKey(t *testing.T) {
	ctx := context.Background()
	svc, st := newTestService(t)
	seedUser(t, st, "u1")

	raw, key, err := svc.MintAPIKey(ctx, "u1", "on-call bot")
	if err != nil {
		t.Fatalf("MintAPIKey: %v", err)
	}
	if !strings.HasPrefix(raw, apiKeyPrefix) {
		t.Errorf("raw key = %q, want prefix %q", raw, apiKeyPrefix)
	}
	if key.ID == "" || key.ID == raw {
		t.Errorf("stored id must be a hash, not the raw key; id=%q raw=%q", key.ID, raw)
	}
	if key.ID != hashToken(raw) {
		t.Errorf("stored id = %q, want hashToken(raw)", key.ID)
	}
	if key.UserID != "u1" || key.CreatedBy != "u1" || key.Label != "on-call bot" {
		t.Errorf("unexpected key fields: %+v", key)
	}
	if key.Revoked() {
		t.Errorf("fresh key should not be revoked")
	}

	// The row is retrievable by the raw key's hash and lists for its owner.
	got, err := st.GetAPIKey(ctx, hashToken(raw))
	if err != nil {
		t.Fatalf("GetAPIKey: %v", err)
	}
	if got.ID != key.ID {
		t.Errorf("stored key id = %q, want %q", got.ID, key.ID)
	}
	if keys, err := st.ListAPIKeys(ctx, "u1"); err != nil {
		t.Fatalf("ListAPIKeys: %v", err)
	} else if len(keys) != 1 {
		t.Fatalf("ListAPIKeys len = %d, want 1", len(keys))
	}

	// Two mints never collide in the raw value or the stored hash.
	raw2, key2, err := svc.MintAPIKey(ctx, "u1", "second")
	if err != nil {
		t.Fatalf("MintAPIKey second: %v", err)
	}
	if raw2 == raw || key2.ID == key.ID {
		t.Errorf("second key collided: raw=%q/%q id=%q/%q", raw, raw2, key.ID, key2.ID)
	}
}

func TestAuthenticateAPIKey(t *testing.T) {
	ctx := context.Background()
	svc, st := newTestService(t)
	seedUser(t, st, "u1")

	raw, key, err := svc.MintAPIKey(ctx, "u1", "on-call bot")
	if err != nil {
		t.Fatalf("MintAPIKey: %v", err)
	}

	u, err := svc.AuthenticateAPIKey(ctx, raw)
	if err != nil {
		t.Fatalf("AuthenticateAPIKey: %v", err)
	}
	if u.ID != "u1" {
		t.Errorf("authenticated user = %q, want u1", u.ID)
	}
	if got, err := st.GetAPIKey(ctx, key.ID); err != nil {
		t.Fatalf("GetAPIKey: %v", err)
	} else if got.LastUsedAt.IsZero() {
		t.Errorf("last-used not touched on authentication")
	}

	if _, err := svc.AuthenticateAPIKey(ctx, apiKeyPrefix+"unknown"); !errors.Is(err, ErrAPIKey) {
		t.Errorf("unknown key err = %v, want ErrAPIKey", err)
	}

	// A revoked key is refused.
	key.RevokedAt = time.Now()
	if err := st.UpdateAPIKey(ctx, key); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := svc.AuthenticateAPIKey(ctx, raw); !errors.Is(err, ErrAPIKey) {
		t.Errorf("revoked key err = %v, want ErrAPIKey", err)
	}

	// A disabled owner's key is refused.
	raw2, _, err := svc.MintAPIKey(ctx, "u1", "second")
	if err != nil {
		t.Fatalf("MintAPIKey second: %v", err)
	}
	owner, err := st.GetUser(ctx, "u1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	owner.DisabledAt = time.Now()
	if err := st.UpdateUser(ctx, owner); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := svc.AuthenticateAPIKey(ctx, raw2); !errors.Is(err, ErrAPIKey) {
		t.Errorf("disabled-owner key err = %v, want ErrAPIKey", err)
	}
}
