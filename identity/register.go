package identity

import (
	"context"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"runbooks/stores"
)

// BeginRegistration starts adding a passkey to an existing user. It returns the
// options to hand to the browser and an opaque challenge token the caller must
// hand back to FinishRegistration.
func (s *Service) BeginRegistration(ctx context.Context, userID string) (token string, options *protocol.CredentialCreation, err error) {
	user, err := s.webauthnUser(ctx, userID)
	if err != nil {
		return "", nil, err
	}

	// Exclude the user's existing passkeys so a client does not create a second
	// credential for an authenticator that already has one.
	exclusions := webauthn.Credentials(user.WebAuthnCredentials()).CredentialDescriptors()
	options, session, err := s.wa.BeginRegistration(user, webauthn.WithExclusions(exclusions))
	if err != nil {
		return "", nil, err
	}

	token, err = s.storeChallenge(ctx, challengeRegistration, session)
	if err != nil {
		return "", nil, err
	}
	return token, options, nil
}

// FinishRegistration verifies the attestation and stores the credential on the
// user the ceremony was begun for — the user id is read from the challenge's
// session, never taken from the caller, so a client cannot point a finish at a
// different account. Body is the raw JSON the browser sent. A credential id
// already registered comes back as stores.ConflictError.
func (s *Service) FinishRegistration(ctx context.Context, token string, body []byte) (stores.Credential, error) {
	session, err := s.consumeChallenge(ctx, challengeRegistration, token)
	if err != nil {
		return stores.Credential{}, err
	}

	userID := string(session.UserID)
	if userID == "" {
		return stores.Credential{}, ErrChallenge
	}
	user, err := s.webauthnUser(ctx, userID)
	if err != nil {
		return stores.Credential{}, err
	}

	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	if err != nil {
		return stores.Credential{}, err
	}

	cred, err := s.wa.CreateCredential(user, *session, parsed)
	if err != nil {
		return stores.Credential{}, err
	}

	id, err := newID()
	if err != nil {
		return stores.Credential{}, err
	}
	stored := stores.Credential{
		ID:           id,
		UserID:       userID,
		CredentialID: cred.ID,
		PublicKey:    cred.PublicKey,
		SignCount:    cred.Authenticator.SignCount,
		Transports:   storeTransports(cred.Transport),
		AAGUID:       cred.Authenticator.AAGUID,
		Flags:        storeCredentialFlags(cred.Flags),
		CreatedAt:    s.now(),
	}
	if err := s.creds.InsertCredential(ctx, stored); err != nil {
		return stores.Credential{}, err
	}
	return stored, nil
}
