package report

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func requireChartToolchain(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"xelatex", "pdftocairo"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("requires %s on PATH", bin)
		}
	}
}

func TestChartBlockRE_MatchesTaggedBlock(t *testing.T) {
	block := Render(sampleFindings(), "pie", "severity")
	m := ChartBlockRE.FindStringSubmatch(block)
	if m == nil {
		t.Fatalf("expected chart block to match ChartBlockRE:\n%s", block)
	}
	if !strings.Contains(m[2], "tikzpicture") {
		t.Errorf("expected tikz body to contain tikzpicture, got:\n%s", m[2])
	}
}

func TestRasterizeCharts_ReplacesBlockWithImageRef(t *testing.T) {
	requireChartToolchain(t)

	block := Render(sampleFindings(), "pie", "severity")
	colors := ResolveChartColors(nil, "")
	outDir := t.TempDir()
	rewritten, err := RasterizeCharts(block, outDir, colors, LocalChartCompiler)
	if err != nil {
		t.Fatalf("RasterizeCharts failed: %v", err)
	}
	if ChartBlockRE.MatchString(rewritten) {
		t.Errorf("expected chart block to be replaced, got:\n%s", rewritten)
	}
	if !strings.HasPrefix(rewritten, "::: {custom-style=\"Figure\"}\n![](charts/chart-") {
		t.Errorf("expected an image reference, got:\n%s", rewritten)
	}
	svgs, _ := filepath.Glob(filepath.Join(outDir, "charts", "*.svg"))
	if len(svgs) != 1 {
		t.Fatalf("expected one chart SVG, got %v", svgs)
	}
	// sized in points, as the PDF draws it (bare numbers read as pixels)
	if svg, _ := os.ReadFile(svgs[0]); !regexp.MustCompile(`<svg[^>]* width="[\d.]+pt"`).Match(svg) {
		t.Errorf("expected the SVG sized in pt:\n%.300s", svg)
	}
}

func TestRasterizeCharts_LeavesNonChartTextUntouched(t *testing.T) {
	colors := ResolveChartColors(nil, "")
	text := "Just some ordinary report prose.\n"
	got, err := RasterizeCharts(text, t.TempDir(), colors, LocalChartCompiler)
	if err != nil {
		t.Fatalf("RasterizeCharts failed: %v", err)
	}
	if got != text {
		t.Errorf("expected text to pass through unchanged, got:\n%s", got)
	}
}
