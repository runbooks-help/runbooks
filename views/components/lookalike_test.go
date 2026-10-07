// SPDX-License-Identifier: FSL-1.1-MIT

package components

import (
	"context"
	"strings"
	"testing"
)

// TestLookalikeRendersAsABlock pins that a lookalike is its own block — the
// title, the marker label and the discriminator body — and never a step card.
func TestLookalikeRendersAsABlock(t *testing.T) {
	var b strings.Builder
	err := Lookalike(
		"GTID gap on the replica",
		"Looks like: a deadlock.\nRule out by: zero lock waits while the delay grows.",
		nil,
	).Render(context.Background(), &b)
	if err != nil {
		t.Fatalf("render Lookalike: %v", err)
	}
	html := b.String()
	for _, want := range []string{
		`notice-lookalike`,
		`lookalike-lead`,
		`Could also be:`,
		`lookalike-name`,
		`<svg`,
		`GTID gap on the replica`,
		`Looks like: a deadlock.`,
		`Rule out by: zero lock waits while the delay grows.`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("Lookalike missing %q:\n%s", want, html)
		}
	}
	if strings.Contains(html, "step-card") || strings.Contains(html, "step-num") {
		t.Errorf("a lookalike must not render as a step:\n%s", html)
	}
}
