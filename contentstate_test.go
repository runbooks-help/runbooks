package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"runbooks/contentsource"
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

// TestContentStateServesGitCacheWhenRemoteIsDown pins the startup order: with a
// populated cache, boot serves last-good even when the remote is unreachable
// (the background refresh then fails and is logged).
func TestContentStateServesGitCacheWhenRemoteIsDown(t *testing.T) {
	src := seedGitRepo(t)
	cache := filepath.Join(t.TempDir(), "cache")
	if _, _, err := contentsource.Open(context.Background(), contentsource.Options{
		Source: "git", Repo: "file://" + src, Branch: "main", Cache: cache,
	}); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	cs, err := newContentState(config{
		ContentSource:   "git",
		ContentDir:      "content",
		GitSyncRepo:     "file:///nonexistent-repo",
		GitSyncBranch:   "main",
		ContentGitPath:  ".",
		ContentGitCache: cache,
	}, nil, nil)
	if err != nil {
		t.Fatalf("boot with a cache must succeed: %v", err)
	}
	cs.bg.Wait()
	if code := serve(cs, "/first"); code != http.StatusOK {
		t.Fatalf("/first from cache = %d, want 200", code)
	}
}

// seedGitRepo creates a repo with one runbook on main, using go-git.
func seedGitRepo(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	r, err := git.PlainInit(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "first.md"), []byte("---\ntitle: First\nslug: first\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("first.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	return src
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
