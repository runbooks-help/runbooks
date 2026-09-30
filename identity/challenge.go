package identity

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/go-webauthn/webauthn/webauthn"

	"runbooks/stores"
)

const (
	challengeRegistration = "registration"
	challengeLogin        = "login"
)

// storeChallenge persists the WebAuthn session data and returns the raw token
// the caller hands back on finish.
func (s *Service) storeChallenge(ctx context.Context, kind string, session *webauthn.SessionData) (string, error) {
	data, err := json.Marshal(session)
	if err != nil {
		return "", err
	}
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	// The library stamps session.Expires from the wall clock; use our own clock
	// so expiry is consistent (and testable) across the begin/finish pair.
	expires := s.now().Add(s.challengeTTL)
	if err := s.challenges.InsertChallenge(ctx, stores.Challenge{ID: hash, Kind: kind, Data: data, ExpiresAt: expires}); err != nil {
		return "", err
	}
	return raw, nil
}

// consumeChallenge loads and deletes a challenge. It is deleted whatever the
// outcome, so a challenge can never be replayed.
func (s *Service) consumeChallenge(ctx context.Context, kind, token string) (*webauthn.SessionData, error) {
	hash := hashToken(token)
	stored, err := s.challenges.GetChallenge(ctx, hash)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return nil, ErrChallenge
		}
		return nil, err
	}
	if err := s.challenges.DeleteChallenge(ctx, hash); err != nil {
		return nil, err
	}
	if stored.Kind != kind || !s.now().Before(stored.ExpiresAt) {
		return nil, ErrChallenge
	}
	var session webauthn.SessionData
	if err := json.Unmarshal(stored.Data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}
