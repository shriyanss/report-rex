package report

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makePNG(t *testing.T, path string, fill func(x, y int) color.RGBA, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, fill(x, y))
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

func lightBackground(x, y int) color.RGBA { return color.RGBA{240, 240, 240, 255} }
func darkBackground(x, y int) color.RGBA  { return color.RGBA{20, 20, 20, 255} }

// withAnnotation mirrors the Python fixture's 30x80 red rectangle at (50,50)-(80,130),
// well above MIN_SATURATED_CLUSTER_PX (200px).
func withAnnotation(base func(x, y int) color.RGBA) func(x, y int) color.RGBA {
	return func(x, y int) color.RGBA {
		if y >= 50 && y < 80 && x >= 50 && x < 130 {
			return color.RGBA{255, 60, 0, 255}
		}
		return base(x, y)
	}
}

func TestCheckScreenshotImageDarkModeIsError(t *testing.T) {
	path := makePNG(t, filepath.Join(t.TempDir(), "dark.png"), darkBackground, 200, 200)
	issues := checkScreenshotImage("vuln.md", path, false)
	if !anyCheck(issues, "screenshot_dark_mode") {
		t.Errorf("expected screenshot_dark_mode, got %v", issues)
	}
}

func TestCheckScreenshotImageLightModePassesLuminanceCheck(t *testing.T) {
	path := makePNG(t, filepath.Join(t.TempDir(), "light.png"), lightBackground, 200, 200)
	issues := checkScreenshotImage("vuln.md", path, false)
	if anyCheck(issues, "screenshot_dark_mode") {
		t.Errorf("should not flag dark mode: %v", issues)
	}
	if !anyCheck(issues, "screenshot_no_annotation") {
		t.Errorf("expected screenshot_no_annotation, got %v", issues)
	}
}

func TestCheckScreenshotImageNoAnnotationIsError(t *testing.T) {
	path := makePNG(t, filepath.Join(t.TempDir(), "plain.png"), lightBackground, 200, 200)
	issues := checkScreenshotImage("vuln.md", path, false)
	if !anyCheck(issues, "screenshot_no_annotation") {
		t.Errorf("expected screenshot_no_annotation, got %v", issues)
	}
}

func TestCheckScreenshotImageAnnotationPresentPasses(t *testing.T) {
	path := makePNG(t, filepath.Join(t.TempDir(), "annotated.png"), withAnnotation(lightBackground), 200, 200)
	issues := checkScreenshotImage("vuln.md", path, false)
	if anyCheck(issues, "screenshot_no_annotation") {
		t.Errorf("should not flag missing annotation: %v", issues)
	}
}

func TestCheckScreenshotImageCorruptIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.png")
	if err := os.WriteFile(path, []byte("not a real png"), 0o644); err != nil {
		t.Fatal(err)
	}
	issues := checkScreenshotImage("vuln.md", path, false)
	if len(issues) != 1 || issues[0].Check != "screenshot_unreadable" {
		t.Errorf("got %v", issues)
	}
}

func TestCheckPlaceholderScreenshotKnownHashIsFlagged(t *testing.T) {
	path := makePNG(t, filepath.Join(t.TempDir(), "demo.png"), withAnnotation(lightBackground), 200, 200)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)

	original := PlaceholderScreenshotSHA256
	PlaceholderScreenshotSHA256 = hex.EncodeToString(sum[:])
	defer func() { PlaceholderScreenshotSHA256 = original }()

	issues := checkPlaceholderScreenshot("vuln.md", path)
	if len(issues) != 1 || issues[0].Check != "screenshot_placeholder_image" {
		t.Errorf("got %v", issues)
	}
}

func TestCheckPlaceholderScreenshotDifferentImagePasses(t *testing.T) {
	path := makePNG(t, filepath.Join(t.TempDir(), "real.png"), withAnnotation(lightBackground), 200, 200)

	original := PlaceholderScreenshotSHA256
	PlaceholderScreenshotSHA256 = strings.Repeat("0", 64)
	defer func() { PlaceholderScreenshotSHA256 = original }()

	issues := checkPlaceholderScreenshot("vuln.md", path)
	if len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

// staggeredText simulates real anti-aliased/serif text: each column has a short (~5px)
// ink run, but the run's vertical start is staggered by column so no single column spans
// the full ~50px line height — only the union across columns does. The old
// median-of-per-column-runs algorithm measured ~5px here (a false "too small" positive);
// row-banding should measure the full ~50px band instead.
func staggeredText(x, y int) color.RGBA {
	start := 40 + (x % 50)
	if y >= start && y < start+5 {
		return color.RGBA{20, 20, 20, 255}
	}
	return color.RGBA{240, 240, 240, 255}
}

// uniformSmallText is a genuinely small ~6px ink band present in every column — a
// regression check that the fix still flags real small text, not just staggered text.
func uniformSmallText(x, y int) color.RGBA {
	if y >= 40 && y < 46 {
		return color.RGBA{20, 20, 20, 255}
	}
	return color.RGBA{240, 240, 240, 255}
}

func TestCheckTextLegibilityStaggeredColumnsDoesNotFalselyFlag(t *testing.T) {
	path := makePNG(t, filepath.Join(t.TempDir(), "staggered.png"), staggeredText, 200, 100)
	issues := checkScreenshotImage("vuln.md", path, false)
	if anyCheck(issues, "screenshot_text_too_small") {
		t.Errorf("should not flag text_too_small for staggered (unioned ~50px) text: %v", issues)
	}
}

func TestCheckTextLegibilityUniformSmallTextStillFlagged(t *testing.T) {
	path := makePNG(t, filepath.Join(t.TempDir(), "small.png"), uniformSmallText, 200, 100)
	issues := checkScreenshotImage("vuln.md", path, false)
	var found *Issue
	for i := range issues {
		if issues[i].Check == "screenshot_text_too_small" {
			found = &issues[i]
		}
	}
	if found == nil {
		t.Fatalf("expected screenshot_text_too_small for genuinely small (~6px) text, got %v", issues)
	}
	if found.Severity != SeverityError {
		t.Errorf("expected screenshot_text_too_small to be error-severity, got %v", found.Severity)
	}
}

func anyCheck(issues []Issue, check string) bool {
	for _, i := range issues {
		if i.Check == check {
			return true
		}
	}
	return false
}
