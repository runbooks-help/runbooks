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

func newTestServer(t *testing.T) *httptest.Server {
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
	mux.HandleFunc("/api/auth/v1/login/begin", a.loginBegin)
	mux.HandleFunc("/api/auth/v1/login/finish", a.loginFinish)
	mux.HandleFunc("/api/auth/v1/setup/begin", a.setupBegin)
	mux.HandleFunc("/api/auth/v1/setup/finish", a.setupFinish)
	mux.HandleFunc("/api/auth/v1/logout", a.logout)
	mux.HandleFunc("/{$}", a.requirePage(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "index")
	}))

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
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
	srv := newTestServer(t)
	rp := testRP()
	authn := virtualwebauthn.NewAuthenticator()
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
