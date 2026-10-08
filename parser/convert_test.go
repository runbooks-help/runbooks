// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"strings"
	"testing"
)

func parseBody(t *testing.T, body string) ([]Block, []Step, []Step) {
	t.Helper()
	intro, steps, rollback, err := convertBody(body, false)
	if err != nil {
		t.Fatalf("convertBody: %v", err)
	}
	return intro, steps, rollback
}

// TestConvertLooseOrderedList pins the conformance fix: blank lines between
// ordered items keep one list block instead of one block per item.
func TestConvertLooseOrderedList(t *testing.T) {
	_, steps, _ := parseBody(t, "## Step\n\n1. first\n\n2. second\n\n3. third\n")
	if len(steps) != 1 || len(steps[0].Blocks) != 1 {
		t.Fatalf("blocks = %#v, want one list block", steps)
	}
	b := steps[0].Blocks[0]
	if b.Kind != KindList || !b.Ordered || len(b.Items) != 3 {
		t.Fatalf("got %#v, want a 3-item ordered list", b)
	}
}

// TestConvertEscapedPipe pins the conformance fix: a `\|` inside a table cell
// stays in one cell rather than splitting the row.
func TestConvertEscapedPipe(t *testing.T) {
	_, steps, _ := parseBody(t, "## Step\n\n| a | b |\n|---|---|\n| `x=\\|y` | z |\n")
	if len(steps) != 1 || len(steps[0].Blocks) != 1 {
		t.Fatalf("blocks = %#v, want one table block", steps)
	}
	tb := steps[0].Blocks[0]
	if tb.Kind != KindTable || len(tb.Rows) != 1 || len(tb.Rows[0]) != 2 {
		t.Fatalf("got %#v, want a 2-column row", tb)
	}
	if tb.Rows[0][0] != "`x=\\|y`" {
		t.Errorf("cell = %q, want the escaped pipe kept in one cell", tb.Rows[0][0])
	}
}

// TestConvertNestedFence pins the conformance fix: a longer fence may contain a
// shorter one without closing early.
func TestConvertNestedFence(t *testing.T) {
	body := "## Step\n\n````markdown\n```\ninner\n```\n````\n"
	_, steps, _ := parseBody(t, body)
	if len(steps) != 1 || len(steps[0].Blocks) != 1 {
		t.Fatalf("blocks = %#v, want one code block", steps)
	}
	b := steps[0].Blocks[0]
	if b.Kind != KindCode || b.Lang != "markdown" {
		t.Fatalf("got %#v, want a markdown code block", b)
	}
	if want := "```\ninner\n```"; b.Body != want {
		t.Errorf("body = %q, want %q", b.Body, want)
	}
}

// TestConvertHeadingInFence pins that `##` inside a fence is code, not a step.
func TestConvertHeadingInFence(t *testing.T) {
	body := "## Step\n\n```bash\n## not a step\n```\n"
	_, steps, _ := parseBody(t, body)
	if len(steps) != 1 || steps[0].Title != "Step" {
		t.Fatalf("steps = %#v, want a single Step", steps)
	}
}

// TestConvertNestedList pins that a nested list becomes the item's Child
// instead of being flattened into its text.
func TestConvertNestedList(t *testing.T) {
	body := "## Step\n\n1. outer one\n    - inner a\n    - inner b\n2. outer two\n"
	_, steps, _ := parseBody(t, body)
	if len(steps) != 1 || len(steps[0].Blocks) != 1 {
		t.Fatalf("blocks = %#v, want one list", steps)
	}
	list := steps[0].Blocks[0]
	if list.Kind != KindList || !list.Ordered || len(list.Items) != 2 {
		t.Fatalf("got %#v, want a 2-item ordered list", list)
	}
	if list.Items[0].Text != "outer one" {
		t.Errorf("item 0 text = %q", list.Items[0].Text)
	}
	child := list.Items[0].Child
	if child == nil || child.Kind != KindList || child.Ordered || len(child.Items) != 2 {
		t.Fatalf("item 0 child = %#v, want a 2-item bullet list", child)
	}
	if child.Items[0].Text != "inner a" || child.Items[1].Text != "inner b" {
		t.Errorf("child items = %q, %q", child.Items[0].Text, child.Items[1].Text)
	}
	if list.Items[1].Child != nil {
		t.Errorf("item 1 should be a leaf, got child %#v", list.Items[1].Child)
	}
}

// TestConvertItemFallbackKeepsText pins the no-silent-drop guarantee: a block
// the model cannot place (here a fenced code block inside a list item) still
// surfaces its text in the item.
func TestConvertItemFallbackKeepsText(t *testing.T) {
	body := "## Step\n\n- item before\n  ```\n  echo hi\n  ```\n"
	_, steps, _ := parseBody(t, body)
	if len(steps) != 1 || len(steps[0].Blocks) != 1 {
		t.Fatalf("blocks = %#v, want one list", steps)
	}
	list := steps[0].Blocks[0]
	if len(list.Items) != 1 {
		t.Fatalf("items = %#v, want one item", list.Items)
	}
	if got := list.Items[0].Text; !strings.Contains(got, "item before") || !strings.Contains(got, "echo hi") {
		t.Errorf("item text = %q, want the code text folded in, not dropped", got)
	}
}
