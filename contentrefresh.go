package main

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"
)

// refreshHandler exposes a manual content refresh: reload the source and swap
// the snapshot. It is admin-only with identity on; otherwise it is gated by the
// shared CONTENT_REFRESH_TOKEN, and disabled when neither is configured.
func (cs *contentState) refreshHandler() http.HandlerFunc {
	refresh := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if err := cs.reload(); err != nil {
			log.Printf("content refresh failed: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "refresh failed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
	}

	if cs.authn != nil {
		return cs.authn.requireAdminAPI(refresh)
	}
	if cs.cfg.ContentRefreshToken != "" {
		return requireBearer(refresh, cs.cfg.ContentRefreshToken)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSONError(w, http.StatusForbidden, "content refresh not configured")
	}
}

// requireBearer gates next behind a shared bearer token, compared in constant
// time.
func requireBearer(next http.HandlerFunc, token string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		got := strings.TrimPrefix(auth, bearerPrefix)
		if !strings.HasPrefix(auth, bearerPrefix) || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}
