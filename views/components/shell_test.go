// SPDX-License-Identifier: FSL-1.1-MIT

package components

import (
	"context"
	"strings"
	"testing"
)

func TestShellMetaTags(t *testing.T) {
	var b strings.Builder
	err := ShellMeta(Meta{
		Title:       "Configuration",
		Description: "Every setting is an env var.",
		Canonical:   "https://example.com/configuration",
		Type:        "article",
	}).Render(context.Background(), &b)
	if err != nil {
		t.Fatalf("render ShellMeta: %v", err)
	}
	html := b.String()
	for _, want := range []string{
		`<meta name="description" content="Every setting is an env var."`,
		`rel="canonical" href="https://example.com/configuration"`,
		`property="og:title" content="Configuration — Runbooks"`,
		`property="og:description" content="Every setting is an env var."`,
		`property="og:type" content="article"`,
		`name="twitter:card" content="summary"`,
		`property="og:url" content="https://example.com/configuration"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("ShellMeta missing %q", want)
		}
	}
}

// TestShellOmitsOptionalTags pins that the non-public pages keep a bare head.
func TestShellOmitsOptionalTags(t *testing.T) {
	var b strings.Builder
	if err := Shell("Admin").Render(context.Background(), &b); err != nil {
		t.Fatalf("render Shell: %v", err)
	}
	html := b.String()
	if strings.Contains(html, `name="description"`) || strings.Contains(html, `<link rel="canonical"`) {
		t.Errorf("plain Shell should omit optional meta:\n%s", html)
	}
	if !strings.Contains(html, "<title>Admin — Runbooks</title>") {
		t.Errorf("plain Shell title missing:\n%s", html)
	}
}
