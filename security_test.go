// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"runbooks/views/components"
)

func TestSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	securityHeaders(next).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	res := rec.Result()

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "same-origin",
		"X-Frame-Options":        "DENY",
	}
	for k, v := range want {
		if got := res.Header.Get(k); got != v {
			t.Errorf("header %s = %q, want %q", k, got, v)
		}
	}
	if csp := res.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("Content-Security-Policy missing frame-ancestors: %q", csp)
	}
}

// TestCSPHashAuthorisesInlineThemeScript renders the shell and checks the CSP
// hash matches the actual inline script, so the pre-paint theme script is not
// blocked by our own policy.
func TestCSPHashAuthorisesInlineThemeScript(t *testing.T) {
	var buf bytes.Buffer
	if err := components.Shell("").Render(context.Background(), &buf); err != nil {
		t.Fatalf("render shell: %v", err)
	}
	html := buf.String()

	_, after, ok := strings.Cut(html, "<script>")
	if !ok {
		t.Fatal("no inline script in shell head")
	}
	rest := after
	before, _, ok := strings.Cut(rest, "</script>")
	if !ok {
		t.Fatal("unterminated inline script in shell head")
	}
	sum := sha256.Sum256([]byte(before))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"

	rec := httptest.NewRecorder()
	securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).
		ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, want) {
		t.Errorf("CSP does not authorise the inline theme script\nwant substring: %s\ngot: %s", want, csp)
	}
}
