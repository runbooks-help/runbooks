// SPDX-License-Identifier: FSL-1.1-MIT

// Package markup turns runbook text into the HTML fragments the views inject:
// the inline Markdown used in prose, notices and table cells, and the
// step-title → anchor slug.
package markup

import (
	"html"
	"regexp"
	"sort"
	"strings"
)

var (
	boldRe   = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	italicRe = regexp.MustCompile(`\*([^*\n]+)\*`)
	codeRe   = regexp.MustCompile("`([^`\n]+)`")
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	slugRe   = regexp.MustCompile(`[^a-z0-9]+`)
)

// Prose escapes text for safe HTML injection then applies inline Markdown:
// **bold**, *italic*, `code`, [text](url).
func Prose(text string) string {
	s := html.EscapeString(text)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")
	s = codeRe.ReplaceAllString(s, "<code>$1</code>")
	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)
	return s
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
