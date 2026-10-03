// SPDX-License-Identifier: FSL-1.1-MIT

package contentsource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestOpenLocal(t *testing.T) {
	dir, reused, err := Open(context.Background(), Options{Source: "local", Dir: "content"})
	if err != nil {
		t.Fatalf("Open local: %v", err)
	}
	if dir != "content" {
		t.Fatalf("dir = %q, want content", dir)
	}
	if reused {
		t.Fatal("a local source is never reused from cache")
	}
}

func TestOpenDefaultsToLocal(t *testing.T) {
	dir, _, err := Open(context.Background(), Options{Dir: "content"})
	if err != nil {
		t.Fatalf("Open default: %v", err)
	}
	if dir != "content" {
		t.Fatalf("dir = %q, want content", dir)
	}
}

func TestOpenUnknownSource(t *testing.T) {
	if _, _, err := Open(context.Background(), Options{Source: "s3"}); err == nil {
		t.Fatal("want an error for an unknown source")
	}
}

func TestOpenGitRequiresRepo(t *testing.T) {
	if _, _, err := Open(context.Background(), Options{Source: "git"}); err == nil {
		t.Fatal("want an error when GITSYNC_REPO is unset")
	}
}

func TestRefreshRequiresLocalDir(t *testing.T) {
	if _, err := Refresh(context.Background(), Options{Source: "local"}); err == nil {
		t.Fatal("want an error when CONTENT_DIR is unset")
	}
}

func TestOpenGitClonesIntoCache(t *testing.T) {
	src := seedRepo(t, "runbook.md")

	cache := filepath.Join(t.TempDir(), "cache")
	dir, reused, err := Open(context.Background(), Options{
		Source: "git",
		Repo:   "file://" + src,
		Branch: "main",
		Cache:  cache,
	})
	if err != nil {
		t.Fatalf("Open git: %v", err)
	}
	if reused {
		t.Fatal("a fresh clone is not reused")
	}
	if _, err := os.Stat(filepath.Join(dir, "runbook.md")); err != nil {
		t.Fatalf("cloned content missing: %v", err)
	}
}

// TestOpenGitReusesCache pins the startup contract: an existing checkout is
// served without touching the remote, so an unreachable remote does not block
// boot.
func TestOpenGitReusesCache(t *testing.T) {
	src := seedRepo(t, "runbook.md")
	cache := filepath.Join(t.TempDir(), "cache")

	if _, _, err := Open(context.Background(), Options{
		Source: "git", Repo: "file://" + src, Branch: "main", Cache: cache,
	}); err != nil {
		t.Fatalf("first Open: %v", err)
	}

	dir, reused, err := Open(context.Background(), Options{
		Source: "git", Repo: "file:///nonexistent-repo", Branch: "main", Cache: cache,
	})
	if err != nil {
		t.Fatalf("Open must reuse the cache, got error: %v", err)
	}
	if !reused {
		t.Fatal("an existing cache must report reused")
	}
	if _, err := os.Stat(filepath.Join(dir, "runbook.md")); err != nil {
		t.Fatalf("cached content missing: %v", err)
	}
}

// TestRefreshGitKeepsLastGood pinpoints the failure mode: an unreachable remote
// after a good fetch reports the error but keeps the checkout.
func TestRefreshGitKeepsLastGood(t *testing.T) {
	src := seedRepo(t, "runbook.md")
	cache := filepath.Join(t.TempDir(), "cache")

	dir, err := Refresh(context.Background(), Options{
		Source: "git", Repo: "file://" + src, Branch: "main", Cache: cache,
	})
	if err != nil {
		t.Fatalf("Refresh clone: %v", err)
	}
	commitRepo(t, src, "second.md")
	dir, err = Refresh(context.Background(), Options{
		Source: "git", Repo: "file://" + src, Branch: "main", Cache: cache,
	})
	if err != nil {
		t.Fatalf("Refresh fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "second.md")); err != nil {
		t.Fatalf("fetch/reset did not update the checkout: %v", err)
	}

	if _, err := Refresh(context.Background(), Options{
		Source: "git", Repo: "file:///nonexistent-repo", Branch: "main", Cache: cache,
	}); err == nil {
		t.Fatal("want an error for an unreachable remote")
	}
	if _, err := os.Stat(filepath.Join(cache, "second.md")); err != nil {
		t.Fatalf("last-good content missing: %v", err)
	}
}

// TestOpenGitUsesConfiguredBranch pins that a non-default branch is honoured,
// not silently replaced by main.
func TestOpenGitUsesConfiguredBranch(t *testing.T) {
	src := seedRepo(t, "runbook.md")

	r, err := git.PlainOpen(src)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "feature.md"), []byte("---\ntitle: F\nslug: f\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add("feature.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("feature", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}

	dir, _, err := Open(context.Background(), Options{
		Source: "git",
		Repo:   "file://" + src,
		Branch: "feature",
		Cache:  filepath.Join(t.TempDir(), "cache"),
	})
	if err != nil {
		t.Fatalf("Open feature: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "feature.md")); err != nil {
		t.Fatalf("feature branch content missing: %v", err)
	}
}

// TestOpenGitDefaultsCacheAndPath pins the Cache and Path defaults: the default
// cache is data/content and an unset path is the repo root.
func TestOpenGitDefaultsCacheAndPath(t *testing.T) {
	src := seedRepo(t, "sub/runbook.md")
	t.Chdir(t.TempDir())

	dir, _, err := Open(context.Background(), Options{
		Source: "git",
		Repo:   "file://" + src,
		Branch: "main",
		Path:   "sub",
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	want := filepath.Join("data", "content", "sub")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "runbook.md")); err != nil {
		t.Fatalf("nested content missing: %v", err)
	}
}

// TestRefreshDefaultsCacheAndPath pins that Refresh applies the same defaults as
// Open, so a refresh of a defaulted git source lands on the same directory.
func TestRefreshDefaultsCacheAndPath(t *testing.T) {
	src := seedRepo(t, "runbook.md")
	t.Chdir(t.TempDir())

	dir, err := Refresh(context.Background(), Options{
		Source: "git",
		Repo:   "file://" + src,
		Branch: "main",
	})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	want := filepath.Join("data", "content")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "runbook.md")); err != nil {
		t.Fatalf("content missing: %v", err)
	}
}

// seedRepo creates a git repo with one runbook at rel, using go-git (no system
// git).
func seedRepo(t *testing.T, rel string) string {
	t.Helper()
	src := t.TempDir()
	r, err := git.PlainInit(src, false)
	if err != nil {
		t.Fatal(err)
	}
	// Unborn HEAD: point it at main so the first commit creates the branch.
	if err := r.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(src, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("---\ntitle: X\nslug: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(rel); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
	return src
}

// commitRepo adds and commits one more runbook to an existing repo.
func commitRepo(t *testing.T, src, rel string) {
	t.Helper()
	r, err := git.PlainOpen(src)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, rel), []byte("---\ntitle: Y\nslug: y\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(rel); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("second", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
}
