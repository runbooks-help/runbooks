// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"strings"
	"testing"

	"runbooks/parser"
)

func testGroups() []parser.SystemGroup {
	return []parser.SystemGroup{{Categories: []parser.CategoryGroup{
		{Runbooks: []parser.RunbookMeta{{Slug: "a"}, {Slug: "b"}}},
	}}}
}

func TestRenderSitemap(t *testing.T) {
	got := renderSitemap(testGroups(), "https://example.com")
	for _, want := range []string{
		`<loc>https://example.com/</loc>`,
		`<loc>https://example.com/a</loc>`,
		`<loc>https://example.com/b</loc>`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("sitemap missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRobots(t *testing.T) {
	if got := renderRobots(true, "https://example.com"); !strings.Contains(got, "Disallow: /") || strings.Contains(got, "Sitemap:") {
		t.Errorf("identity-on robots = %q, want disallow and no sitemap", got)
	}
	got := renderRobots(false, "https://example.com")
	if !strings.Contains(got, "Allow: /") || !strings.Contains(got, "Sitemap: https://example.com/sitemap.xml") {
		t.Errorf("public robots = %q", got)
	}
	if got := renderRobots(false, ""); strings.Contains(got, "Sitemap:") {
		t.Errorf("robots without a base should omit the sitemap line: %q", got)
	}
}
