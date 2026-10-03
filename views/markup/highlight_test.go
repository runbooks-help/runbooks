// SPDX-License-Identifier: FSL-1.1-MIT

package markup

import "testing"

func TestHighlightMarksTerms(t *testing.T) {
	got := Highlight("shows Last_SQL_Error here", []string{"last_sql_error"})
	want := "shows <mark>Last_SQL_Error</mark> here"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHighlightEscapes(t *testing.T) {
	got := Highlight("<b>&x</b>", []string{"x"})
	want := "&lt;b&gt;&amp;<mark>x</mark>&lt;/b&gt;"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHighlightPrefersLongerPhrase(t *testing.T) {
	got := Highlight("replication lag today", []string{"lag", "replication lag"})
	want := "<mark>replication lag</mark> today"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHighlightNoTermsEscapesOnly(t *testing.T) {
	got := Highlight("<i>x</i>", nil)
	want := "&lt;i&gt;x&lt;/i&gt;"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHighlightTreatsTermsLiterally(t *testing.T) {
	got := Highlight("a.c", []string{"."})
	want := "a<mark>.</mark>c"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
