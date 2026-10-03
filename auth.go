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
	"slices"
	"strconv"
	"strings"
	"time"

	"runbooks/identity"
	"runbooks/parser"
	"runbooks/stores"
	"runbooks/views"
)

const sessionCookieName = "runbooks_session"

// defaultInviteTTL is how long a new invite link lives when the caller does not
// set one.
const defaultInviteTTL = 72 * time.Hour

// auditPageSize is how many events the admin audit page shows at a time.
const auditPageSize = 50

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
	if a.adminExists(r.Context()) {
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
	a.recordAuthEvent(r, stores.ActionLogin, user.ID, user.ID)
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
	if a.adminExists(r.Context()) {
		writeJSONError(w, http.StatusConflict, "already set up")
		return
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		writeJSONError(w, http.StatusBadRequest, "display name is required")
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	email := strings.TrimSpace(req.Email)
	user, pending := a.pendingAdmin(r.Context())
	if pending {
		// Re-use the orphan of an abandoned /setup, refreshing the details the
		// operator just typed, so credential-less admin rows do not accumulate.
		user.DisplayName = displayName
		user.Email = email
		if err := a.st.UpdateUser(r.Context(), user); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not update user")
			return
		}
	} else {
		user = stores.User{
			ID:          newUserID(),
			Email:       email,
			DisplayName: displayName,
			Role:        stores.RoleAdmin,
			CreatedAt:   time.Now(),
		}
		if err := a.st.InsertUser(r.Context(), user); err != nil {
			writeJSONError(w, http.StatusInternalServerError, "could not create user")
			return
		}
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
	cred, err := a.svc.FinishRegistration(r.Context(), req.Challenge, req.Credential)
	if err != nil {
		logAuthFailure("setup/finish", err)
		writeJSONError(w, http.StatusBadRequest, "registration failed")
		return
	}
	raw, err := a.svc.Create(r.Context(), cred.UserID, r.UserAgent(), clientIP(r))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	a.setSession(w, raw)
	a.recordAuthEvent(r, stores.ActionEnrol, cred.UserID, cred.UserID)
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
	cred, err := a.svc.FinishRegistration(r.Context(), req.Challenge, req.Credential)
	if err != nil {
		logAuthFailure("invite/finish", err)
		writeJSONError(w, http.StatusBadRequest, "registration failed")
		return
	}
	if err := a.svc.UseInvite(r.Context(), inv); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not accept invite")
		return
	}
	raw, err := a.svc.Create(r.Context(), cred.UserID, r.UserAgent(), clientIP(r))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	a.setSession(w, raw)
	a.recordAuthEvent(r, stores.ActionEnrol, cred.UserID, cred.UserID)
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
	_, ok, err := a.soleAdmin(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not look up admins")
		return
	}
	if !ok {
		writeJSONError(w, http.StatusConflict, "recovery is for an instance with a single admin")
		return
	}
	cred, err := a.svc.FinishRegistration(r.Context(), req.Challenge, req.Credential)
	if err != nil {
		logAuthFailure("recovery/finish", err)
		writeJSONError(w, http.StatusBadRequest, "registration failed")
		return
	}
	raw, err := a.svc.Create(r.Context(), cred.UserID, r.UserAgent(), clientIP(r))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not start session")
		return
	}
	a.setSession(w, raw)
	a.recordAuthEvent(r, stores.ActionEnrol, cred.UserID, cred.UserID)
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
	a.recordAuthEvent(r, stores.ActionInvite, admin.ID, userID)
	writeJSON(w, http.StatusOK, map[string]any{
		"url":        a.cfg.IdentityPublicURL + "/invite/" + raw,
		"token":      raw,
		"expires_at": time.Now().Add(ttl),
	})
}

// revokeSessions ends sessions: an admin ends every session a user holds (lost
// device, offboarding) by user_id, and anyone may end one of their own by
// session_id. A member can never touch another user's sessions.
func (a *auth) revokeSessions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID    string `json:"user_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	me := userFrom(r.Context())
	userID := strings.TrimSpace(req.UserID)
	sessionID := strings.TrimSpace(req.SessionID)

	if userID != "" {
		if userID != me.ID && !me.IsAdmin() {
			writeJSONError(w, http.StatusForbidden, "admin only")
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
		a.recordAuthEvent(r, stores.ActionRevoke, me.ID, userID)
		writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
		return
	}

	if sessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "user_id or session_id is required")
		return
	}
	if err := a.svc.RevokeSession(r.Context(), me.ID, sessionID); err != nil {
		if errors.Is(err, identity.ErrSession) {
			writeJSONError(w, http.StatusNotFound, "unknown session")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not revoke session")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// accountPage shows the signed-in user's profile, passkeys and sessions.
func (a *auth) accountPage(groups []parser.SystemGroup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r.Context())
		creds, err := a.svc.Credentials(r.Context(), u.ID)
		if err != nil {
			http.Error(w, "could not load passkeys", http.StatusInternalServerError)
			return
		}
		sessions, err := a.svc.Sessions(r.Context(), u.ID)
		if err != nil {
			http.Error(w, "could not load sessions", http.StatusInternalServerError)
			return
		}
		current := ""
		if c, err := r.Cookie(sessionCookieName); err == nil {
			current = identity.SessionID(c.Value)
		}
		views.AccountPage(groups, u, creds, sessions, current).Render(r.Context(), w)
	}
}

// auditPage renders the append-only auth_events feed, newest first. The store
// returns oldest-first; the page reverses and slices it. No row is ever edited
// or deleted here.
func (a *auth) auditPage(groups []parser.SystemGroup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := a.st.ListAuthEvents(r.Context())
		if err != nil {
			http.Error(w, "could not load audit events", http.StatusInternalServerError)
			return
		}
		slices.Reverse(events)

		page := 1
		if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
			page = p
		}
		pageCount := (len(events) + auditPageSize - 1) / auditPageSize
		if pageCount == 0 {
			pageCount = 1
		}
		if page > pageCount {
			page = pageCount
		}
		start := (page - 1) * auditPageSize
		pageEvents := events[start:min(start+auditPageSize, len(events))]

		users, err := a.st.ListUsers(r.Context())
		if err != nil {
			http.Error(w, "could not load users", http.StatusInternalServerError)
			return
		}
		names := make(map[string]string, len(users))
		for _, u := range users {
			names[u.ID] = u.DisplayName
		}

		views.AuditPage(groups, pageEvents, names, page, pageCount).Render(r.Context(), w)
	}
}

// listSessions returns the signed-in user's own sessions as JSON.
func (a *auth) listSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	u := userFrom(r.Context())
	sessions, err := a.svc.Sessions(r.Context(), u.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not load sessions")
		return
	}
	current := ""
	if c, err := r.Cookie(sessionCookieName); err == nil {
		current = identity.SessionID(c.Value)
	}
	out := make([]map[string]any, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, map[string]any{
			"id":           s.ID,
			"created_at":   s.CreatedAt,
			"last_seen_at": s.LastSeenAt,
			"user_agent":   s.UserAgent,
			"ip":           s.IP,
			"current":      s.ID == current,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

// passkeyBegin starts adding a passkey to the signed-in user.
func (a *auth) passkeyBegin(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	token, options, err := a.svc.BeginRegistration(r.Context(), u.ID)
	if err != nil {
		logAuthFailure("account/passkey/begin", err)
		writeJSONError(w, http.StatusInternalServerError, "could not begin registration")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"challenge": token, "options": options})
}

// passkeyFinish stores the new passkey, optionally naming it.
func (a *auth) passkeyFinish(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Challenge  string          `json:"challenge"`
		Credential json.RawMessage `json:"credential"`
		Label      string          `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	cred, err := a.svc.FinishRegistration(r.Context(), req.Challenge, req.Credential)
	if err != nil {
		logAuthFailure("account/passkey/finish", err)
		writeJSONError(w, http.StatusBadRequest, "registration failed")
		return
	}
	if label := strings.TrimSpace(req.Label); label != "" {
		_ = a.svc.RenameCredential(r.Context(), cred.UserID, cred.ID, label)
	}
	a.recordAuthEvent(r, stores.ActionEnrol, cred.UserID, cred.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// passkeyRename sets a passkey's label.
func (a *auth) passkeyRename(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var req struct {
		CredentialID string `json:"credential_id"`
		Label        string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.CredentialID) == "" {
		writeJSONError(w, http.StatusBadRequest, "credential_id is required")
		return
	}
	if err := a.svc.RenameCredential(r.Context(), u.ID, req.CredentialID, req.Label); err != nil {
		if errors.Is(err, identity.ErrCredential) {
			writeJSONError(w, http.StatusNotFound, "unknown passkey")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not rename passkey")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// passkeyRemove deletes a passkey, refusing to remove the user's last one.
func (a *auth) passkeyRemove(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	var req struct {
		CredentialID string `json:"credential_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if err := a.svc.RemoveCredential(r.Context(), u.ID, strings.TrimSpace(req.CredentialID)); err != nil {
		switch {
		case errors.Is(err, identity.ErrLastPasskey):
			writeJSONError(w, http.StatusConflict, "You cannot remove your last passkey.")
		case errors.Is(err, identity.ErrCredential):
			writeJSONError(w, http.StatusNotFound, "unknown passkey")
		default:
			writeJSONError(w, http.StatusInternalServerError, "could not remove passkey")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// updateProfile changes the signed-in user's display name and optional email.
func (a *auth) updateProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	u := userFrom(r.Context())
	var req struct {
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		writeJSONError(w, http.StatusBadRequest, "display name is required")
		return
	}
	updated, err := a.svc.UpdateProfile(r.Context(), u.ID, req.DisplayName, req.Email)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "could not save profile")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"display_name": updated.DisplayName, "email": updated.Email})
}

// disableUser blocks a user from signing in and ends their sessions.
func (a *auth) disableUser(w http.ResponseWriter, r *http.Request) {
	a.setUserEnabled(w, r, false)
}

// enableUser restores a disabled user's ability to sign in.
func (a *auth) enableUser(w http.ResponseWriter, r *http.Request) {
	a.setUserEnabled(w, r, true)
}

// setUserEnabled is the shared body of disable/enable. The last-admin guard
// lives in the service; the transition is audited either way.
func (a *auth) setUserEnabled(w http.ResponseWriter, r *http.Request, enabled bool) {
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
	action := stores.ActionDisable
	var err error
	if enabled {
		action = stores.ActionEnable
		err = a.svc.EnableUser(r.Context(), userID)
	} else {
		err = a.svc.DisableUser(r.Context(), userID)
	}
	switch {
	case errors.Is(err, identity.ErrLastAdmin):
		writeJSONError(w, http.StatusConflict, "cannot disable the last admin")
		return
	case errors.Is(err, identity.ErrUser):
		writeJSONError(w, http.StatusNotFound, "unknown user")
		return
	case err != nil:
		writeJSONError(w, http.StatusInternalServerError, "could not update user")
		return
	}
	a.recordAuthEvent(r, action, userFrom(r.Context()).ID, userID)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (a *auth) logout(w http.ResponseWriter, r *http.Request) {
	if u, ok := a.sessionUser(r); ok {
		a.recordAuthEvent(r, stores.ActionLogout, u.ID, u.ID)
	}
	if c, err := r.Cookie(sessionCookieName); err == nil {
		_ = a.svc.Revoke(r.Context(), c.Value)
	}
	a.clearSession(w)
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// recordAuthEvent appends an audit row, best-effort. A failed insert is logged
// and never fails the request: the state change it describes has already
// happened.
func (a *auth) recordAuthEvent(r *http.Request, action stores.AuthAction, actorID, targetID string) {
	e := stores.AuthEvent{
		At:           time.Now().UTC(),
		ActorUserID:  actorID,
		Action:       action,
		TargetUserID: targetID,
		IP:           clientIP(r),
		UserAgent:    r.UserAgent(),
	}
	if err := a.st.InsertAuthEvent(r.Context(), e); err != nil {
		logAuthFailure("auth/event "+string(action), err)
	}
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

// adminExists reports whether the instance already has an enabled admin who can
// log in — an admin holding at least one passkey. An admin row with no credential
// is the orphan of an abandoned /setup and must not close bootstrap.
func (a *auth) adminExists(ctx context.Context) bool {
	users, err := a.st.ListUsers(ctx)
	if err != nil {
		return false
	}
	for _, u := range users {
		if u.Role != stores.RoleAdmin || !u.Enabled() {
			continue
		}
		creds, err := a.st.ListCredentials(ctx, u.ID)
		if err == nil && len(creds) > 0 {
			return true
		}
	}
	return false
}

// pendingAdmin returns an enabled admin that holds no credential — the orphan of
// an abandoned /setup, which the next setup/begin re-uses rather than duplicating.
func (a *auth) pendingAdmin(ctx context.Context) (stores.User, bool) {
	users, err := a.st.ListUsers(ctx)
	if err != nil {
		return stores.User{}, false
	}
	for _, u := range users {
		if u.Role != stores.RoleAdmin || !u.Enabled() {
			continue
		}
		creds, err := a.st.ListCredentials(ctx, u.ID)
		if err != nil {
			continue
		}
		if len(creds) == 0 {
			return u, true
		}
	}
	return stores.User{}, false
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

// requireAPI requires any authenticated user for a JSON endpoint, answering 401
// rather than redirecting. Used for the runbook acknowledgement audit.
func (a *auth) requireAPI(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.resolveUser(r)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "sign in required")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey{}, u)))
	}
}

// ackRunbook records that a reader acknowledged a destructive runbook, as an
// audit event attributed to the signed-in user. The slug is the event's detail.
func (a *auth) ackRunbook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad request")
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		writeJSONError(w, http.StatusBadRequest, "slug is required")
		return
	}
	u := userFrom(r.Context())
	e := stores.AuthEvent{
		At:          time.Now().UTC(),
		ActorUserID: u.ID,
		Action:      stores.ActionAck,
		Detail:      slug,
		IP:          clientIP(r),
		UserAgent:   r.UserAgent(),
	}
	if err := a.st.InsertAuthEvent(r.Context(), e); err != nil {
		logAuthFailure("ack/record", err)
		writeJSONError(w, http.StatusInternalServerError, "could not record")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
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
