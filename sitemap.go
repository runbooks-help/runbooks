// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"

	"runbooks/parser"
)

// handleSitemap serves an XML sitemap of the index and every runbook. Absolute
// URLs are required by the sitemap spec, so the route is only registered when
// PUBLIC_URL is set.
func handleSitemap(groups []parser.SystemGroup, publicURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = io.WriteString(w, renderSitemap(groups, publicURL))
	}
}

// renderSitemap builds the sitemap body from the same groups the sidebar uses,
// so it matches site order and never drifts from the content.
func renderSitemap(groups []parser.SystemGroup, publicURL string) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	write := func(loc string) {
		b.WriteString("  <url><loc>")
		_ = xml.EscapeText(&b, []byte(loc))
		b.WriteString("</loc></url>\n")
	}
	write(publicURL + "/")
	for _, g := range groups {
		for _, c := range g.Categories {
			for _, rb := range c.Runbooks {
				write(publicURL + "/" + rb.Slug)
			}
		}
	}
	b.WriteString("</urlset>\n")
	return b.String()
}

// handleRobots serves robots.txt. An identity-gated instance is not a public
// site, so it disallows everything; a public one allows crawling and points at
// the sitemap when a base URL is configured.
func handleRobots(identityEnabled bool, publicURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, renderRobots(identityEnabled, publicURL))
	}
}

func renderRobots(identityEnabled bool, publicURL string) string {
	var b strings.Builder
	b.WriteString("User-agent: *\n")
	if identityEnabled {
		b.WriteString("Disallow: /\n")
		return b.String()
	}
	b.WriteString("Allow: /\n")
	if publicURL != "" {
		b.WriteString("Sitemap: " + publicURL + "/sitemap.xml\n")
	}
	return b.String()
}
