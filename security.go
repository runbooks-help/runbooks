package main

import (
	"net/http"

	"runbooks/views/components"
)

// healthzHandler is a liveness probe: unauthenticated and information-free, so a
// container or load balancer can probe an identity-gated instance.
func healthzHandler(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("ok"))
}

// securityHeaders sets the baseline response headers. The CSP permits only the
// one inline script the shell needs — the pre-paint theme script — by hash; the
// inline JSON blocks (runbook source, page config) are data, not scripts, so
// script-src does not apply to them.
func securityHeaders(next http.Handler) http.Handler {
	csp := "default-src 'self'; " +
		"script-src 'self' 'sha256-" + components.ThemeScriptCSPHash() + "'; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob:; " +
		"font-src 'self'; " +
		"connect-src 'self'; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'"

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}
