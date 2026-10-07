// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite the corpus golden files")

// trackedCorpus lists the committed documents whose parsed Block model is
// pinned: the playground runbooks. The gitignored testdata runbooks are private
// and deliberately excluded, and the public docs moved to the separate
// runbooks-docs repo.
func trackedCorpus(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{filepath.Join("..", "content")} {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				return err
			}
			if rel, _ := filepath.Rel(dir, path); strings.HasPrefix(filepath.ToSlash(rel), "testdata/") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	return files
}

// TestCorpusGolden pins the parsed Block model of the committed corpus. It is
// the broad regression net for the goldmark converter: any change to the model
// (a kind, an order, a text run) shows up as a golden diff. Run with
// `go test ./parser -update` to accept an intended change.
func TestCorpusGolden(t *testing.T) {
	files := trackedCorpus(t)
	if len(files) == 0 {
		t.Fatal("no corpus files found")
	}
	var b strings.Builder
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		def, err := Parse(data)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		fmt.Fprintf(&b, "===== %s =====\n", filepath.ToSlash(f))
		dumpDef(&b, def)
	}

	golden := filepath.Join("testdata", "corpus.golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden (run `go test ./parser -update`): %v", err)
	}
	if got := b.String(); got != string(want) {
		t.Errorf("corpus golden out of date; run `go test ./parser -update`\n--- got ---\n%s", firstDiff(want, []byte(got)))
	}
}

func dumpDef(b *strings.Builder, def RunbookDef) {
	if len(def.Intro) > 0 {
		b.WriteString("intro:\n")
		dumpBlocks(b, def.Intro)
	}
	for _, s := range def.Steps {
		dumpStep(b, "step", s)
	}
	for _, s := range def.Rollback {
		dumpStep(b, "rollback", s)
	}
}

func dumpStep(b *strings.Builder, kind string, s Step) {
	fmt.Fprintf(b, "%s: %q section=%v\n", kind, s.Title, s.Section)
	dumpBlocks(b, s.Blocks)
}

func dumpBlocks(b *strings.Builder, blocks []Block) {
	for _, blk := range blocks {
		switch blk.Kind {
		case KindProse:
			fmt.Fprintf(b, "  prose %q\n", blk.Text)
		case KindHeading:
			fmt.Fprintf(b, "  heading %q\n", blk.Text)
		case KindCode:
			fmt.Fprintf(b, "  code lang=%q label=%q body=%q\n", blk.Lang, blk.Label, blk.Body)
		case KindNotice:
			fmt.Fprintf(b, "  notice variant=%q message=%q\n", blk.Variant, blk.Message)
		case KindBranch:
			fmt.Fprintf(b, "  branch %q\n", blk.Body)
		case KindLookalike:
			fmt.Fprintf(b, "  lookalike title=%q body=%q\n", blk.Title, blk.Body)
		case KindList:
			fmt.Fprintf(b, "  list ordered=%v\n", blk.Ordered)
			dumpListItems(b, blk.Items, "    ")
		case KindTable:
			fmt.Fprintf(b, "  table headers=%q\n", blk.Headers)
			for _, row := range blk.Rows {
				fmt.Fprintf(b, "    row %q\n", row)
			}
		}
	}
}

func dumpListItems(b *strings.Builder, items []ListItem, indent string) {
	for _, item := range items {
		fmt.Fprintf(b, "%sitem %q\n", indent, item.Text)
		if item.Child != nil {
			fmt.Fprintf(b, "%schild ordered=%v\n", indent, item.Child.Ordered)
			dumpListItems(b, item.Child.Items, indent+"  ")
		}
	}
}

// firstDiff is a small line-oriented helper so a golden failure points at the
// first differing line instead of dumping both documents.
func firstDiff(want, got []byte) string {
	wl, gl := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			return fmt.Sprintf("line %d:\n  want: %s\n  got:  %s", i+1, w, g)
		}
	}
	return "(documents are equal)"
}
