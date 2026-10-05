// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type VarField struct {
	ID          string `yaml:"id"`
	Label       string `yaml:"label"`
	VarName     string `yaml:"var"`
	Secret      bool   `yaml:"secret"`
	Placeholder string `yaml:"placeholder"`
	Hint        string `yaml:"hint"`
}

type BlockKind string

const (
	KindProse   BlockKind = "prose"
	KindCode    BlockKind = "code"
	KindNotice  BlockKind = "notice"
	KindBranch  BlockKind = "branch"
	KindTable   BlockKind = "table"
	KindList    BlockKind = "list"
	KindHeading BlockKind = "heading"
)

type Block struct {
	Kind    BlockKind
	Text    string     // KindProse
	Label   string     // KindCode
	Lang    string     // KindCode: fence language ("sql", "bash", ...)
	Body    string     // KindCode
	Variant string     // KindNotice: "info", "warn", "danger"
	Message string     // KindNotice
	Headers []string   // KindTable
	Rows    [][]string // KindTable
	Items   []ListItem // KindList
	Ordered bool       // KindList: render as <ol> rather than <ul>
}

// ListItem is one entry in a list. Text is the item's own inline markdown;
// Child is an optional nested list (KindList) drawn inside the item. The flat
// Block model has no slot for other item blocks (a second paragraph, a code
// block), so the converter folds their text into Text rather than dropping it.
type ListItem struct {
	Text  string
	Child *Block
}

type Step struct {
	Title   string
	Section bool // a plain section, not a numbered step (layout: sections / a sections region)
	Blocks  []Block
}

type RunbookMeta struct {
	Title       string   `yaml:"title"`
	Slug        string   `yaml:"slug"`
	Description string   `yaml:"description"`
	Symptoms    []string `yaml:"symptoms"`
	Common      bool     `yaml:"common"` // opt in to the index "Common issues" shortlist
	Order       int      `yaml:"order"`  // optional: lower sorts first within its category
	Layout      string   `yaml:"layout"` // "sections": render ## unnumbered (default: numbered steps)
	Group       string   `yaml:"-"`      // top-level system, set from directory name
	Category    string   `yaml:"-"`      // subcategory, set from directory name

	// Resolved from the directory's _meta.yaml at load: the sidebar display name
	// (title or title-cased directory) and its sort order.
	GroupTitle    string `yaml:"-"`
	GroupOrder    int    `yaml:"-"`
	CategoryTitle string `yaml:"-"`
	CategoryOrder int    `yaml:"-"`
}

// CategoryGroup groups runbooks under a display heading within a system.
type CategoryGroup struct {
	Name     string
	Runbooks []RunbookMeta
}

// SystemGroup groups categories under a top-level system (e.g. MySQL).
type SystemGroup struct {
	Name       string
	Categories []CategoryGroup
}

// GroupBySystem returns runbooks grouped by system then category, in the order
// they appear in defs.
func GroupBySystem(defs []RunbookDef) []SystemGroup {
	var groups []SystemGroup
	sysIdx := map[string]int{}
	catIdx := map[string]int{}
	for _, def := range defs {
		si, ok := sysIdx[def.Group]
		if !ok {
			si = len(groups)
			groups = append(groups, SystemGroup{Name: def.GroupTitle})
			sysIdx[def.Group] = si
		}
		key := def.Group + "\x00" + def.Category
		ci, ok := catIdx[key]
		if !ok {
			ci = len(groups[si].Categories)
			groups[si].Categories = append(groups[si].Categories, CategoryGroup{Name: def.CategoryTitle})
			catIdx[key] = ci
		}
		groups[si].Categories[ci].Runbooks = append(groups[si].Categories[ci].Runbooks, def.RunbookMeta)
	}
	return groups
}

// CommonIssue is a symptom-first shortcut on the index page. A runbook opts in
// with `common: true`; its first symptom becomes the label. Symptoms themselves
// are search keywords on every runbook, independent of this shortlist.
type CommonIssue struct {
	Label string
	Slug  string
}

// CommonIssues scans groups in sidebar order and returns one shortcut per
// runbook marked `common: true`, labelled by its first symptom (or its title).
func CommonIssues(groups []SystemGroup) []CommonIssue {
	var out []CommonIssue
	for _, g := range groups {
		for _, c := range g.Categories {
			for _, rb := range c.Runbooks {
				if !rb.Common {
					continue
				}
				label := rb.Title
				if len(rb.Symptoms) > 0 {
					label = rb.Symptoms[0]
				}
				out = append(out, CommonIssue{Label: label, Slug: rb.Slug})
			}
		}
	}
	return out
}

// SearchText is the lowercased haystack the index and sidebar filters match
// against: title, description, symptoms, and any extra context (the system and
// category names).
func (rb RunbookMeta) SearchText(extra ...string) string {
	parts := append([]string{rb.Title, rb.Description}, rb.Symptoms...)
	parts = append(parts, extra...)
	return strings.ToLower(strings.Join(parts, " "))
}

// defaultDirOrder is where an unlisted directory sorts. A lower value pulls it
// towards the front of its level.
const defaultDirOrder = 100

// manifestFile is the content taxonomy at the top of the content directory: an
// ordered list of systems, and under each, categories. List position sets the
// sidebar order; a title overrides the title-cased directory name.
const manifestFile = "_meta.yml"

// manifestEntry is one manifest item: a bare directory name, or a single-key map
// name -> {title, categories}.
type manifestEntry struct {
	Name       string
	Title      string
	Categories []manifestEntry
}

// UnmarshalYAML accepts either form, so `- backend` and
// `- mysql: {title: MySQL, categories: [...]}` both work.
func (e *manifestEntry) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		e.Name = node.Value
		return nil
	case yaml.MappingNode:
		if len(node.Content) != 2 {
			return fmt.Errorf("line %d: expected a single name: entry", node.Line)
		}
		e.Name = node.Content[0].Value
		var body struct {
			Title      string          `yaml:"title"`
			Categories []manifestEntry `yaml:"categories"`
		}
		if err := node.Content[1].Decode(&body); err != nil {
			return err
		}
		e.Title = body.Title
		e.Categories = body.Categories
		return nil
	default:
		return fmt.Errorf("line %d: expected a directory name or name: {…}", node.Line)
	}
}

// dirMeta is a resolved directory presentation: the explicit title (empty means
// title-case the directory name) and its 1-based order (0 means unlisted).
type dirMeta struct {
	title string
	order int
}

// manifest is the taxonomy as name->meta lookups, so an entry whose directory does
// not exist is simply never consulted (the file may list a superset).
type manifest struct {
	systems    map[string]dirMeta
	categories map[string]dirMeta
	rootTitle  string // reserved `_root` entry: title for content at the root
}

func loadManifest(path string) (manifest, error) {
	m := manifest{systems: map[string]dirMeta{}, categories: map[string]dirMeta{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return m, nil
		}
		return m, err
	}
	var entries []manifestEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return m, fmt.Errorf("%s: %w", path, err)
	}
	for i, e := range entries {
		if e.Name == "" {
			continue
		}
		if e.Name == "_root" {
			m.rootTitle = e.Title
			continue
		}
		m.systems[e.Name] = dirMeta{title: e.Title, order: i + 1}
		for j, c := range e.Categories {
			if c.Name == "" {
				continue
			}
			m.categories[e.Name+"\x00"+c.Name] = dirMeta{title: c.Title, order: j + 1}
		}
	}
	return m, nil
}

// parseDirective recognises a region directive on its own line, in either the
// app's bare form (---steps) or the GitHub-safe comment form
// (<!-- steps -->), which is invisible in a rendered Markdown document.
func parseDirective(line string) (string, bool) {
	m := directiveRe.FindStringSubmatch(line)
	if m == nil {
		return "", false
	}
	if m[1] != "" {
		return m[1], true
	}
	return m[2], true
}

// noticeVariant maps an alert keyword (case-insensitive, with the GitHub alert
// aliases) onto the three rendered variants.
func noticeVariant(kind string) string {
	switch strings.ToLower(kind) {
	case "warn", "warning", "important", "caution":
		return "warn"
	case "danger", "error":
		return "danger"
	default: // info, note, tip
		return "info"
	}
}

// titleCase turns a hyphenated directory name into a display label. The empty name
// is a runbook directly under content/.
func titleCase(dir string) string {
	if dir == "" {
		return "Playbooks"
	}
	words := strings.Split(dir, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// resolveTitle is a directory's display name: the manifest title, else the
// title-cased directory name.
func resolveTitle(key string, meta dirMeta) string {
	if meta.title != "" {
		return meta.title
	}
	return titleCase(key)
}

// defaultRunbookOrder is where a runbook without an explicit `order:` sorts.
// A lower value pulls it to the front of its category.
const defaultRunbookOrder = 100

func lessRunbook(a, b RunbookMeta) bool {
	ao, bo := a.Order, b.Order
	if ao == 0 {
		ao = defaultRunbookOrder
	}
	if bo == 0 {
		bo = defaultRunbookOrder
	}
	if ao != bo {
		return ao < bo
	}
	return a.Title < b.Title
}

// dirKey identifies a directory for ordering: its key, resolved display name and
// _meta.yaml order (0 = unset).
type dirKey struct {
	key   string
	title string
	order int
}

// lessDir orders directories by _meta.yaml order (unset last), then by display
// name, then by key so the result is deterministic.
func lessDir(a, b dirKey) bool {
	ao, bo := a.order, b.order
	if ao == 0 {
		ao = defaultDirOrder
	}
	if bo == 0 {
		bo = defaultDirOrder
	}
	if ao != bo {
		return ao < bo
	}
	if a.title != b.title {
		return a.title < b.title
	}
	return a.key < b.key
}

type RunbookDef struct {
	RunbookMeta `yaml:",inline"`
	// Notice is a passive banner. Acknowledge, when set, marks the runbook
	// destructive: the reader must accept it in a modal before the page is usable.
	Notice      string     `yaml:"notice"`
	Acknowledge string     `yaml:"acknowledge"`
	Vars        []VarField `yaml:"vars"`
	Intro       []Block    // lead blocks before the first heading
	Steps       []Step
	Rollback    []Step
	Source      string // raw source markdown (frontmatter included)
}

var (
	noticeRe      = regexp.MustCompile(`^>\s+\[!(?i)(info|note|tip|warn|warning|important|caution|danger|error)\]\s*(.*)$`)
	branchStartRe = regexp.MustCompile(`^>\s+\[!branch\]\s*$`)
	blockquoteRe  = regexp.MustCompile(`^>\s?(.*)$`)
	fenceInfoRe   = regexp.MustCompile(`^([a-z]*)\s*(?:\[([^\]]*)\])?$`)
	directiveRe   = regexp.MustCompile(`^(?:---(rollback|sections|steps)|<!--\s*(rollback|sections|steps)\s*-->)\s*$`)
)

// LoadDir reads every *.md under dir, at any depth. A runbook's system and
// category are the two directory levels directly above it: content/mysql/
// replication/x.md is MySQL › Replication, and a leading wrapper is ignored
// (content/testdata/mysql/replication/x.md groups the same way). A file directly
// in a top-level directory has a system and no category; a file directly in dir
// has neither.
//
// Display names and sidebar order come from an optional content/_meta.yml (see
// manifestFile). Results are sorted by system (order, title), then category
// (order, title), then runbook order and title.
//
// exclude lists paths (relative to dir) the walk skips: the git-sync records
// folder, when content lives at the root of a repo that also holds records.
func LoadDir(dir string, exclude ...string) ([]RunbookDef, error) {
	man, err := loadManifest(filepath.Join(dir, manifestFile))
	if err != nil {
		return nil, err
	}

	var defs []RunbookDef
	var rels []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		slashRel := filepath.ToSlash(rel)
		for _, ex := range exclude {
			ex = strings.Trim(filepath.ToSlash(ex), "/")
			if ex == "" || slashRel == "." {
				continue
			}
			if slashRel == ex || strings.HasPrefix(slashRel, ex+"/") {
				if d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		parts := strings.Split(slashRel, "/")
		parts = parts[:len(parts)-1] // drop the file name

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		def, err := Parse(data)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.ToSlash(rel), err)
		}
		switch {
		case len(parts) == 1:
			sm := man.systems[parts[0]]
			def.Group = parts[0]
			def.GroupTitle = resolveTitle(parts[0], sm)
			def.GroupOrder = sm.order
		case len(parts) >= 2:
			sysKey, catKey := parts[len(parts)-2], parts[len(parts)-1]
			sm := man.systems[sysKey]
			cm := man.categories[sysKey+"\x00"+catKey]
			def.Group = sysKey
			def.GroupTitle = resolveTitle(sysKey, sm)
			def.GroupOrder = sm.order
			def.Category = catKey
			def.CategoryTitle = resolveTitle(catKey, cm)
			def.CategoryOrder = cm.order
		default:
			title := titleCase("")
			if man.rootTitle != "" {
				title = man.rootTitle
			}
			def.GroupTitle = title
		}
		defs = append(defs, def)
		rels = append(rels, strings.TrimSuffix(slashRel, ".md"))
		return nil
	})
	if err != nil {
		return nil, err
	}

	rewriteDocLinks(defs, rels)

	sort.Slice(defs, func(i, j int) bool {
		a, b := defs[i], defs[j]
		if a.Group != b.Group {
			return lessDir(
				dirKey{a.Group, a.GroupTitle, a.GroupOrder},
				dirKey{b.Group, b.GroupTitle, b.GroupOrder},
			)
		}
		if a.Category != b.Category {
			return lessDir(
				dirKey{a.Category, a.CategoryTitle, a.CategoryOrder},
				dirKey{b.Category, b.CategoryTitle, b.CategoryOrder},
			)
		}
		return lessRunbook(a.RunbookMeta, b.RunbookMeta)
	})
	return defs, nil
}

// Parse extracts the YAML frontmatter and converts the body to a RunbookDef
// with the goldmark-backed converter (see convert.go).
func Parse(data []byte) (RunbookDef, error) {
	src := string(data)

	// Frontmatter is delimited by --- on its own line at the very start.
	if !strings.HasPrefix(src, "---\n") {
		return RunbookDef{}, fmt.Errorf("missing frontmatter")
	}
	rest := src[4:] // skip opening "---\n"
	before, after, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return RunbookDef{}, fmt.Errorf("unclosed frontmatter")
	}
	fm := before
	body := strings.TrimSpace(after)

	var def RunbookDef
	if err := yaml.Unmarshal([]byte(fm), &def); err != nil {
		return RunbookDef{}, fmt.Errorf("frontmatter: %w", err)
	}

	intro, steps, rollback := convertBody(body, def.Layout == "sections")
	def.Intro, def.Steps, def.Rollback = intro, steps, rollback

	def.Source = string(data)
	return def, nil
}
