package parser

import (
	"fmt"
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
			groups = append(groups, SystemGroup{Name: systemDisplayName(def.Group)})
			sysIdx[def.Group] = si
		}
		key := def.Group + "\x00" + def.Category
		ci, ok := catIdx[key]
		if !ok {
			ci = len(groups[si].Categories)
			groups[si].Categories = append(groups[si].Categories, CategoryGroup{Name: categoryDisplayName(def.Category)})
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

// systemOrder and categoryOrder give explicit sidebar orderings for names whose
// operational priority is not alphabetical. Names not listed sort after the
// listed ones, alphabetically; categories are ordered per system.
var (
	systemOrder   = []string{"mysql", "kubernetes", "backend"}
	categoryOrder = map[string][]string{
		"mysql": {"replication", "failover", "backup", "maintenance", "disaster-recovery"},
	}
)

func rank(name string, order []string) int {
	for i, n := range order {
		if n == name {
			return i
		}
	}
	return len(order)
}

// lessOrdered sorts by position in order, then alphabetically for names that
// share a rank (e.g. every name absent from the list).
func lessOrdered(a, b string, order []string) bool {
	ra, rb := rank(a, order), rank(b, order)
	if ra != rb {
		return ra < rb
	}
	return a < b
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

// systemLabels overrides the generated casing for directory names whose proper
// spelling is not plain title case.
var systemLabels = map[string]string{
	"mysql": "MySQL",
}

func systemDisplayName(dir string) string {
	if dir == "" {
		return "Playbooks"
	}
	if label, ok := systemLabels[dir]; ok {
		return label
	}
	return categoryDisplayName(dir)
}

// SystemName returns the display name for a system directory, for callers
// outside this package (e.g. a page label).
func SystemName(dir string) string { return systemDisplayName(dir) }

// CategoryName returns the display name for a category directory; empty when
// the runbook sits directly under its system.
func CategoryName(dir string) string { return categoryDisplayName(dir) }

// categoryDisplayName title-cases a hyphenated directory name. An empty name
// means the runbook sits directly under its system, with no subheading.
func categoryDisplayName(dir string) string {
	if dir == "" {
		return ""
	}
	words := strings.Split(dir, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
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

// LoadDir reads *.md files in dir and up to two levels of subdirectories.
// content/<system>/<category>/<file>.md sets Group to the system and Category
// to the category; content/<system>/<file>.md sets Group only. Results are
// sorted by the explicit systemOrder/categoryOrder, then by runbook order and
// title.
func LoadDir(dir string) ([]RunbookDef, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var defs []RunbookDef
	for _, e := range entries {
		if !e.IsDir() {
			if !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", e.Name(), err)
			}
			def, err := Parse(data)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", e.Name(), err)
			}
			defs = append(defs, def)
			continue
		}
		system := e.Name()
		systemDir := filepath.Join(dir, system)
		systemEntries, err := os.ReadDir(systemDir)
		if err != nil {
			return nil, err
		}
		for _, se := range systemEntries {
			if se.IsDir() {
				category := se.Name()
				categoryDir := filepath.Join(systemDir, category)
				categoryEntries, err := os.ReadDir(categoryDir)
				if err != nil {
					return nil, err
				}
				for _, ce := range categoryEntries {
					if ce.IsDir() || !strings.HasSuffix(ce.Name(), ".md") {
						continue
					}
					data, err := os.ReadFile(filepath.Join(categoryDir, ce.Name()))
					if err != nil {
						return nil, fmt.Errorf("%s/%s/%s: %w", system, category, ce.Name(), err)
					}
					def, err := Parse(data)
					if err != nil {
						return nil, fmt.Errorf("%s/%s/%s: %w", system, category, ce.Name(), err)
					}
					def.Group = system
					def.Category = category
					defs = append(defs, def)
				}
				continue
			}
			if !strings.HasSuffix(se.Name(), ".md") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(systemDir, se.Name()))
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", system, se.Name(), err)
			}
			def, err := Parse(data)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", system, se.Name(), err)
			}
			def.Group = system
			defs = append(defs, def)
		}
	}
	sort.Slice(defs, func(i, j int) bool {
		gi, gj := defs[i].Group, defs[j].Group
		if gi != gj {
			return lessOrdered(gi, gj, systemOrder)
		}
		ci, cj := defs[i].Category, defs[j].Category
		if ci != cj {
			return lessOrdered(ci, cj, categoryOrder[gi])
		}
		return lessRunbook(defs[i].RunbookMeta, defs[j].RunbookMeta)
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
