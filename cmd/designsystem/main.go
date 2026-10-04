// SPDX-License-Identifier: FSL-1.1-MIT

// Command designsystem fetches the pinned Runbooks design-system release into
// public/design-system/ so the app serves the shared token layer, fonts and
// brand from the same source every other repo uses, rather than carrying a copy.
//
// The version is pinned (override with DESIGN_SYSTEM_VERSION); a stamp file
// records what is on disk so repeat builds are a no-op. Run from the repository
// root: `mise run design-system` or `go run ./cmd/designsystem`.
package main

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const (
	defaultVersion = "v0.2.1"
	destDir        = "public/design-system"
	stampName      = ".version"
	releaseBase    = "https://github.com/runbooks-help/design-system/releases/download"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "designsystem:", err)
		os.Exit(1)
	}
}

func run() error {
	version := os.Getenv("DESIGN_SYSTEM_VERSION")
	if version == "" {
		version = defaultVersion
	}
	base := releaseBase + "/" + version

	stamp := filepath.Join(destDir, stampName)
	if b, err := os.ReadFile(stamp); err == nil && strings.TrimSpace(string(b)) == version {
		fmt.Println("design-system up to date:", version)
		return nil
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	for _, name := range []string{"tokens.css", "fonts.css"} {
		if err := fetch(base+"/"+name, filepath.Join(destDir, name)); err != nil {
			return err
		}
	}
	for _, name := range []string{"fonts.tar.gz", "brand.tar.gz"} {
		if err := fetchTar(base+"/"+name, destDir); err != nil {
			return err
		}
	}
	// The app serves the mark favicon from /public/favicon.svg.
	if err := copyFile(filepath.Join(destDir, "favicon.svg"), filepath.Join("public", "favicon.svg")); err != nil {
		return err
	}
	if err := os.WriteFile(stamp, []byte(version+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Println("fetched design-system", version)
	return nil
}

func fetch(url, dst string) error {
	resp, err := http.Get(url) //nolint:gosec // the URL is a fixed, pinned release
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: %s", url, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// fetchTar extracts a .tar.gz into dir, refusing entries that escape it.
func fetchTar(url, dir string) error {
	resp, err := http.Get(url) //nolint:gosec // the URL is a fixed, pinned release
	if err != nil {
		return fmt.Errorf("get %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("get %s: %s", url, resp.Status)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.Clean("/"+h.Name))
		if !strings.HasPrefix(target, filepath.Clean(dir)+string(os.PathSeparator)) {
			return fmt.Errorf("tar entry escapes %s: %s", dir, h.Name)
		}
		if h.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, h.FileInfo().Mode().Perm())
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
