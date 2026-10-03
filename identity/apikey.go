package identity

import (
	"context"
	"errors"
	"strings"

	"runbooks/stores"
)

// apiKeyPrefix marks a raw agent key so it is recognisable to a human and to
// secret scanners. It is part of the raw value, and therefore part of its hash.
const apiKeyPrefix = "rbk_"

// ErrAPIKey is a raw key that is unknown, revoked, or whose owner is disabled.
var ErrAPIKey = errors.New("identity: api key missing, revoked or disabled")

// MintAPIKey creates a read-scoped API key owned by userID and returns the raw
// key. Only the SHA-256 hash is stored, so the raw value is shown once and can
// never be recovered. The key is read-only by construction: the gate that
// consumes it wraps only the read machine endpoints, never a write.
func (s *Service) MintAPIKey(ctx context.Context, userID, label string) (string, stores.APIKey, error) {
	raw, _, err := newToken()
	if err != nil {
		return "", stores.APIKey{}, err
	}
	// Re-hash the prefixed value: the prefix is part of the key presented on the
	// wire, so it must be part of the hash looked up at authentication.
	raw = apiKeyPrefix + raw
	key := stores.APIKey{
		ID:        hashToken(raw),
		UserID:    userID,
		Label:     strings.TrimSpace(label),
		CreatedBy: userID,
		CreatedAt: s.now(),
	}
	if err := s.apiKeys.InsertAPIKey(ctx, key); err != nil {
		return "", stores.APIKey{}, err
	}
	return raw, key, nil
}

// AuthenticateAPIKey resolves a raw key to its owning user and touches the key's
// last-used time. A revoked key, an unknown user, or a disabled owner is
// ErrAPIKey. The lookup is by the key's hash, an exact primary-key match, so no
// constant-time comparison is needed.
func (s *Service) AuthenticateAPIKey(ctx context.Context, raw string) (stores.User, error) {
	key, err := s.apiKeys.GetAPIKey(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return stores.User{}, ErrAPIKey
		}
		return stores.User{}, err
	}
	if key.Revoked() {
		return stores.User{}, ErrAPIKey
	}
	u, err := s.users.GetUser(ctx, key.UserID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return stores.User{}, ErrAPIKey
		}
		return stores.User{}, err
	}
	if !u.Enabled() {
		return stores.User{}, ErrAPIKey
	}
	key.LastUsedAt = s.now()
	if err := s.apiKeys.UpdateAPIKey(ctx, key); err != nil {
		return stores.User{}, err
	}
	return u, nil
}
