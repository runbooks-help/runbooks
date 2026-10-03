package gitrepo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

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
