package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"runbooks/parser"
)

// serveMarkdown writes a runbook's raw source markdown — the highest-fidelity
// form for an LLM, frontmatter included. It is a read-only machine endpoint.
func serveMarkdown(w http.ResponseWriter, r *http.Request, rb parser.RunbookDef) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, rb.Source)
}

// handleLLMSIndex serves a generated llms.txt index: the runbooks grouped by
// system, each linking to its raw markdown. It is a read-only machine endpoint.
func handleLLMSIndex(groups []parser.SystemGroup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, renderLLMSIndex(groups))
	}
}

// renderLLMSIndex builds the llms.txt body from the same groups the sidebar
// uses, so the index order matches the site.
func renderLLMSIndex(groups []parser.SystemGroup) string {
	var b strings.Builder
	b.WriteString("# Runbooks\n\n")
	b.WriteString("> Operational runbooks. Fetch any one as raw markdown by appending")
	b.WriteString(" `.md` to its link; search every body at /api/runbooks/v1/search?q=…\n\n")
	for _, g := range groups {
		if g.Name != "" {
			fmt.Fprintf(&b, "## %s\n\n", g.Name)
		}
		for _, c := range g.Categories {
			if c.Name != "" {
				fmt.Fprintf(&b, "### %s\n\n", c.Name)
			}
			for _, rb := range c.Runbooks {
				fmt.Fprintf(&b, "- [%s](/%s.md)", rb.Title, rb.Slug)
				if rb.Description != "" {
					fmt.Fprintf(&b, ": %s", rb.Description)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}
