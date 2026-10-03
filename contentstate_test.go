package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestContentStateReload pins the rebuild-and-swap: a reload serves slugs added
// since startup, without a restart.
func TestContentStateReload(t *testing.T) {
	dir := t.TempDir()
	writeRunbook(t, dir, "first.md", "First", "first")

	cs, err := newContentState(config{ContentSource: "local", ContentDir: dir}, nil, nil)
	if err != nil {
		t.Fatalf("newContentState: %v", err)
	}
	if code := serve(cs, "/first"); code != http.StatusOK {
		t.Fatalf("/first = %d, want 200", code)
	}
	if code := serve(cs, "/second"); code != http.StatusNotFound {
		t.Fatalf("/second before reload = %d, want 404", code)
	}
	if code := serve(cs, "/first.md"); code != http.StatusOK {
		t.Fatalf("/first.md = %d, want 200", code)
	}

	writeRunbook(t, dir, "second.md", "Second", "second")
	if err := cs.reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if code := serve(cs, "/second"); code != http.StatusOK {
		t.Fatalf("/second after reload = %d, want 200", code)
	}
	if code := serve(cs, "/second.md"); code != http.StatusOK {
		t.Fatalf("/second.md after reload = %d, want 200", code)
	}
}

func serve(h http.Handler, path string) int {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w.Code
}

func writeRunbook(t *testing.T, dir, name, title, slug string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "system"), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "---\ntitle: " + title + "\nslug: " + slug + "\n---\n\n## Step\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "system", name), []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}
