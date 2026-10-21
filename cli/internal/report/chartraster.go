package report

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// chartPaletteHex is the fallback categorical palette (rex-default's
// chartc1..10). Charts don't have the LaTeX template's preamble when
// rasterized standalone, so ResolveChartColors reads the template's own
// chartcN \definecolor lines.
var chartPaletteHex = []string{
	"0F3460", "16697A", "489FB5", "82C0CC", "E9724C",
	"C05299", "6D597A", "B56576", "937B63", "355070",
}

// defaultChartColors are the fallback severity/brand colors (rex-default's
// theme.yaml). ResolveChartColors overrides these with the template's
// theme.yaml, then the report's colors.
var defaultChartColors = map[string]string{
	"primary": "1A1A2E", "accent": "0F3460", "critical": "B71C1C",
	"high": "E65100", "medium": "F57F17", "low": "1565C0", "info": "37474F",
}

var svgSizeRE = regexp.MustCompile(`(\s(?:width|height)=")([\d.]+)"`)

var chartcRE = regexp.MustCompile(`\\definecolor\{(chartc\d+)\}\{HTML\}\{([0-9A-Fa-f]{6})\}`)

// chartPreambleRE matches template.tex lines that change how a chart
// typesets (body font, table row height, TikZ libraries); they're copied into
// the standalone preamble so the PNG matches the PDF. Lines with a pandoc
// $variable$ are skipped.
var chartPreambleRE = regexp.MustCompile(`(?m)^\\(?:setmainfont|renewcommand\{\\arraystretch\}|SetTblrInner|usetikzlibrary)\{[^$\n]*$`)

// ResolveChartColors resolves chart colors as the PDF sees them: the
// template's chartcN palette and theme.yaml colors (templateDir may be ""),
// then the report's index.md `colors:` overrides. Mirrors
// core/src/report_rex_core/chart_raster.py's resolve_colors(). The template's
// chart-relevant preamble lines ride along under "preamble".
func ResolveChartColors(reportColors map[string]any, templateDir string) map[string]string {
	colors := map[string]string{"preamble": ""}
	for k, v := range defaultChartColors {
		colors[k] = v
	}
	for i, hexval := range chartPaletteHex {
		colors[fmt.Sprintf("chartc%d", i+1)] = hexval
	}
	if templateDir != "" {
		if tex, err := os.ReadFile(filepath.Join(templateDir, "template.tex")); err == nil {
			for _, m := range chartcRE.FindAllStringSubmatch(string(tex), -1) {
				colors[m[1]] = m[2]
			}
			colors["preamble"] = strings.Join(chartPreambleRE.FindAllString(string(tex), -1), "\n")
		}
		if raw, err := os.ReadFile(filepath.Join(templateDir, "theme.yaml")); err == nil {
			var theme struct {
				Colors map[string]any `yaml:"colors"`
			}
			if yaml.Unmarshal(raw, &theme) == nil {
				for k, v := range theme.Colors {
					if v != nil && fmt.Sprint(v) != "" {
						colors[k] = fmt.Sprint(v)
					}
				}
			}
		}
	}
	for k, v := range reportColors {
		if s, ok := v.(string); ok && s != "" {
			colors[k] = s
		}
	}
	return colors
}

func chartColorDefs(colors map[string]string) string {
	named := []struct{ name, key string }{
		{"colorPrimary", "primary"}, {"colorAccent", "accent"},
		{"colorCritical", "critical"}, {"colorHigh", "high"},
		{"colorMedium", "medium"}, {"colorLow", "low"}, {"colorInfo", "info"},
	}
	out := ""
	for _, n := range named {
		out += fmt.Sprintf("\\definecolor{%s}{HTML}{%s}\n", n.name, colors[n.key])
	}
	for i := range chartPaletteHex {
		out += fmt.Sprintf("\\definecolor{chartc%d}{HTML}{%s}\n", i+1, colors[fmt.Sprintf("chartc%d", i+1)])
	}
	return out
}

// 11pt and the body font match every bundled template.tex's article class
// and default \setmainfont.
const chartTexTemplate = `\documentclass[11pt,border=4pt]{standalone}
\usepackage{fontspec}
\setmainfont{TeX Gyre Heros}[Scale=0.92]
\usepackage{tikz}
\usepackage{pgf-pie}
\usepackage{tabularray}
\usepackage{xcolor}
\usepackage{fontawesome5}
%s
%s
\begin{document}
%s
\end{document}
`

// stripCenter drops charts.go's \begin{center}/\end{center} wrapper.
// standalone's auto-crop only recognizes specific top-level environments
// (tikzpicture, tblr, ...); wrapped in \center, it can't find the content's
// bounding box and falls back to a full page. Centering is meaningless for a
// standalone cropped image anyway (the DOCX paragraph centers the picture).
func stripCenter(tikz string) string {
	const begin, end = "\\begin{center}", "\\end{center}"
	t := strings.TrimSpace(tikz)
	if strings.HasPrefix(t, begin) && strings.HasSuffix(t, end) {
		t = strings.TrimSpace(t[len(begin) : len(t)-len(end)])
	}
	return t
}

// ChartCompiler turns workDir/chart.tex into workDir/chart.svg (via xelatex
// + pdftocairo, or an equivalent). RasterizeCharts is agnostic to where that
// happens — LocalChartCompiler runs the tools directly on the host (for
// --no-docker builds), while Docker-mode builds supply a compiler that runs
// the same two commands inside the report-rex image instead, since the host
// itself isn't guaranteed to have xelatex/pdftocairo installed.
type ChartCompiler func(workDir string) error

// LocalChartCompiler runs xelatex + pdftocairo directly on the host. Callers
// must ensure both are on PATH first (renderDocxNative already does).
func LocalChartCompiler(workDir string) error {
	cmd := exec.Command("xelatex", "-interaction=nonstopmode", "-halt-on-error", "chart.tex")
	cmd.Dir = workDir
	out, err := cmd.CombinedOutput()
	pdfPath := filepath.Join(workDir, "chart.pdf")
	if err != nil || !fileExists(pdfPath) {
		return fmt.Errorf("xelatex failed rasterizing a chart for DOCX: %w\n%s", err, out)
	}

	// Vector, not raster: the SVG draws exactly like the PDF's chart.
	svgPath := filepath.Join(workDir, "chart.svg")
	cmd = exec.Command("pdftocairo", "-svg", pdfPath, svgPath)
	out, err = cmd.CombinedOutput()
	if err != nil || !fileExists(svgPath) {
		return fmt.Errorf("pdftocairo failed converting a chart for DOCX: %w\n%s", err, out)
	}
	return nil
}

// compileChart compiles one chart's TikZ to a cropped SVG in workDir, returning
// the SVG path.
func compileChart(tikz string, colors map[string]string, workDir string, compile ChartCompiler) (string, error) {
	texPath := filepath.Join(workDir, "chart.tex")
	src := fmt.Sprintf(chartTexTemplate, chartColorDefs(colors), colors["preamble"], stripCenter(tikz))
	if err := os.WriteFile(texPath, []byte(src), 0o644); err != nil {
		return "", err
	}

	if err := compile(workDir); err != nil {
		return "", err
	}
	svgPath := filepath.Join(workDir, "chart.svg")
	if !fileExists(svgPath) {
		return "", fmt.Errorf("chart compiler did not produce %s", svgPath)
	}
	return svgPath, nil
}

// svgInPoints labels the root <svg> width/height as pt: pdftocairo sizes the
// SVG in PostScript points but writes bare numbers, which pandoc reads as
// 96dpi pixels (a 75% chart).
func svgInPoints(svg []byte) []byte {
	start := bytes.Index(svg, []byte("<svg"))
	if start < 0 {
		return svg
	}
	end := bytes.IndexByte(svg[start:], '>')
	if end < 0 {
		return svg
	}
	tag := svgSizeRE.ReplaceAll(svg[start:start+end], []byte(`${1}${2}pt"`))
	return append(append(append([]byte{}, svg[:start]...), tag...), svg[start+end:]...)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// RasterizeCharts replaces ChartBlockRE-tagged chart blocks in combinedMD with
// rasterized PNG image refs. PNGs are written under outDir/charts/. Returns
// the rewritten Markdown; combinedMD itself is left untouched (callers pass a
// copy of its text).
func RasterizeCharts(combinedMD string, outDir string, colors map[string]string, compile ChartCompiler) (string, error) {
	matches := ChartBlockRE.FindAllStringSubmatch(combinedMD, -1)
	if len(matches) == 0 {
		return combinedMD, nil
	}

	chartsDir := filepath.Join(outDir, "charts")
	if err := os.MkdirAll(chartsDir, 0o755); err != nil {
		return "", err
	}

	replacements := map[string]string{}
	for _, m := range matches {
		chartID, tikz := m[1], m[2]
		if _, ok := replacements[chartID]; ok {
			continue
		}
		dest := filepath.Join(chartsDir, "chart-"+chartID+".svg")
		if !fileExists(dest) {
			workDir, err := os.MkdirTemp("", "rex-chart-")
			if err != nil {
				return "", err
			}
			svgPath, err := compileChart(tikz, colors, workDir, compile)
			if err != nil {
				os.RemoveAll(workDir)
				return "", err
			}
			data, err := os.ReadFile(svgPath)
			os.RemoveAll(workDir)
			if err != nil {
				return "", err
			}
			data = svgInPoints(data)
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				return "", err
			}
		}
		rel, err := filepath.Rel(outDir, dest)
		if err != nil {
			return "", err
		}
		// Uncaptioned (an alt text would become a "Chart" caption), centered
		// via the reference.docx "Figure" paragraph style like the PDF's center.
		// .tblr marks a table chart (templates may space those differently).
		cls := ""
		if strings.Contains(tikz, "\\begin{tblr}") {
			cls = "{.tblr}"
		}
		replacements[chartID] = fmt.Sprintf("::: {custom-style=\"Figure\"}\n![](%s)%s\n:::", filepath.ToSlash(rel), cls)
	}

	return ChartBlockRE.ReplaceAllStringFunc(combinedMD, func(block string) string {
		sub := ChartBlockRE.FindStringSubmatch(block)
		return replacements[sub[1]]
	}), nil
}
