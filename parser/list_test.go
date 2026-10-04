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
	if want := "first line indented continuation lazy continuation"; b.Items[0].Text != want {
		t.Errorf("item 0 = %q, want %q", b.Items[0].Text, want)
	}
	if b.Items[1].Text != "second item" {
		t.Errorf("item 1 = %q", b.Items[1].Text)
	}
}

// TestParseOrderedList pins that `1.` / `2.` render as a numbered list (Ordered),
// distinct from a bullet list, and that a kind change starts a new block.
func TestParseOrderedList(t *testing.T) {
	src := "---\ntitle: X\nslug: x\n---\n\n## Step\n\n" +
		"1. first\n2. second\n\n" +
		"- bullet\n"
	def, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	blocks := def.Steps[0].Blocks
	if len(blocks) != 2 {
		t.Fatalf("blocks = %#v, want a numbered list then a bullet list", blocks)
	}
	if blocks[0].Kind != KindList || !blocks[0].Ordered || len(blocks[0].Items) != 2 {
		t.Errorf("block 0 = %#v, want an ordered 2-item list", blocks[0])
	}
	if want := "first"; blocks[0].Items[0].Text != want {
		t.Errorf("item 0 = %q, want %q", blocks[0].Items[0].Text, want)
	}
	if blocks[1].Kind != KindList || blocks[1].Ordered || len(blocks[1].Items) != 1 {
		t.Errorf("block 1 = %#v, want an unordered 1-item list", blocks[1])
	}
}

// TestParseListThenTable pins that a table line directly after a list item is
// a lazy continuation (CommonMark: a table cannot interrupt a paragraph), so it
// stays part of the list item. A blank line between them yields a real table.
func TestParseListThenTable(t *testing.T) {
	src := "---\ntitle: X\nslug: x\n---\n\n## Step\n\n- item\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	def, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if blocks := def.Steps[0].Blocks; len(blocks) != 1 || blocks[0].Kind != KindList {
		t.Fatalf("block kinds = %#v, want one list (table swallowed as continuation)", blocks)
	}

	src = "---\ntitle: X\nslug: x\n---\n\n## Step\n\n- item\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"
	def, err = Parse([]byte(src))
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
