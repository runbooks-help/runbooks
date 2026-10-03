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

func TestVarsPanelFoldsOnlyWhenMany(t *testing.T) {
	two := renderVars(t, 2)
	if strings.Contains(two, "is-folded") {
		t.Errorf("two variables should not fold by default:\n%s", two)
	}
	five := renderVars(t, 5)
	if !strings.Contains(five, "is-folded") {
		t.Errorf("five variables should fold by default:\n%s", five)
	}
}

func TestVarsPanelHasNoHeadingAndPreviewsLabels(t *testing.T) {
	html := renderVars(t, 3)
	if strings.Contains(html, ">Variables<") {
		t.Errorf("the standalone Variables heading should be gone:\n%s", html)
	}
	if !strings.Contains(html, "Field 0 · Field 1 · Field 2") {
		t.Errorf("folded summary should preview the labels:\n%s", html)
	}
	// The fields themselves are still rendered, just hidden by CSS when folded.
	if !strings.Contains(html, `data-var="VAR_0"`) {
		t.Errorf("variable inputs missing:\n%s", html)
	}
}
