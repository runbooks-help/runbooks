// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"runbooks/parser"
)

// searchIndex is a small two-runbook index with a step-level hit.
func searchIndex() *parser.SearchIndex {
	return parser.BuildIndex([]parser.RunbookDef{
		{
			RunbookMeta: parser.RunbookMeta{
				Title:         "Replication Lag (MTS Deadlock)",
				Slug:          "mts-deadlock",
				Description:   "A parallel worker deadlock stalls the replica.",
				GroupTitle:    "MySQL",
				CategoryTitle: "Replication",
			},
			Steps: []parser.Step{
				{Title: "Inspect the replica", Blocks: []parser.Block{
					{Kind: parser.KindProse, Text: "Check SHOW SLAVE STATUS."},
				}},
				{Title: "Clear the blocked worker", Blocks: []parser.Block{
					{Kind: parser.KindCode, Lang: "sql", Body: "KILL 4123; -- ER_LOCK_DEADLOCK"},
				}},
			},
		},
		{
			RunbookMeta: parser.RunbookMeta{
				Title:         "Deadlock Recovery",
				Slug:          "deadlock-recovery",
				Description:   "Recover from a stuck deadlock.",
				GroupTitle:    "MySQL",
				CategoryTitle: "Replication",
			},
			Steps: []parser.Step{{Title: "Restart", Blocks: []parser.Block{
				{Kind: parser.KindProse, Text: "Restart the replica."},
			}}},
		},
	})
}

func TestHandleSearch(t *testing.T) {
	handler := handleSearch(searchIndex())

	search := func(target string) (int, searchResponse) {
		t.Helper()
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(http.MethodGet, target, nil))
		var resp searchResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v (body %s)", err, rec.Body.String())
		}
		return rec.Code, resp
	}

	// Only GET is served.
	rec := httptest.NewRecorder()
	handler(rec, httptest.NewRequest(http.MethodPost, "/api/runbooks/v1/search?q=x", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST search = %d, want 405", rec.Code)
	}

	// A matching query returns a highlighted snippet and the query is echoed.
	code, resp := search("/api/runbooks/v1/search?q=deadlock")
	if code != http.StatusOK {
		t.Fatalf("search = %d, want 200", code)
	}
	if resp.Query != "deadlock" || resp.Count != 2 || len(resp.Results) != 2 {
		t.Fatalf("search = query %q count %d results %d, want deadlock/2/2", resp.Query, resp.Count, len(resp.Results))
	}
	if !strings.Contains(resp.Results[0].Snippet, "<mark>") {
		t.Errorf("snippet is not highlighted: %q", resp.Results[0].Snippet)
	}
	if resp.Results[0].System != "MySQL" || resp.Results[0].Category != "Replication" {
		t.Errorf("system/category not carried: %+v", resp.Results[0])
	}

	// A step-level hit carries its anchor.
	_, stepResp := search("/api/runbooks/v1/search?q=ER_LOCK_DEADLOCK")
	if len(stepResp.Results) != 1 {
		t.Fatalf("step search returned %d results, want 1", len(stepResp.Results))
	}
	if got := stepResp.Results[0]; got.Step != "Clear the blocked worker" || got.StepAnchor != "clear-the-blocked-worker" {
		t.Errorf("step hit = %q / %q, want the step title and its anchor", got.Step, got.StepAnchor)
	}

	// limit caps the result count; a bad limit falls back to the default.
	if _, limited := search("/api/runbooks/v1/search?q=deadlock&limit=1"); limited.Count != 1 {
		t.Errorf("limit=1 returned %d results, want 1", limited.Count)
	}
	if _, bad := search("/api/runbooks/v1/search?q=deadlock&limit=notanumber"); bad.Count != 2 {
		t.Errorf("a non-numeric limit returned %d results, want the default (2)", bad.Count)
	}

	// An empty query is an empty result set, not an error.
	code, empty := search("/api/runbooks/v1/search?q=%20%20")
	if code != http.StatusOK || empty.Count != 0 || len(empty.Results) != 0 {
		t.Errorf("empty query = %d count %d, want 200/0", code, empty.Count)
	}
}
