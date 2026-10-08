// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// skillNameRe is the agentskills.io `name` constraint: lowercase alphanumerics
// and single hyphens, no leading/trailing/double hyphen.
var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// TestReviewSkillFrontmatter pins the served file to the Agent Skills spec:
// frontmatter with a `name` that matches the directory and a `description`.
func TestReviewSkillFrontmatter(t *testing.T) {
	src := string(runbookReviewSkill)
	if !strings.HasPrefix(src, "---\n") {
		t.Fatal("skill must open with frontmatter")
	}
	before, _, ok := strings.Cut(src[4:], "\n---\n")
	if !ok {
		t.Fatal("skill frontmatter is unclosed")
	}
	var fm struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(before), &fm); err != nil {
		t.Fatalf("frontmatter: %v", err)
	}
	if fm.Name != "runbook-review" {
		t.Errorf("name = %q, want runbook-review (must match the directory)", fm.Name)
	}
	if len(fm.Name) > 64 || !skillNameRe.MatchString(fm.Name) {
		t.Errorf("name = %q violates the agentskills.io name rule", fm.Name)
	}
	if n := len(fm.Description); n == 0 || n > 1024 {
		t.Errorf("description length = %d, want 1..1024", n)
	}
}

// TestServeReviewSkill pins the handler: GET returns the embedded skill
// verbatim as markdown; a write method is refused.
func TestServeReviewSkill(t *testing.T) {
	rec := httptest.NewRecorder()
	serveReviewSkill(rec, httptest.NewRequest(http.MethodGet, "/skill/runbook-review.md", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/markdown; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/markdown", ct)
	}
	if rec.Body.String() != string(runbookReviewSkill) {
		t.Errorf("body is not the embedded skill verbatim")
	}

	rec = httptest.NewRecorder()
	serveReviewSkill(rec, httptest.NewRequest(http.MethodPost, "/skill/runbook-review.md", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", rec.Code)
	}
}

// TestReviewSkillRoute pins that the skill is reachable on the built mux, so it
// cannot be embedded but unserved.
func TestReviewSkillRoute(t *testing.T) {
	dir := t.TempDir()
	writeRunbook(t, dir, "first.md", "First", "first")
	cs, err := newContentState(config{ContentSource: "local", ContentDir: dir}, nil, nil)
	if err != nil {
		t.Fatalf("newContentState: %v", err)
	}
	rec := httptest.NewRecorder()
	cs.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/skill/runbook-review.md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != string(runbookReviewSkill) {
		t.Errorf("route body is not the embedded skill")
	}
}

// TestReviewSkillFetchesRecords pins the remote path: the skill must name the
// records surface, or a remote agent has no way to reach the notes the review
// is built on.
func TestReviewSkillFetchesRecords(t *testing.T) {
	if !strings.Contains(string(runbookReviewSkill), "/api/runbooks/v1/notes") {
		t.Error("skill does not name the records surface; a remote reviewer cannot fetch the notes")
	}
}

// TestLLMSIndexListsSkill pins discoverability: the skill is advertised in the
// same index an agent already reads.
func TestLLMSIndexListsSkill(t *testing.T) {
	body := renderLLMSIndex(llmsGroups())
	if !strings.Contains(body, "## Skills") {
		t.Errorf("llms index has no Skills section:\n%s", body)
	}
	if !strings.Contains(body, "[runbook-review](/skill/runbook-review.md)") {
		t.Errorf("llms index does not link the skill:\n%s", body)
	}
}
