// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// Field weights. Title and the metadata frontmatter dominate; prose sits in the
// middle; code bodies are lowest so a command match never outranks a title.
const (
	weightTitle       = 100
	weightSymptom     = 60
	weightDescription = 50
	weightStepTitle   = 40
	weightHeading     = 40
	weightGroup       = 30
	weightProse       = 20
	weightCode        = 10
)

// DefaultSearchLimit and MaxSearchLimit clamp the number of hits returned.
const (
	DefaultSearchLimit = 20
	MaxSearchLimit     = 50
)

// searchField is one indexed span of a runbook: a weight, the owning step (-1 for
// the runbook-level fields), and the byte ranges of its text in the doc's
// case-preserving and lowercased buffers.
type searchField struct {
	weight int
	step   int
	oStart int
	oEnd   int
	lStart int
	lEnd   int
}

// searchDoc is one indexed runbook. Every field's text lives in the two shared
// buffers, so the index holds two copies of the body (case-preserved for snippets,
// lowercase for matching) plus a small field table, not one string per field.
type searchDoc struct {
	meta     RunbookMeta
	steps    []string // step titles, indexed by searchField.step
	original string
	lower    string
	fields   []searchField
}

// SearchIndex is the in-memory full-text index over the loaded runbooks. It is
// built once at startup and read-only thereafter.
type SearchIndex struct {
	docs []searchDoc
}

// Hit is one runbook matching a query. Snippet is plain text; the caller escapes
// it and marks the query terms for display.
type Hit struct {
	Slug     string
	Title    string
	System   string
	Category string
	Step     string // step title the snippet came from, or ""
	Snippet  string
	Score    int
}

// BuildIndex indexes the runbook bodies in defs. It never returns nil.
func BuildIndex(defs []RunbookDef) *SearchIndex {
	idx := &SearchIndex{docs: make([]searchDoc, 0, len(defs))}
	for _, def := range defs {
		idx.docs = append(idx.docs, buildSearchDoc(def))
	}
	return idx
}

func buildSearchDoc(def RunbookDef) searchDoc {
	d := searchDoc{meta: def.RunbookMeta}
	add := func(weight, step int, text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		lower := strings.ToLower(text)
		d.fields = append(d.fields, searchField{
			weight: weight,
			step:   step,
			oStart: len(d.original),
			oEnd:   len(d.original) + len(text),
			lStart: len(d.lower),
			lEnd:   len(d.lower) + len(lower),
		})
		d.original += text
		d.lower += lower
	}

	add(weightTitle, -1, def.Title)
	add(weightDescription, -1, def.Description)
	for _, symptom := range def.Symptoms {
		add(weightSymptom, -1, symptom)
	}
	add(weightGroup, -1, def.GroupTitle)
	add(weightGroup, -1, def.CategoryTitle)

	addBlocks := func(si int, blocks []Block) {
		for _, b := range blocks {
			switch b.Kind {
			case KindCode:
				add(weightCode, si, b.Body)
			case KindProse:
				add(weightProse, si, b.Text)
			case KindNotice:
				add(weightProse, si, b.Message)
			case KindBranch:
				add(weightProse, si, b.Body)
			case KindHeading:
				add(weightHeading, si, b.Text)
			case KindList:
				add(weightProse, si, itemsText(b.Items))
			case KindTable:
				rows := make([]string, 0, len(b.Rows))
				for _, row := range b.Rows {
					rows = append(rows, strings.Join(row, " "))
				}
				add(weightProse, si, strings.Join(b.Headers, " ")+"\n"+strings.Join(rows, "\n"))
			}
		}
	}

	addSteps := func(steps []Step) {
		for _, step := range steps {
			si := len(d.steps)
			d.steps = append(d.steps, step.Title)
			add(weightStepTitle, si, step.Title)
			addBlocks(si, step.Blocks)
		}
	}
	if len(def.Intro) > 0 {
		si := len(d.steps)
		d.steps = append(d.steps, "")
		addBlocks(si, def.Intro)
	}
	addSteps(def.Steps)
	addSteps(def.Rollback)

	return d
}

// Search returns the runbooks matching q, best first, at most limit of them. A
// runbook matches when every query term appears somewhere in it; a contiguous
// phrase match scores double. An empty query (or a nil index) returns nothing.
func (idx *SearchIndex) Search(q string, limit int) []Hit {
	if idx == nil {
		return nil
	}
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(q)))
	if len(terms) == 0 {
		return nil
	}
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	phrase := strings.Join(terms, " ")

	hits := make([]Hit, 0, len(idx.docs))
	for i := range idx.docs {
		if hit, ok := idx.docs[i].match(terms, phrase); ok {
			hits = append(hits, hit)
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].Score != hits[b].Score {
			return hits[a].Score > hits[b].Score
		}
		if hits[a].Title != hits[b].Title {
			return hits[a].Title < hits[b].Title
		}
		return hits[a].Slug < hits[b].Slug
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

func (d *searchDoc) match(terms []string, phrase string) (Hit, bool) {
	present := make([]bool, len(terms))
	best, bestScore := -1, 0
	matchedFields := 0

	for fi := range d.fields {
		f := &d.fields[fi]
		low := d.lower[f.lStart:f.lEnd]
		termHits := 0
		for ti, term := range terms {
			if strings.Contains(low, term) {
				termHits++
				present[ti] = true
			}
		}
		if termHits == 0 {
			continue
		}
		matchedFields++
		score := f.weight * termHits
		if len(terms) > 1 && strings.Contains(low, phrase) {
			score = f.weight * 2 * len(terms)
		}
		if score > bestScore {
			bestScore, best = score, fi
		}
	}
	if best < 0 {
		return Hit{}, false
	}
	for _, ok := range present {
		if !ok {
			return Hit{}, false
		}
	}

	// A small bounded bonus for spreading across several fields, so a runbook
	// that mentions the terms in a heading and the prose outranks one that only
	// scrapes them from unrelated code.
	spread := matchedFields
	if spread > 5 {
		spread = 5
	}

	hit := Hit{
		Slug:     d.meta.Slug,
		Title:    d.meta.Title,
		System:   d.meta.GroupTitle,
		Category: d.meta.CategoryTitle,
		Snippet:  d.snippet(best, terms, phrase),
		Score:    bestScore + spread,
	}
	if step := d.fields[best].step; step >= 0 && step < len(d.steps) {
		hit.Step = d.steps[step]
	}
	return hit, true
}

// snippet returns a plain-text window of fields[fi]'s text around its first match,
// with runs of whitespace collapsed and ellipses where the text was trimmed.
func (d *searchDoc) snippet(fi int, terms []string, phrase string) string {
	f := d.fields[fi]
	low := d.lower[f.lStart:f.lEnd]
	orig := d.original[f.oStart:f.oEnd]

	at := -1
	if len(terms) > 1 {
		at = strings.Index(low, phrase)
	}
	if at < 0 {
		for _, term := range terms {
			if i := strings.Index(low, term); i >= 0 && (at < 0 || i < at) {
				at = i
			}
		}
	}
	if at < 0 {
		at = 0
	}
	return excerpt(orig, at)
}

// excerpt cuts a window of about 180 bytes around at, snaps to rune and word
// boundaries, collapses whitespace, and marks the cuts with an ellipsis.
func excerpt(text string, at int) string {
	const radius = 90
	start := max(at-radius, 0)
	end := min(at+radius, len(text))
	for start > 0 && !utf8.RuneStart(text[start]) {
		start--
	}
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end++
	}
	trimmedStart := start > 0
	if trimmedStart {
		if i := strings.IndexByte(text[start:end], ' '); i >= 0 {
			start += i + 1
		}
	}
	trimmedEnd := end < len(text)
	if trimmedEnd {
		if i := strings.LastIndexByte(text[start:end], ' '); i >= 0 {
			end = start + i
		}
	}
	s := strings.Join(strings.Fields(text[start:end]), " ")
	if trimmedStart {
		s = "…" + s
	}
	if trimmedEnd {
		s += "…"
	}
	return s
}
