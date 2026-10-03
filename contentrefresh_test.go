package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefreshDisabledWithoutToken(t *testing.T) {
	cs, err := newContentState(config{ContentSource: "local", ContentDir: t.TempDir()}, nil, nil)
	if err != nil {
		t.Fatalf("newContentState: %v", err)
	}
	if code := serveRefresh(cs, http.MethodPost, ""); code != http.StatusForbidden {
		t.Fatalf("refresh without a configured token = %d, want 403", code)
	}
}

func TestRefreshTokenReloads(t *testing.T) {
	dir := t.TempDir()
	writeRunbook(t, dir, "first.md", "First", "first")
	cs, err := newContentState(config{
		ContentSource:       "local",
		ContentDir:          dir,
		ContentRefreshToken: "secret",
	}, nil, nil)
	if err != nil {
		t.Fatalf("newContentState: %v", err)
	}

	if code := serveRefresh(cs, http.MethodPost, ""); code != http.StatusUnauthorized {
		t.Fatalf("refresh with no token = %d, want 401", code)
	}
	if code := serveRefresh(cs, http.MethodPost, "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("refresh with a wrong token = %d, want 401", code)
	}
	if code := serveRefresh(cs, http.MethodGet, "secret"); code != http.StatusMethodNotAllowed {
		t.Fatalf("GET refresh = %d, want 405", code)
	}
	if code := serve(cs, "/second"); code != http.StatusNotFound {
		t.Fatalf("/second before refresh = %d, want 404", code)
	}

	writeRunbook(t, dir, "second.md", "Second", "second")
	if code := serveRefresh(cs, http.MethodPost, "secret"); code != http.StatusOK {
		t.Fatalf("refresh = %d, want 200", code)
	}
	if code := serve(cs, "/second"); code != http.StatusOK {
		t.Fatalf("/second after refresh = %d, want 200", code)
	}
}

func serveRefresh(cs *contentState, method, token string) int {
	req := httptest.NewRequest(method, "/api/content/v1/refresh", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	cs.ServeHTTP(w, req)
	return w.Code
}
