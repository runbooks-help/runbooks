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
	Items   []string   // KindList
}

type Step struct {
	Title  string
	Blocks []Block
}

type RunbookMeta struct {
	Title       string   `yaml:"title"`
	Slug        string   `yaml:"slug"`
	Description string   `yaml:"description"`
	Symptoms    []string `yaml:"symptoms"`
	Common      bool     `yaml:"common"` // opt in to the index "Common issues" shortlist
	Order       int      `yaml:"order"`  // optional: lower sorts first within its category
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
	Title string
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
				out = append(out, CommonIssue{Label: label, Title: rb.Title, Slug: rb.Slug})
			}
		}
	}
	return out
}

// defaultDirOrder is where a directory without a _meta.yaml order sorts. A lower
// value pulls it towards the front of its level.
const defaultDirOrder = 100

// DirMeta is a content directory's presentation, read from an optional _meta.yaml
// in that directory. The content declares its own taxonomy; the parser stays
// generic.
type DirMeta struct {
	Title string `yaml:"title"`
	Order int    `yaml:"order"`
}

const dirMetaFile = "_meta.yaml"

// loadDirMeta reads a directory's _meta.yaml. A missing file yields a zero
// DirMeta (title-case the name, sort alphabetically); a malformed one is an error,
// so a content typo cannot silently misorder the sidebar.
func loadDirMeta(dir string) (DirMeta, error) {
	data, err := os.ReadFile(filepath.Join(dir, dirMetaFile))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return DirMeta{}, nil
		}
		return DirMeta{}, err
	}
	var m DirMeta
	if err := yaml.Unmarshal(data, &m); err != nil {
		return DirMeta{}, fmt.Errorf("%s: %w", filepath.Join(dir, dirMetaFile), err)
	}
	return m, nil
}

// displayName resolves a directory's sidebar label: its _meta.yaml title, else the
// title-cased directory name. The empty key is a runbook directly under content/.
func displayName(dir string, meta DirMeta) string {
	if meta.Title != "" {
		return meta.Title
	}
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
	Steps       []Step
	Rollback    []Step
	Source      string // raw source markdown (frontmatter included)
}

var (
	noticeRe      = regexp.MustCompile(`^>\s+\[!(warn|danger|info)\]\s*(.*)$`)
	branchStartRe = regexp.MustCompile(`^>\s+\[!branch\]\s*$`)
	blockquoteRe  = regexp.MustCompile(`^>\s?(.*)$`)
	fenceInfoRe   = regexp.MustCompile(`^([a-z]*)\s*(?:\[([^\]]*)\])?$`)
)

// parseTableLine splits a markdown table row into trimmed cell strings.
func parseTableLine(line string) []string {
	line = strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// isTableSeparator reports whether a row is a divider (e.g. |---|:---:|).
func isTableSeparator(cells []string) bool {
	if len(cells) == 0 {
		return false
	}
	for _, c := range cells {
		for _, ch := range strings.Trim(c, ": ") {
			if ch != '-' {
				return false
			}
		}
	}
	return true
}

// LoadDir reads every *.md under dir, at any depth. A runbook's system and
// category are the two directory levels directly above it: content/mysql/
// replication/x.md is MySQL › Replication, and a leading wrapper is ignored
// (content/testdata/mysql/replication/x.md groups the same way). A file directly
// in a top-level directory has a system and no category; a file directly in dir
// has neither.
//
// Each directory may carry an optional _meta.yaml giving its display title and
// sort order (see DirMeta). Results are sorted by system (order, title), then
// category (order, title), then runbook order and title.
func LoadDir(dir string) ([]RunbookDef, error) {
	metas := map[string]DirMeta{}
	dirMeta := func(path string) (DirMeta, error) {
		if m, ok := metas[path]; ok {
			return m, nil
		}
		m, err := loadDirMeta(path)
		if err != nil {
			return DirMeta{}, err
		}
		metas[path] = m
		return m, nil
	}

	var defs []RunbookDef
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
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
			m, err := dirMeta(filepath.Join(dir, parts[0]))
			if err != nil {
				return err
			}
			def.Group = parts[0]
			def.GroupTitle = displayName(parts[0], m)
			def.GroupOrder = m.Order
		case len(parts) >= 2:
			sysKey, catKey := parts[len(parts)-2], parts[len(parts)-1]
			sm, err := dirMeta(filepath.Join(dir, filepath.Join(parts[:len(parts)-1]...)))
			if err != nil {
				return err
			}
			cm, err := dirMeta(filepath.Join(dir, filepath.Join(parts...)))
			if err != nil {
				return err
			}
			def.Group = sysKey
			def.GroupTitle = displayName(sysKey, sm)
			def.GroupOrder = sm.Order
			def.Category = catKey
			def.CategoryTitle = displayName(catKey, cm)
			def.CategoryOrder = cm.Order
		}
		defs = append(defs, def)
		return nil
	})
	if err != nil {
		return nil, err
	}

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

// Parse extracts YAML frontmatter and walks the body line-by-line to build a RunbookDef.
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

	var (
		currentSteps = &def.Steps
		currentStep  *Step
		proseLines   []string
		inFence      bool
		fenceLabel   string
		fenceLang    string
		fenceLines   []string
		inBranch     bool
		branchLines  []string
		inTable      bool
		tableLines   []string
		inList       bool
		listItems    []string
		inNotice     bool
		noticeVar    string
		noticeLines  []string
	)

	flushProse := func() {
		if len(proseLines) == 0 || currentStep == nil {
			proseLines = nil
			return
		}
		text := strings.TrimSpace(strings.Join(proseLines, " "))
		if text != "" {
			currentStep.Blocks = append(currentStep.Blocks, Block{Kind: KindProse, Text: text})
		}
		proseLines = nil
	}

	commitStep := func() {
		if currentStep != nil {
			*currentSteps = append(*currentSteps, *currentStep)
			currentStep = nil
		}
	}

	flushBranch := func() {
		if !inBranch {
			return
		}
		b := strings.Join(branchLines, "\n")
		if currentStep != nil && strings.TrimSpace(b) != "" {
			currentStep.Blocks = append(currentStep.Blocks, Block{Kind: KindBranch, Body: b})
		}
		inBranch = false
		branchLines = nil
	}

	flushNotice := func() {
		if !inNotice {
			return
		}
		inNotice = false
		if currentStep != nil {
			currentStep.Blocks = append(currentStep.Blocks, Block{
				Kind:    KindNotice,
				Variant: noticeVar,
				Message: strings.Join(noticeLines, "\n"),
			})
		}
		noticeVar = ""
		noticeLines = nil
	}

	flushList := func() {
		if !inList {
			return
		}
		inList = false
		if currentStep != nil && len(listItems) > 0 {
			currentStep.Blocks = append(currentStep.Blocks, Block{
				Kind:  KindList,
				Items: listItems,
			})
		}
		listItems = nil
	}

	flushTable := func() {
		if !inTable {
			return
		}
		inTable = false
		if len(tableLines) < 2 || currentStep == nil {
			tableLines = nil
			return
		}
		headers := parseTableLine(tableLines[0])
		var rows [][]string
		for _, line := range tableLines[1:] {
			cells := parseTableLine(line)
			if !isTableSeparator(cells) {
				rows = append(rows, cells)
			}
		}
		if len(rows) > 0 {
			currentStep.Blocks = append(currentStep.Blocks, Block{
				Kind:    KindTable,
				Headers: headers,
				Rows:    rows,
			})
		}
		tableLines = nil
	}

	for line := range strings.SplitSeq(body, "\n") {
		// Accumulate branch block lines until a non-blockquote line ends it.
		if inNotice {
			if m := blockquoteRe.FindStringSubmatch(line); m != nil && !noticeRe.MatchString(line) && !branchStartRe.MatchString(line) {
				noticeLines = append(noticeLines, m[1])
				continue
			}
			flushNotice()
			// fall through to handle this line normally
		}

		if inBranch {
			if m := blockquoteRe.FindStringSubmatch(line); m != nil {
				branchLines = append(branchLines, m[1])
				continue
			}
			flushBranch()
			// fall through to handle this line normally
		}

		if inFence {
			if strings.HasPrefix(line, "```") {
				b := strings.Join(fenceLines, "\n")
				if currentStep != nil {
					currentStep.Blocks = append(currentStep.Blocks, Block{
						Kind:  KindCode,
						Label: fenceLabel,
						Lang:  fenceLang,
						Body:  b,
					})
				}
				inFence = false
				fenceLines = nil
				fenceLabel = ""
				fenceLang = ""
			} else {
				fenceLines = append(fenceLines, line)
			}
			continue
		}

		if strings.HasPrefix(line, "```") {
			flushProse()
			flushNotice()
			flushList()
			flushTable()
			inFence = true
			info := strings.TrimSpace(strings.TrimPrefix(line, "```"))
			if m := fenceInfoRe.FindStringSubmatch(info); m != nil {
				fenceLang = m[1]
				fenceLabel = m[2]
			}
			continue
		}

		if line == "---rollback" {
			flushProse()
			flushNotice()
			flushList()
			flushTable()
			commitStep()
			currentSteps = &def.Rollback
			continue
		}

		if strings.HasPrefix(line, "## ") {
			flushProse()
			flushNotice()
			flushList()
			flushTable()
			commitStep()
			currentStep = &Step{Title: strings.TrimPrefix(line, "## ")}
			continue
		}

		if strings.HasPrefix(line, "### ") {
			flushProse()
			flushNotice()
			flushList()
			flushTable()
			if currentStep != nil {
				currentStep.Blocks = append(currentStep.Blocks, Block{
					Kind: KindHeading,
					Text: strings.TrimPrefix(line, "### "),
				})
			}
			continue
		}

		if branchStartRe.MatchString(line) {
			flushProse()
			flushNotice()
			flushList()
			flushTable()
			inBranch = true
			branchLines = nil
			continue
		}

		if m := noticeRe.FindStringSubmatch(line); m != nil {
			flushProse()
			flushNotice()
			flushList()
			flushTable()
			inNotice = true
			noticeVar = m[1]
			noticeLines = []string{m[2]}
			continue
		}

		if strings.TrimSpace(line) == "" {
			flushProse()
			flushNotice()
			flushList()
			flushTable()
			continue
		}

		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			if !inList {
				flushProse()
				inList = true
			}
			listItems = append(listItems, strings.TrimSpace(line[2:]))
			continue
		}

		if inList {
			flushList()
		}

		if strings.HasPrefix(line, "|") {
			if !inTable {
				flushProse()
				inTable = true
			}
			tableLines = append(tableLines, line)
			continue
		}

		if inTable {
			flushTable()
		}

		proseLines = append(proseLines, strings.TrimSpace(line))
	}
	flushBranch()
	flushProse()
	flushNotice()
	flushList()
	flushTable()
	commitStep()

	def.Source = string(data)
	return def, nil
}
