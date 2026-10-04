// SPDX-License-Identifier: FSL-1.1-MIT

package views

import (
	"strings"
	"testing"

	"runbooks/parser"
)

// TestGlossaryScriptJSONKeys pins the wire contract the client reads: the popup
// JS looks up entry.term/expansion/description/link, so the keys must be
// lowercase, not Go's default field names.
func TestGlossaryScriptJSONKeys(t *testing.T) {
	got := glossaryScript([]parser.GlossaryEntry{{Term: "MTS", Expansion: "Multi-Threaded Replica", Description: "d", Link: "l"}})
	for _, want := range []string{`"term":"MTS"`, `"expansion":"Multi-Threaded Replica"`, `"description":"d"`, `"link":"l"`} {
		if !strings.Contains(got, want) {
			t.Errorf("glossaryScript missing %s in %q", want, got)
		}
	}
}

func TestGlossaryScriptEmptyEmitsNothing(t *testing.T) {
	if got := glossaryScript(nil); got != "" {
		t.Fatalf("no glossary should emit no script, got %q", got)
	}
}
