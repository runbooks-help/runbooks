package parser

import (
	"strings"
	"testing"
)

func searchDefs() []RunbookDef {
	return []RunbookDef{
		{
			RunbookMeta: RunbookMeta{
				Title:         "Replication Lag (MTS Deadlock)",
				Slug:          "mts-deadlock",
				Description:   "Parallel worker deadlock stalls the replica SQL thread.",
				Symptoms:      []string{"Last_SQL_Error duplicate entry"},
				GroupTitle:    "MySQL",
				CategoryTitle: "Replication",
			},
			Steps: []Step{
				{Title: "Inspect the replica", Blocks: []Block{
					{Kind: KindProse, Text: "Check SHOW SLAVE STATUS and read Last_SQL_Error."},
				}},
				{Title: "Clear the blocked worker", Blocks: []Block{
					{Kind: KindCode, Lang: "sql", Body: "KILL 4123; -- ER_LOCK_DEADLOCK"},
				}},
			},
		},
		{
			RunbookMeta: RunbookMeta{
				Title:         "Disk Full (inode exhaustion)",
				Slug:          "disk-full",
				Description:   "No space left on device although df shows free bytes.",
				GroupTitle:    "Linux",
				CategoryTitle: "Storage",
			},
			Steps: []Step{{Title: "Find the inode hog", Blocks: []Block{
				{Kind: KindProse, Text: "lsof and find reveal the runaway log file."},
			}}},
		},
	}
}

func TestSearchMatchesTitle(t *testing.T) {
	idx := BuildIndex(searchDefs())
	hits := idx.Search("replication lag", DefaultSearchLimit)
	if len(hits) == 0 || hits[0].Slug != "mts-deadlock" {
		t.Fatalf("got %+v, want mts-deadlock first", hits)
	}
	if hits[0].System != "MySQL" || hits[0].Category != "Replication" {
		t.Errorf("system/category not carried: %+v", hits[0])
	}
}

func TestSearchMatchesCodeBody(t *testing.T) {
	idx := BuildIndex(searchDefs())
	hits := idx.Search("ER_LOCK_DEADLOCK", DefaultSearchLimit)
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1: %+v", len(hits), hits)
	}
	h := hits[0]
	if h.Slug != "mts-deadlock" {
		t.Errorf("slug = %q, want mts-deadlock", h.Slug)
	}
	if h.Step != "Clear the blocked worker" {
		t.Errorf("step = %q, want the code block's step", h.Step)
	}
	if !strings.Contains(h.Snippet, "ER_LOCK_DEADLOCK") {
		t.Errorf("snippet %q does not contain the match", h.Snippet)
	}
}

func TestSearchRequiresEveryTerm(t *testing.T) {
	idx := BuildIndex(searchDefs())
	if hits := idx.Search("replica deadlock", DefaultSearchLimit); len(hits) != 1 || hits[0].Slug != "mts-deadlock" {
		t.Fatalf("terms spread across fields should match one runbook, got %+v", hits)
	}
	if hits := idx.Search("replica inode", DefaultSearchLimit); len(hits) != 0 {
		t.Fatalf("a term absent everywhere must reject the runbook, got %+v", hits)
	}
}

func TestSearchPhraseOutranksSplitTerms(t *testing.T) {
	defs := []RunbookDef{
		{RunbookMeta: RunbookMeta{Title: "Split", Slug: "split"}, Steps: []Step{{Title: "A", Blocks: []Block{
			{Kind: KindProse, Text: "the primary is fine"},
			{Kind: KindCode, Body: "down"},
		}}}},
		{RunbookMeta: RunbookMeta{Title: "Phrase", Slug: "phrase"}, Steps: []Step{{Title: "B", Blocks: []Block{
			{Kind: KindProse, Text: "the primary down outage"},
		}}}},
	}
	hits := BuildIndex(defs).Search("primary down", DefaultSearchLimit)
	if len(hits) != 2 || hits[0].Slug != "phrase" {
		t.Fatalf("phrase match should rank first, got %+v", hits)
	}
}

func TestSearchEmptyAndNoMatch(t *testing.T) {
	idx := BuildIndex(searchDefs())
	if hits := idx.Search("   ", DefaultSearchLimit); hits != nil {
		t.Fatalf("empty query should return nothing, got %+v", hits)
	}
	if hits := idx.Search("kubernetes", DefaultSearchLimit); len(hits) != 0 {
		t.Fatalf("no-match should be empty, got %+v", hits)
	}
}

func TestSearchLimitClamped(t *testing.T) {
	idx := BuildIndex(searchDefs())
	if hits := idx.Search("the", 1); len(hits) != 1 {
		t.Fatalf("limit 1 returned %d hits", len(hits))
	}
	if hits := idx.Search("the", MaxSearchLimit+1000); len(hits) > MaxSearchLimit {
		t.Fatalf("limit not clamped: %d hits", len(hits))
	}
}

func TestSearchSnippetTrimsLongText(t *testing.T) {
	long := strings.Repeat("word ", 60) + "needle" + strings.Repeat(" word", 60)
	defs := []RunbookDef{{RunbookMeta: RunbookMeta{Title: "Long", Slug: "long"}, Steps: []Step{{
		Title:  "Step",
		Blocks: []Block{{Kind: KindProse, Text: long}},
	}}}}
	hits := BuildIndex(defs).Search("needle", DefaultSearchLimit)
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
	s := hits[0].Snippet
	if !strings.Contains(s, "needle") {
		t.Errorf("snippet missing the match: %q", s)
	}
	if !strings.HasPrefix(s, "…") || !strings.HasSuffix(s, "…") {
		t.Errorf("trimmed snippet should be ellipsised both ends: %q", s)
	}
}

func TestSearchNilIndex(t *testing.T) {
	var idx *SearchIndex
	if hits := idx.Search("anything", DefaultSearchLimit); hits != nil {
		t.Fatalf("nil index should return nothing, got %+v", hits)
	}
}
