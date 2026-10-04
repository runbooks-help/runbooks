// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"strings"
	"testing"
)

// TestParseNoticeVariants pins that the marker is case-insensitive and accepts
// the GitHub alert keywords, so a `> [!WARNING]` copied from a repo renders as
// a notice rather than raw prose.
func TestParseNoticeVariants(t *testing.T) {
	cases := []struct{ marker, want string }{
		{"[!WARNING]", "warn"},
		{"[!warning]", "warn"},
		{"[!warn]", "warn"},
		{"[!IMPORTANT]", "warn"},
		{"[!CAUTION]", "warn"},
		{"[!NOTE]", "info"},
		{"[!TIP]", "info"},
		{"[!info]", "info"},
		{"[!DANGER]", "danger"},
		{"[!ERROR]", "danger"},
	}
	for _, c := range cases {
		src := "---\ntitle: X\nslug: x\n---\n\n## Step\n\n> " + c.marker + "\n> the body\n"
		def, err := Parse([]byte(src))
		if err != nil {
			t.Fatalf("%s: Parse: %v", c.marker, err)
		}
		if len(def.Steps) != 1 || len(def.Steps[0].Blocks) != 1 {
			t.Fatalf("%s: blocks = %#v", c.marker, def.Steps[0].Blocks)
		}
		b := def.Steps[0].Blocks[0]
		if b.Kind != KindNotice || b.Variant != c.want || strings.TrimSpace(b.Message) != "the body" {
			t.Errorf("%s: got kind=%s variant=%s msg=%q, want notice/%s/%q",
				c.marker, b.Kind, b.Variant, b.Message, c.want, "the body")
		}
	}
}
