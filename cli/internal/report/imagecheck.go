package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"sort"
)

// checkScreenshotImage runs the image heuristics against one screenshot.
// skipDarkCheck is set when the markdown body has a preceding
// `> [!OVERRIDE-DARK-IMG]` marker for this image — the annotation and
// legibility checks still run even when the dark-mode check is skipped.
func checkScreenshotImage(findingPath, imagePath string, skipDarkCheck bool) []Issue {
	img, err := loadImage(imagePath)
	if err != nil {
		return []Issue{{findingPath, "screenshot_unreadable",
			fmt.Sprintf("Could not open image as a valid PNG/JPEG/GIF: %s", imagePath), SeverityError, 0}}
	}

	var issues []Issue
	if !skipDarkCheck {
		issues = append(issues, checkLightMode(findingPath, imagePath, img)...)
	}
	issues = append(issues, checkAnnotationPresent(findingPath, imagePath, img)...)
	issues = append(issues, checkTextLegibility(findingPath, imagePath, img)...)
	return issues
}

// checkPlaceholderScreenshot flags the shipped Lorem Ipsum placeholder
// screenshot (see scripts/gen_placeholder_screenshot.py) via a flat SHA-256
// equality check — not an image-similarity system, just "is this literally
// the demo image".
func checkPlaceholderScreenshot(findingPath, imagePath string) []Issue {
	raw, err := os.ReadFile(imagePath)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) == PlaceholderScreenshotSHA256 {
		return []Issue{{findingPath, "screenshot_placeholder_image",
			fmt.Sprintf("Screenshot is the shipped Lorem Ipsum placeholder image — replace it with a real annotated screenshot: %s", imagePath),
			SeverityError, 0}}
	}
	return nil
}

func loadImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func rgb8(img image.Image, x, y int) (int, int, int) {
	r, g, b, _ := img.At(x, y).RGBA()
	return int(r >> 8), int(g >> 8), int(b >> 8)
}

func checkLightMode(findingPath, imagePath string, img image.Image) []Issue {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	minX, minY := bounds.Min.X, bounds.Min.Y

	my := int(float64(h) * luminanceSampleMarginFraction)
	if my < 1 {
		my = 1
	}
	mx := int(float64(w) * luminanceSampleMarginFraction)
	if mx < 1 {
		mx = 1
	}

	var total float64
	var count int
	add := func(x, y int) {
		r, g, b := rgb8(img, minX+x, minY+y)
		total += 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
		count++
	}
	for y := 0; y < my; y++ {
		for x := 0; x < w; x++ {
			add(x, y)
		}
	}
	for y := h - my; y < h; y++ {
		for x := 0; x < w; x++ {
			add(x, y)
		}
	}
	for x := 0; x < mx; x++ {
		for y := 0; y < h; y++ {
			add(x, y)
		}
	}
	for x := w - mx; x < w; x++ {
		for y := 0; y < h; y++ {
			add(x, y)
		}
	}

	avgLuminance := total / float64(count)
	if avgLuminance < minLuminanceLightMode {
		return []Issue{{findingPath, "screenshot_dark_mode",
			fmt.Sprintf("Screenshot appears to be dark-mode (avg border luminance %.1f, minimum %d): %s. "+
				"Add a '> [!OVERRIDE-DARK-IMG]' line directly above the image reference if this is intentional.",
				avgLuminance, minLuminanceLightMode, imagePath),
			SeverityError, 0}}
	}
	return nil
}

func checkAnnotationPresent(findingPath, imagePath string, img image.Image) []Issue {
	bounds := img.Bounds()
	count := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b := rgb8(img, x, y)
			maxc := max3(r, g, b)
			minc := min3(r, g, b)
			var saturation float64
			if maxc > 0 {
				denom := maxc
				if denom < 1 {
					denom = 1
				}
				saturation = float64(maxc-minc) / float64(denom)
			}
			redOrangeYellow := maxc == r && (r-b > 40)
			if redOrangeYellow && saturation > 0.6 && maxc > 128 {
				count++
			}
		}
	}
	if count < minSaturatedClusterPx {
		return []Issue{{findingPath, "screenshot_no_annotation",
			fmt.Sprintf("No annotation marks (arrows/highlight boxes) detected — only %d saturated "+
				"red/orange/yellow pixels found (minimum %d): %s", count, minSaturatedClusterPx, imagePath),
			SeverityError, 0}}
	}
	return nil
}

// checkTextLegibility approximates typical text-line height via a horizontal
// ink-density row profile: a row counts as "textish" once enough of its
// pixels are dark relative to the image mean, and contiguous textish rows are
// run-length encoded into bands. Any single glyph touching a row is enough to
// mark it textish, so a band's height reflects a full line's vertical extent
// (union across every column in the line) rather than one column's incidental
// — usually partial — intersection with a curve, serif, or anti-aliasing
// edge. Not OCR — an approximation of visual text size, not a DPI-correct
// legibility measurement, hence warning.
func checkTextLegibility(findingPath, imagePath string, img image.Image) []Issue {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	minX, minY := bounds.Min.X, bounds.Min.Y

	gray := make([]float64, w*h)
	var total float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b := rgb8(img, minX+x, minY+y)
			v := (float64(r) + float64(g) + float64(b)) / 3.0
			gray[y*w+x] = v
			total += v
		}
	}
	threshold := (total / float64(w*h)) * 0.6

	rowInk := make([]int, h)
	inkCount := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if gray[y*w+x] < threshold {
				rowInk[y]++
				inkCount++
			}
		}
	}
	if inkCount < 50 {
		return nil
	}

	var bandHeights []int
	inBand := false
	bandStart := 0
	for y := 0; y < h; y++ {
		textish := rowInk[y] >= minRowInkPx
		if textish && !inBand {
			inBand = true
			bandStart = y
		} else if !textish && inBand {
			inBand = false
			bandHeights = append(bandHeights, y-bandStart)
		}
	}
	if inBand {
		bandHeights = append(bandHeights, h-bandStart)
	}

	var plausible []int
	for _, bh := range bandHeights {
		if bh >= 2 && bh <= 60 {
			plausible = append(plausible, bh)
		}
	}
	if len(plausible) == 0 {
		return nil
	}
	medianHeight := medianInt(plausible)
	if medianHeight < minTextHeightPx {
		return []Issue{{findingPath, "screenshot_text_too_small",
			fmt.Sprintf("Estimated text height ~%.1fpx is below the legibility threshold (%dpx) — "+
				"text may be too small to read at typical rendered width. This is an approximate heuristic, "+
				"not a precise measurement: %s", medianHeight, minTextHeightPx, imagePath),
			SeverityError, 0}}
	}
	return nil
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

func medianInt(vals []int) float64 {
	sorted := append([]int{}, vals...)
	sort.Ints(sorted)
	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2.0
}
