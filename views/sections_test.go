// SPDX-License-Identifier: FSL-1.1-MIT

package views

import (
	"testing"

	"runbooks/parser"
)

// TestStepNumber pins that only numbered steps are counted, so a section in the
// middle of a mixed page does not offset the procedure numbering.
func TestStepNumber(t *testing.T) {
	steps := []parser.Step{
		{Title: "Server", Section: true},
		{Title: "Restart"},
		{Title: "Verify"},
		{Title: "Reference", Section: true},
	}
	want := []int{0, 1, 2, 2}
	for i, w := range want {
		if got := stepNumber(steps, i); got != w {
			t.Errorf("stepNumber(%d) = %d, want %d", i, got, w)
		}
	}
}

// TestPagePrevNext pins the sidebar-order neighbours, including the ends.
func TestPagePrevNext(t *testing.T) {
	groups := []parser.SystemGroup{{Categories: []parser.CategoryGroup{
		{Runbooks: []parser.RunbookMeta{{Slug: "a"}, {Slug: "b"}, {Slug: "c"}}},
	}}}
	if pagePrev(groups, "a") != nil {
		t.Error("first page should have no prev")
	}
	if got := pageNext(groups, "a"); got == nil || got.Slug != "b" {
		t.Errorf("pageNext(a) = %v, want b", got)
	}
	if got := pagePrev(groups, "b"); got == nil || got.Slug != "a" {
		t.Errorf("pagePrev(b) = %v, want a", got)
	}
	if pageNext(groups, "c") != nil {
		t.Error("last page should have no next")
	}
	if pagePrev(groups, "missing") != nil {
		t.Error("unknown slug should have no prev")
	}
}

func TestHasRunbookSteps(t *testing.T) {
	if hasRunbookSteps([]parser.Step{{Title: "A", Section: true}}) {
		t.Error("section-only page should have no numbered steps")
	}
	if !hasRunbookSteps([]parser.Step{{Title: "A", Section: true}, {Title: "B"}}) {
		t.Error("mixed page should report numbered steps")
	}
}
