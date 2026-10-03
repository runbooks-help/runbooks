// SPDX-License-Identifier: FSL-1.1-MIT

package identity

import (
	"context"
	"errors"
	"time"

	"runbooks/stores"
)

// CreateInvite mints a single-use enrolment invite and returns the raw token for
// the link. userID is empty for a new-user invite, and set to bind a re-enrolment
// to an existing user.
func (s *Service) CreateInvite(ctx context.Context, createdBy string, role stores.Role, userID string, ttl time.Duration) (string, error) {
	raw, hash, err := newToken()
	if err != nil {
		return "", err
	}
	now := s.now()
	inv := stores.Invite{
		ID:        hash,
		UserID:    userID,
		Role:      role,
		CreatedBy: createdBy,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.invites.InsertInvite(ctx, inv); err != nil {
		return "", err
	}
	return raw, nil
}

// Invite returns a usable invite for the raw token, or ErrInvite when it is
// missing, already used or expired.
func (s *Service) Invite(ctx context.Context, raw string) (stores.Invite, error) {
	inv, err := s.invites.GetInvite(ctx, hashToken(raw))
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return stores.Invite{}, ErrInvite
		}
		return stores.Invite{}, err
	}
	if inv.Used() || inv.Expired(s.now()) {
		return stores.Invite{}, ErrInvite
	}
	return inv, nil
}

// UseInvite marks an invite consumed.
func (s *Service) UseInvite(ctx context.Context, inv stores.Invite) error {
	inv.UsedAt = s.now()
	return s.invites.UpdateInvite(ctx, inv)
}
