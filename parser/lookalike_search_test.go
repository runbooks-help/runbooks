// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import "testing"

// TestSearchIndexesLookalikeBodies pins that a lookalike's title and body join
// the index: searching the lookalike symptom finds the runbook that rules it out.
func TestSearchIndexesLookalikeBodies(t *testing.T) {
	defs := []RunbookDef{{
		RunbookMeta: RunbookMeta{Title: "Replication Lag", Slug: "mts-deadlock"},
		Steps: []Step{{Title: "Confirm the lag", Blocks: []Block{
			{Kind: KindLookalike, Title: "Inode exhaustion", Body: "Rule out by: `df -i` at 100% while bytes are free."},
		}}},
	}}
	idx := BuildIndex(defs)
	hits := idx.Search("inode exhaustion", DefaultSearchLimit)
	if len(hits) == 0 || hits[0].Slug != "mts-deadlock" {
		t.Fatalf("got %+v, want mts-deadlock via its lookalike", hits)
	}
}
