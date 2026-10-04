// SPDX-License-Identifier: FSL-1.1-MIT

package markup

import "testing"

// TestProseInline covers the inline forms the old regex renderer either
// handled differently (bold, italic, code, links) or could not handle at all
// (underscore emphasis, autolinks, entity round-tripping).
func TestProseInline(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain text", "plain text"},
		{"**bold**", "<strong>bold</strong>"},
		{"*italic*", "<em>italic</em>"},
		{"_italic_", "<em>italic</em>"},
		{"a_b_c", "a_b_c"}, // intraword underscores are not emphasis
		{"`code **not bold**`", "<code>code **not bold**</code>"},
		{"[text](https://example.com)", `<a href="https://example.com">text</a>`},
		{"<https://example.com>", `<a href="https://example.com">https://example.com</a>`},
		{"<b>raw</b>", "&lt;b&gt;raw&lt;/b&gt;"},  // raw HTML is shown literally
		{"AT&amp;T", "AT&amp;T"},                  // entities round-trip
		{"`a&amp;b`", "<code>a&amp;amp;b</code>"}, // code spans stay literal
		{"5 < 6", "5 &lt; 6"},
	}
	for _, c := range cases {
		if got := Prose(c.in); got != c.want {
			t.Errorf("Prose(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestProseGlossaryInEmphasis pins that glossary terms are still wrapped inside
// emphasis, while code and link text stay untouched.
func TestProseGlossaryInEmphasis(t *testing.T) {
	got := ProseGlossary("**MTS** and `MTS` and [MTS](https://x)", []string{"MTS"})
	want := `<strong><abbr class="glossary-term" tabindex="0" data-glossary="MTS">MTS</abbr></strong>` +
		" and <code>MTS</code> and " +
		`<a href="https://x">MTS</a>`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
