package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"runbooks/parser"
)

// llmsGroups is a two-runbook, one-system grouping used by the index tests.
func llmsGroups() []parser.SystemGroup {
	return parser.GroupBySystem([]parser.RunbookDef{
		{RunbookMeta: parser.RunbookMeta{
			Title: "Replication Lag", Slug: "mts-deadlock",
			Description: "A worker deadlock stalls the replica.",
			Group:       "mysql", GroupTitle: "MySQL",
			Category: "replication", CategoryTitle: "Replication",
		}},
		{RunbookMeta: parser.RunbookMeta{
			Title: "Failover", Slug: "failover",
			Group: "mysql", GroupTitle: "MySQL",
		}},
	})
}

func TestRenderLLMSIndex(t *testing.T) {
	body := renderLLMSIndex(llmsGroups())
	for _, want := range []string{
		"# Runbooks",
		"## MySQL",
		"- [Replication Lag](/mts-deadlock.md): A worker deadlock stalls the replica.",
		"- [Failover](/failover.md)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("llms index missing %q\n---\n%s", want, body)
		}
	}
	// A runbook with no description gets no dangling separator.
	if strings.Contains(body, "/failover.md):") {
		t.Errorf("descriptionless runbook gained a separator:\n%s", body)
	}
}

func TestHandleLLMSIndex(t *testing.T) {
	rec := httptest.NewRecorder()
	handleLLMSIndex(llmsGroups())(rec, httptest.NewRequest(http.MethodGet, "/llms.txt", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/markdown", ct)
	}
	if !strings.Contains(rec.Body.String(), "# Runbooks") {
		t.Errorf("body missing heading:\n%s", rec.Body.String())
	}
}

func TestServeMarkdown(t *testing.T) {
	const source = "---\ntitle: X\nslug: x\n---\n\n## Step\n"
	rb := parser.RunbookDef{RunbookMeta: parser.RunbookMeta{Slug: "x"}, Source: source}

	rec := httptest.NewRecorder()
	serveMarkdown(rec, httptest.NewRequest(http.MethodGet, "/x.md", nil), rb)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/markdown", ct)
	}
	if rec.Body.String() != source {
		t.Errorf("body = %q, want the raw source verbatim", rec.Body.String())
	}
}
