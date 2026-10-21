package report

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Severity is "error" (fails `rex report validate`) or "warning" (printed but
// does not fail it).
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is a single QA-gate finding. Line is the 1-based line it points at,
// or 0 when it applies to the whole file.
type Issue struct {
	File     string   `json:"file"`
	Check    string   `json:"check"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
	Line     int      `json:"line,omitempty"`
}

// Result collects every QA-gate issue found — validation never fails fast on
// the first issue.
type Result struct {
	Issues []Issue
}

func (r Result) Errors() []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Severity == SeverityError {
			out = append(out, i)
		}
	}
	return out
}

func (r Result) Warnings() []Issue {
	var out []Issue
	for _, i := range r.Issues {
		if i.Severity == SeverityWarning {
			out = append(out, i)
		}
	}
	return out
}

// OK reports whether there are no error-severity issues.
func (r Result) OK() bool {
	return len(r.Errors()) == 0
}

// ── Tunable thresholds (mirrors validator.py) ────────────────────────────────

const (
	minStepsToReproduceChars = 100
	minCaptionChars          = 35

	// Light-mode check: average border luminance (0-255) below this is dark-mode.
	minLuminanceLightMode = 128
	// Annotation check: minimum saturated red/orange/yellow pixel count.
	minSaturatedClusterPx = 200
	// Text-legibility check: minimum estimated glyph height in pixels.
	minTextHeightPx = 10
	// Text-legibility check: minimum ink pixel count for a row to count as
	// "part of a text line".
	minRowInkPx = 3
	// Border sampling margin fraction for the light-mode check.
	luminanceSampleMarginFraction = 0.03
)

var (
	validateHeadingRe    = regexp.MustCompile(`(?m)^(#{1,5})(\s)`)
	mdImageRefRe         = regexp.MustCompile(`(?i)!\[([^\]]*)\]\(\s*([^)\s]+\.(?:png|jpe?g|gif))(?:\s+[^)]*)?\)(?:\{[^}]*\})?`)
	darkOverrideMarkerRe = regexp.MustCompile(`(?m)^>\s*\[!OVERRIDE-DARK-IMG\]\s*$`)
	stubMarkerRe         = regexp.MustCompile(`(?m)^[ \t]*#?[ \t]*>\s*\[!REX-STUB\]\s*$`)
)

// Validate runs every QA-gate check against an already-parsed report
// structure. Assumes structure.Reports/Sections/Appendices paths already
// exist on disk (ParseIndex would have errored otherwise).
func Validate(structure Structure) (Result, error) {
	var result Result
	var mdFiles []mdFile // every content file, for checkNetworkArtifacts
	findingNames := map[string]bool{}
	for _, p := range structure.Reports {
		base := filepath.Base(p)
		findingNames[base] = true
		findingNames[strings.TrimSuffix(base, filepath.Ext(base))] = true
	}

	checkFile := func(path string, isReport bool) error {
		raw, err := readContent(path, structure.Overlay)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		mdFiles = append(mdFiles, mdFile{path, raw})
		if issue := checkFrontmatter(path, raw); issue != nil {
			result.Issues = append(result.Issues, *issue)
			if issue.Check == "frontmatter_invalid" {
				return nil // mirrors validator.py: other checks would just be noise
			}
		}
		meta := ParseFrontmatter(raw)
		_, body := SplitFrontmatter(raw)

		if isReport {
			result.Issues = append(result.Issues, checkTitlePresent(path, raw, meta)...)
			result.Issues = append(result.Issues, checkStepsToReproduce(path, body)...)
			result.Issues = append(result.Issues, checkCvssPresent(path, meta)...)
			result.Issues = append(result.Issues, checkTimelinePresent(path, meta)...)
			result.Issues = append(result.Issues, checkScreenshotPresent(path, body)...)
		}
		result.Issues = append(result.Issues, checkPlaceholderText(path, meta, body)...)
		result.Issues = append(result.Issues, checkScreenshots(path, body, structure.BaseDir)...)
		result.Issues = append(result.Issues, checkStubMarkers(path, raw)...)
		result.Issues = append(result.Issues, checkMarkdownStructure(path, raw, isReport)...)
		result.Issues = append(result.Issues, checkDanglingRefs(path, raw, findingNames)...)
		return nil
	}

	for _, path := range structure.Reports {
		if err := checkFile(path, true); err != nil {
			return Result{}, err
		}
	}
	for _, path := range append(append([]string{}, structure.Sections...), structure.Appendices...) {
		if err := checkFile(path, false); err != nil {
			return Result{}, err
		}
	}
	indexPath := filepath.Join(structure.BaseDir, "index.md")
	result.Issues = append(result.Issues, checkPlaceholderText(indexPath, structure.Metadata, "")...)
	result.Issues = append(result.Issues, checkRequiredMetadata(indexPath, structure.Metadata)...)
	result.Issues = append(result.Issues, checkTestingWindow(indexPath, structure.Metadata)...)
	result.Issues = append(result.Issues, checkNetworkArtifacts(structure.BaseDir, mdFiles)...)
	return result, nil
}

// readContent returns path's overlay content if there is one, else its
// content on disk.
func readContent(path string, overlay map[string]string) (string, error) {
	if len(overlay) > 0 {
		if abs, err := filepath.Abs(path); err == nil {
			if content, ok := overlay[abs]; ok {
				return content, nil
			}
		}
	}
	raw, err := os.ReadFile(path)
	return string(raw), err
}

var yamlErrLineRe = regexp.MustCompile(`line \d+: `)

// checkFrontmatter flags frontmatter that ParseFrontmatter would silently
// drop (#58): YAML that doesn't parse, or a '---' block that isn't at the
// very top of the file (blank lines or a BOM before it).
func checkFrontmatter(path, content string) *Issue {
	if !strings.HasPrefix(content, "---") {
		trimmed := strings.TrimLeft(strings.TrimPrefix(content, "\ufeff"), " \t\r\n")
		firstLine, _, _ := strings.Cut(trimmed, "\n")
		if strings.TrimSpace(firstLine) != "---" {
			return nil
		}
		line := strings.Count(content[:len(content)-len(trimmed)], "\n") + 1
		return &Issue{path, "frontmatter_not_at_top",
			"Frontmatter must start on the very first line — anything before the '---' (even a blank line) makes Rex ignore it.",
			SeverityError, line}
	}
	end := strings.Index(content[3:], "\n---")
	if end == -1 {
		return nil
	}
	block := content[3 : end+3]
	var meta map[string]any
	if yaml.Unmarshal([]byte(block), &meta) == nil {
		return nil
	}
	// yaml.v3 often names the enclosing mapping's line rather than the bad
	// one (an unclosed quote reports line 1), so point at the first line
	// whose inclusion makes the block unparseable. The block starts right
	// after the opening '---' on line 1, so line k of it is file line k.
	// ponytail: a valid multi-line flow value/quoted string before the real
	// error would be blamed instead; parse bisection if that ever bites.
	lines := strings.Split(block, "\n")
	for k := 1; k <= len(lines); k++ {
		err := yaml.Unmarshal([]byte(strings.Join(lines[:k], "\n")), &meta)
		if err == nil {
			continue
		}
		msg := yamlErrLineRe.ReplaceAllString(strings.TrimPrefix(err.Error(), "yaml: "), "")
		msg = strings.TrimSpace(strings.TrimPrefix(msg, "unmarshal errors:"))
		if strings.Contains(msg, "unexpected end of stream") {
			msg = "unclosed quote or bracket on this line"
		}
		return &Issue{path, "frontmatter_invalid",
			"Invalid YAML frontmatter (" + msg + ") — this finding's metadata is lost and it renders as \"Untitled\".",
			SeverityError, k}
	}
	return nil
}

// checkTitlePresent: a finding with no title renders as a nameless one.
func checkTitlePresent(path, content string, meta map[string]any) []Issue {
	if v := meta["title"]; v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" {
		return nil
	}
	msg := "No 'title' in frontmatter — the finding renders as \"Untitled\"."
	if fm, _ := SplitFrontmatter(content); fm == "" {
		msg = "Finding has no frontmatter block (a '---' block at the very top of the file) — it renders as \"Untitled\"."
	}
	return []Issue{{path, "title_missing", msg, SeverityError, 0}}
}

// requiredMetadata are the index.md frontmatter fields every template renders
// on the cover/header — without them the report comes out "Untitled".
var requiredMetadata = []string{"title", "client", "date"}

// checkTestingWindow: the cover/Document Control "Testing Window" is
// start_date – end_date; with only one of them it shows a single date (or none)
// instead of a range (#74). Mirrors validator.py's _check_testing_window.
func checkTestingWindow(path string, meta map[string]any) []Issue {
	set := func(key string) bool { v := meta[key]; return v != nil && strings.TrimSpace(fmt.Sprint(v)) != "" }
	hasStart, hasEnd := set("start_date"), set("end_date")
	if hasStart == hasEnd {
		return nil
	}
	missing := "start_date"
	if hasStart {
		missing = "end_date"
	}
	return []Issue{{path, "testing_window_incomplete",
		fmt.Sprintf("No '%s' in index.md frontmatter — the Testing Window shows a single date instead of a start–end range.", missing),
		SeverityWarning, 0}}
}

func checkRequiredMetadata(path string, meta map[string]any) []Issue {
	var issues []Issue
	for _, key := range requiredMetadata {
		if v := meta[key]; v == nil || strings.TrimSpace(fmt.Sprint(v)) == "" {
			issues = append(issues, Issue{path, "metadata_missing",
				fmt.Sprintf("No '%s' in index.md frontmatter — it's required.", key), SeverityError, 0})
		}
	}
	return issues
}

// ── Check 1 — Replication (Steps to Reproduce) length ────────────────────────

func sectionBody(body, heading string) (string, bool) {
	matches := validateHeadingRe.FindAllStringSubmatchIndex(body, -1)
	for idx, m := range matches {
		level := m[3] - m[2]
		lineEnd := strings.Index(body[m[1]:], "\n")
		if lineEnd == -1 {
			lineEnd = len(body)
		} else {
			lineEnd += m[1]
		}
		headingText := strings.TrimSpace(body[m[1]:lineEnd])
		headingText = strings.TrimSpace(regexp.MustCompile(`\{[^}]*\}\s*$`).ReplaceAllString(headingText, ""))
		if !strings.EqualFold(headingText, heading) {
			continue
		}
		start := lineEnd + 1
		if lineEnd >= len(body) {
			start = len(body)
		}
		end := len(body)
		for _, next := range matches[idx+1:] {
			nextLevel := next[3] - next[2]
			if nextLevel <= level {
				end = next[0]
				break
			}
		}
		return body[start:end], true
	}
	return "", false
}

// replicationHeadings: `## Replication` (steps + evidence) replaced `## Steps to
// Reproduce` + `## Evidence` in the scaffolds (#68); reports written with the
// older heading still validate. Mirrors validator.py's _REPLICATION_HEADINGS.
var replicationHeadings = []string{"Replication", "Steps to Reproduce"}

func checkStepsToReproduce(path, body string) []Issue {
	var heading, section string
	for _, h := range replicationHeadings {
		if sec, ok := sectionBody(body, h); ok {
			heading, section = h, sec
			break
		}
	}
	if heading == "" {
		return []Issue{{path, "steps_to_reproduce_missing", "No '## Replication' section found (or the older '## Steps to Reproduce').", SeverityError, 0}}
	}
	textLen := len(strings.TrimSpace(section))
	if textLen < minStepsToReproduceChars {
		return []Issue{{path, "steps_to_reproduce_length",
			fmt.Sprintf("'%s' section is %d characters (minimum %d).", heading, textLen, minStepsToReproduceChars),
			SeverityError, 0}}
	}
	return nil
}

// ── Check 2 & 3 — Screenshot caption length + image heuristics ──────────────

func resolveImagePath(filePath, relTarget string) string {
	abs, err := filepath.Abs(filepath.Join(filepath.Dir(filePath), relTarget))
	if err != nil {
		return filepath.Join(filepath.Dir(filePath), relTarget)
	}
	return abs
}

func hasDarkOverride(body string, imageMatchStart int) bool {
	lines := strings.Split(body[:imageMatchStart], "\n")
	i := len(lines) - 1
	for i >= 0 && strings.TrimSpace(lines[i]) == "" {
		i--
	}
	if i < 0 {
		return false
	}
	return darkOverrideMarkerRe.MatchString(lines[i])
}

func checkScreenshots(path, body, baseDir string) []Issue {
	var issues []Issue
	for _, m := range mdImageRefRe.FindAllStringSubmatchIndex(body, -1) {
		caption := body[m[2]:m[3]]
		relTarget := body[m[4]:m[5]]
		trimmedCaption := strings.TrimSpace(caption)
		if len(trimmedCaption) < minCaptionChars {
			issues = append(issues, Issue{path, "screenshot_caption_length",
				fmt.Sprintf("Screenshot caption %q is %d characters (minimum %d): %s",
					trimmedCaption, len(trimmedCaption), minCaptionChars, relTarget),
				SeverityError, 0})
		}
		imagePath := resolveImagePath(path, relTarget)
		if _, err := os.Stat(imagePath); err != nil {
			issues = append(issues, Issue{path, "screenshot_missing",
				fmt.Sprintf("Referenced screenshot does not exist: %s", imagePath), SeverityError, 0})
			continue
		}
		skipDark := hasDarkOverride(body, m[0])
		issues = append(issues, checkScreenshotImage(path, imagePath, skipDark)...)
		issues = append(issues, checkPlaceholderScreenshot(path, imagePath)...)
	}
	return issues
}

// checkScreenshotPresent requires every finding to embed at least one screenshot.
func checkScreenshotPresent(path, body string) []Issue {
	if mdImageRefRe.MatchString(body) {
		return nil
	}
	return []Issue{{path, "screenshot_none",
		"No screenshot found — every finding needs at least one ![caption](path.png) image.", SeverityError, 0}}
}

// ── Check 4 — Missing CVSS field ─────────────────────────────────────────────

func checkCvssPresent(path string, meta map[string]any) []Issue {
	hasVector := strings.TrimSpace(toStr(meta["cvss_vector"])) != ""
	hasManual := meta["severity"] != nil || meta["cvss_score"] != nil
	if !hasVector && !hasManual {
		return []Issue{{path, "cvss_missing", "No CVSS data: missing both 'cvss_vector' and 'severity'/'cvss_score'.", SeverityError, 0}}
	}
	return nil
}

// ── Check 5 — Missing per-finding timeline entry ─────────────────────────────

func checkTimelinePresent(path string, meta map[string]any) []Issue {
	entries, ok := meta["timeline"].([]any)
	if !ok || len(entries) == 0 {
		return []Issue{{path, "timeline_missing", "No 'timeline' entry in this finding's frontmatter.", SeverityError, 0}}
	}
	return nil
}

// ── Check 6 — Unresolved [!REX-STUB] template markers ────────────────────────

func checkStubMarkers(path, raw string) []Issue {
	count := len(stubMarkerRe.FindAllString(raw, -1))
	if count > 0 {
		return []Issue{{path, "stub_marker_present",
			fmt.Sprintf("%d unresolved '[!REX-STUB]' marker(s) — replace the stub value/section and remove the marker line before building.", count),
			SeverityError, 0}}
	}
	return nil
}

// ── Check 8 — Markdown that won't render as the author meant (#71) ──────────
//
// Text pasted from elsewhere often loses its markdown: indentation that turns a
// list into a code block, section headings that are plain lines, and leftover
// TODO notes. All warnings: each is sometimes intended. Mirrors validator.py's
// _check_markdown_structure.

var (
	fenceLineRe  = regexp.MustCompile("^[ \t]*(```|~~~)")
	listItemRe   = regexp.MustCompile(`^[ \t]*(?:[-*+]|\d+[.)])[ \t]`)
	inlineCodeRe = regexp.MustCompile("`[^`]*`")
	todoMarkerRe = regexp.MustCompile(`\b(TODO|TBD|FIXME)\b`)
	sectionNames = map[string]bool{
		"description": true, "impact": true, "steps to reproduce": true, "evidence": true,
		"recommendation": true, "remediation": true, "replication": true,
	}
)

func checkMarkdownStructure(path, raw string, isReport bool) []Issue {
	var issues []Issue
	lines := strings.Split(raw, "\n")
	start := 0
	if strings.HasPrefix(raw, "---") {
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				start = i + 1
				break
			}
		}
	}
	inFence := false
	prev, lastText := "", "" // the previous line / the last non-blank line
	for i := start; i < len(lines); i++ {
		line, n := lines[i], i+1
		isFence := fenceLineRe.MatchString(line)
		if isFence {
			inFence = !inFence
		} else if !inFence && strings.TrimSpace(line) != "" {
			indented := strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
			if indented && strings.TrimSpace(prev) == "" && lastText != "" &&
				!strings.HasPrefix(lastText, " ") && !strings.HasPrefix(lastText, "\t") && !listItemRe.MatchString(lastText) {
				issues = append(issues, Issue{path, "indented_code_block",
					"Indented text renders as a monospace code block. Use a fenced ``` block for code or diagrams, or remove the indentation for prose and lists.",
					SeverityWarning, n})
			}
			if trimmed := strings.TrimSpace(line); isReport && !indented && sectionNames[strings.ToLower(trimmed)] {
				issues = append(issues, Issue{path, "section_heading_plain_text",
					fmt.Sprintf("'%s' is a plain line, not a heading — write '## %s'.", trimmed, trimmed),
					SeverityWarning, n})
			}
			if m := todoMarkerRe.FindStringSubmatch(inlineCodeRe.ReplaceAllString(line, "")); m != nil {
				issues = append(issues, Issue{path, "todo_marker",
					fmt.Sprintf("'%s' note left in the text.", m[1]), SeverityWarning, n})
			}
		}
		if !inFence || isFence {
			prev = line
			if strings.TrimSpace(line) != "" {
				lastText = line
			}
		}
	}
	return issues
}

// ── Check 9 — Links to findings that aren't in the report (#72) ─────────────

// checkDanglingRefs: `[text](vuln-2.md)` / `![](vuln-2.md)` become "Finding #N"
// links only when vuln-2.md is one of structure.Reports; otherwise they render
// as broken links. Mirrors validator.py's _check_dangling_refs.
func checkDanglingRefs(path, raw string, findingNames map[string]bool) []Issue {
	var issues []Issue
	inFence := false
	for i, line := range strings.Split(raw, "\n") {
		if fenceLineRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		var targets []string
		for _, m := range mdImgRefRe.FindAllStringSubmatch(line, -1) {
			targets = append(targets, m[1])
		}
		for _, m := range mdLinkRefRe.FindAllStringSubmatch(line, -1) {
			targets = append(targets, m[3])
		}
		for _, target := range targets {
			base := filepath.Base(target)
			if !findingNames[base] && !findingNames[strings.TrimSuffix(base, filepath.Ext(base))] {
				issues = append(issues, Issue{path, "dangling_reference",
					fmt.Sprintf("Link to '%s' doesn't match any finding in structure.reports — it would render as a broken link.", target),
					SeverityError, i + 1})
			}
		}
	}
	return issues
}

// ── Check 7 — Leftover scaffold placeholder text ─────────────────────────────

var loremIpsumRe = regexp.MustCompile(`(?i)lorem\s*ipsum`)

func checkPlaceholderText(path string, meta map[string]any, body string) []Issue {
	haystacks := []string{fmt.Sprintf("%v", meta), body}
	var issues []Issue
	for _, needle := range placeholderStrings {
		for _, h := range haystacks {
			found := strings.Contains(h, needle)
			if needle == PlaceholderLoremIpsum {
				// Any case, with or without the space: "Lorem ipsum dolor", "loremipsum.example" (#72).
				found = loremIpsumRe.MatchString(h)
			}
			if found {
				issues = append(issues, Issue{path, "placeholder_text",
					fmt.Sprintf("Scaffold placeholder text left in report: %q", needle), SeverityError, 0})
				break
			}
		}
	}
	return issues
}
