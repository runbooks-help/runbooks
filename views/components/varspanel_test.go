// SPDX-License-Identifier: FSL-1.1-MIT

package components

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"runbooks/parser"
)

func testVarFields(n int) []parser.VarField {
	out := make([]parser.VarField, n)
	for i := range out {
		out[i] = parser.VarField{
			ID:      "field-" + strconv.Itoa(i),
			Label:   "Field " + strconv.Itoa(i),
			VarName: "VAR_" + strconv.Itoa(i),
		}
	}
	return out
}

func renderVars(t *testing.T, n int) string {
	t.Helper()
	var b strings.Builder
	if err := VarsPanel(testVarFields(n)).Render(context.Background(), &b); err != nil {
		t.Fatalf("render VarsPanel: %v", err)
	}
	return b.String()
}

func TestVarsPanelInlineWhenFew(t *testing.T) {
	two := renderVars(t, 2)
	if strings.Contains(two, "vars-summary") || strings.Contains(two, "data-vars-dialog") {
		t.Errorf("two variables should render inline, not as a summary + editor:\n%s", two)
	}
	if !strings.Contains(two, `data-var="VAR_0"`) {
		t.Errorf("inline inputs missing:\n%s", two)
	}
}

func TestVarsPanelSummaryAndEditorWhenMany(t *testing.T) {
	five := renderVars(t, 5)
	if !strings.Contains(five, `class="vars-summary"`) || !strings.Contains(five, "data-vars-open") {
		t.Errorf("five variables should collapse to a summary trigger:\n%s", five)
	}
	if !strings.Contains(five, "data-vars-dialog") {
		t.Errorf("five variables should carry the editor dialog:\n%s", five)
	}
	if !strings.Contains(five, "Field 0 · Field 1 · Field 2 · Field 3 · Field 4") {
		t.Errorf("summary should preview the labels:\n%s", five)
	}
	if !strings.Contains(five, `data-var="VAR_0"`) {
		t.Errorf("editor inputs missing:\n%s", five)
	}
}

func TestVarsPanelHasNoHeading(t *testing.T) {
	html := renderVars(t, 3)
	if strings.Contains(html, ">Variables<") {
		t.Errorf("the standalone Variables heading should be gone:\n%s", html)
	}
}
