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

func writeManifest(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "_meta.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestLoadDirManifest pins the taxonomy: content/_meta.yml supplies titles and
// order by list position, absent entries are inert, and unlisted directories fall
// back to title-case after the listed ones.
func TestLoadDirManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, `
- mysql:
    title: MySQL
    categories:
      - replication
      - dr: { title: Disaster Recovery }
- kubernetes
- backend
`)
	writeDoc(t, filepath.Join(dir, "mysql", "replication", "lag.md"), "Lag", "lag")
	writeDoc(t, filepath.Join(dir, "mysql", "dr", "restore.md"), "Restore", "restore")
	writeDoc(t, filepath.Join(dir, "mysql", "backup", "verify.md"), "Verify", "verify")
	writeDoc(t, filepath.Join(dir, "kubernetes", "cluster", "upgrade.md"), "Upgrade", "upgrade")
	// Not in the manifest: title-cased, alphabetical, after the listed systems.
	writeDoc(t, filepath.Join(dir, "zebra", "pen", "x.md"), "X", "x")

	defs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	var order []string
	for _, d := range defs {
		order = append(order, d.GroupTitle+" › "+d.CategoryTitle)
	}
	want := "MySQL › Replication|MySQL › Disaster Recovery|MySQL › Backup|Kubernetes › Cluster|Zebra › Pen"
	if got := strings.Join(order, "|"); got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
	if defs[0].GroupTitle != "MySQL" || defs[0].GroupOrder != 1 {
		t.Errorf("mysql = %q/%d, want MySQL/1", defs[0].GroupTitle, defs[0].GroupOrder)
	}
	// The `dr` map gave a title and position 2.
	if defs[1].CategoryTitle != "Disaster Recovery" || defs[1].CategoryOrder != 2 {
		t.Errorf("dr = %q/%d, want Disaster Recovery/2", defs[1].CategoryTitle, defs[1].CategoryOrder)
	}
	// An unlisted category is title-cased and sorts after the ordered ones.
	if defs[2].CategoryTitle != "Backup" || defs[2].CategoryOrder != 0 {
		t.Errorf("backup = %q/%d, want Backup/0", defs[2].CategoryTitle, defs[2].CategoryOrder)
	}
	// `backend` has no directory: the entry is inert, not an error.
	if defs[3].GroupTitle != "Kubernetes" || defs[4].GroupTitle != "Zebra" {
		t.Errorf("tail = %q, %q, want Kubernetes, Zebra", defs[3].GroupTitle, defs[4].GroupTitle)
	}
}

func TestLoadDirMalformedManifest(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "- mysql: [unclosed\n")
	writeDoc(t, filepath.Join(dir, "mysql", "x.md"), "X", "x")

	if _, err := LoadDir(dir); err == nil {
		t.Error("want an error for a malformed content/_meta.yml")
	}
}

// TestLoadDirExcludesRecordsPath pins that the git-sync records folder is never
// ingested as runbooks when content sits at a repo root that also holds records.
func TestLoadDirExcludesRecordsPath(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, filepath.Join(dir, "mysql", "backup", "verify.md"), "Verify", "verify")
	// A record has no frontmatter; without the exclusion it fails the walk.
	rec := filepath.Join(dir, "runbook_runs", "2026-10-03", "verify", "notes.md")
	if err := os.MkdirAll(filepath.Dir(rec), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rec, []byte("just a record, no frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadDir(dir); err == nil {
		t.Fatal("without the exclusion the records folder should fail the parse")
	}

	defs, err := LoadDir(dir, "runbook_runs")
	if err != nil {
		t.Fatalf("LoadDir with exclusion: %v", err)
	}
	if len(defs) != 1 || defs[0].Slug != "verify" {
		t.Fatalf("defs = %+v, want just the verify runbook", defs)
	}
}
