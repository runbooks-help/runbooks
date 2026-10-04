// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGlossary(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, glossaryFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadGlossaryReadsEntries(t *testing.T) {
	dir := t.TempDir()
	writeGlossary(t, dir, "- term: MTS\n  expansion: Multi-Threaded Replica\n  description: A parallel replica.\n  link: https://x/glossary\n- term: MTTR\n  expansion: Mean Time To Recovery\n")

	got, err := LoadGlossary(dir)
	if err != nil {
		t.Fatalf("LoadGlossary: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0] != (GlossaryEntry{Term: "MTS", Expansion: "Multi-Threaded Replica", Description: "A parallel replica.", Link: "https://x/glossary"}) {
		t.Fatalf("unexpected first entry: %+v", got[0])
	}
	if terms := GlossaryTerms(got); strings.Join(terms, ",") != "MTS,MTTR" {
		t.Fatalf("GlossaryTerms = %v", terms)
	}
}

func TestLoadGlossaryMissingFileIsEmpty(t *testing.T) {
	got, err := LoadGlossary(t.TempDir())
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d entries, want 0", len(got))
	}
}

func TestLoadGlossaryRequiresTermAndExpansion(t *testing.T) {
	dir := t.TempDir()
	writeGlossary(t, dir, "- term: MTS\n  description: no expansion\n")
	if _, err := LoadGlossary(dir); err == nil {
		t.Fatal("expected an error for a missing expansion")
	}
}

func TestLoadGlossaryRejectsMalformedYAML(t *testing.T) {
	dir := t.TempDir()
	writeGlossary(t, dir, "term: : :\n")
	if _, err := LoadGlossary(dir); err == nil {
		t.Fatal("expected an error for malformed YAML")
	}
}
