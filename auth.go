package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"runbooks/identity"
	"runbooks/stores"
	"runbooks/views"
)

const sessionCookieName = "runbooks_session"

// defaultInviteTTL is how long a new invite link lives when the caller does not
// set one.
const defaultInviteTTL = 72 * time.Hour

// sessionAuthKey marks a git-sync request as authorised by a user session, so
// the handler skips its shared-token check.
type sessionAuthKey struct{}

func sessionAuthorized(ctx context.Context) bool {
	v, _ := ctx.Value(sessionAuthKey{}).(bool)
	return v
}

// userKey carries the authenticated user through a gated request, so a handler
// does not resolve the session a second time.
type userKey struct{}

// userFrom returns the user stored by requirePage or requireAdmin, or the zero
// user when the request was not gated.
func userFrom(ctx context.Context) stores.User {
	if u, ok := ctx.Value(userKey{}).(stores.User); ok {
		return u
	}
	return stores.User{}
}

// auth wires the passkey service to HTTP: the setup/login pages, the JSON
// ceremony endpoints and the session cookie.
type auth struct {
	svc *identity.Service
	st  stores.Store
	cfg config
}

func newAuth(svc *identity.Service, st stores.Store, cfg config) *auth {
	return &auth{svc: svc, st: st, cfg: cfg}
}

func (a *auth) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.resolveUser(r); ok {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	views.LoginPage().Render(r.Context(), w)
}

func (a *auth) setupPage(w http.ResponseWriter, r *http.Request) {
	if a.adminExists(r) {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	views.SetupPage().Render(r.Context(), w)
}

func (a *auth) loginBegin(w http.ResponseWriter, r *http.Request) {
	token, options, err := a.svc.BeginDiscoverableLogin(r.Context())
	if err != nil {
		logAuthFailure("login/begin", err)
		writeJSONError(w, http.StatusInternalServerError, "could not begin login")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challenge": token, "options": options})
}

func (a *auth) loginFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	raw, user, err := a.svc.FinishLogin(r.Context(), req.Challenge, req.Credential, r.UserAgent(), clientIP(r))
	if err != nil {
		logAuthFailure("login/finish", err)
		writeJSONError(w, http.StatusUnauthorized, "sign in failed")
		return
	}
	a.setSession(w, raw)
	writeJSON(w, http.StatusOK, map[string]string{"id": user.ID, "displayName": user.DisplayName})
}

func (a *auth) setupBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !a.validBootstrap(req.Token) {
		writeJSONError(w, http.StatusUnauthorized, "invalid bootstrap token")
		return
	}
	if a.adminExists(r) {
		writeJSONError(w, http.StatusConflict, "already set up")
		return
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		writeJSONError(w, http.StatusBadRequest, "display name is required")
		return
	}

	user := stores.User{
		ID:          newUserID(),
		Email:       strings.TrimSpace(req.Email),
		DisplayName: strings.TrimSpace(req.DisplayName),
		Role:        stores.RoleAdmin,
		CreatedAt:   time.Now(),
	}
	if err := a.st.InsertUser(r.Context(), user); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not create user")
		return
	}

	token, options, err := a.svc.BeginRegistration(r.Context(), user.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not begin registration")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user_id": user.ID, "challenge": token, "options": options})
}

func (a *auth) setupFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token      string          `json:"token"`
		UserID     string          `json:"user_id"`
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !a.validBootstrap(req.Token) {
		writeJSONError(w, http.StatusUnauthorized, "invalid bootstrap token")
		return
	}
	if _, err := a.svc.FinishRegistration(r.Context(), req.UserID, req.Challenge, req.Credential); err != nil {
		logAuthFailure("setup/finish", err)
		writeJSONError(w, http.StatusBadRequest, "registration failed")
		return
	}
	raw, err := a.svc.Create(r.Context(), req.UserID, r.UserAgent(), clientIP(r))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	a.setSession(w, raw)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *auth) invitePage(w http.ResponseWriter, r *http.Request) {
	inv, err := a.svc.Invite(r.Context(), r.PathValue("token"))
	if err != nil {
		views.InviteInvalidPage().Render(r.Context(), w)
		return
	}
	views.InvitePage(r.PathValue("token"), inv.UserID != "").Render(r.Context(), w)
}

// inviteBegin validates an invite and starts the passkey ceremony. For a
// new-user invite it creates the user and binds it to the invite, so the finish
// step cannot be pointed at a different account.
func (a *auth) inviteBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	inv, err := a.svc.Invite(r.Context(), req.Token)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invite is invalid or expired")
		return
	}

	if inv.UserID == "" {
		if strings.TrimSpace(req.DisplayName) == "" {
			writeJSONError(w, http.StatusBadRequest, "display name is required")
			return
		}
		user := stores.User{
			ID:          newUserID(),
			Email:       strings.TrimSpace(req.Email),
			DisplayName: strings.TrimSpace(req.DisplayName),
			Role:        inv.Role,
			CreatedAt:   time.Now(),
		}
		if err := a.st.InsertUser(r.Context(), user); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not create user")
			return
		}
		inv.UserID = user.ID
		if err := a.st.UpdateInvite(r.Context(), inv); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not accept invite")
			return
		}
	}

	token, options, err := a.svc.BeginRegistration(r.Context(), inv.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not begin registration")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challenge": token, "options": options})
}

func (a *auth) inviteFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token      string          `json:"token"`
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	inv, err := a.svc.Invite(r.Context(), req.Token)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invite is invalid or expired")
		return
	}
	if inv.UserID == "" {
		writeJSONError(w, http.StatusConflict, "invite has not been started")
		return
	}
	if _, err := a.svc.FinishRegistration(r.Context(), inv.UserID, req.Challenge, req.Credential); err != nil {
		logAuthFailure("invite/finish", err)
		writeJSONError(w, http.StatusBadRequest, "registration failed")
		return
	}
	if err := a.svc.UseInvite(r.Context(), inv); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not accept invite")
		return
	}
	raw, err := a.svc.Create(r.Context(), inv.UserID, r.UserAgent(), clientIP(r))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	a.setSession(w, raw)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *auth) recoveryPage(w http.ResponseWriter, r *http.Request) {
	views.RecoveryPage().Render(r.Context(), w)
}

// recoveryBegin starts a passkey ceremony for the instance's sole admin, guarded
// by IDENTITY_RECOVERY_TOKEN. The target is derived server-side, never trusted
// from the client.
func (a *auth) recoveryBegin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !a.validRecovery(req.Token) {
		writeJSONError(w, http.StatusUnauthorized, "invalid recovery token")
		return
	}
	admin, ok, err := a.soleAdmin(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not look up admins")
		return
	}
	if !ok {
		writeJSONError(w, http.StatusConflict, "recovery is for an instance with a single admin")
		return
	}
	token, options, err := a.svc.BeginRegistration(r.Context(), admin.ID)
	if err != nil {
		logAuthFailure("recovery/begin", err)
		writeJSONError(w, http.StatusInternalServerError, "could not begin registration")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challenge": token, "options": options})
}

func (a *auth) recoveryFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token      string          `json:"token"`
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !a.validRecovery(req.Token) {
		writeJSONError(w, http.StatusUnauthorized, "invalid recovery token")
		return
	}
	admin, ok, err := a.soleAdmin(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not look up admins")
		return
	}
	if !ok {
		writeJSONError(w, http.StatusConflict, "recovery is for an instance with a single admin")
		return
	}
	if _, err := a.svc.FinishRegistration(r.Context(), admin.ID, req.Challenge, req.Credential); err != nil {
		logAuthFailure("recovery/finish", err)
		writeJSONError(w, http.StatusBadRequest, "registration failed")
		return
	}
	raw, err := a.svc.Create(r.Context(), admin.ID, r.UserAgent(), clientIP(r))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	a.setSession(w, raw)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// validRecovery reports whether the break-glass token matches the configured
// one. An unset token disables recovery entirely (the routes are not registered).
func (a *auth) validRecovery(token string) bool {
	if a.cfg.IdentityRecoveryToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.cfg.IdentityRecoveryToken)) == 1
}

// soleAdmin returns the only enabled admin. Break-glass recovery is for a sole
// admin with no other admin to help; with none or several the operator has
// another way in.
func (a *auth) soleAdmin(ctx context.Context) (stores.User, bool, error) {
	users, err := a.st.ListUsers(ctx)
	if err != nil {
		return stores.User{}, false, err
	}
	var admins []stores.User
	for _, u := range users {
		if u.IsAdmin() && u.Enabled() {
			admins = append(admins, u)
		}
	}
	if len(admins) != 1 {
		return stores.User{}, false, nil
	}
	return admins[0], true, nil
}

// createInvite mints an invite for a new user, or a re-enrolment link when
// user_id is set. It returns the absolute URL to share.
func (a *auth) createInvite(w http.ResponseWriter, r *http.Request) {
	admin := userFrom(r.Context())
	var req struct {
		Role   string `json:"role"`
		UserID string `json:"user_id"`
		TTL    string `json:"ttl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}

	role := stores.Role(req.Role)
	if role == "" {
		role = stores.RoleMember
	}
	if role != stores.RoleMember && role != stores.RoleAdmin {
		writeJSONError(w, http.StatusBadRequest, "unknown role")
		return
	}

	ttl := defaultInviteTTL
	if req.TTL != "" {
		d, err := time.ParseDuration(req.TTL)
		if err != nil || d <= 0 {
			writeJSONError(w, http.StatusBadRequest, "invalid expiry")
			return
		}
		ttl = d
	}

	userID := strings.TrimSpace(req.UserID)
	if userID != "" {
		u, err := a.st.GetUser(r.Context(), userID)
		if err != nil {
			if errors.Is(err, stores.ErrNotFound) {
				writeJSONError(w, http.StatusNotFound, "unknown user")
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "could not look up user")
			return
		}
		// A bound invite re-enrols an existing user; the invite's role is unused.
		role = u.Role
	}

	raw, err := a.svc.CreateInvite(r.Context(), admin.ID, role, userID, ttl)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not create invite")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        a.cfg.IdentityPublicURL + "/invite/" + raw,
		"token":      raw,
		"expires_at": time.Now().Add(ttl),
	})
}

// revokeSessions ends every session a user holds (lost device, offboarding).
func (a *auth) revokeSessions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	userID := strings.TrimSpace(req.UserID)
	if userID == "" {
		writeJSONError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	if _, err := a.st.GetUser(r.Context(), userID); err != nil {
		if errors.Is(err, stores.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "unknown user")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not look up user")
		return
	}
	if err := a.svc.RevokeForUser(r.Context(), userID); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not revoke sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *auth) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = a.svc.Revoke(r.Context(), c.Value)
	}
	a.clearSession(w)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// sessionUser resolves the session cookie to a user.
func (a *auth) sessionUser(r *http.Request) (stores.User, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return stores.User{}, false
	}
	u, err := a.svc.Authenticate(r.Context(), c.Value)
	if err != nil {
		return stores.User{}, false
	}
	return u, true
}

// proxyUser resolves an identity asserted by the upstream proxy, provisioning a
// member on first sight. It returns false when delegation is off or the identity
// header is absent or empty, so a request with neither a session nor an
// assertion is simply unauthenticated. This is a per-request assertion, not a
// session: no cookie is set and no session row is written.
func (a *auth) proxyUser(r *http.Request) (stores.User, bool) {
	if !a.cfg.IdentityTrustProxyAuth {
		return stores.User{}, false
	}
	header := a.cfg.IdentityProxyUserHeader
	if header == "" {
		header = "Auth-Request-Email"
	}
	email := strings.TrimSpace(r.Header.Get(header))
	if email == "" {
		return stores.User{}, false
	}
	u, err := a.svc.ProvisionProxyUser(r.Context(), email, r.Header.Get(a.cfg.IdentityProxyNameHeader))
	if err != nil {
		logAuthFailure("proxy/provision", err)
		return stores.User{}, false
	}
	return u, true
}

// resolveUser is the one authentication rule for every gate: a session cookie
// first, then an upstream proxy assertion. A failure to provision the asserted
// identity is logged and treated as unauthenticated, never surfaced.
func (a *auth) resolveUser(r *http.Request) (stores.User, bool) {
	if u, ok := a.sessionUser(r); ok {
		return u, true
	}
	return a.proxyUser(r)
}

func (a *auth) setSession(w http.ResponseWriter, raw string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cfg.IdentitySecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(a.cfg.IdentitySessionTTL.Seconds()),
	})
}

func (a *auth) clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cfg.IdentitySecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// adminExists reports whether the instance already has an enabled admin, so
// bootstrap is closed.
func (a *auth) adminExists(r *http.Request) bool {
	users, err := a.st.ListUsers(r.Context())
	if err != nil {
		return false
	}
	for _, u := range users {
		if u.Role == stores.RoleAdmin && u.Enabled() {
			return true
		}
	}
	return false
}

func (a *auth) validBootstrap(token string) bool {
	if a.cfg.IdentityBootstrapToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.cfg.IdentityBootstrapToken)) == 1
}

// requirePage redirects an unauthenticated request to the login page.
func (a *auth) requirePage(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.resolveUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	}
}

// requireAdmin gates an authenticated page to admins.
func (a *auth) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.resolveUser(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		if !u.IsAdmin() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	}
}

// requireAdminAPI gates an authenticated JSON endpoint to admins, answering 401
// and 403 rather than redirecting.
func (a *auth) requireAdminAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.resolveUser(r)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		if !u.IsAdmin() {
			writeJSONError(w, http.StatusForbidden, "admin only")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	}
}

// gateGitSync marks an authenticated git-sync request so the handler skips its
// shared-token check, and carries the resolved user so a commit can be
// attributed per user. Anything else falls through to the handler's own token
// rules (the CI path).
func (a *auth) gateGitSync(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, ok := a.resolveUser(r); ok {
			r = r.WithContext(context.WithValue(r.Context(), sessionAuthKey{}, true))
			r = r.WithContext(context.WithValue(r.Context(), userKey{}, u))
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// logAuthFailure records a ceremony failure server-side. The client only ever
// gets a generic message, so this is where the real reason is visible.
func logAuthFailure(step string, err error) {
	log.Printf("identity: %s: %v", step, err)
}

// newUserID returns a random id for a new user.
func newUserID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// clientIP is the request's remote host, for the session record.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
