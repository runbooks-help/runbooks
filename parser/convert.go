// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// mdParser is the CommonMark + GFM-table parser the converter walks. Product
// syntax (steps, sections, admonitions, directives, fences with labels) is
// stripped before goldmark sees it, so no extension is needed for it.
var mdParser = goldmark.New(goldmark.WithExtensions(extension.Table)).Parser()

// pieceKind distinguishes the pre-scanned product blocks from a markdown chunk
// goldmark parses.
type pieceKind int

const (
	pieceMarkdown pieceKind = iota
	pieceNotice
	pieceBranch
)

type piece struct {
	kind    pieceKind
	raw     string // pieceMarkdown: a markdown source chunk
	variant string // pieceNotice
	message string // pieceNotice
	body    string // pieceBranch
}

// region is one step (or the intro) with its blocks in source order. Product
// blocks are pre-scanned into pieces so admonitions, branches and directives
// keep their exact line semantics; the remaining markdown goes to goldmark.
type region struct {
	isStep   bool
	title    string
	section  bool
	rollback bool
	pieces   []piece
}

// convertBody reproduces Parse's body walk on top of goldmark. It returns the
// intro blocks and the numbered/section steps, split into the forward and
// rollback regions.
func convertBody(body string, sections bool) (intro []Block, steps, rollback []Step) {
	for _, r := range scanRegions(body, sections) {
		blocks := r.convert()
		if !r.isStep {
			// Legacy keeps only prose in the intro; other block kinds before
			// the first heading are dropped. Replicated for a neutral swap.
			for _, b := range blocks {
				if b.Kind == KindProse {
					intro = append(intro, b)
				}
			}
			continue
		}
		s := Step{Title: r.title, Section: r.section, Blocks: blocks}
		if r.rollback {
			rollback = append(rollback, s)
			continue
		}
		steps = append(steps, s)
	}
	return intro, steps, rollback
}

func (r region) convert() []Block {
	var blocks []Block
	for _, p := range r.pieces {
		switch p.kind {
		case pieceNotice:
			blocks = append(blocks, Block{Kind: KindNotice, Variant: p.variant, Message: p.message})
		case pieceBranch:
			blocks = append(blocks, Block{Kind: KindBranch, Body: p.body})
		case pieceMarkdown:
			blocks = append(blocks, convertMarkdown(p.raw)...)
		}
	}
	return blocks
}

// scanRegions splits the body into step/intro regions, absorbing the product
// directives and admonitions. Fences are tracked so a `##` or `---` inside a
// code block never starts a step or a directive.
func scanRegions(body string, sections bool) []region {
	var (
		regions  []region
		cur      = region{section: sections}
		md       []string
		inNotice bool
		noticeVn string
		noticeLn []string
		inBranch bool
		branchLn []string
		fence    int
	)
	rollback := false

	flushMarkdown := func() {
		if len(md) == 0 {
			return
		}
		cur.pieces = append(cur.pieces, piece{kind: pieceMarkdown, raw: strings.Join(md, "\n")})
		md = nil
	}
	flushNotice := func() {
		if !inNotice {
			return
		}
		cur.pieces = append(cur.pieces, piece{kind: pieceNotice, variant: noticeVn, message: strings.Join(noticeLn, "\n")})
		inNotice, noticeVn, noticeLn = false, "", nil
	}
	flushBranch := func() {
		if !inBranch {
			return
		}
		if b := strings.Join(branchLn, "\n"); strings.TrimSpace(b) != "" {
			cur.pieces = append(cur.pieces, piece{kind: pieceBranch, body: b})
		}
		inBranch, branchLn = false, nil
	}
	flushAll := func() { flushMarkdown(); flushNotice(); flushBranch() }
	closeRegion := func() {
		flushAll()
		if cur.isStep || len(cur.pieces) > 0 {
			regions = append(regions, cur)
		}
		cur = region{section: sections, rollback: rollback}
	}

	for line := range strings.SplitSeq(body, "\n") {
		if fence > 0 {
			md = append(md, line)
			if fenceClose(line, fence) {
				fence = 0
			}
			continue
		}
		if n := fenceOpen(line); n > 0 {
			md = append(md, line)
			fence = n
			continue
		}

		if inNotice {
			if m := blockquoteRe.FindStringSubmatch(line); m != nil && !noticeRe.MatchString(line) && !branchStartRe.MatchString(line) {
				noticeLn = append(noticeLn, m[1])
				continue
			}
			flushNotice()
		}
		if inBranch {
			if m := blockquoteRe.FindStringSubmatch(line); m != nil {
				branchLn = append(branchLn, m[1])
				continue
			}
			flushBranch()
		}

		if dir, ok := parseDirective(line); ok {
			closeRegion()
			switch dir {
			case "rollback":
				rollback = true
				cur.rollback = true
			case "sections":
				sections = true
				cur.section = true
			case "steps":
				sections = false
				cur.section = false
			}
			continue
		}

		if strings.HasPrefix(line, "## ") {
			closeRegion()
			cur.isStep = true
			cur.title = strings.TrimPrefix(line, "## ")
			continue
		}

		if m := noticeRe.FindStringSubmatch(line); m != nil {
			flushMarkdown()
			inNotice = true
			noticeVn = noticeVariant(m[1])
			noticeLn = []string{m[2]}
			continue
		}
		if branchStartRe.MatchString(line) {
			flushMarkdown()
			inBranch = true
			branchLn = nil
			continue
		}

		md = append(md, line)
	}
	closeRegion()
	return regions
}

// fenceOpen reports the length of a backtick fence opening on a line, or 0.
// The closing check accepts a run at least as long as the opener, so a
// four-backtick block can contain a three-backtick line.
func fenceOpen(line string) int {
	i := 0
	for i < len(line) && line[i] == ' ' && i < 3 {
		i++
	}
	start := i
	for i < len(line) && line[i] == '`' {
		i++
	}
	if n := i - start; n >= 3 {
		return n
	}
	return 0
}

// fenceClose reports whether line closes a fence opened with n backticks.
func fenceClose(line string, n int) bool {
	i := 0
	for i < len(line) && line[i] == ' ' && i < 3 {
		i++
	}
	start := i
	for i < len(line) && line[i] == '`' {
		i++
	}
	if i-start < n {
		return false
	}
	return strings.TrimSpace(line[i:]) == ""
}

// convertMarkdown parses a markdown chunk and converts its top-level blocks.
func convertMarkdown(source string) []Block {
	src := []byte(source)
	doc := mdParser.Parse(text.NewReader(src))
	var blocks []Block
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		blocks = append(blocks, convertNode(src, n)...)
	}
	return blocks
}

func convertNode(src []byte, n ast.Node) []Block {
	switch n := n.(type) {
	case *ast.Paragraph:
		return proseBlock(rawLines(src, n.Lines()))
	case *ast.TextBlock:
		return proseBlock(rawLines(src, n.Lines()))
	case *ast.Heading:
		if t := rawLines(src, n.Lines()); strings.TrimSpace(t) != "" {
			return []Block{{Kind: KindHeading, Text: t}}
		}
		return nil
	case *ast.FencedCodeBlock:
		lang, label := fenceInfo(n, src)
		return []Block{{Kind: KindCode, Lang: lang, Label: label, Body: linesBody(src, n.Lines())}}
	case *ast.CodeBlock:
		return []Block{{Kind: KindCode, Body: linesBody(src, n.Lines())}}
	case *ast.List:
		return []Block{convertList(src, n)}
	case *extensionast.Table:
		return []Block{convertTable(src, n)}
	case *ast.Blockquote:
		if t := rawLines(src, n.Lines()); strings.TrimSpace(t) != "" {
			return []Block{{Kind: KindProse, Text: t}}
		}
		return nil
	case *ast.ThematicBreak:
		return proseBlock("---")
	case *ast.HTMLBlock:
		if t := linesBody(src, n.Lines()); strings.TrimSpace(t) != "" {
			return []Block{{Kind: KindProse, Text: t}}
		}
		return nil
	default:
		return nil
	}
}

func proseBlock(text string) []Block {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return []Block{{Kind: KindProse, Text: text}}
}

func convertList(src []byte, n *ast.List) Block {
	b := Block{Kind: KindList, Ordered: n.IsOrdered()}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		item, ok := c.(*ast.ListItem)
		if !ok {
			continue
		}
		b.Items = append(b.Items, convertListItem(src, item))
	}
	return b
}

// convertListItem keeps an item's own paragraph(s) as Text and a nested list as
// Child. Any other block (a code block, a blockquote, a second nested list) has
// no structural slot, so its text is folded into Text — lossy in structure, but
// never silently dropped.
func convertListItem(src []byte, item *ast.ListItem) ListItem {
	var li ListItem
	var extra []string
	for c := item.FirstChild(); c != nil; c = c.NextSibling() {
		switch n := c.(type) {
		case *ast.Paragraph:
			li.Text = joinText(li.Text, rawLines(src, n.Lines()))
		case *ast.TextBlock:
			li.Text = joinText(li.Text, rawLines(src, n.Lines()))
		case *ast.List:
			if li.Child == nil {
				child := convertList(src, n)
				li.Child = &child
				continue
			}
			extra = append(extra, listText(src, n))
		default:
			if t := strings.TrimSpace(blockPlainText(src, c)); t != "" {
				extra = append(extra, t)
			}
		}
	}
	if len(extra) > 0 {
		li.Text = joinText(li.Text, strings.Join(extra, " "))
	}
	return li
}

// listText is the plain text of a nested list, used only for the fallback when
// an item already has a Child.
func listText(src []byte, list *ast.List) string {
	var parts []string
	for c := list.FirstChild(); c != nil; c = c.NextSibling() {
		item, ok := c.(*ast.ListItem)
		if !ok {
			continue
		}
		li := convertListItem(src, item)
		parts = append(parts, li.Text)
		if li.Child != nil {
			parts = append(parts, itemsText(li.Child.Items))
		}
	}
	return strings.Join(parts, " ")
}

func itemsText(items []ListItem) string {
	parts := make([]string, 0, len(items))
	for _, li := range items {
		parts = append(parts, li.Text)
		if li.Child != nil {
			parts = append(parts, itemsText(li.Child.Items))
		}
	}
	return strings.Join(parts, " ")
}

// blockPlainText is a best-effort plain text of a block goldmark parsed inside a
// list item but our model cannot place.
func blockPlainText(src []byte, n ast.Node) string {
	if ln, ok := n.(interface{ Lines() *text.Segments }); ok {
		if t := rawLines(src, ln.Lines()); t != "" {
			return t
		}
	}
	var parts []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t := blockPlainText(src, c); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ")
}

func joinText(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + " " + b
}

func convertTable(src []byte, n *extensionast.Table) Block {
	b := Block{Kind: KindTable}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch row := c.(type) {
		case *extensionast.TableHeader:
			b.Headers = tableCells(src, row)
		case *extensionast.TableRow:
			b.Rows = append(b.Rows, tableCells(src, row))
		}
	}
	return b
}

func tableCells(src []byte, row ast.Node) []string {
	var cells []string
	for cc := row.FirstChild(); cc != nil; cc = cc.NextSibling() {
		cell, ok := cc.(*extensionast.TableCell)
		if !ok {
			continue
		}
		cells = append(cells, rawLines(src, cell.Lines()))
	}
	return cells
}

func fenceInfo(n *ast.FencedCodeBlock, src []byte) (lang, label string) {
	if n.Info == nil {
		return "", ""
	}
	seg := n.Info.Segment
	if seg.Stop > len(src) || seg.Start < 0 || seg.Start > seg.Stop {
		return "", ""
	}
	if m := fenceInfoRe.FindStringSubmatch(strings.TrimSpace(string(src[seg.Start:seg.Stop]))); m != nil {
		return m[1], m[2]
	}
	return "", ""
}

// rawLines joins a block's source lines, each trimmed, with a single space —
// the inline-markdown text the Block model carries and Prose renders.
func rawLines(src []byte, lines *text.Segments) string {
	if lines == nil || lines.Len() == 0 {
		return ""
	}
	parts := make([]string, 0, lines.Len())
	for i := 0; i < lines.Len(); i++ {
		parts = append(parts, strings.TrimSpace(segmentValue(src, lines.At(i))))
	}
	return strings.Join(parts, " ")
}

// linesBody joins a block's raw source lines with newlines (code blocks).
func linesBody(src []byte, lines *text.Segments) string {
	if lines == nil || lines.Len() == 0 {
		return ""
	}
	parts := make([]string, 0, lines.Len())
	for i := 0; i < lines.Len(); i++ {
		parts = append(parts, strings.TrimSuffix(segmentValue(src, lines.At(i)), "\n"))
	}
	return strings.Join(parts, "\n")
}

func segmentValue(src []byte, s text.Segment) string {
	if s.Start < 0 {
		s.Start = 0
	}
	if s.Stop > len(src) {
		s.Stop = len(src)
	}
	if s.Start > s.Stop {
		return ""
	}
	return string(src[s.Start:s.Stop])
}
