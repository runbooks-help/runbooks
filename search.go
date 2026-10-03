// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"net/http"
	"strconv"
	"strings"

	"runbooks/parser"
	"runbooks/views/markup"
)

// searchResult is one hit on the wire. Snippet is pre-escaped HTML with the query
// terms wrapped in <mark>, so the client can inject it directly.
type searchResult struct {
	Slug       string `json:"slug"`
	Title      string `json:"title"`
	System     string `json:"system"`
	Category   string `json:"category"`
	Step       string `json:"step,omitempty"`
	StepAnchor string `json:"stepAnchor,omitempty"`
	Snippet    string `json:"snippet"`
	Score      int    `json:"score"`
}

type searchResponse struct {
	Query   string         `json:"query"`
	Count   int            `json:"count"`
	Results []searchResult `json:"results"`
}

// handleSearch serves the in-memory body search: GET /api/runbooks/v1/search?q=…
// &limit=…. An empty query is an empty result set, not an error.
func handleSearch(index *parser.SearchIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		limit := parser.DefaultSearchLimit
		if v := r.URL.Query().Get("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				limit = n
			}
		}

		terms := strings.Fields(query)
		hits := index.Search(query, limit)
		resp := searchResponse{Query: query, Count: len(hits), Results: make([]searchResult, 0, len(hits))}
		for _, h := range hits {
			res := searchResult{
				Slug:     h.Slug,
				Title:    h.Title,
				System:   h.System,
				Category: h.Category,
				Step:     h.Step,
				Snippet:  markup.Highlight(h.Snippet, terms),
				Score:    h.Score,
			}
			if h.Step != "" {
				res.StepAnchor = markup.Slug(h.Step)
			}
			resp.Results = append(resp.Results, res)
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, resp)
	}
}
