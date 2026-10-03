// Command spdx checks that every source file Runbooks authors carries the
// project's SPDX licence header, and with -fix adds it where missing. It is the
// file-level end of the licence chain (file → binary → container): the header is
// machine-readable, and `mise run lint:spdx` (CI) fails on a file that lacks it.
//
// In scope: Go (except generated *_templ.go), *.templ, public/js/src, public/css/src
// and the e2e suites. Out of scope: vendored third-party assets, generated
// bundles, docs and content.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const identifier = "SPDX-License-Identifier: FSL-1.1-MIT"

func main() {
	fix := flag.Bool("fix", false, "prepend the header to files missing it")
	flag.Parse()
	if err := run(*fix); err != nil {
		fmt.Fprintln(os.Stderr, "spdx:", err)
		os.Exit(1)
	}
}

func run(fix bool) error {
	var missing []string
	stamped := 0
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		header, ok := inScope(path)
		if !ok {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if hasHeader(data) {
			return nil
		}
		if !fix {
			missing = append(missing, filepath.ToSlash(path))
			return nil
		}
		if err := os.WriteFile(path, prepend(header, data), 0o644); err != nil {
			return err
		}
		stamped++
		return nil
	})
	if err != nil {
		return err
	}
	if fix {
		fmt.Printf("stamped %d file(s)\n", stamped)
		return nil
	}
	if len(missing) > 0 {
		for _, p := range missing {
			fmt.Println(p)
		}
		return fmt.Errorf("%d authored source file(s) missing the SPDX header", len(missing))
	}
	fmt.Println("all authored source files carry the SPDX header")
	return nil
}

// skipDir prunes directories that never hold authored source.
func skipDir(name string) bool {
	switch name {
	case ".git", ".cache", "node_modules", "tmp", "sbom", "data":
		return true
	}
	return false
}

// inScope returns the header comment for a file we author, if it needs one.
func inScope(path string) (string, bool) {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	if strings.HasPrefix(path, "public/js/vendor/") || strings.HasPrefix(path, "public/fonts/") {
		return "", false
	}
	if strings.HasSuffix(path, "_templ.go") {
		return "", false
	}
	if strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".templ") || strings.HasSuffix(path, ".mjs") {
		return "// " + identifier, true
	}
	if strings.HasPrefix(path, "public/js/src/") && strings.HasSuffix(path, ".js") {
		return "// " + identifier, true
	}
	if strings.HasPrefix(path, "public/css/src/") && strings.HasSuffix(path, ".css") {
		return "/* " + identifier + " */", true
	}
	return "", false
}

// hasHeader reports whether the identifier appears near the top of data.
func hasHeader(data []byte) bool {
	lines := strings.SplitN(string(data), "\n", 5)
	return strings.Contains(strings.Join(lines, "\n"), identifier)
}

// prepend puts the header, then a blank line, above the existing content.
func prepend(header string, data []byte) []byte {
	return []byte(header + "\n\n" + string(data))
}
