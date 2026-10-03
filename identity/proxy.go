// SPDX-License-Identifier: FSL-1.1-MIT

package identity

import (
	"context"
	"errors"
	"strings"

	"runbooks/stores"
)

// ProvisionProxyUser resolves an identity asserted by the upstream proxy,
// creating the local user on first sight (just-in-time provisioning). There is
// no pre-seeding step: the proxy's assertion is the source of truth, so the
// users row is created lazily the first time an identity is seen and later
// requests resolve to it by email.
//
// A provisioned user is always a member. Admin is never granted from a header,
// so a proxied-but-unprivileged user cannot escalate by asserting an email. A
// disabled user is refused, like a disabled session.
func (s *Service) ProvisionProxyUser(ctx context.Context, email, displayName string) (stores.User, error) {
	email = strings.TrimSpace(email)
	if email == "" {
		return stores.User{}, ErrUser
	}

	user, err := s.users.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		if !user.Enabled() {
			return stores.User{}, ErrUser
		}
		return user, nil
	case !errors.Is(err, stores.ErrNotFound):
		return stores.User{}, err
	}

	name := strings.TrimSpace(displayName)
	if name == "" {
		name = email
	}
	id, err := newID()
	if err != nil {
		return stores.User{}, err
	}
	created := stores.User{
		ID:          id,
		Email:       email,
		DisplayName: name,
		Role:        stores.RoleMember,
		CreatedAt:   s.now(),
	}
	if err := s.users.InsertUser(ctx, created); err != nil {
		// A concurrent first request may have inserted the same email. Resolve
		// to that row rather than failing this one.
		var conflict *stores.ConflictError
		if errors.As(err, &conflict) {
			existing, gerr := s.users.GetUserByEmail(ctx, email)
			if gerr == nil {
				if !existing.Enabled() {
					return stores.User{}, ErrUser
				}
				return existing, nil
			}
		}
		return stores.User{}, err
	}
	return created, nil
}
