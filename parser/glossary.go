// SPDX-License-Identifier: FSL-1.1-MIT

package parser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// glossaryFile is the optional shared glossary at the content root, beside
// _meta.yml. LoadDir reads only *.md, so it is never mistaken for a runbook.
const glossaryFile = "_glossary.yml"

// GlossaryEntry is one configured term. Term and Expansion are required;
// Description and Link are optional.
type GlossaryEntry struct {
	Term        string `yaml:"term" json:"term"`
	Expansion   string `yaml:"expansion" json:"expansion"`
	Description string `yaml:"description" json:"description"`
	Link        string `yaml:"link" json:"link"`
}

// LoadGlossary reads dir/_glossary.yml. A missing file is not an error: the
// glossary is optional and an empty one leaves every page unchanged. A malformed
// file, or an entry missing term or expansion, is an error, so a typo fails
// loudly at startup rather than silently never highlighting.
func LoadGlossary(dir string) ([]GlossaryEntry, error) {
	path := filepath.Join(dir, glossaryFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []GlossaryEntry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s: %w", glossaryFile, err)
	}
	for i, e := range entries {
		e.Term = strings.TrimSpace(e.Term)
		e.Expansion = strings.TrimSpace(e.Expansion)
		if e.Term == "" || e.Expansion == "" {
			return nil, fmt.Errorf("%s: entry %d: term and expansion are required", glossaryFile, i+1)
		}
		entries[i] = e
	}
	return entries, nil
}

// GlossaryTerms returns the configured terms in order, for the markup pass.
func GlossaryTerms(entries []GlossaryEntry) []string {
	terms := make([]string, 0, len(entries))
	for _, e := range entries {
		terms = append(terms, e.Term)
	}
	return terms
}
