package identity

import (
	"context"
	"errors"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"runbooks/stores"
)

// BeginLogin starts an assertion for a known user (identifier-first). It returns
// the options and an opaque challenge token for FinishLogin.
func (s *Service) BeginLogin(ctx context.Context, userID string) (string, *protocol.CredentialAssertion, error) {
	user, err := s.webauthnUser(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	options, session, err := s.wa.BeginLogin(user)
	if err != nil {
		return "", nil, err
	}
	token, err := s.storeChallenge(ctx, challengeLogin, session)
	if err != nil {
		return "", nil, err
	}
	return token, options, nil
}

// BeginDiscoverableLogin starts a usernameless assertion. FinishLogin resolves
// the user from the assertion's user handle.
func (s *Service) BeginDiscoverableLogin(ctx context.Context) (string, *protocol.CredentialAssertion, error) {
	options, session, err := s.wa.BeginDiscoverableLogin()
	if err != nil {
		return "", nil, err
	}
	token, err := s.storeChallenge(ctx, challengeLogin, session)
	if err != nil {
		return "", nil, err
	}
	return token, options, nil
}

// FinishLogin verifies the assertion, updates the credential's sign count and
// last-used time, then creates a session. It returns the raw session cookie
// value and the authenticated user.
func (s *Service) FinishLogin(ctx context.Context, token string, body []byte, userAgent, ip string) (string, stores.User, error) {
	session, err := s.consumeChallenge(ctx, challengeLogin, token)
	if err != nil {
		return "", stores.User{}, err
	}

	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	if err != nil {
		return "", stores.User{}, err
	}

	userID, err := s.loginUserID(ctx, session, parsed)
	if err != nil {
		return "", stores.User{}, err
	}
	user, err := s.webauthnUser(ctx, userID)
	if err != nil {
		return "", stores.User{}, err
	}

	// The library binds a session to one user; a discoverable begin leaves that
	// empty, so bind the user we just resolved.
	session.UserID = []byte(userID)
	cred, err := s.wa.ValidateLogin(user, *session, parsed)
	if err != nil {
		return "", stores.User{}, err
	}
	if err := s.touchCredential(ctx, cred); err != nil {
		return "", stores.User{}, err
	}

	raw, err := s.Create(ctx, user.user.ID, userAgent, ip)
	if err != nil {
		return "", stores.User{}, err
	}
	return raw, user.user, nil
}

// loginUserID picks the user an assertion belongs to: the challenge's user for
// identifier-first, the assertion's user handle for a discoverable login, or —
// when a discoverable assertion carries no handle — the credential's owner.
func (s *Service) loginUserID(ctx context.Context, session *webauthn.SessionData, parsed *protocol.ParsedCredentialAssertionData) (string, error) {
	if len(session.UserID) > 0 {
		return string(session.UserID), nil
	}
	if handle := parsed.Response.UserHandle; len(handle) > 0 {
		return string(handle), nil
	}
	cred, err := s.creds.GetCredential(ctx, parsed.RawID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return "", ErrUser
		}
		return "", err
	}
	return cred.UserID, nil
}

// touchCredential records the new sign count and last-used time of a credential.
func (s *Service) touchCredential(ctx context.Context, cred *webauthn.Credential) error {
	stored, err := s.creds.GetCredential(ctx, cred.ID)
	if err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			return ErrUser
		}
		return err
	}
	stored.SignCount = cred.Authenticator.SignCount
	stored.Flags = storeCredentialFlags(cred.Flags)
	stored.LastUsedAt = s.now()
	return s.creds.UpdateCredential(ctx, stored)
}
