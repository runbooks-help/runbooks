// SPDX-License-Identifier: FSL-1.1-MIT

package main

import (
	_ "embed"
	"net/http"
)

// runbookReviewSkill is the shipped agent skill, embedded so the served bytes
// ARE the repo file: the skill cannot drift from the format it reviews.
//
//go:embed skills/runbook-review/SKILL.md
var runbookReviewSkill []byte

// llmsSkillLine is the skill's entry in the llms.txt index.
const llmsSkillLine = "- [runbook-review](/skill/runbook-review.md): reviews a runbook against the notes captured during its executions and proposes classified edits for a human to apply.\n"

// serveReviewSkill serves the runbook-review agent skill verbatim as markdown.
// It is a read-only machine endpoint, gated like the other agent surfaces.
func serveReviewSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(runbookReviewSkill)
}
