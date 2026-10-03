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

func TestResolveLocal(t *testing.T) {
	dir, err := Resolve(context.Background(), Options{Source: "local", Dir: "content"})
	if err != nil {
		t.Fatalf("Resolve local: %v", err)
	}
	if dir != "content" {
		t.Fatalf("dir = %q, want content", dir)
	}
}

func TestResolveDefaultsToLocal(t *testing.T) {
	dir, err := Resolve(context.Background(), Options{Dir: "content"})
	if err != nil {
		t.Fatalf("Resolve default: %v", err)
	}
	if dir != "content" {
		t.Fatalf("dir = %q, want content", dir)
	}
}

func TestResolveUnknownSource(t *testing.T) {
	if _, err := Resolve(context.Background(), Options{Source: "s3"}); err == nil {
		t.Fatal("want an error for an unknown source")
	}
}

func TestResolveGitRequiresRepo(t *testing.T) {
	if _, err := Resolve(context.Background(), Options{Source: "git"}); err == nil {
		t.Fatal("want an error when GITSYNC_REPO is unset")
	}
}

func TestResolveGitClonesIntoCache(t *testing.T) {
	src := seedRepo(t, "runbook.md")

	cache := filepath.Join(t.TempDir(), "cache")
	dir, err := Resolve(context.Background(), Options{
		Source: "git",
		Repo:   "file://" + src,
		Branch: "main",
		Cache:  cache,
	})
	if err != nil {
		t.Fatalf("Resolve git: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "runbook.md")); err != nil {
		t.Fatalf("cloned content missing: %v", err)
	}
}

// TestResolveGitKeepsLastGood pinpoints the failure mode: an unreachable remote
// with an existing checkout must not fail startup.
func TestResolveGitKeepsLastGood(t *testing.T) {
	src := seedRepo(t, "runbook.md")
	cache := filepath.Join(t.TempDir(), "cache")

	if _, err := Resolve(context.Background(), Options{
		Source: "git", Repo: "file://" + src, Branch: "main", Cache: cache,
	}); err != nil {
		t.Fatalf("first Resolve: %v", err)
	}

	dir, err := Resolve(context.Background(), Options{
		Source: "git", Repo: "file:///nonexistent-repo", Branch: "main", Cache: cache,
	})
	if err != nil {
		t.Fatalf("want the last-good checkout, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "runbook.md")); err != nil {
		t.Fatalf("last-good content missing: %v", err)
	}
}

// TestResolveGitUsesConfiguredBranch pins that a non-default branch is honoured,
// not silently replaced by main.
func TestResolveGitUsesConfiguredBranch(t *testing.T) {
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

	dir, err := Resolve(context.Background(), Options{
		Source: "git",
		Repo:   "file://" + src,
		Branch: "feature",
		Cache:  filepath.Join(t.TempDir(), "cache"),
	})
	if err != nil {
		t.Fatalf("Resolve feature: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "feature.md")); err != nil {
		t.Fatalf("feature branch content missing: %v", err)
	}
}

// TestResolveGitDefaultsCacheAndPath pins the Cache and Path defaults: the
// default cache is data/content and an unset path is the repo root.
func TestResolveGitDefaultsCacheAndPath(t *testing.T) {
	src := seedRepo(t, "sub/runbook.md")
	t.Chdir(t.TempDir())

	dir, err := Resolve(context.Background(), Options{
		Source: "git",
		Repo:   "file://" + src,
		Branch: "main",
		Path:   "sub",
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join("data", "content", "sub")
	if dir != want {
		t.Fatalf("dir = %q, want %q", dir, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "runbook.md")); err != nil {
		t.Fatalf("nested content missing: %v", err)
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
