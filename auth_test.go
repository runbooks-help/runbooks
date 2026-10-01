package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

	"runbooks/identity"
	"runbooks/stores"
	"runbooks/stores/sqlite"
	"runbooks/views"
)

const (
	testRPID     = "localhost"
	testOrigin   = "http://localhost"
	testRPName   = "Runbooks"
	testToken    = "test-bootstrap"
	testRecovery = "test-recovery"
)

func testRP() virtualwebauthn.RelyingParty {
	return virtualwebauthn.RelyingParty{Name: testRPName, ID: testRPID, Origin: testOrigin}
}

func newTestServer(t *testing.T) (*httptest.Server, *identity.Service, stores.Store) {
	t.Helper()
	return newTestServerWith(t, nil)
}

// newTestServerWith builds the test server, letting a test tweak the config
// (e.g. to disable the recovery token) before wiring.
func newTestServerWith(t *testing.T, mutate func(*config)) (*httptest.Server, *identity.Service, stores.Store) {
	t.Helper()
	st, err := sqlite.Open(context.Background(), "file:"+filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })

	cfg := config{
		IdentityEnabled:        true,
		IdentityDriver:         "sqlite",
		IdentityPublicURL:      testOrigin,
		IdentityBootstrapToken: testToken,
		IdentityRecoveryToken:  testRecovery,
		IdentitySecureCookies:  false,
		IdentitySessionTTL:     720 * time.Hour,
		IdentitySessionIdleTTL: 168 * time.Hour,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	svc, err := identity.New(identity.Config{
		RPID:           testRPID,
		RPDisplayName:  testRPName,
		RPOrigins:      []string{testOrigin},
		SessionTTL:     cfg.IdentitySessionTTL,
		SessionIdleTTL: cfg.IdentitySessionIdleTTL,
	}, st)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	a := newAuth(svc, st, cfg)

	mux := http.NewServeMux()
	mux.HandleFunc("/login", a.loginPage)
	mux.HandleFunc("/setup", a.setupPage)
	mux.HandleFunc("/invite/{token}", a.invitePage)
	mux.HandleFunc("/api/auth/v1/login/begin", a.loginBegin)
	mux.HandleFunc("/api/auth/v1/login/finish", a.loginFinish)
	mux.HandleFunc("/api/auth/v1/setup/begin", a.setupBegin)
	mux.HandleFunc("/api/auth/v1/setup/finish", a.setupFinish)
	mux.HandleFunc("/api/auth/v1/invite/begin", a.inviteBegin)
	mux.HandleFunc("/api/auth/v1/invite/finish", a.inviteFinish)
	mux.HandleFunc("/api/auth/v1/logout", a.logout)
	mux.HandleFunc("/admin", a.requireAdmin(func(w http.ResponseWriter, r *http.Request) {
		users, err := a.st.ListUsers(r.Context())
		if err != nil {
			http.Error(w, "could not load users", http.StatusInternalServerError)
			return
		}
		u := userFrom(r.Context())
		views.AdminPage(nil, users, u.IsAdmin(), a.cfg.IdentityEnabled).Render(r.Context(), w)
	}))
	mux.HandleFunc("/api/auth/v1/invites", a.requireAdminAPI(a.createInvite))
	mux.HandleFunc("/api/auth/v1/sessions/revoke", a.requireAdminAPI(a.revokeSessions))
	mux.HandleFunc("/api/runbooks/v1/ack", a.requireAPI(a.ackRunbook))
	mux.HandleFunc("/api/git-sync/v1", a.gateGitSync(handleGitSync(cfg)))
	if cfg.IdentityRecoveryToken != "" {
		mux.HandleFunc("/recovery", a.recoveryPage)
		mux.HandleFunc("/api/auth/v1/recovery/begin", a.recoveryBegin)
		mux.HandleFunc("/api/auth/v1/recovery/finish", a.recoveryFinish)
	}
	mux.HandleFunc("/{$}", a.requirePage(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "index")
	}))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, svc, st
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func postJSON(t *testing.T, client *http.Client, url string, body any) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return res.StatusCode, data
}

type beginResponse struct {
	UserID    string          `json:"user_id"`
	Challenge string          `json:"challenge"`
	Options   json.RawMessage `json:"options"`
}

func TestClientIP(t *testing.T) {
	if got := clientIP(&http.Request{RemoteAddr: "127.0.0.1:1234"}); got != "127.0.0.1" {
		t.Errorf("clientIP with port = %q, want 127.0.0.1", got)
	}
	if got := clientIP(&http.Request{RemoteAddr: "127.0.0.1"}); got != "127.0.0.1" {
		t.Errorf("clientIP without port = %q, want the raw address", got)
	}
}

func TestSetupLoginGatingFlow(t *testing.T) {
	srv, _, _ := newTestServer(t)
	rp := testRP()
	// Model a real synced passkey (BackupEligible), not the library default: the
	// ceremony asserts BE=1 and login validation compares it to the stored flag.
	authn := virtualwebauthn.NewAuthenticatorWithOptions(virtualwebauthn.AuthenticatorOptions{
		BackupEligible: true,
		BackupState:    true,
	})
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)

	// A fresh client is unauthenticated: the index redirects to /login.
	anon := newClient(t)
	res, err := anon.Get(srv.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/login" {
		t.Fatalf("unauthenticated GET / = %d %q, want 302 /login", res.StatusCode, res.Header.Get("Location"))
	}
	if code, _ := get(t, anon, srv.URL+"/login"); code != http.StatusOK {
		t.Fatalf("GET /login = %d, want 200", code)
	}

	// Bootstrap: create the admin and register its passkey.
	client := newClient(t)
	begin := decodeBegin(t, client, srv.URL+"/api/auth/v1/setup/begin", map[string]string{
		"token": testToken, "display_name": "Ben", "email": "ben@example.com",
	})
	attestation, err := virtualwebauthn.ParseAttestationOptions(string(begin.Options))
	if err != nil {
		t.Fatalf("parse attestation options: %v", err)
	}
	attestationResp := virtualwebauthn.CreateAttestationResponse(rp, authn, cred, *attestation)
	code, body := postJSON(t, client, srv.URL+"/api/auth/v1/setup/finish", map[string]any{
		"token": testToken, "user_id": begin.UserID, "challenge": begin.Challenge,
		"credential": json.RawMessage(attestationResp),
	})
	if code != http.StatusOK {
		t.Fatalf("setup/finish = %d: %s", code, body)
	}
	authn.AddCredential(cred)

	// The session cookie now opens the index.
	if code, _ := get(t, client, srv.URL+"/"); code != http.StatusOK {
		t.Fatalf("authenticated GET / = %d, want 200", code)
	}

	// Setup is closed once an admin exists (exercises adminExists's scan).
	if code, _ := get(t, anon, srv.URL+"/setup"); code != http.StatusFound {
		t.Fatalf("GET /setup with an admin = %d, want 302", code)
	}

	// Log out: the session row is revoked server-side and the cookie is cleared.
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	var sessionValue string
	for _, c := range client.Jar.Cookies(u) {
		if c.Name == sessionCookieName {
			sessionValue = c.Value
		}
	}
	if sessionValue == "" {
		t.Fatal("no session cookie before logout")
	}
	logoutReq, err := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/v1/logout", nil)
	if err != nil {
		t.Fatalf("new logout request: %v", err)
	}
	logoutRes, err := client.Do(logoutReq)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	logoutRes.Body.Close()
	if logoutRes.StatusCode != http.StatusOK {
		t.Fatalf("logout = %d, want 200", logoutRes.StatusCode)
	}
	if sc := logoutRes.Header.Get("Set-Cookie"); !strings.Contains(sc, "Max-Age=0") {
		t.Errorf("logout Set-Cookie = %q, want Max-Age=0", sc)
	}
	if code, _ := get(t, client, srv.URL+"/"); code != http.StatusFound {
		t.Fatalf("GET / after logout = %d, want 302", code)
	}
	// The revoked session is dead even if its cookie is replayed.
	replay := newClient(t)
	replay.Jar.SetCookies(u, []*http.Cookie{{Name: sessionCookieName, Value: sessionValue, Path: "/"}})
	if code, _ := get(t, replay, srv.URL+"/"); code != http.StatusFound {
		t.Fatalf("replayed revoked cookie GET / = %d, want 302", code)
	}
	code, body = postJSON(t, client, srv.URL+"/api/auth/v1/login/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("login/begin = %d: %s", code, body)
	}
	var lb beginResponse
	if err := json.Unmarshal(body, &lb); err != nil {
		t.Fatalf("login/begin json: %v", err)
	}
	assertion, err := virtualwebauthn.ParseAssertionOptions(string(lb.Options))
	if err != nil {
		t.Fatalf("parse assertion options: %v", err)
	}
	assertionResp := virtualwebauthn.CreateAssertionResponse(rp, authn, cred, *assertion)
	if code, body := postJSON(t, client, srv.URL+"/api/auth/v1/login/finish", map[string]any{
		"challenge": lb.Challenge, "credential": json.RawMessage(assertionResp),
	}); code != http.StatusOK {
		t.Fatalf("login/finish = %d: %s", code, body)
	}
	if code, _ := get(t, client, srv.URL+"/"); code != http.StatusOK {
		t.Fatalf("GET / after login = %d, want 200", code)
	}
}

// bootstrapAdmin runs the first-admin /setup flow and returns the client holding
// the admin session plus the admin's user id.
func bootstrapAdmin(t *testing.T, srv *httptest.Server, rp virtualwebauthn.RelyingParty, authn virtualwebauthn.Authenticator, cred virtualwebauthn.Credential) (*http.Client, string) {
	t.Helper()
	client := newClient(t)
	begin := decodeBegin(t, client, srv.URL+"/api/auth/v1/setup/begin", map[string]string{
		"token": testToken, "display_name": "Ben", "email": "ben@example.com",
	})
	attestation, err := virtualwebauthn.ParseAttestationOptions(string(begin.Options))
	if err != nil {
		t.Fatalf("parse attestation options: %v", err)
	}
	attestationResp := virtualwebauthn.CreateAttestationResponse(rp, authn, cred, *attestation)
	code, body := postJSON(t, client, srv.URL+"/api/auth/v1/setup/finish", map[string]any{
		"token": testToken, "user_id": begin.UserID, "challenge": begin.Challenge,
		"credential": json.RawMessage(attestationResp),
	})
	if code != http.StatusOK {
		t.Fatalf("setup/finish = %d: %s", code, body)
	}
	return client, begin.UserID
}

func TestInviteFlow(t *testing.T) {
	srv, svc, st := newTestServer(t)
	rp := testRP()
	adminAuthn := virtualwebauthn.NewAuthenticator()
	adminCred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	_, adminID := bootstrapAdmin(t, srv, rp, adminAuthn, adminCred)

	// The admin mints an invite (the admin HTTP endpoint lands in the next slice).
	raw, err := svc.CreateInvite(context.Background(), adminID, stores.RoleMember, "", time.Hour)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	// A live invite renders the join page; an unknown one renders the invalid page.
	invited := newClient(t)
	if code, body := get(t, invited, srv.URL+"/invite/"+raw); code != http.StatusOK || !bytes.Contains(body, []byte(`name="display_name"`)) {
		t.Fatalf("GET /invite = %d %q, want the join form", code, body)
	}
	if code, body := get(t, invited, srv.URL+"/invite/nope"); code != http.StatusOK || !bytes.Contains(body, []byte("not valid")) {
		t.Fatalf("GET /invite/nope = %d %q, want invalid page", code, body)
	}

	// Enrol the invited member's passkey.
	memberAuthn := virtualwebauthn.NewAuthenticator()
	memberCred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	begin := decodeBegin(t, invited, srv.URL+"/api/auth/v1/invite/begin", map[string]string{
		"token": raw, "display_name": "Sam", "email": "sam@example.com",
	})
	attestation, err := virtualwebauthn.ParseAttestationOptions(string(begin.Options))
	if err != nil {
		t.Fatalf("parse attestation options: %v", err)
	}
	attestationResp := virtualwebauthn.CreateAttestationResponse(rp, memberAuthn, memberCred, *attestation)
	code, body := postJSON(t, invited, srv.URL+"/api/auth/v1/invite/finish", map[string]any{
		"token": raw, "challenge": begin.Challenge, "credential": json.RawMessage(attestationResp),
	})
	if code != http.StatusOK {
		t.Fatalf("invite/finish = %d: %s", code, body)
	}
	memberAuthn.AddCredential(memberCred)

	// The member is signed in and the gated index opens.
	if code, _ := get(t, invited, srv.URL+"/"); code != http.StatusOK {
		t.Fatalf("member GET / = %d, want 200", code)
	}

	// The invite's role was applied and the token is spent.
	users, err := st.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	var member stores.User
	for _, u := range users {
		if u.DisplayName == "Sam" {
			member = u
		}
	}
	if member.ID == "" || member.Role != stores.RoleMember {
		t.Fatalf("invited user = %+v, want a member named Sam", member)
	}
	if code, _ := postJSON(t, newClient(t), srv.URL+"/api/auth/v1/invite/begin", map[string]string{"token": raw, "display_name": "Eve"}); code != http.StatusUnauthorized {
		t.Fatalf("reused invite begin = %d, want 401", code)
	}

	// A re-enrolment invite is bound to the existing member and adds a second
	// passkey without creating another user.
	re, err := svc.CreateInvite(context.Background(), adminID, stores.RoleMember, member.ID, time.Hour)
	if err != nil {
		t.Fatalf("CreateInvite re-enrol: %v", err)
	}
	reClient := newClient(t)
	// A bound invite hides the name step — it re-enrols an existing user.
	if code, body := get(t, reClient, srv.URL+"/invite/"+re); code != http.StatusOK || bytes.Contains(body, []byte(`name="display_name"`)) {
		t.Fatalf("re-enrolment invite page = %d %q, want no name step", code, body)
	}
	reBegin := decodeBegin(t, reClient, srv.URL+"/api/auth/v1/invite/begin", map[string]string{"token": re})
	reAttestation, err := virtualwebauthn.ParseAttestationOptions(string(reBegin.Options))
	if err != nil {
		t.Fatalf("parse attestation options: %v", err)
	}
	secondAuthn := virtualwebauthn.NewAuthenticator()
	secondCred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	reResp := virtualwebauthn.CreateAttestationResponse(rp, secondAuthn, secondCred, *reAttestation)
	if code, body := postJSON(t, reClient, srv.URL+"/api/auth/v1/invite/finish", map[string]any{
		"token": re, "challenge": reBegin.Challenge, "credential": json.RawMessage(reResp),
	}); code != http.StatusOK {
		t.Fatalf("re-enrol finish = %d: %s", code, body)
	}
	if users, err = st.ListUsers(context.Background()); err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("users after re-enrol = %d, want 2", len(users))
	}
}

func TestAdminFlow(t *testing.T) {
	srv, _, st := newTestServer(t)
	rp := testRP()
	adminAuthn := virtualwebauthn.NewAuthenticator()
	adminCred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	admin, _ := bootstrapAdmin(t, srv, rp, adminAuthn, adminCred)

	// The admin page renders for an admin; the APIs reject the unauthenticated.
	if code, _ := get(t, admin, srv.URL+"/admin"); code != http.StatusOK {
		t.Fatalf("admin GET /admin = %d, want 200", code)
	}
	if code, _ := postJSON(t, newClient(t), srv.URL+"/api/auth/v1/invites", map[string]string{"role": "member"}); code != http.StatusUnauthorized {
		t.Fatalf("anonymous create invite = %d, want 401", code)
	}

	// The admin mints an invite through the API, then the invitee enrols.
	code, body := postJSON(t, admin, srv.URL+"/api/auth/v1/invites", map[string]string{"role": "member", "ttl": "1h"})
	if code != http.StatusOK {
		t.Fatalf("create invite = %d: %s", code, body)
	}
	var invite struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &invite); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	if invite.Token == "" || invite.URL != testOrigin+"/invite/"+invite.Token {
		t.Fatalf("invite = %+v", invite)
	}

	// An admin-role invite is accepted; an unknown role and a bad expiry are not.
	if code, body := postJSON(t, admin, srv.URL+"/api/auth/v1/invites", map[string]string{"role": "admin"}); code != http.StatusOK {
		t.Fatalf("create admin invite = %d: %s", code, body)
	}
	if code, _ := postJSON(t, admin, srv.URL+"/api/auth/v1/invites", map[string]string{"role": "root"}); code != http.StatusBadRequest {
		t.Fatalf("create invite with unknown role = %d, want 400", code)
	}
	if code, _ := postJSON(t, admin, srv.URL+"/api/auth/v1/invites", map[string]string{"role": "member", "ttl": "0s"}); code != http.StatusBadRequest {
		t.Fatalf("create invite with zero expiry = %d, want 400", code)
	}
	if code, _ := postJSON(t, admin, srv.URL+"/api/auth/v1/invites", map[string]string{"role": "member", "ttl": "nonsense"}); code != http.StatusBadRequest {
		t.Fatalf("create invite with bad expiry = %d, want 400", code)
	}

	member, memberUser := enrolInvite(t, srv, st, rp, invite.Token, "Sam")

	// A member can neither open the admin page nor call the admin APIs.
	if code, _ := get(t, member, srv.URL+"/admin"); code != http.StatusForbidden {
		t.Fatalf("member GET /admin = %d, want 403", code)
	}
	if code, _ := postJSON(t, member, srv.URL+"/api/auth/v1/invites", map[string]string{"role": "member"}); code != http.StatusForbidden {
		t.Fatalf("member create invite = %d, want 403", code)
	}

	// A bound re-enrolment invite is accepted (creates no new user).
	if code, body := postJSON(t, admin, srv.URL+"/api/auth/v1/invites", map[string]string{"user_id": memberUser.ID}); code != http.StatusOK {
		t.Fatalf("re-enrol invite = %d: %s", code, body)
	}

	// Revoking the member's sessions logs them out immediately.
	if code, _ := get(t, member, srv.URL+"/"); code != http.StatusOK {
		t.Fatalf("member GET / = %d, want 200", code)
	}
	if code, body := postJSON(t, admin, srv.URL+"/api/auth/v1/sessions/revoke", map[string]string{"user_id": memberUser.ID}); code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", code, body)
	}
	if code, _ := get(t, member, srv.URL+"/"); code != http.StatusFound {
		t.Fatalf("member GET / after revoke = %d, want 302", code)
	}
	if code, _ := postJSON(t, admin, srv.URL+"/api/auth/v1/sessions/revoke", map[string]string{"user_id": "nope"}); code != http.StatusNotFound {
		t.Fatalf("revoke unknown = %d, want 404", code)
	}
}

func TestRecoveryFlow(t *testing.T) {
	srv, _, st := newTestServer(t)
	rp := testRP()
	ctx := context.Background()
	adminAuthn := virtualwebauthn.NewAuthenticator()
	adminCred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	_, adminID := bootstrapAdmin(t, srv, rp, adminAuthn, adminCred)

	// Simulate a lost passkey: remove the admin's only credential.
	creds, err := st.ListCredentials(ctx, adminID)
	if err != nil {
		t.Fatalf("ListCredentials: %v", err)
	}
	if len(creds) != 1 {
		t.Fatalf("admin credentials = %d, want 1", len(creds))
	}
	if err := st.DeleteCredential(ctx, creds[0].ID); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}

	// The recovery page renders, and a wrong token is refused.
	if code, _ := get(t, newClient(t), srv.URL+"/recovery"); code != http.StatusOK {
		t.Fatalf("GET /recovery = %d, want 200", code)
	}
	if code, _ := postJSON(t, newClient(t), srv.URL+"/api/auth/v1/recovery/begin", map[string]string{"token": "wrong"}); code != http.StatusUnauthorized {
		t.Fatalf("recovery/begin wrong token = %d, want 401", code)
	}

	// The break-glass token re-enrols a passkey for the sole admin.
	client := newClient(t)
	code, body := postJSON(t, client, srv.URL+"/api/auth/v1/recovery/begin", map[string]string{"token": testRecovery})
	if code != http.StatusOK {
		t.Fatalf("recovery/begin = %d: %s", code, body)
	}
	var begin beginResponse
	if err := json.Unmarshal(body, &begin); err != nil {
		t.Fatalf("decode begin: %v", err)
	}
	recoveryAuthn := virtualwebauthn.NewAuthenticator()
	recoveryCred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	attestation, err := virtualwebauthn.ParseAttestationOptions(string(begin.Options))
	if err != nil {
		t.Fatalf("parse attestation options: %v", err)
	}
	resp := virtualwebauthn.CreateAttestationResponse(rp, recoveryAuthn, recoveryCred, *attestation)
	if code, body := postJSON(t, client, srv.URL+"/api/auth/v1/recovery/finish", map[string]any{
		"token": testRecovery, "challenge": begin.Challenge, "credential": json.RawMessage(resp),
	}); code != http.StatusOK {
		t.Fatalf("recovery/finish = %d: %s", code, body)
	}

	// The recovered session opens the gated index.
	if code, _ := get(t, client, srv.URL+"/"); code != http.StatusOK {
		t.Fatalf("recovered GET / = %d, want 200", code)
	}

	// And the re-enrolled passkey signs in on its own.
	recoveryAuthn.AddCredential(recoveryCred)
	if code, _ := postJSON(t, client, srv.URL+"/api/auth/v1/logout", nil); code != http.StatusOK {
		t.Fatalf("logout = %d, want 200", code)
	}
	lbCode, lbBody := postJSON(t, client, srv.URL+"/api/auth/v1/login/begin", nil)
	if lbCode != http.StatusOK {
		t.Fatalf("login/begin = %d: %s", lbCode, lbBody)
	}
	var lb beginResponse
	if err := json.Unmarshal(lbBody, &lb); err != nil {
		t.Fatalf("decode login begin: %v", err)
	}
	assertion, err := virtualwebauthn.ParseAssertionOptions(string(lb.Options))
	if err != nil {
		t.Fatalf("parse assertion options: %v", err)
	}
	assertionResp := virtualwebauthn.CreateAssertionResponse(rp, recoveryAuthn, recoveryCred, *assertion)
	if code, body := postJSON(t, client, srv.URL+"/api/auth/v1/login/finish", map[string]any{
		"challenge": lb.Challenge, "credential": json.RawMessage(assertionResp),
	}); code != http.StatusOK {
		t.Fatalf("login/finish after recovery = %d: %s", code, body)
	}

	// Break-glass is refused once a second admin exists.
	secondAdmin := stores.User{ID: "second-admin", DisplayName: "Second", Role: stores.RoleAdmin, CreatedAt: time.Now()}
	if err := st.InsertUser(ctx, secondAdmin); err != nil {
		t.Fatalf("InsertUser: %v", err)
	}
	if code, _ := postJSON(t, client, srv.URL+"/api/auth/v1/recovery/begin", map[string]string{"token": testRecovery}); code != http.StatusConflict {
		t.Fatalf("recovery/begin with two admins = %d, want 409", code)
	}
}

func TestRecoveryDisabledWithoutToken(t *testing.T) {
	srv, _, _ := newTestServerWith(t, func(c *config) { c.IdentityRecoveryToken = "" })
	if code, _ := get(t, newClient(t), srv.URL+"/recovery"); code != http.StatusNotFound {
		t.Fatalf("/recovery without a token = %d, want 404", code)
	}
	if code, _ := postJSON(t, newClient(t), srv.URL+"/api/auth/v1/recovery/begin", map[string]string{"token": "x"}); code != http.StatusNotFound {
		t.Fatalf("recovery/begin without a token = %d, want 404", code)
	}
}

// enrolInvite accepts an invite token, registers a passkey and returns the now
// signed-in client and the created user.
func enrolInvite(t *testing.T, srv *httptest.Server, st stores.Store, rp virtualwebauthn.RelyingParty, token, name string) (*http.Client, stores.User) {
	t.Helper()
	client := newClient(t)
	begin := decodeBegin(t, client, srv.URL+"/api/auth/v1/invite/begin", map[string]string{"token": token, "display_name": name})
	authn := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	attestation, err := virtualwebauthn.ParseAttestationOptions(string(begin.Options))
	if err != nil {
		t.Fatalf("parse attestation options: %v", err)
	}
	resp := virtualwebauthn.CreateAttestationResponse(rp, authn, cred, *attestation)
	if code, body := postJSON(t, client, srv.URL+"/api/auth/v1/invite/finish", map[string]any{
		"token": token, "challenge": begin.Challenge, "credential": json.RawMessage(resp),
	}); code != http.StatusOK {
		t.Fatalf("invite/finish = %d: %s", code, body)
	}
	users, err := st.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	for _, u := range users {
		if u.DisplayName == name {
			return client, u
		}
	}
	t.Fatalf("no user named %q after enrolment", name)
	return nil, stores.User{}
}

func get(t *testing.T, client *http.Client, url string) (int, []byte) {
	t.Helper()
	res, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return res.StatusCode, data
}

func decodeBegin(t *testing.T, client *http.Client, url string, body any) beginResponse {
	t.Helper()
	code, raw := postJSON(t, client, url, body)
	if code != http.StatusOK {
		t.Fatalf("POST %s = %d: %s", url, code, raw)
	}
	var b beginResponse
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatalf("decode %s: %v", url, err)
	}
	return b
}

// proxyConfig turns on proxy delegation for a test server, with explicit header
// names (the production defaults are the same).
func proxyConfig(c *config) {
	c.IdentityTrustProxyAuth = true
	c.IdentityProxyUserHeader = "Auth-Request-Email"
	c.IdentityProxyNameHeader = "Auth-Request-Name"
}

func getWithHeaders(t *testing.T, client *http.Client, url string, headers map[string]string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return res.StatusCode, data
}

func TestProxyAuthProvisionsAndGates(t *testing.T) {
	srv, _, st := newTestServerWith(t, proxyConfig)
	client := newClient(t)
	headers := map[string]string{
		"Auth-Request-Email": "proxied@example.com",
		"Auth-Request-Name":  "Proxied Person",
	}

	if code, _ := getWithHeaders(t, client, srv.URL+"/", nil); code != http.StatusFound {
		t.Fatalf("index without an assertion = %d, want 302", code)
	}

	if code, body := getWithHeaders(t, client, srv.URL+"/", headers); code != http.StatusOK {
		t.Fatalf("index with an assertion = %d: %s", code, body)
	}

	users, err := st.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("users = %d, want 1 provisioned", len(users))
	}
	u := users[0]
	if u.Email != "proxied@example.com" || u.DisplayName != "Proxied Person" || u.Role != stores.RoleMember {
		t.Errorf("provisioned user = %+v", u)
	}

	// A member is not an admin: the admin page is forbidden, not redirected.
	if code, _ := getWithHeaders(t, client, srv.URL+"/admin", headers); code != http.StatusForbidden {
		t.Errorf("/admin as a proxy member = %d, want 403", code)
	}
}

func TestProxyAuthNameFallsBackToEmail(t *testing.T) {
	srv, _, st := newTestServerWith(t, proxyConfig)
	client := newClient(t)

	if code, _ := getWithHeaders(t, client, srv.URL+"/", map[string]string{"Auth-Request-Email": "noname@example.com"}); code != http.StatusOK {
		t.Fatalf("index with an assertion = %d, want 200", code)
	}
	users, _ := st.ListUsers(context.Background())
	if len(users) != 1 || users[0].DisplayName != "noname@example.com" {
		t.Errorf("users = %+v, want one with the email as display name", users)
	}
}

func TestProxyAuthIgnoredWhenOff(t *testing.T) {
	srv, _, st := newTestServer(t)
	client := newClient(t)

	code, _ := getWithHeaders(t, client, srv.URL+"/", map[string]string{"Auth-Request-Email": "proxied@example.com"})
	if code != http.StatusFound {
		t.Fatalf("index with an assertion while off = %d, want 302", code)
	}
	users, _ := st.ListUsers(context.Background())
	if len(users) != 0 {
		t.Errorf("provisioned %d users with proxy auth off, want 0", len(users))
	}
}

func TestProxyAuthRefusesDisabledUser(t *testing.T) {
	srv, _, st := newTestServerWith(t, proxyConfig)
	if err := st.InsertUser(context.Background(), stores.User{
		ID: "gone", Email: "gone@example.com", DisplayName: "Gone",
		Role: stores.RoleMember, CreatedAt: time.Now(), DisabledAt: time.Now(),
	}); err != nil {
		t.Fatalf("insert disabled user: %v", err)
	}
	client := newClient(t)

	code, _ := getWithHeaders(t, client, srv.URL+"/", map[string]string{"Auth-Request-Email": "gone@example.com"})
	if code != http.StatusFound {
		t.Fatalf("disabled proxy user = %d, want 302", code)
	}
}

func TestAuthEvents(t *testing.T) {
	srv, _, st := newTestServer(t)
	rp := testRP()
	adminAuthn := virtualwebauthn.NewAuthenticator()
	adminCred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	adminClient, adminID := bootstrapAdmin(t, srv, rp, adminAuthn, adminCred)
	adminAuthn.AddCredential(adminCred)

	// The admin mints an invite; the invitee enrols, producing a member.
	code, body := postJSON(t, adminClient, srv.URL+"/api/auth/v1/invites", map[string]string{"role": "member"})
	if code != http.StatusOK {
		t.Fatalf("create invite = %d: %s", code, body)
	}
	var invite struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &invite); err != nil {
		t.Fatalf("decode invite: %v", err)
	}
	_, member := enrolInvite(t, srv, st, rp, invite.Token, "Sam")

	// The admin revokes the member's sessions, then signs out and back in.
	if code, body := postJSON(t, adminClient, srv.URL+"/api/auth/v1/sessions/revoke", map[string]string{"user_id": member.ID}); code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", code, body)
	}
	if code, _ := postJSON(t, adminClient, srv.URL+"/api/auth/v1/logout", nil); code != http.StatusOK {
		t.Fatalf("logout = %d, want 200", code)
	}
	code, body = postJSON(t, adminClient, srv.URL+"/api/auth/v1/login/begin", nil)
	if code != http.StatusOK {
		t.Fatalf("login/begin = %d: %s", code, body)
	}
	var lb beginResponse
	if err := json.Unmarshal(body, &lb); err != nil {
		t.Fatalf("login/begin json: %v", err)
	}
	assertion, err := virtualwebauthn.ParseAssertionOptions(string(lb.Options))
	if err != nil {
		t.Fatalf("parse assertion options: %v", err)
	}
	assertionResp := virtualwebauthn.CreateAssertionResponse(rp, adminAuthn, adminCred, *assertion)
	if code, body := postJSON(t, adminClient, srv.URL+"/api/auth/v1/login/finish", map[string]any{
		"challenge": lb.Challenge, "credential": json.RawMessage(assertionResp),
	}); code != http.StatusOK {
		t.Fatalf("login/finish = %d: %s", code, body)
	}

	events, err := st.ListAuthEvents(context.Background())
	if err != nil {
		t.Fatalf("ListAuthEvents: %v", err)
	}
	want := []struct {
		action stores.AuthAction
		actor  string
		target string
	}{
		{stores.ActionEnrol, adminID, adminID},
		{stores.ActionInvite, adminID, ""},
		{stores.ActionEnrol, member.ID, member.ID},
		{stores.ActionRevoke, adminID, member.ID},
		{stores.ActionLogout, adminID, adminID},
		{stores.ActionLogin, adminID, adminID},
	}
	if len(events) != len(want) {
		t.Fatalf("auth events = %d, want %d: %+v", len(events), len(want), events)
	}
	for i, w := range want {
		e := events[i]
		if e.Action != w.action || e.ActorUserID != w.actor || e.TargetUserID != w.target {
			t.Errorf("event %d = {%s %s->%s}, want {%s %s->%s}",
				i, e.Action, e.ActorUserID, e.TargetUserID, w.action, w.actor, w.target)
		}
		if e.At.IsZero() {
			t.Errorf("event %d (%s) has no timestamp", i, e.Action)
		}
	}
	if events[0].IP == "" || events[0].UserAgent == "" {
		t.Errorf("first event missing request metadata: ip=%q ua=%q", events[0].IP, events[0].UserAgent)
	}
}

func TestAckRunbookRecordsEvent(t *testing.T) {
	srv, _, st := newTestServer(t)
	rp := testRP()
	authn := virtualwebauthn.NewAuthenticator()
	cred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	client, adminID := bootstrapAdmin(t, srv, rp, authn, cred)

	// Anonymous is refused, and an empty slug is rejected.
	if code, _ := postJSON(t, newClient(t), srv.URL+"/api/runbooks/v1/ack", map[string]string{"slug": "x"}); code != http.StatusUnauthorized {
		t.Fatalf("anonymous ack = %d, want 401", code)
	}
	if code, _ := postJSON(t, client, srv.URL+"/api/runbooks/v1/ack", map[string]string{"slug": "  "}); code != http.StatusBadRequest {
		t.Fatalf("empty slug = %d, want 400", code)
	}

	if code, body := postJSON(t, client, srv.URL+"/api/runbooks/v1/ack", map[string]string{"slug": "mts-deadlock-recovery"}); code != http.StatusOK {
		t.Fatalf("ack = %d: %s", code, body)
	}

	events, err := st.ListAuthEvents(context.Background())
	if err != nil {
		t.Fatalf("ListAuthEvents: %v", err)
	}
	var ack *stores.AuthEvent
	for i := range events {
		if events[i].Action == stores.ActionAck {
			ack = &events[i]
		}
	}
	if ack == nil {
		t.Fatalf("no ack event recorded: %+v", events)
	}
	if ack.ActorUserID != adminID || ack.Detail != "mts-deadlock-recovery" {
		t.Errorf("ack event = %+v, want actor %s detail mts-deadlock-recovery", *ack, adminID)
	}
}

// TestGitSyncSessionAttribution exercises the gated endpoint end-to-end: a
// signed-in user's commits carry their name/email, a plain member may sync (v1
// is instance-wide), and a revoked session gets 401.
func TestGitSyncSessionAttribution(t *testing.T) {
	repoURL := initTestRepo(t)
	srv, svc, st := newTestServerWith(t, func(cfg *config) {
		cfg.GitSyncRepo = repoURL
		cfg.GitSyncBranch = "main"
		cfg.GitSyncBasePath = "runs"
		cfg.GitSyncAuthorName = "Machine"
		cfg.GitSyncAuthorEmail = "machine@test.com"
		cfg.GitSyncUsername = "oauth2"
		cfg.GitSyncToken = "test-token"
		cfg.GitSyncEnabled = true
	})
	rp := testRP()
	adminAuthn := virtualwebauthn.NewAuthenticator()
	adminCred := virtualwebauthn.NewCredential(virtualwebauthn.KeyTypeEC2)
	admin, adminID := bootstrapAdmin(t, srv, rp, adminAuthn, adminCred)

	sync := func(client *http.Client) int {
		t.Helper()
		body, err := json.Marshal(gitSyncRequest{RunbookSlug: "session-sync", RunbookTitle: "Session Sync", Notes: "hi"})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Post(srv.URL+"/api/git-sync/v1", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("sync: %v", err)
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}

	if code := sync(admin); code != http.StatusOK {
		t.Fatalf("admin sync = %d, want 200", code)
	}
	clone := t.TempDir()
	mustGit(t, clone, "clone", repoURL, ".")
	if got, want := mustGitOutput(t, clone, "log", "-1", "--no-show-signature", "--format=%an <%ae>"), "Ben <ben@example.com>"; got != want {
		t.Errorf("commit author = %q, want %q", got, want)
	}

	// A member may sync too — v1 authorization is instance-wide, not per-runbook.
	raw, err := svc.CreateInvite(context.Background(), adminID, stores.RoleMember, "", time.Hour)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	member, _ := enrolInvite(t, srv, st, rp, raw, "Sam")
	if code := sync(member); code != http.StatusOK {
		t.Errorf("member sync = %d, want 200", code)
	}

	// Revoking the session closes the door: no session and no token → 401.
	if code, body := postJSON(t, admin, srv.URL+"/api/auth/v1/sessions/revoke", map[string]string{"user_id": adminID}); code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", code, body)
	}
	if code := sync(admin); code != http.StatusUnauthorized {
		t.Errorf("sync after revoke = %d, want 401", code)
	}
}
