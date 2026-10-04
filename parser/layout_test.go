// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import "testing"

// TestParseSectionsLayout pins the region-scoped mix: layout: sections makes the
// lead prose an Intro and ## a section; ---steps flips the following ## to
// numbered steps; ---sections flips them back.
func TestParseSectionsLayout(t *testing.T) {
	src := `---
title: Configuration
slug: configuration
layout: sections
---
Lead paragraph.

## Server
table here

---steps
## Restart
step body

## Verify
step body

---sections
## Reference
section
`
	def, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(def.Intro) != 1 || def.Intro[0].Text != "Lead paragraph." {
		t.Fatalf("Intro = %#v, want one lead paragraph", def.Intro)
	}

	want := []struct {
		title   string
		section bool
	}{
		{"Server", true},
		{"Restart", false},
		{"Verify", false},
		{"Reference", true},
	}
	if len(def.Steps) != len(want) {
		t.Fatalf("got %d steps, want %d", len(def.Steps), len(want))
	}
	for i, w := range want {
		if def.Steps[i].Title != w.title || def.Steps[i].Section != w.section {
			t.Errorf("step %d = %q section=%v, want %q section=%v",
				i, def.Steps[i].Title, def.Steps[i].Section, w.title, w.section)
		}
	}
}

// TestParseCommentDirectives pins the GitHub-safe separator form: an HTML
// comment is invisible when the Markdown is rendered, so docs can mix regions
// without artifacts.
func TestParseCommentDirectives(t *testing.T) {
	src := "---\ntitle: X\nslug: x\nlayout: sections\n---\n\n## A\n\nbody\n\n<!-- steps -->\n\n## One\n\nbody\n\n## Two\n\nbody\n\n<!-- sections -->\n\n## B\n\nbody\n"
	def, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := []struct {
		title   string
		section bool
	}{
		{"A", true},
		{"One", false},
		{"Two", false},
		{"B", true},
	}
	if len(def.Steps) != len(want) {
		t.Fatalf("got %d steps, want %d", len(def.Steps), len(want))
	}
	for i, w := range want {
		if def.Steps[i].Title != w.title || def.Steps[i].Section != w.section {
			t.Errorf("step %d = %q section=%v, want %q section=%v",
				i, def.Steps[i].Title, def.Steps[i].Section, w.title, w.section)
		}
	}
}

// TestParseCommentRollback pins the comment form of the rollback directive.
func TestParseCommentRollback(t *testing.T) {
	src := "---\ntitle: X\nslug: x\n---\n\n## Step\n\nbody\n\n<!-- rollback -->\n\n## Undo\n\nbody\n"
	def, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(def.Steps) != 1 || len(def.Rollback) != 1 || def.Rollback[0].Title != "Undo" {
		t.Fatalf("steps=%d rollback=%#v, want one step and one rollback", len(def.Steps), def.Rollback)
	}
}

// TestParseStepsLayoutIsDefault guards the existing behaviour: with no layout,
// every ## is a numbered step and nothing is a section.
func TestParseStepsLayoutIsDefault(t *testing.T) {
	def, err := Parse([]byte("---\ntitle: X\nslug: x\n---\n\n## Step one\n\nbody\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(def.Intro) != 0 {
		t.Fatalf("Intro = %#v, want empty", def.Intro)
	}
	if len(def.Steps) != 1 || def.Steps[0].Section {
		t.Fatalf("Steps = %#v, want one numbered step", def.Steps)
	}
}

// TestSearchIndexesIntro pins that the lead prose is searchable.
func TestSearchIndexesIntro(t *testing.T) {
	def, err := Parse([]byte("---\ntitle: X\nslug: x\nlayout: sections\n---\n\nUltramarine lead.\n\n## Section\n\nbody\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	idx := BuildIndex([]RunbookDef{def})
	if hits := idx.Search("ultramarine", 10); len(hits) != 1 {
		t.Fatalf("Search(ultramarine) = %d hits, want 1", len(hits))
	}
}
