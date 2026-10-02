package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDoc(t *testing.T, path, title, slug string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "---\ntitle: " + title + "\nslug: " + slug + "\n---\n\n## Step\n\nbody\n"
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadDirNesting pins the taxonomy: a runbook's system and category are the
// two directories directly above it, so a leading wrapper (testdata/) is ignored
// and deeper sources still group on their nearest two.
func TestLoadDirNesting(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "top.md"), "Top", "top")
	writeDoc(t, filepath.Join(dir, "mysql", "plain.md"), "Plain", "plain")
	writeDoc(t, filepath.Join(dir, "mysql", "replication", "lag.md"), "Lag", "lag")
	writeDoc(t, filepath.Join(dir, "testdata", "mysql", "backup", "verify.md"), "Verify", "verify")
	writeDoc(t, filepath.Join(dir, "testdata", "kubernetes", "cluster", "upgrade.md"), "Upgrade", "upgrade")

	defs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	got := map[string][2]string{}
	for _, d := range defs {
		got[d.Slug] = [2]string{d.Group, d.Category}
	}
	want := map[string][2]string{
		"top":     {"", ""},
		"plain":   {"mysql", ""},
		"lag":     {"mysql", "replication"},
		"verify":  {"mysql", "backup"},
		"upgrade": {"kubernetes", "cluster"},
	}
	if len(defs) != len(want) {
		t.Fatalf("runbooks = %d, want %d: %+v", len(defs), len(want), got)
	}
	for slug, w := range want {
		if got[slug] != w {
			t.Errorf("%s = group %q category %q, want %q/%q", slug, got[slug][0], got[slug][1], w[0], w[1])
		}
	}
}

func writeMeta(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_meta.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadDirDirMeta pins the per-directory taxonomy: _meta.yaml supplies the
// display title and sort order, absent meta falls back to title-case, and the
// order is system (order, title) then category (order, title).
func TestLoadDirDirMeta(t *testing.T) {
	dir := t.TempDir()
	writeMeta(t, filepath.Join(dir, "mysql"), "title: MySQL\norder: 2\n")
	writeMeta(t, filepath.Join(dir, "mysql", "replication"), "order: 1\n")
	writeMeta(t, filepath.Join(dir, "kubernetes"), "title: Kubernetes\norder: 1\n")

	writeDoc(t, filepath.Join(dir, "mysql", "replication", "lag.md"), "Lag", "lag")
	writeDoc(t, filepath.Join(dir, "mysql", "backup", "verify.md"), "Verify", "verify")
	writeDoc(t, filepath.Join(dir, "kubernetes", "cluster", "upgrade.md"), "Upgrade", "upgrade")

	defs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	var order []string
	for _, d := range defs {
		order = append(order, d.GroupTitle+" › "+d.CategoryTitle)
	}
	// kubernetes (order 1) before mysql (order 2); within mysql, replication
	// (order 1) before backup (no order → after ordered names).
	if got, want := strings.Join(order, "|"), "Kubernetes › Cluster|MySQL › Replication|MySQL › Backup"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
	if defs[1].GroupTitle != "MySQL" || defs[1].GroupOrder != 2 {
		t.Errorf("mysql meta = %q/%d, want MySQL/2", defs[1].GroupTitle, defs[1].GroupOrder)
	}
	if defs[1].CategoryTitle != "Replication" || defs[1].CategoryOrder != 1 {
		t.Errorf("replication meta = %q/%d, want Replication/1", defs[1].CategoryTitle, defs[1].CategoryOrder)
	}
	// A category with no meta is title-cased and sorts after the ordered one.
	if defs[2].CategoryTitle != "Backup" || defs[2].CategoryOrder != 0 {
		t.Errorf("backup meta = %q/%d, want Backup/0", defs[2].CategoryTitle, defs[2].CategoryOrder)
	}
}

func TestLoadDirMalformedMeta(t *testing.T) {
	dir := t.TempDir()
	writeMeta(t, filepath.Join(dir, "mysql"), "title: [unclosed\n")
	writeDoc(t, filepath.Join(dir, "mysql", "x.md"), "X", "x")

	if _, err := LoadDir(dir); err == nil {
		t.Error("want an error for a malformed _meta.yaml")
	}
}
