// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"strings"
	"testing"
)

// TestConvertLookalike pins the marker: a title off the marker line and the
// body lines collected as the discriminator.
func TestConvertLookalike(t *testing.T) {
	body := "## Step\n\n> [!lookalike] GTID gap on the replica\n> Looks like: a deadlock.\n> Rule out by: zero lock waits while the delay grows.\n"
	_, steps, _ := parseBody(t, body)
	if len(steps) != 1 || len(steps[0].Blocks) != 1 {
		t.Fatalf("blocks = %#v, want one lookalike block", steps)
	}
	b := steps[0].Blocks[0]
	if b.Kind != KindLookalike {
		t.Fatalf("kind = %q, want lookalike", b.Kind)
	}
	if b.Title != "GTID gap on the replica" {
		t.Errorf("title = %q, want the marker line's text", b.Title)
	}
	if !strings.Contains(b.Body, "Looks like: a deadlock.") || !strings.Contains(b.Body, "Rule out by: zero lock waits while the delay grows.") {
		t.Errorf("body = %q, want both discriminator lines", b.Body)
	}
}

// TestConvertLookalikeMarkerCaseInsensitive pins the case-insensitive marker,
// matching the other admonitions.
func TestConvertLookalikeMarkerCaseInsensitive(t *testing.T) {
	_, steps, _ := parseBody(t, "## Step\n\n> [!Lookalike] Disk full\n> Rule out by: `df -h` under 5%.\n")
	if len(steps) != 1 || len(steps[0].Blocks) != 1 || steps[0].Blocks[0].Kind != KindLookalike {
		t.Fatalf("blocks = %#v, want one lookalike block", steps)
	}
	if steps[0].Blocks[0].Title != "Disk full" {
		t.Errorf("title = %q", steps[0].Blocks[0].Title)
	}
}

// TestConvertLookalikeKeepsFollowingMarkdown pins that a lookalike flushes when
// the blockquote ends and the prose after it is not swallowed.
func TestConvertLookalikeKeepsFollowingMarkdown(t *testing.T) {
	_, steps, _ := parseBody(t, "## Step\n\n> [!lookalike] Disk full\n> Rule out by: `df`.\n\nBack to the step.\n")
	if len(steps) != 1 || len(steps[0].Blocks) != 2 {
		t.Fatalf("blocks = %#v, want a lookalike then prose", steps)
	}
	if steps[0].Blocks[1].Kind != KindProse || steps[0].Blocks[1].Text != "Back to the step." {
		t.Errorf("second block = %#v", steps[0].Blocks[1])
	}
}

// TestConvertLookalikeNeedsTitle pins the loud failure: a bare marker is an
// error, not a silently dropped block.
func TestConvertLookalikeNeedsTitle(t *testing.T) {
	_, _, _, err := convertBody("## Step\n\n> [!lookalike]\n> body\n", false)
	if err == nil {
		t.Fatal("want an error for a titleless lookalike")
	}
	if !strings.Contains(err.Error(), "needs a title") {
		t.Errorf("err = %v, want a title error", err)
	}
}
