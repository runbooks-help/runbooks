// SPDX-License-Identifier: FSL-1.1-MIT

package markup

import "testing"

func TestProseGlossaryWrapsTerm(t *testing.T) {
	got := ProseGlossary("Check MTS status", []string{"MTS"})
	want := `Check <abbr class="glossary-term" tabindex="0" data-glossary="MTS">MTS</abbr> status`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestProseGlossaryIsCaseSensitive(t *testing.T) {
	got := ProseGlossary("Check mts status", []string{"MTS"})
	if got != "Check mts status" {
		t.Fatalf("lowercase should not match: %q", got)
	}
}

func TestProseGlossaryIsWholeWord(t *testing.T) {
	got := ProseGlossary("MTSomething and Last_SQL_Error", []string{"MTS", "SQL"})
	if got != "MTSomething and Last_SQL_Error" {
		t.Fatalf("partial words should not match: %q", got)
	}
}

func TestProseGlossarySkipsCodeAndLinks(t *testing.T) {
	got := ProseGlossary("run `MTS` then [MTS](https://mts.example/x)", []string{"MTS"})
	want := "run <code>MTS</code> then <a href=\"https://mts.example/x\">MTS</a>"
	if got != want {
		t.Fatalf("code and links must be skipped: got %q", got)
	}
}

func TestProseGlossaryPrefersLongerTerm(t *testing.T) {
	got := ProseGlossary("GTID mode", []string{"GT", "GTID"})
	want := `<abbr class="glossary-term" tabindex="0" data-glossary="GTID">GTID</abbr> mode`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestProseGlossaryNoTermsIsPlainProse(t *testing.T) {
	got := ProseGlossary("**bold** MTS", nil)
	if got != Prose("**bold** MTS") {
		t.Fatalf("nil terms should equal Prose: %q", got)
	}
}
