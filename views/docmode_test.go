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

// TestDocPrevNext pins the sidebar-order neighbours, including the ends.
func TestDocPrevNext(t *testing.T) {
	groups := []parser.SystemGroup{{Categories: []parser.CategoryGroup{
		{Runbooks: []parser.RunbookMeta{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}}},
	}}}
	if docPrev(groups, "a") != nil {
		t.Error("first page should have no prev")
	}
	if got := docNext(groups, "a"); got == nil || got.Slug != "b" {
		t.Errorf("docNext(a) = %v, want b", got)
	}
	if got := docPrev(groups, "b"); got == nil || got.Slug != "a" {
		t.Errorf("docPrev(b) = %v, want a", got)
	}
	if docNext(groups, "c") != nil {
		t.Error("last page should have no next")
	}
	if docPrev(groups, "missing") != nil {
		t.Error("unknown slug should have no prev")
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
