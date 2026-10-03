// SPDX-License-Identifier: FSL-1.1-MIT

package identity

import (
	"context"
	"errors"
	"strings"

	"runbooks/stores"
)

// ErrCredential means a passkey does not exist or does not belong to the user.
var ErrCredential = errors.New("identity: unknown passkey")

// ErrLastPasskey means removing the passkey would leave the user with none, so
// they could no longer sign in.
var ErrLastPasskey = errors.New("identity: cannot remove the last passkey")

// SessionID maps a raw session cookie value to its stored (hashed) id, so a
// caller can pick the current session out of a list.
func SessionID(raw string) string { return hashToken(raw) }

// Credentials lists a user's passkeys.
func (s *Service) Credentials(ctx context.Context, userID string) ([]stores.Credential, error) {
	return s.creds.ListCredentials(ctx, userID)
}

// RenameCredential sets a passkey's label. The passkey must belong to userID.
func (s *Service) RenameCredential(ctx context.Context, userID, credentialID, label string) error {
	creds, err := s.creds.ListCredentials(ctx, userID)
	if err != nil {
		return err
	}
	for _, c := range creds {
		if c.ID != credentialID {
			continue
		}
		c.Label = strings.TrimSpace(label)
		return s.creds.UpdateCredential(ctx, c)
	}
	return ErrCredential
}

// RemoveCredential deletes one of a user's passkeys. Removing the last one is
// refused, so a user cannot lock themselves out.
func (s *Service) RemoveCredential(ctx context.Context, userID, credentialID string) error {
	creds, err := s.creds.ListCredentials(ctx, userID)
	if err != nil {
		return err
	}
	for _, c := range creds {
		if c.ID != credentialID {
			continue
		}
		if len(creds) == 1 {
			return ErrLastPasskey
		}
		return s.creds.DeleteCredential(ctx, credentialID)
	}
	return ErrCredential
}

// Sessions lists a user's active sessions, most recently seen first.
func (s *Service) Sessions(ctx context.Context, userID string) ([]stores.Session, error) {
	return s.sessions.ListSessions(ctx, userID)
}

// RevokeSession ends one of a user's sessions by its stored id. A session that
// belongs to someone else is reported as ErrSession, never deleted.
func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	sess, err := s.sessions.GetSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return ErrSession
		}
		return err
	}
	if sess.UserID != userID {
		return ErrSession
	}
	return s.sessions.DeleteSession(ctx, sessionID)
}

// UpdateProfile changes a user's display name and optional email, returning the
// updated record.
func (s *Service) UpdateProfile(ctx context.Context, userID, displayName, email string) (stores.User, error) {
	u, err := s.users.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return stores.User{}, ErrUser
		}
		return stores.User{}, err
	}
	u.DisplayName = strings.TrimSpace(displayName)
	u.Email = strings.TrimSpace(email)
	if err := s.users.UpdateUser(ctx, u); err != nil {
		return stores.User{}, err
	}
	return u, nil
}
