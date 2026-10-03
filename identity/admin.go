package identity

import (
	"context"
	"errors"
	"time"

	"runbooks/stores"
)

// ErrLastAdmin means disabling the user would leave the instance with no enabled
// admin who can sign in.
var ErrLastAdmin = errors.New("identity: cannot disable the last enabled admin")

// DisableUser blocks a user from signing in and ends their sessions. It is
// idempotent for an already-disabled user.
func (s *Service) DisableUser(ctx context.Context, userID string) error {
	u, err := s.users.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return ErrUser
		}
		return err
	}
	if !u.Enabled() {
		return nil
	}
	if u.IsAdmin() {
		admins, err := s.enabledAdmins(ctx)
		if err != nil {
			return err
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	u.DisabledAt = s.now()
	if err := s.users.UpdateUser(ctx, u); err != nil {
		return err
	}
	return s.sessions.DeleteSessionsForUser(ctx, userID)
}

// EnableUser restores a disabled user's ability to sign in.
func (s *Service) EnableUser(ctx context.Context, userID string) error {
	u, err := s.users.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return ErrUser
		}
		return err
	}
	u.DisabledAt = time.Time{}
	return s.users.UpdateUser(ctx, u)
}

// enabledAdmins counts the instance's admins who can sign in (i.e. who are not
// disabled), so the last one cannot be locked out.
func (s *Service) enabledAdmins(ctx context.Context) (int, error) {
	users, err := s.users.ListUsers(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, u := range users {
		if u.IsAdmin() && u.Enabled() {
			n++
		}
	}
	return n, nil
}
