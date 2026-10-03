// SPDX-License-Identifier: FSL-1.1-MIT

package identity

import (
	"context"
	"errors"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"runbooks/stores"
)

// waUser adapts a stores.User and its credentials to webauthn.User.
type waUser struct {
	user  stores.User
	creds []stores.Credential
}

func (u waUser) WebAuthnID() []byte          { return []byte(u.user.ID) }
func (u waUser) WebAuthnDisplayName() string { return u.user.DisplayName }

// WebAuthnName is the identifier shown to the authenticator. Email when set,
// otherwise the display name.
func (u waUser) WebAuthnName() string {
	if u.user.Email != "" {
		return u.user.Email
	}
	return u.user.DisplayName
}

func (u waUser) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, 0, len(u.creds))
	for _, c := range u.creds {
		out = append(out, webauthn.Credential{
			ID:        c.CredentialID,
			PublicKey: c.PublicKey,
			Transport: webauthnTransports(c.Transports),
			Flags:     credentialFlags(c.Flags),
			Authenticator: webauthn.Authenticator{
				AAGUID:    c.AAGUID,
				SignCount: c.SignCount,
			},
		})
	}
	return out
}

// webauthnUser loads a user and its credentials, mapping a missing user to
// ErrUser.
func (s *Service) webauthnUser(ctx context.Context, userID string) (waUser, error) {
	user, err := s.users.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return waUser{}, ErrUser
		}
		return waUser{}, err
	}
	creds, err := s.creds.ListCredentials(ctx, userID)
	if err != nil {
		return waUser{}, err
	}
	return waUser{user: user, creds: creds}, nil
}

func webauthnTransports(ts stores.Transports) []protocol.AuthenticatorTransport {
	out := make([]protocol.AuthenticatorTransport, 0, len(ts))
	for _, t := range ts {
		out = append(out, protocol.AuthenticatorTransport(t))
	}
	return out
}

func storeTransports(ts []protocol.AuthenticatorTransport) stores.Transports {
	out := make(stores.Transports, 0, len(ts))
	for _, t := range ts {
		out = append(out, string(t))
	}
	return out
}
