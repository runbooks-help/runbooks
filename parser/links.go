// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"path"
	"regexp"
	"strings"
)

// mdLinkRe matches a Markdown inline link, mirroring views/markup's inline
// renderer so anything rewritten here is still rendered as a link.
var mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

// rewriteDocLinks rewrites relative Markdown links between content files to the
// served slug route: [x](operations.md#ssh) becomes [x](/operations#ssh). rels
// holds each def's content-root-relative path without the .md extension, so a
// link resolves against the linking file's directory. Absolute, scheme-bearing
// and non-.md links are left alone, as is the raw Source.
func rewriteDocLinks(defs []RunbookDef, rels []string) {
	slugByRel := make(map[string]string, len(defs))
	for i, rel := range rels {
		slugByRel[rel] = defs[i].Slug
	}
	for i := range defs {
		dir := path.Dir(rels[i])
		rewrite := func(text string) string {
			return mdLinkRe.ReplaceAllStringFunc(text, func(m string) string {
				sub := mdLinkRe.FindStringSubmatch(m)
				href, frag, hasFrag := strings.Cut(sub[2], "#")
				if !strings.HasSuffix(href, ".md") ||
					strings.Contains(href, "://") ||
					strings.HasPrefix(href, "/") {
					return m
				}
				slug, ok := slugByRel[path.Join(dir, strings.TrimSuffix(href, ".md"))]
				if !ok {
					return m
				}
				out := "/" + slug
				if hasFrag {
					out += "#" + frag
				}
				return "[" + sub[1] + "](" + out + ")"
			})
		}
		rewriteBlocks(defs[i].Intro, rewrite)
		for si := range defs[i].Steps {
			rewriteBlocks(defs[i].Steps[si].Blocks, rewrite)
		}
		for si := range defs[i].Rollback {
			rewriteBlocks(defs[i].Rollback[si].Blocks, rewrite)
		}
	}
}

// rewriteBlocks applies rewrite to every text-bearing field of blocks.
func rewriteBlocks(blocks []Block, rewrite func(string) string) {
	for i := range blocks {
		rewriteBlock(&blocks[i], rewrite)
	}
}

func rewriteBlock(b *Block, rewrite func(string) string) {
	switch b.Kind {
	case KindProse:
		b.Text = rewrite(b.Text)
	case KindNotice:
		b.Message = rewrite(b.Message)
	case KindBranch:
		b.Body = rewrite(b.Body)
	case KindList:
		for j := range b.Items {
			b.Items[j].Text = rewrite(b.Items[j].Text)
			if b.Items[j].Child != nil {
				rewriteBlock(b.Items[j].Child, rewrite)
			}
		}
	case KindTable:
		for j := range b.Headers {
			b.Headers[j] = rewrite(b.Headers[j])
		}
		for j := range b.Rows {
			for k := range b.Rows[j] {
				b.Rows[j][k] = rewrite(b.Rows[j][k])
			}
		}
	}
}
