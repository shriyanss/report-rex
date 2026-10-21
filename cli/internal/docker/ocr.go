package docker

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif" // register decoders for image.Decode
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// tesseractArgs read the image as one block of text (--psm 6) and emit TSV
// word boxes, which LayoutTSV turns back into indented, column-aligned lines
// — tesseract's own text output drops leading whitespace.
var tesseractArgs = []string{"stdout", "--psm", "6", "tsv"}

// OCR extracts the text from imgPath with tesseract, inside the report-rex
// image by default (bypassing its Python entrypoint, like encryptDockerPDF),
// or via a local `tesseract` install when noDocker is set.
func OCR(image, imgPath string, noDocker bool) (string, error) {
	workDir, err := os.MkdirTemp("", "report-rex-ocr-")
	if err != nil {
		return "", fmt.Errorf("creating ocr work dir: %w", err)
	}
	defer os.RemoveAll(workDir)
	prepared := filepath.Join(workDir, "in.png")
	if err := prepareImage(imgPath, prepared); err != nil {
		return "", err
	}

	var cmd *exec.Cmd
	if noDocker {
		if _, err := exec.LookPath("tesseract"); err != nil {
			return "", fmt.Errorf("--no-docker requires a local tesseract install (not found in PATH): %w", err)
		}
		cmd = exec.Command("tesseract", append([]string{prepared}, tesseractArgs...)...)
	} else {
		if err := Preflight(); err != nil {
			return "", err
		}
		if !ImageExists(image) {
			if err := BuildImage(image); err != nil {
				return "", fmt.Errorf("auto-building missing image %q: %w", image, err)
			}
		}
		args := []string{"run", "--rm", "--entrypoint", "tesseract", "-v", workDir + ":/in:ro", image, "/in/in.png"}
		cmd = exec.Command("docker", append(args, tesseractArgs...)...)
	}

	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("tesseract failed: %w\n%s", err, stderr.String())
	}
	return LayoutTSV(out.String()), nil
}

// prepareImage writes src as a grayscale PNG with dark text on a light
// background (dark-theme screenshots are inverted), upscaled so small UI
// fonts reach the ~30px glyph height tesseract reads best.
func prepareImage(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("decoding %s (supported: png, jpeg, gif): %w", src, err)
	}

	b := img.Bounds()
	gray := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	var sum int
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			v := color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y
			gray.Pix[y*gray.Stride+x] = v
			sum += int(v)
		}
	}
	if n := b.Dx() * b.Dy(); n > 0 && sum/n < 128 {
		for i, v := range gray.Pix {
			gray.Pix[i] = 255 - v
		}
	}

	// ponytail: width-based scale guess; a real font-size estimate if
	// high-DPI screenshots of tiny text read badly.
	scale := 1
	switch {
	case b.Dx() < 1500:
		scale = 3
	case b.Dx() < 3000:
		scale = 2
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, upscale(gray, scale))
}

// upscale resizes src by an integer factor with bilinear interpolation.
func upscale(src *image.Gray, k int) *image.Gray {
	if k == 1 {
		return src
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewGray(image.Rect(0, 0, w*k, h*k))
	at := func(x, y int) float64 {
		x = min(max(x, 0), w-1)
		y = min(max(y, 0), h-1)
		return float64(src.Pix[y*src.Stride+x])
	}
	for y := 0; y < h*k; y++ {
		fy := (float64(y)+0.5)/float64(k) - 0.5
		y0, ty := int(math.Floor(fy)), fy-math.Floor(fy)
		for x := 0; x < w*k; x++ {
			fx := (float64(x)+0.5)/float64(k) - 0.5
			x0, tx := int(math.Floor(fx)), fx-math.Floor(fx)
			top := at(x0, y0)*(1-tx) + at(x0+1, y0)*tx
			bot := at(x0, y0+1)*(1-tx) + at(x0+1, y0+1)*tx
			dst.Pix[y*dst.Stride+x] = uint8(top*(1-ty) + bot*ty + 0.5)
		}
	}
	return dst
}

type ocrWord struct {
	left, width int
	text        string
}

type ocrLine struct {
	top, height int
	words       []ocrWord
}

// asciiQuotes maps the typographic quotes tesseract likes to guess back to
// the plain ones code and HTTP traffic actually contain.
var asciiQuotes = strings.NewReplacer("“", `"`, "”", `"`, "„", `"`, "‘", "'", "’", "'", "‚", "'")

// LayoutTSV rebuilds text from tesseract's TSV word boxes, assuming a
// monospace font: each word is placed at the column its pixel offset implies,
// restoring indentation and alignment, and large vertical gaps become blank lines.
func LayoutTSV(tsv string) string {
	lines := map[string]*ocrLine{}
	for _, row := range strings.Split(tsv, "\n") {
		f := strings.Split(row, "\t")
		if len(f) < 12 || f[0] != "5" || strings.TrimSpace(f[11]) == "" {
			continue
		}
		n := make([]int, 4)
		for i, col := range []int{6, 7, 8, 9} {
			n[i], _ = strconv.Atoi(f[col])
		}
		key := f[2] + "." + f[3] + "." + f[4]
		l := lines[key]
		if l == nil {
			l = &ocrLine{top: n[1], height: n[3]}
			lines[key] = l
		}
		l.top, l.height = min(l.top, n[1]), max(l.height, n[3])
		l.words = append(l.words, ocrWord{n[0], n[2], f[11]})
	}
	if len(lines) == 0 {
		return ""
	}

	sorted := make([]*ocrLine, 0, len(lines))
	var inkWidths, advances, heights []float64
	minLeft := math.MaxInt
	for _, l := range lines {
		sorted = append(sorted, l)
		heights = append(heights, float64(l.height))
		sort.Slice(l.words, func(a, c int) bool { return l.words[a].left < l.words[c].left })
		for j, w := range l.words {
			inkWidths = append(inkWidths, float64(w.width)/float64(len([]rune(w.text))))
			minLeft = min(minLeft, w.left)
			if j > 0 {
				// Most neighbouring words are one space apart, so the start-to-start
				// distance gives the glyph advance; ink width alone undershoots it.
				prev := l.words[j-1]
				advances = append(advances, float64(w.left-prev.left)/float64(len([]rune(prev.text))+1))
			}
		}
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].top < sorted[j].top })
	cw, pitch := median(inkWidths), median(heights)*1.4
	if len(advances) > 0 {
		cw = median(advances)
	}
	var steps []float64
	for i := 1; i < len(sorted); i++ {
		steps = append(steps, float64(sorted[i].top-sorted[i-1].top))
	}
	if len(steps) > 0 {
		pitch = median(steps) // most lines are consecutive, so this is the line pitch
	}

	var b strings.Builder
	for i, l := range sorted {
		if i > 0 {
			gap := float64(l.top-sorted[i-1].top) / pitch
			for range max(int(math.Round(gap))-1, 0) {
				b.WriteString("\n")
			}
		}
		// The first word's offset is the indent; later words are spaced relative
		// to the previous one, so a slightly-off cw can't drift across a long
		// line, and words tesseract split without a real gap are glued back.
		for j, w := range l.words {
			spaces := int(math.Round(float64(w.left-minLeft) / cw))
			if j > 0 {
				prev := l.words[j-1]
				spaces = int(math.Round(float64(w.left-prev.left)/cw)) - len([]rune(prev.text))
			}
			b.WriteString(strings.Repeat(" ", max(spaces, 0)))
			b.WriteString(w.text)
		}
		b.WriteString("\n")
	}
	return asciiQuotes.Replace(b.String())
}

func median(xs []float64) float64 {
	sort.Float64s(xs)
	return xs[len(xs)/2]
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

// FormatOCR tidies OCR text (trailing spaces, form feeds, runs of blank
// lines) and wraps it in a fenced Markdown code block tagged lang.
func FormatOCR(raw, lang string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\f", ""), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	text := strings.Trim(blankRuns.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"), "\n")
	return "```" + lang + "\n" + text + "\n```\n"
}
