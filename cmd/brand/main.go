// SPDX-License-Identifier: FSL-1.1-MIT

// Command brand generates the Runbooks brand assets into brand/: the logomark,
// the wordmark, and horizontal + stacked lockups, for light and dark
// backgrounds, as outlined SVG (text converted to paths, so the files need no
// font installed) plus PNG at 512 and 1024.
//
// The palette is the design system's (see public/css/src/tokens.css); the glyphs
// come from the self-hosted fonts. Run from the repository root. Needs
// woff2_decompress, inkscape and rsvg-convert on PATH — generation is a local,
// manual task and the output is committed.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	outDir    = "brand"
	fontsDir  = "public/fonts"
	shadeFont = "Shade Mono"
	sansFont  = "Atkinson Hyperlegible Next"
)

// Palette, copied from public/css/src/tokens.css. The logomark always uses the
// dark-theme brand greens; the wordmark follows the background it is drawn for.
const (
	markHead    = "#3d6756" // --accent (dark)
	markBright  = "#6aa88d" // --accent-strong (dark)
	onAccent    = "#eef4f0" // --on-accent (dark)
	inkOnDark   = "#d7d9e3" // --nav-text (dark theme)
	inkOnLight  = "#2e2d38" // --nav-text (light theme)
	runOnDark   = "#6aa88d" // --accent-strong (dark theme)
	runOnLight  = "#345c4c" // --accent-strong (light theme)
	markRune    = "\u2591"  // ░
	markRuneAlt = "\u2593"  // ▓
)

// sources are the self-hosted woff2 files the outliner needs: the pinned shade
// subset for the mark, and the sans for the wordmark.
var sources = []string{
	"noto-sans-mono-shades.woff2",
	"atkinson-hyperlegible-next-latin-wght-normal.woff2",
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "brand:", err)
		os.Exit(1)
	}
}

func run() error {
	for _, tool := range []string{"woff2_decompress", "inkscape", "rsvg-convert"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found on PATH (see docs / mise)", tool)
		}
	}

	tmp, err := os.MkdirTemp("", "runbooks-brand-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	fontEnv, err := fontEnvironment(tmp)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	for _, v := range variants() {
		svgPath := filepath.Join(outDir, v.name+".svg")
		if err := outline(tmp, v, fontEnv, svgPath); err != nil {
			return err
		}
		for _, size := range []int{1024, 512} {
			png := filepath.Join(outDir, fmt.Sprintf("%s-%d.png", v.name, size))
			if err := rasterise(svgPath, png, size); err != nil {
				return err
			}
		}
		fmt.Println("wrote", filepath.Join(outDir, v.name+".svg"))
	}

	// The app's favicon is the icon, so the served mark cannot drift from the
	// generated one.
	if err := copyFile(filepath.Join(outDir, "icon.svg"), filepath.Join("public", "favicon.svg")); err != nil {
		return err
	}
	fmt.Println("wrote public/favicon.svg")
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// variant is one exported asset: a base filename and the source SVG body.
type variant struct {
	name string
	body string
}

// canvas wraps an SVG body in a generous viewBox; inkscape crops it to the
// drawing on export.
func canvas(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" width="1400" height="700" viewBox="0 0 1400 700">` + body + `</svg>`
}

func variants() []variant {
	mark := func(x, y int, size int, anchor string) string {
		anchorAttr := ""
		if anchor != "" {
			anchorAttr = fmt.Sprintf(` text-anchor="%s"`, anchor)
		}
		return fmt.Sprintf(`<text x="%d" y="%d" font-family="%s" font-weight="400" font-size="%d"%s><tspan fill="%s">%s</tspan><tspan fill="%s">%s</tspan></text>`,
			x, y, shadeFont, size, anchorAttr, markHead, markRune, markBright, markRuneAlt)
	}
	word := func(x, y int, size int, run, ink string, anchor string) string {
		anchorAttr := ""
		if anchor != "" {
			anchorAttr = fmt.Sprintf(` text-anchor="%s"`, anchor)
		}
		return fmt.Sprintf(`<text x="%d" y="%d" font-family="%s" font-weight="700" font-size="%d"%s><tspan fill="%s">run</tspan><tspan fill="%s">books</tspan></text>`,
			x, y, sansFont, size, anchorAttr, run, ink)
	}

	iconMark := fmt.Sprintf(`<text x="256" y="362" text-anchor="middle" font-family="%s" font-weight="400" font-size="300"><tspan fill="%s" fill-opacity="0.7">%s</tspan><tspan fill="%s">%s</tspan></text>`,
		shadeFont, onAccent, markRune, onAccent, markRuneAlt)
	icon := `<g transform="translate(444 94)">` +
		`<rect width="512" height="512" rx="112" fill="` + markHead + `"/>` +
		iconMark +
		`</g>`

	return []variant{
		{"logomark", mark(500, 460, 320, "")},
		{"wordmark-on-dark", word(500, 460, 300, runOnDark, inkOnDark, "")},
		{"wordmark-on-light", word(500, 460, 300, runOnLight, inkOnLight, "")},
		{"lockup-horizontal-on-dark", mark(420, 460, 300, "") + word(880, 460, 220, runOnDark, inkOnDark, "")},
		{"lockup-horizontal-on-light", mark(420, 460, 300, "") + word(880, 460, 220, runOnLight, inkOnLight, "")},
		{"lockup-stacked-on-dark", mark(700, 420, 300, "middle") + word(700, 620, 200, runOnDark, inkOnDark, "middle")},
		{"lockup-stacked-on-light", mark(700, 420, 300, "middle") + word(700, 620, 200, runOnLight, inkOnLight, "middle")},
		{"icon", icon},
	}
}

// fontEnvironment decompresses the self-hosted woff2 files and writes a
// fontconfig file pointing only at them, so the outliner uses the pinned fonts
// regardless of what is installed system-wide. It returns the environment for
// the inkscape calls.
func fontEnvironment(tmp string) ([]string, error) {
	for _, name := range sources {
		data, err := os.ReadFile(filepath.Join(fontsDir, name))
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(tmp, name), data, 0o644); err != nil {
			return nil, err
		}
		cmd := exec.Command("woff2_decompress", name)
		cmd.Dir = tmp
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("woff2_decompress %s: %v: %s", name, err, out)
		}
	}

	conf := fmt.Sprintf(`<?xml version="1.0"?><!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig><dir>%s</dir><cachedir>%s</cachedir></fontconfig>
`, tmp, filepath.Join(tmp, "cache"))
	confPath := filepath.Join(tmp, "fonts.conf")
	if err := os.WriteFile(confPath, []byte(conf), 0o644); err != nil {
		return nil, err
	}
	return append(os.Environ(), "FONTCONFIG_FILE="+confPath), nil
}

// outline converts the source SVG's text to paths and crops to the drawing, so
// the exported file is portable and tightly bounded.
func outline(tmp string, v variant, env []string, dst string) error {
	src := filepath.Join(tmp, v.name+".src.svg")
	if err := os.WriteFile(src, []byte(canvas(v.body)), 0o644); err != nil {
		return err
	}
	cmd := exec.Command("inkscape", src,
		"--export-text-to-path", "--export-plain-svg",
		"--export-area-drawing", "--export-margin=4",
		"--export-filename="+dst)
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("inkscape %s: %v: %s", v.name, err, out)
	}
	return nil
}

func rasterise(svg, png string, width int) error {
	cmd := exec.Command("rsvg-convert", "-w", fmt.Sprint(width), svg, "-o", png)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("rsvg-convert %s: %v: %s", png, err, out)
	}
	return nil
}
