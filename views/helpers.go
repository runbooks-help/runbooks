package views

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"runbooks/parser"
)

var (
	varRe    = regexp.MustCompile(`\{\{([A-Z_]+)\}\}`)
	boldRe   = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	italicRe = regexp.MustCompile(`\*([^*\n]+)\*`)
	codeRe   = regexp.MustCompile("`([^`\n]+)`")
	linkRe   = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	slugRe   = regexp.MustCompile(`[^a-z0-9]+`)
)

// toInitialDisplay replaces {{VAR_NAME}} tokens with <VAR_NAME> for the initial
// render before any variable values are entered.
func toInitialDisplay(tmpl string) string {
	return varRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		return "<" + m[2:len(m)-2] + ">"
	})
}

// proseHTML escapes text for safe HTML injection then applies inline Markdown:
// **bold**, *italic*, `code`, [text](url).
func proseHTML(text string) string {
	s := html.EscapeString(text)
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")
	s = codeRe.ReplaceAllString(s, "<code>$1</code>")
	s = linkRe.ReplaceAllString(s, `<a href="$2">$1</a>`)
	return s
}

// slugify converts a step title to a URL-safe anchor id.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// branchHTML converts a branch block body (markdown list lines) to an HTML list.
func branchHTML(body string) string {
	var buf strings.Builder
	buf.WriteString("<ul>")
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") {
			buf.WriteString("<li>")
			buf.WriteString(proseHTML(line[2:]))
			buf.WriteString("</li>")
		}
	}
	buf.WriteString("</ul>")
	return buf.String()
}

// noticeHTML renders notice body text as HTML, supporting paragraphs and lists.
func noticeHTML(msg string) string {
	var buf strings.Builder
	var listItems []string
	var paraLines []string

	flushPara := func() {
		text := strings.TrimSpace(strings.Join(paraLines, " "))
		if text != "" {
			buf.WriteString("<p>")
			buf.WriteString(proseHTML(text))
			buf.WriteString("</p>")
		}
		paraLines = nil
	}
	flushList := func() {
		if len(listItems) == 0 {
			return
		}
		buf.WriteString("<ul>")
		for _, item := range listItems {
			buf.WriteString("<li>")
			buf.WriteString(proseHTML(item))
			buf.WriteString("</li>")
		}
		buf.WriteString("</ul>")
		listItems = nil
	}

	for line := range strings.SplitSeq(msg, "\n") {
		if strings.TrimSpace(line) == "" {
			flushPara()
			flushList()
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			flushPara()
			listItems = append(listItems, line[2:])
			continue
		}
		if len(listItems) > 0 {
			flushList()
		}
		paraLines = append(paraLines, line)
	}
	flushPara()
	flushList()
	return buf.String()
}

func navClass(active, route string) string {
	if active == route {
		return "active"
	}
	return ""
}

// pageTitle builds the <title> text; an empty title (the index page) is just the
// site name.
func pageTitle(title string) string {
	if title == "" {
		return "Runbooks"
	}
	return title + " — Runbooks"
}

// runbookSearchText is the lowercased haystack the index filter matches against:
// title, description, symptoms, and any extra context (system/category names).
func runbookSearchText(rb parser.RunbookMeta, extra ...string) string {
	parts := append([]string{rb.Title, rb.Description}, rb.Symptoms...)
	parts = append(parts, extra...)
	return strings.ToLower(strings.Join(parts, " "))
}

// jsonString JSON-encodes a string for safe inline embedding in HTML (e.g. inside <script> tags).
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// PageConfig is the runtime configuration the client reads from #page-config:
// where records land on disk and whether the sync endpoint wants a bearer token.
type PageConfig struct {
	GitSyncEnabled       bool
	GitSyncRequiresToken bool
	RecordsBasePath      string
	IsAdmin              bool
	IdentityEnabled      bool
}

// pageConfigJSON renders PageConfig as the JSON embedded in the page.
func pageConfigJSON(c PageConfig) string {
	b, _ := json.Marshal(map[string]any{
		"gitSyncRequiresToken": c.GitSyncRequiresToken,
		"recordsBasePath":      c.RecordsBasePath,
	})
	return string(b)
}

// themeScript is inlined before first paint to prevent flash of wrong theme.
const themeScript = `(function(){var r=document.documentElement;var p=localStorage.getItem('runbooks-theme')||'dark';var d=p==='dark'||(p==='system'&&window.matchMedia('(prefers-color-scheme: dark)').matches);r.dataset.theme=d?'dark':'light';r.dataset.codeWeight=localStorage.getItem('runbooks-code-weight')||'regular';})()`
