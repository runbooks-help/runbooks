// SPDX-License-Identifier: FSL-1.1-MIT

package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"

	"runbooks/stores"
	"runbooks/stores/sqlite"
)

const (
	testRPID   = "localhost"
	testOrigin = "http://localhost"
	testRPName = "Runbooks Test"
)

func newTestService(t *testing.T) (*Service, stores.Store) {
	t.Helper()
	st, err := sqlite.Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "identity.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	svc, err := New(Config{RPID: testRPID, RPDisplayName: testRPName, RPOrigins: []string{testOrigin}}, st)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, st
}

func seedUser(t *testing.T, st stores.Store, id string) {
	t.Helper()
	u := stores.User{ID: id, Email: id + "@example.com", DisplayName: "Test " + id, Role: stores.RoleMember, CreatedAt: time.Now()}
	if err := st.InsertUser(context.Background(), u); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func testRP() virtualwebauthn.RelyingParty {
	return virtualwebauthn.RelyingParty{Name: testRPName, ID: testRPID, Origin: testOrigin}
}

// register runs a full registration ceremony and returns the stored credential,
// the authenticator and the virtual credential, ready to assert.
func register(t *testing.T, svc *Service, userID string) (stores.Credential, virtualwebauthn.Authenticator, virtualwebauthn.Credential) {
	t.Helper()
	ctx := context.Background()

	token, options, err := svc.BeginRegistration(ctx, userID)
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	parsed, err := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}

	auth := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	response := virtualwebauthn.CreateAttestationResponse(testRP(), auth, cred, *parsed)

	stored, err := svc.FinishRegistration(ctx, token, []byte(response))
	if err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	auth.AddCredential(cred)
	return stored, auth, cred
}

func assert(t *testing.T, svc *Service, auth virtualwebauthn.Authenticator, cred virtualwebauthn.Credential) string {
	t.Helper()
	ctx := context.Background()

	token, options, err := svc.BeginLogin(ctx, "u1")
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	parsed, err := virtualwebauthn.ParseAssertionOptions(string(optionsJSON))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	response := virtualwebauthn.CreateAssertionResponse(testRP(), auth, cred, *parsed)
	return token + "\x00" + response
}

func TestRegisterAndLogin(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")

	stored, auth, cred := register(t, svc, "u1")
	if stored.ID == "" || len(stored.CredentialID) == 0 || len(stored.PublicKey) == 0 {
		t.Fatalf("stored credential is empty: %+v", stored)
	}

	packed := assert(t, svc, auth, cred)
	token, response, _ := split(packed)

	raw, user, err := svc.FinishLogin(context.Background(), token, []byte(response), "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("FinishLogin: %v", err)
	}
	if user.ID != "u1" {
		t.Errorf("user = %q, want u1", user.ID)
	}

	got, err := svc.Authenticate(context.Background(), raw)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.ID != "u1" {
		t.Errorf("Authenticate user = %q, want u1", got.ID)
	}

	// The credential's sign count advanced and last-used was set.
	updated, err := st.GetCredential(context.Background(), stored.CredentialID)
	if err != nil {
		t.Fatalf("GetCredential: %v", err)
	}
	if updated.LastUsedAt.IsZero() {
		t.Error("credential LastUsedAt not set after login")
	}

	if err := svc.Revoke(context.Background(), raw); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := svc.Authenticate(context.Background(), raw); !errors.Is(err, ErrSession) {
		t.Errorf("Authenticate after revoke err = %v, want ErrSession", err)
	}
}

// Login rejects an assertion whose backup-eligibility flag disagrees with the
// stored credential, so a synced passkey (BE=1) fails unless registration
// persists the flag.
func TestLoginWithBackupEligibleCredential(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")
	ctx := context.Background()

	auth := virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{
		BackupEligible: true,
		BackupState:    true,
	})
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	token, options, err := svc.BeginRegistration(ctx, "u1")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	parsedAttestation, err := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	attestation := virtualwebauthn.CreateAttestationResponse(testRP(), auth, cred, *parsedAttestation)

	stored, err := svc.FinishRegistration(ctx, token, []byte(attestation))
	if err != nil {
		t.Fatalf("FinishRegistration: %v", err)
	}
	if !credentialFlags(stored.Flags).BackupEligible {
		t.Fatalf("stored flags lost backup eligibility: %#x", stored.Flags)
	}
	auth.AddCredential(cred)

	loginToken, loginOptions, err := svc.BeginLogin(ctx, "u1")
	if err != nil {
		t.Fatalf("BeginLogin: %v", err)
	}
	loginJSON, err := json.Marshal(loginOptions)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	parsedAssertion, err := virtualwebauthn.ParseAssertionOptions(string(loginJSON))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	assertion := virtualwebauthn.CreateAssertionResponse(testRP(), auth, cred, *parsedAssertion)

	if _, _, err := svc.FinishLogin(ctx, loginToken, []byte(assertion), "test", "127.0.0.1"); err != nil {
		t.Fatalf("FinishLogin with backup-eligible credential: %v", err)
	}
}

func TestDiscoverableLogin(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")
	_, auth, cred := register(t, svc, "u1")

	ctx := context.Background()
	token, options, err := svc.BeginDiscoverableLogin(ctx)
	if err != nil {
		t.Fatalf("BeginDiscoverableLogin: %v", err)
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	parsed, err := virtualwebauthn.ParseAssertionOptions(string(optionsJSON))
	if err != nil {
		t.Fatalf("ParseAssertionOptions: %v", err)
	}
	response := virtualwebauthn.CreateAssertionResponse(testRP(), auth, cred, *parsed)

	_, user, err := svc.FinishLogin(ctx, token, []byte(response), "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("FinishLogin discoverable: %v", err)
	}
	if user.ID != "u1" {
		t.Errorf("user = %q, want u1", user.ID)
	}
}

func TestChallengeCannotBeReplayed(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")

	ctx := context.Background()
	token, options, err := svc.BeginRegistration(ctx, "u1")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("marshal options: %v", err)
	}
	parsed, err := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	if err != nil {
		t.Fatalf("ParseAttestationOptions: %v", err)
	}
	auth := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	response := virtualwebauthn.CreateAttestationResponse(testRP(), auth, cred, *parsed)

	if _, err := svc.FinishRegistration(ctx, token, []byte(response)); err != nil {
		t.Fatalf("first FinishRegistration: %v", err)
	}
	if _, err := svc.FinishRegistration(ctx, token, []byte(response)); !errors.Is(err, ErrChallenge) {
		t.Errorf("replayed FinishRegistration err = %v, want ErrChallenge", err)
	}
}

func TestExpiredChallenge(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")

	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }

	ctx := context.Background()
	token, options, err := svc.BeginRegistration(ctx, "u1")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	optionsJSON, _ := json.Marshal(options)
	parsed, _ := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	auth := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	response := virtualwebauthn.CreateAttestationResponse(testRP(), auth, cred, *parsed)

	svc.now = func() time.Time { return base.Add(svc.challengeTTL + time.Minute) }
	if _, err := svc.FinishRegistration(ctx, token, []byte(response)); !errors.Is(err, ErrChallenge) {
		t.Errorf("expired FinishRegistration err = %v, want ErrChallenge", err)
	}
}

func TestDuplicateCredentialIsConflict(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")

	ctx := context.Background()
	// Register once through the helper, then replay the same virtual credential
	// through a second registration ceremony.
	_, auth, cred := register(t, svc, "u1")

	token, options, err := svc.BeginRegistration(ctx, "u1")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}
	optionsJSON, _ := json.Marshal(options)
	parsed, _ := virtualwebauthn.ParseAttestationOptions(string(optionsJSON))
	response := virtualwebauthn.CreateAttestationResponse(testRP(), auth, cred, *parsed)

	_, err = svc.FinishRegistration(ctx, token, []byte(response))
	var conflict *stores.ConflictError
	if !errors.As(err, &conflict) {
		t.Errorf("duplicate FinishRegistration err = %v, want ConflictError", err)
	}
}

// TestRegistrationRejectsChallengeWithoutUser pins the trust boundary: the user
// comes from the challenge's session, so a challenge that names no user cannot be
// finished; there is no caller-supplied id to fall back on.
func TestRegistrationRejectsChallengeWithoutUser(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()

	data, err := json.Marshal(webauthn.SessionData{})
	if err != nil {
		t.Fatalf("marshal session: %v", err)
	}
	token := "challenge-with-no-user"
	if err := st.InsertChallenge(ctx, stores.Challenge{
		ID:        hashToken(token),
		Kind:      challengeRegistration,
		Data:      data,
		ExpiresAt: svc.now().Add(svc.challengeTTL),
	}); err != nil {
		t.Fatalf("InsertChallenge: %v", err)
	}

	if _, err := svc.FinishRegistration(ctx, token, []byte("{}")); !errors.Is(err, ErrChallenge) {
		t.Errorf("err = %v, want ErrChallenge", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")

	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }
	ctx := context.Background()

	raw, err := svc.Create(ctx, "u1", "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Authenticate(ctx, raw); err != nil {
		t.Fatalf("fresh session: %v", err)
	}

	// Exactly at the idle TTL is still valid; a nanosecond past is not.
	svc.now = func() time.Time { return base }
	raw, err = svc.Create(ctx, "u1", "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.now = func() time.Time { return base.Add(svc.sessionIdleTTL) }
	if _, err := svc.Authenticate(ctx, raw); err != nil {
		t.Errorf("session valid exactly at idle TTL: %v", err)
	}

	svc.now = func() time.Time { return base }
	raw, err = svc.Create(ctx, "u1", "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.now = func() time.Time { return base.Add(svc.sessionIdleTTL + time.Nanosecond) }
	if _, err := svc.Authenticate(ctx, raw); !errors.Is(err, ErrSession) {
		t.Errorf("idle-expired session err = %v, want ErrSession", err)
	}

	// Absolute: a fresh session past the absolute TTL.
	svc.now = func() time.Time { return base }
	raw, err = svc.Create(ctx, "u1", "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	svc.now = func() time.Time { return base.Add(svc.sessionTTL + time.Minute) }
	if _, err := svc.Authenticate(ctx, raw); !errors.Is(err, ErrSession) {
		t.Errorf("absolute-expired session err = %v, want ErrSession", err)
	}
}

func TestSessionTouchThrottled(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")
	ctx := context.Background()

	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return base }
	raw, err := svc.Create(ctx, "u1", "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Within the touch interval a request must not write the session back.
	svc.now = func() time.Time { return base.Add(time.Minute) }
	if _, err := svc.Authenticate(ctx, raw); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	sess, err := st.GetSession(ctx, hashToken(raw))
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !sess.LastSeenAt.Equal(base) {
		t.Errorf("last-seen refreshed within interval: got %v, want %v", sess.LastSeenAt, base)
	}

	// Past the interval it refreshes to now.
	now := base.Add(svc.touchInterval + time.Minute)
	svc.now = func() time.Time { return now }
	if _, err := svc.Authenticate(ctx, raw); err != nil {
		t.Fatalf("Authenticate past interval: %v", err)
	}
	sess, err = st.GetSession(ctx, hashToken(raw))
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !sess.LastSeenAt.Equal(now) {
		t.Errorf("last-seen not refreshed past interval: got %v, want %v", sess.LastSeenAt, now)
	}
}

func TestDisabledUserSessionRejected(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")
	ctx := context.Background()

	raw, err := svc.Create(ctx, "u1", "curl/8", "127.0.0.1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	u, err := st.GetUser(ctx, "u1")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	u.DisabledAt = time.Now()
	if err := st.UpdateUser(ctx, u); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if _, err := svc.Authenticate(ctx, raw); !errors.Is(err, ErrSession) {
		t.Errorf("disabled user session err = %v, want ErrSession", err)
	}
}

func TestDefaultTTLs(t *testing.T) {
	// newTestService passes no TTLs, so these are the defaults.
	svc, _ := newTestService(t)
	if svc.challengeTTL != 5*time.Minute {
		t.Errorf("challengeTTL = %v, want 5m", svc.challengeTTL)
	}
	if svc.sessionTTL != 720*time.Hour {
		t.Errorf("sessionTTL = %v, want 720h", svc.sessionTTL)
	}
	if svc.sessionIdleTTL != 168*time.Hour {
		t.Errorf("sessionIdleTTL = %v, want 168h", svc.sessionIdleTTL)
	}
}

func TestWebAuthnName(t *testing.T) {
	withEmail := waUser{user: stores.User{Email: "a@example.com", DisplayName: "Ada"}}
	if got := withEmail.WebAuthnName(); got != "a@example.com" {
		t.Errorf("WebAuthnName with email = %q, want a@example.com", got)
	}
	withoutEmail := waUser{user: stores.User{DisplayName: "Ada"}}
	if got := withoutEmail.WebAuthnName(); got != "Ada" {
		t.Errorf("WebAuthnName without email = %q, want Ada", got)
	}
}

func TestRegistrationOptionsArePasskeyFriendly(t *testing.T) {
	svc, st := newTestService(t)
	seedUser(t, st, "u1")
	stored, _, _ := register(t, svc, "u1")

	_, options, err := svc.BeginRegistration(context.Background(), "u1")
	if err != nil {
		t.Fatalf("BeginRegistration: %v", err)
	}

	// Password managers only offer to save a discoverable credential.
	sel := options.Response.AuthenticatorSelection
	if sel.ResidentKey != protocol.ResidentKeyRequirementRequired {
		t.Errorf("residentKey = %q, want required", sel.ResidentKey)
	}
	if sel.RequireResidentKey == nil || !*sel.RequireResidentKey {
		t.Error("requireResidentKey must be true for WebAuthn L1 clients")
	}
	if sel.UserVerification != protocol.VerificationPreferred {
		t.Errorf("userVerification = %q, want preferred", sel.UserVerification)
	}
	// Requesting attestation is what makes synced-passkey clients misbehave.
	if options.Response.Attestation != protocol.PreferNoAttestation {
		t.Errorf("attestation = %q, want none", options.Response.Attestation)
	}
	// The client shows these; both must be present and human-readable.
	if options.Response.User.Name == "" || options.Response.User.DisplayName == "" {
		t.Errorf("user name/displayName must be set, got %+v", options.Response.User)
	}
	// An existing passkey is excluded, so the same authenticator is not re-added.
	found := false
	for _, ex := range options.Response.CredentialExcludeList {
		if bytes.Equal(ex.CredentialID, stored.CredentialID) {
			found = true
		}
	}
	if !found {
		t.Errorf("existing credential not excluded: %+v", options.Response.CredentialExcludeList)
	}
}

// split separates the token and response packed by assert.
func split(s string) (token, response string, _ bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}
