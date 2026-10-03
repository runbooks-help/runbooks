// SPDX-License-Identifier: FSL-1.1-MIT

package identity

import (
	"context"
	"errors"

	"runbooks/stores"
)

// Create starts a session for a user and returns the raw cookie value. Only its
// hash is stored.
func (s *Service) Create(ctx context.Context, userID, userAgent, ip string) (string, error) {
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	sess := stores.Session{
		ID:         hash,
		UserID:     userID,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.sessionTTL),
		LastSeenAt: now,
		UserAgent:  userAgent,
		IP:         ip,
	}
	if err := s.sessions.InsertSession(ctx, sess); err != nil {
		return "", err
	}
	return raw, nil
}

// Authenticate resolves a raw cookie value to a user, refreshing last-seen. An
// absolute- or idle-expired session is deleted and reported as ErrSession.
func (s *Service) Authenticate(ctx context.Context, raw string) (stores.User, error) {
	hash := hashToken(raw)
	sess, err := s.sessions.GetSession(ctx, hash)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return stores.User{}, ErrSession
		}
		return stores.User{}, err
	}

	now := s.now()
	if !now.Before(sess.ExpiresAt) || now.Sub(sess.LastSeenAt) > s.sessionIdleTTL {
		_ = s.sessions.DeleteSession(ctx, hash)
		return stores.User{}, ErrSession
	}
	sess.LastSeenAt = now
	if err := s.sessions.UpdateSession(ctx, sess); err != nil {
		return stores.User{}, err
	}

	user, err := s.users.GetUser(ctx, sess.UserID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return stores.User{}, ErrSession
		}
		return stores.User{}, err
	}
	if !user.Enabled() {
		return stores.User{}, ErrSession
	}
	return user, nil
}

// Revoke ends one session. An unknown or already-ended session is not an error.
func (s *Service) Revoke(ctx context.Context, raw string) error {
	return s.sessions.DeleteSession(ctx, hashToken(raw))
}

// RevokeForUser ends every session for a user (sign out everywhere, or an admin
// revoke).
func (s *Service) RevokeForUser(ctx context.Context, userID string) error {
	return s.sessions.DeleteSessionsForUser(ctx, userID)
}
