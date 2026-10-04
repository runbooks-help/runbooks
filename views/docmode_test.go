// SPDX-License-Identifier: FSL-1.1-MIT

package views

import (
	"testing"

	"runbooks/parser"
)

// TestStepNumber pins that only runbook steps are counted, so a doc section in
// the middle of a mixed page does not offset the procedure numbering.
func TestStepNumber(t *testing.T) {
	steps := []parser.Step{
		{Title: "Server", Doc: true},
		{Title: "Restart"},
		{Title: "Verify"},
		{Title: "Reference", Doc: true},
	}
	want := []int{0, 1, 2, 2}
	for i, w := range want {
		if got := stepNumber(steps, i); got != w {
			t.Errorf("stepNumber(%d) = %d, want %d", i, got, w)
		}
	}
}

func TestHasRunbookSteps(t *testing.T) {
	if hasRunbookSteps([]parser.Step{{Title: "A", Doc: true}}) {
		t.Error("doc-only page should have no runbook steps")
	}
	if !hasRunbookSteps([]parser.Step{{Title: "A", Doc: true}, {Title: "B"}}) {
		t.Error("mixed page should report runbook steps")
	}
}
