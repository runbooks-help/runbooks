// SPDX-License-Identifier: FSL-1.1-MIT

// Package markup turns runbook text into the HTML fragments the views inject:
// the inline Markdown used in prose, notices and table cells, and the
// step-title → anchor slug.
//
// Inline Markdown is parsed with goldmark and the inline AST is rendered by
// hand, so classes and the glossary survive, and code/link nodes are skipped
// structurally rather than by a regex pass over already-rendered HTML.
package markup

import (
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// inlineParser parses a fragment as CommonMark. Prose input is inline by
// contract, but a fragment that happens to contain block syntax is still walked
// down to its inline children.
var inlineParser = goldmark.New().Parser()

// Prose renders inline Markdown: **bold**, *italic*, `code`, [text](url),
// autolinks. Text is HTML-escaped; raw HTML in the source is shown literally.
func Prose(s string) string {
	return renderInline(s, nil)
}

// ProseGlossary renders inline Markdown then wraps every configured glossary
// term in an <abbr> carrying its key, for the hover popup. Terms match exactly
// and case-sensitively, whole-word and longest-first. Code spans and link text
// are skipped because the walk never descends into them for glossary terms.
func ProseGlossary(s string, terms []string) string {
	return renderInline(s, glossaryRe(terms))
}

// renderInline parses s and renders its inline nodes. glossary, when non-nil,
// wraps matching text runs.
func renderInline(s string, glossary *regexp.Regexp) string {
	src := []byte(s)
	doc := inlineParser.Parse(text.NewReader(src))
	var b strings.Builder
	st := inlineState{source: src, glossary: glossary}
	st.renderChildren(&b, doc)
	return b.String()
}

// inlineState carries the source and the render context through the walk.
// code suppresses entity resolution and glossary matching (code spans, raw
// HTML); a nil glossary suppresses glossary matching (link text).
type inlineState struct {
	source   []byte
	glossary *regexp.Regexp
	code     bool
}

// renderChildren renders every inline child of parent, descending through
// block wrappers without emitting them.
func (st inlineState) renderChildren(b *strings.Builder, parent ast.Node) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Type() == ast.TypeBlock {
			st.renderChildren(b, c)
			continue
		}
		st.renderNode(b, c)
	}
}

func (st inlineState) renderNode(b *strings.Builder, n ast.Node) {
	switch n := n.(type) {
	case *ast.Text:
		st.writeText(b, string(n.Value(st.source)))
		if n.HardLineBreak() {
			b.WriteString("<br>")
		} else if n.SoftLineBreak() {
			b.WriteByte(' ')
		}
	case *ast.String:
		st.writeText(b, string(n.Value))
	case *ast.CodeSpan:
		b.WriteString("<code>")
		st.code = true
		st.glossary = nil
		st.renderChildren(b, n)
		b.WriteString("</code>")
	case *ast.Emphasis:
		tag := "em"
		if n.Level == 2 {
			tag = "strong"
		}
		b.WriteString("<" + tag + ">")
		st.renderChildren(b, n)
		b.WriteString("</" + tag + ">")
	case *ast.Link:
		b.WriteString(`<a href="`)
		b.WriteString(html.EscapeString(string(n.Destination)))
		b.WriteString(`">`)
		st.glossary = nil // link text never takes glossary terms
		st.renderChildren(b, n)
		b.WriteString("</a>")
	case *ast.AutoLink:
		b.WriteString(`<a href="`)
		b.WriteString(html.EscapeString(string(n.URL(st.source))))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(string(n.Label(st.source))))
		b.WriteString("</a>")
	case *ast.Image:
		b.WriteString(`<img src="`)
		b.WriteString(html.EscapeString(string(n.Destination)))
		b.WriteString(`" alt="`)
		b.WriteString(html.EscapeString(string(n.Text(st.source))))
		b.WriteString(`">`)
	case *ast.RawHTML:
		st.code = true
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			if seg.Start < 0 || seg.Stop > len(st.source) || seg.Start > seg.Stop {
				continue
			}
			st.writeText(b, string(st.source[seg.Start:seg.Stop]))
		}
	default:
		st.renderChildren(b, n)
	}
}

// writeText resolves entities (unless in a code context), then escapes s and,
// when a glossary is set, wraps matching terms in <abbr>.
func (st inlineState) writeText(b *strings.Builder, s string) {
	if !st.code {
		s = html.UnescapeString(s)
	}
	if st.glossary == nil {
		b.WriteString(html.EscapeString(s))
		return
	}
	last := 0
	for _, m := range st.glossary.FindAllStringIndex(s, -1) {
		b.WriteString(html.EscapeString(s[last:m[0]]))
		term := s[m[0]:m[1]]
		b.WriteString(`<abbr class="glossary-term" tabindex="0" data-glossary="`)
		b.WriteString(html.EscapeString(term))
		b.WriteString(`">`)
		b.WriteString(html.EscapeString(term))
		b.WriteString(`</abbr>`)
		last = m[1]
	}
	b.WriteString(html.EscapeString(s[last:]))
}

// glossaryRe compiles the word-boundary alternation of configured terms,
// longest first so a longer term is preferred over one it contains. Matching is
// case-sensitive and exact (no (?i)), so an expansion like "IT" never matches
// the English word. Empty terms are dropped.
func glossaryRe(terms []string) *regexp.Regexp {
	uniq := make([]string, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		uniq = append(uniq, regexp.QuoteMeta(t))
	}
	if len(uniq) == 0 {
		return nil
	}
	sort.SliceStable(uniq, func(i, j int) bool { return len(uniq[i]) > len(uniq[j]) })
	re, err := regexp.Compile(`\b(?:` + strings.Join(uniq, "|") + `)\b`)
	if err != nil {
		return nil
	}
	return re
}

// Highlight escapes s for HTML injection and wraps each case-insensitive
// occurrence of the query terms in <mark>. Terms match literally, never as
// patterns. The result is the only snippet HTML the search endpoint emits, so
// all escaping lives here and the client can set it directly.
func Highlight(s string, terms []string) string {
	re := highlightRe(terms)
	if re == nil {
		return html.EscapeString(s)
	}
	var b strings.Builder
	last := 0
	for _, m := range re.FindAllStringIndex(s, -1) {
		b.WriteString(html.EscapeString(s[last:m[0]]))
		b.WriteString("<mark>")
		b.WriteString(html.EscapeString(s[m[0]:m[1]]))
		b.WriteString("</mark>")
		last = m[1]
	}
	b.WriteString(html.EscapeString(s[last:]))
	return b.String()
}

// highlightRe compiles the alternation of quoted terms, longest first so a
// phrase is preferred over a single word it contains. Empty terms are dropped.
func highlightRe(terms []string) *regexp.Regexp {
	uniq := make([]string, 0, len(terms))
	seen := make(map[string]bool, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		uniq = append(uniq, regexp.QuoteMeta(t))
	}
	if len(uniq) == 0 {
		return nil
	}
	sort.SliceStable(uniq, func(i, j int) bool { return len(uniq[i]) > len(uniq[j]) })
	re, err := regexp.Compile("(?i)" + strings.Join(uniq, "|"))
	if err != nil {
		return nil
	}
	return re
}

// Slug converts a step title to a URL-safe anchor id.
func Slug(s string) string {
	s = strings.ToLower(s)
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)
