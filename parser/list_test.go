// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import "testing"

// TestParseMultiLineListItem pins that a wrapped bullet stays one item: both the
// indented and the lazy (column-zero) continuation lines join the item.
func TestParseMultiLineListItem(t *testing.T) {
	src := "---\ntitle: X\nslug: x\n---\n\n## Step\n\n" +
		"- first line\n  indented continuation\n" +
		"lazy continuation\n" +
		"- second item\n"
	def, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(def.Steps) != 1 || len(def.Steps[0].Blocks) != 1 {
		t.Fatalf("blocks = %#v, want one list", def.Steps[0].Blocks)
	}
	b := def.Steps[0].Blocks[0]
	if b.Kind != KindList || len(b.Items) != 2 {
		t.Fatalf("got %#v, want a 2-item list", b)
	}
	if want := "first line indented continuation lazy continuation"; b.Items[0] != want {
		t.Errorf("item 0 = %q, want %q", b.Items[0], want)
	}
	if b.Items[1] != "second item" {
		t.Errorf("item 1 = %q", b.Items[1])
	}
}

// TestParseListThenTable pins that a table line still ends the list rather than
// being swallowed as a continuation.
func TestParseListThenTable(t *testing.T) {
	src := "---\ntitle: X\nslug: x\n---\n\n## Step\n\n- item\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	def, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	kinds := make([]BlockKind, 0, len(def.Steps[0].Blocks))
	for _, b := range def.Steps[0].Blocks {
		kinds = append(kinds, b.Kind)
	}
	if len(kinds) != 2 || kinds[0] != KindList || kinds[1] != KindTable {
		t.Fatalf("block kinds = %v, want [list table]", kinds)
	}
}
