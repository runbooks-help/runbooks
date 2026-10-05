// SPDX-License-Identifier: FSL-1.1-MIT

// Package identity is the identity service: passkey ceremonies and sessions on
// top of stores. It holds no HTTP and no configuration loading; callers hand it
// raw ceremony bodies and get back users and session tokens.
package identity

import (
	"errors"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"runbooks/stores"
)

// Errors returned by the service. Store errors are mapped onto these; they never
// leak.
var (
	// ErrChallenge means a challenge is missing, expired or already used.
	ErrChallenge = errors.New("identity: challenge missing, expired or already used")
	// ErrSession means a session is missing, expired, revoked or belongs to a
	// disabled user.
	ErrSession = errors.New("identity: session missing, expired or disabled")
	// ErrUser means the user does not exist.
	ErrUser = errors.New("identity: unknown user")
	// ErrInvite means an invite is missing, already used or expired.
	ErrInvite = errors.New("identity: invite missing, used or expired")
)

// defaultTouchInterval throttles last-seen/last-used refreshes on the hot read
// path: a session or API key is only written back once its timestamp is older
// than this, so ordinary reads stay reads. The idle window is days, so a
// timestamp stale by minutes loses nothing.
const defaultTouchInterval = 5 * time.Minute

// Config configures a Service. RPID, RPDisplayName and RPOrigins come from the
// deployment's public URL.
type Config struct {
	RPID          string
	RPDisplayName string
	RPOrigins     []string

	// ChallengeTTL limits how long a begin/finish pair may take. Default 5m.
	ChallengeTTL time.Duration
	// SessionTTL is the absolute session lifetime. Default 720h.
	SessionTTL time.Duration
	// SessionIdleTTL is the idle session lifetime, refreshed on each request.
	// Default 168h.
	SessionIdleTTL time.Duration
}

// Service runs WebAuthn ceremonies and manages sessions over a stores.Store.
type Service struct {
	wa         *webauthn.WebAuthn
	users      stores.UserStore
	creds      stores.CredentialStore
	sessions   stores.SessionStore
	challenges stores.ChallengeStore
	invites    stores.InviteStore
	apiKeys    stores.APIKeyStore

	challengeTTL   time.Duration
	sessionTTL     time.Duration
	sessionIdleTTL time.Duration
	touchInterval  time.Duration

	now func() time.Time
}

// New builds a Service over st.
func New(cfg Config, st stores.Store) (*Service, error) {
	if cfg.ChallengeTTL == 0 {
		cfg.ChallengeTTL = 5 * time.Minute
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = 720 * time.Hour
	}
	if cfg.SessionIdleTTL == 0 {
		cfg.SessionIdleTTL = 168 * time.Hour
	}

	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.RPID,
		RPDisplayName: cfg.RPDisplayName,
		RPOrigins:     cfg.RPOrigins,
		// A passkey is a discoverable credential: require a resident key so
		// password managers offer to save one and our discoverable login can
		// find it. Ask for attestation "none": requesting attestation is what
		// makes the synced-passkey clients misbehave.
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey:        protocol.ResidentKeyRequirementRequired,
			RequireResidentKey: protocol.ResidentKeyRequired(),
			UserVerification:   protocol.VerificationPreferred,
		},
		AttestationPreference: protocol.PreferNoAttestation,
		Timeouts: webauthn.TimeoutsConfig{
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: cfg.ChallengeTTL, TimeoutUVD: cfg.ChallengeTTL},
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: cfg.ChallengeTTL, TimeoutUVD: cfg.ChallengeTTL},
		},
	})
	if err != nil {
		return nil, err
	}

	return &Service{
		wa:             wa,
		users:          st,
		creds:          st,
		sessions:       st,
		challenges:     st,
		invites:        st,
		apiKeys:        st,
		challengeTTL:   cfg.ChallengeTTL,
		sessionTTL:     cfg.SessionTTL,
		sessionIdleTTL: cfg.SessionIdleTTL,
		touchInterval:  defaultTouchInterval,
		now:            time.Now,
	}, nil
}
