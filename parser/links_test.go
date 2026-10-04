// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRewriteDocLinks pins that a relative .md link becomes the slug route,
// while external and non-.md links are left untouched.
func TestRewriteDocLinks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"),
		"---\ntitle: A\nslug: a\n---\n\n## Step\n\nSee [B](b.md#frag), [ext](https://example.com/x.md) and [raw](notes.txt).\n")
	writeFile(t, filepath.Join(dir, "b.md"),
		"---\ntitle: B\nslug: b\n---\n\n## Step\n\nbody\n")

	defs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}

	var prose string
	for _, d := range defs {
		if d.Slug != "a" {
			continue
		}
		for _, s := range d.Steps {
			for _, b := range s.Blocks {
				if b.Kind == KindProse {
					prose += b.Text
				}
			}
		}
	}
	if !strings.Contains(prose, "[B](/b#frag)") {
		t.Errorf("relative link not rewritten: %q", prose)
	}
	if !strings.Contains(prose, "https://example.com/x.md") {
		t.Errorf("external link mangled: %q", prose)
	}
	if !strings.Contains(prose, "notes.txt") {
		t.Errorf("non-.md link mangled: %q", prose)
	}
}

// TestRootManifestTitle pins that the reserved `_root` entry titles content
// directly in the content root.
func TestRootManifestTitle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "_meta.yml"), "- _root:\n    title: Documentation\n")
	writeFile(t, filepath.Join(dir, "a.md"), "---\ntitle: A\nslug: a\n---\n\n## Step\n\nbody\n")

	defs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if len(defs) != 1 || defs[0].GroupTitle != "Documentation" {
		t.Fatalf("GroupTitle = %q, want Documentation", defs[0].GroupTitle)
	}
}
