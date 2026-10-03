package gitrepo

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

func TestAuthMethod(t *testing.T) {
	if a, err := (Credentials{}).AuthMethod(); err != nil || a != nil {
		t.Fatalf("anonymous = %v, %v; want nil, nil", a, err)
	}

	tok, err := (Credentials{Token: "t"}).AuthMethod()
	if err != nil {
		t.Fatal(err)
	}
	if basic, ok := tok.(*githttp.BasicAuth); !ok || basic.Username != "oauth2" || basic.Password != "t" {
		t.Fatalf("token = %#v; want oauth2/t", tok)
	}

	tok, err = (Credentials{Token: "t", Username: "u"}).AuthMethod()
	if err != nil {
		t.Fatal(err)
	}
	if basic, ok := tok.(*githttp.BasicAuth); !ok || basic.Username != "u" {
		t.Fatalf("token+user = %#v; want u", tok)
	}

	key := writeTestKey(t)
	if a, err := (Credentials{SSHKey: key}).AuthMethod(); err != nil || a == nil {
		t.Fatalf("ssh = %v, %v; want a method", a, err)
	}
	if _, err := (Credentials{SSHKey: "/nonexistent/key"}).AuthMethod(); err == nil {
		t.Fatal("want an error for a missing ssh key")
	}
}

// writeTestKey writes a fresh PKCS#8 ed25519 private key and returns its path.
func writeTestKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestFreshCommitPush exercises the notes-sync write path end to end without a
// system git binary: clone an empty remote, write, commit, push.
func TestFreshCommitPush(t *testing.T) {
	remote := t.TempDir()
	if _, err := git.PlainInit(remote, true); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(t.TempDir(), "work")

	r, err := Fresh(context.Background(), work, remote, "main", Credentials{})
	if err != nil {
		t.Fatalf("Fresh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(work, "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	hash, changed, err := Commit(r, "add f", Author{Name: "Test", Email: "test@example.com"})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !changed {
		t.Fatal("Commit reported no change for a new file")
	}
	if err := Push(context.Background(), r, "main", Credentials{}); err != nil {
		t.Fatalf("Push: %v", err)
	}

	rr, err := git.PlainOpen(remote)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := rr.Reference(plumbing.NewBranchReferenceName("main"), true)
	if err != nil {
		t.Fatalf("remote main: %v", err)
	}
	if ref.Hash() != hash {
		t.Fatalf("remote main = %s, want %s", ref.Hash(), hash)
	}

	// A second commit with nothing staged is not a change.
	if _, changed, err := Commit(r, "noop", Author{Name: "Test", Email: "test@example.com"}); err != nil || changed {
		t.Fatalf("empty Commit: changed=%v err=%v", changed, err)
	}
}

// TestFreshClonesAndCreatesBranch covers checkoutOrCreate: an existing branch is
// checked out, a missing one is created.
func TestFreshClonesAndCreatesBranch(t *testing.T) {
	src := seedSource(t)

	existing := filepath.Join(t.TempDir(), "existing")
	r, err := Fresh(context.Background(), existing, src, "main", Credentials{})
	if err != nil {
		t.Fatalf("Fresh main: %v", err)
	}
	if _, err := os.Stat(filepath.Join(existing, "runbook.md")); err != nil {
		t.Fatalf("cloned content missing: %v", err)
	}
	if head, err := r.Head(); err != nil || head.Name() != plumbing.NewBranchReferenceName("main") {
		t.Fatalf("head = %v, %v; want main", head, err)
	}

	created := filepath.Join(t.TempDir(), "created")
	r, err = Fresh(context.Background(), created, src, "feature", Credentials{})
	if err != nil {
		t.Fatalf("Fresh feature: %v", err)
	}
	if head, err := r.Head(); err != nil || head.Name() != plumbing.NewBranchReferenceName("feature") {
		t.Fatalf("head = %v, %v; want feature", head, err)
	}
}

// TestEnsureCloneFetchResetAndLastGood covers the read path: clone, then refresh
// on a second call, then fall back to the checkout when the remote is gone.
func TestEnsureCloneFetchResetAndLastGood(t *testing.T) {
	src := seedSource(t)
	cache := filepath.Join(t.TempDir(), "cache")

	if _, err := Ensure(context.Background(), cache, src, "main", Credentials{}, nil); err != nil {
		t.Fatalf("clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cache, "runbook.md")); err != nil {
		t.Fatalf("cloned content missing: %v", err)
	}

	commitFile(t, src, "second.md", "---\ntitle: Second\nslug: second\n---\n")
	if _, err := Ensure(context.Background(), cache, src, "main", Credentials{}, nil); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cache, "second.md")); err != nil {
		t.Fatalf("fetch/reset did not update the checkout: %v", err)
	}

	// Re-point at a dead remote: last-good, and the failure is reported.
	called := false
	if _, err := Ensure(context.Background(), cache, "file:///nonexistent-repo", "main", Credentials{}, func(error) {
		called = true
	}); err != nil {
		t.Fatalf("last-good: %v", err)
	}
	if !called {
		t.Fatal("onFetchError was not called for an unreachable remote")
	}
	if _, err := os.Stat(filepath.Join(cache, "second.md")); err != nil {
		t.Fatalf("last-good content missing: %v", err)
	}
}

// TestExists covers the cache probe: a repository is true, a missing directory
// and a plain directory are false.
func TestExists(t *testing.T) {
	repo := seedSource(t)
	ok, err := Exists(repo)
	if err != nil {
		t.Fatalf("Exists repo: %v", err)
	}
	if !ok {
		t.Fatal("want true for a repository")
	}

	missing := filepath.Join(t.TempDir(), "missing")
	if ok, err := Exists(missing); err != nil || ok {
		t.Fatalf("Exists missing = %v, %v; want false, nil", ok, err)
	}

	plain := t.TempDir()
	if ok, err := Exists(plain); err != nil || ok {
		t.Fatalf("Exists plain = %v, %v; want false, nil", ok, err)
	}
}

// seedSource creates a non-bare repo with one runbook on main, using go-git.
func seedSource(t *testing.T) string {
	t.Helper()
	src := t.TempDir()
	r, err := git.PlainInit(src, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main"))); err != nil {
		t.Fatal(err)
	}
	commitFile(t, src, "runbook.md", "---\ntitle: X\nslug: x\n---\n")
	return src
}

// commitFile writes name in repoDir and commits it.
func commitFile(t *testing.T, repoDir, name, content string) {
	t.Helper()
	r, err := git.PlainOpen(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Add(name); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("add "+name, &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Now()},
	}); err != nil {
		t.Fatal(err)
	}
}
