// Package markup turns runbook text into the HTML fragments the views inject:
// the inline Markdown used in prose, notices and table cells, and the
// step-title → anchor slug.
package markup

import (
	"html"
	"regexp"
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

// Slug converts a step title to a URL-safe anchor id.
func Slug(s string) string {
	s = strings.ToLower(s)
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
