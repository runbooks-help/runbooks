package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/descope/virtualwebauthn"

	"runbooks/identity"
	"runbooks/stores"
	"runbooks/stores/sqlite"
)

const (
	testRPID   = "localhost"
	testOrigin = "http://localhost"
	testRPName = "Runbooks"
	testToken  = "test-bootstrap"
)

func testRP() virtualwebauthn.RelyingParty {
	return virtualwebauthn.RelyingParty{Name: testRPName, ID: testRPID, Origin: testOrigin}
}

func newTestServer(t *testing.T) (*httptest.Server, *identity.Service, stores.Store) {
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
		IdentitySecureCookies:  false,
		IdentitySessionTTL:     720 * time.Hour,
		IdentitySessionIdleTTL: 168 * time.Hour,
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
	mux.HandleFunc("/admin", a.requireAdmin(a.adminPage))
	mux.HandleFunc("/api/auth/v1/invites", a.requireAdminAPI(a.createInvite))
	mux.HandleFunc("/api/auth/v1/sessions/revoke", a.requireAdminAPI(a.revokeSessions))
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

	// Log out, then sign back in with the passkey.
	if code, _ := postJSON(t, client, srv.URL+"/api/auth/v1/logout", nil); code != http.StatusOK {
		t.Fatalf("logout = %d, want 200", code)
	}
	if code, _ := get(t, client, srv.URL+"/"); code != http.StatusFound {
		t.Fatalf("GET / after logout = %d, want 302", code)
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
	if code, _ := get(t, invited, srv.URL+"/invite/"+raw); code != http.StatusOK {
		t.Fatalf("GET /invite = %d, want 200", code)
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
