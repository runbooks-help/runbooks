package content

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
	src := seedRepo(t)

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
	src := seedRepo(t)
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

// seedRepo creates a git repo with one runbook and returns its path.
func seedRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	src := t.TempDir()
	run(t, src, "git", "init", "-b", "main")
	run(t, src, "git", "config", "user.email", "test@example.com")
	run(t, src, "git", "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(src, "runbook.md"), []byte("---\ntitle: X\nslug: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, src, "git", "add", ".")
	run(t, src, "git", "commit", "-m", "init")
	return src
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
}
