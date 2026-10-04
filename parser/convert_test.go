// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import "testing"

func parseBody(t *testing.T, body string) ([]Block, []Step, []Step) {
	t.Helper()
	intro, steps, rollback := convertBody(body, false)
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
