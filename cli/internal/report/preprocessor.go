package report

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shriyanss/report-rex/cli/internal/cvss"
	"github.com/shriyanss/report-rex/cli/internal/version"
	"gopkg.in/yaml.v3"
)

// ── Path rebasing ────────────────────────────────────────────────────────────

// rebasePaths rewrites relative image/link paths so they work from the
// assembled root, stripping the "../" segments a nested file needed to reach
// base_dir (since the assembled combined.md lives at base_dir's level).
func rebasePaths(body, filePath, baseDir string) string {
	rel, err := filepath.Rel(baseDir, filepath.Dir(filePath))
	if err != nil || strings.HasPrefix(rel, "..") {
		return body
	}
	depth := 0
	if rel != "." {
		depth = len(strings.Split(rel, string(filepath.Separator)))
	}
	if depth == 0 {
		return body
	}
	prefix := strings.Repeat("../", depth)
	re := regexp.MustCompile(`\]\(` + regexp.QuoteMeta(prefix))
	return re.ReplaceAllString(body, "](")
}

// ── Finding metadata / CVSS ──────────────────────────────────────────────────

func toStr(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		if t == math.Trunc(t) {
			return strconv.FormatFloat(t, 'f', 1, 64)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "True"
		}
		return "False"
	default:
		return fmt.Sprint(t)
	}
}

// enrich returns finding metadata with computed severity/score/version. An
// explicit cvss_vector drives severity and score; a manually supplied
// severity/cvss_score is used only when no vector is present.
func enrich(meta map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range meta {
		out[k] = v
	}
	vector := strings.TrimSpace(toStr(meta["cvss_vector"]))
	if vector != "" {
		result, err := cvss.Compute(vector)
		if err == nil {
			sev := result.Severity
			if sev == "None" {
				sev = "Info"
			}
			out["severity"] = sev
			out["cvss_score"] = fmt.Sprintf("%.1f", result.Score)
			out["cvss_version"] = result.Version
		} else if _, ok := out["severity"]; !ok {
			out["severity"] = "Info"
		}
	} else {
		if _, ok := out["severity"]; !ok {
			out["severity"] = "Info"
		}
		if meta["cvss_score"] != nil {
			out["cvss_score"] = toStr(meta["cvss_score"])
		}
	}
	return out
}

type finding struct {
	path string
	meta map[string]any
}

func sortFindings(findings []finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		ri := cvss.SeverityRank[toStr(findings[i].meta["severity"])]
		rj := cvss.SeverityRank[toStr(findings[j].meta["severity"])]
		if ri != rj {
			return ri > rj
		}
		// Within a severity, the higher CVSS score first (#67).
		if si, sj := cvssScore(findings[i].meta), cvssScore(findings[j].meta); si != sj {
			return si > sj
		}
		ti := strings.ToLower(toStr(findings[i].meta["title"]))
		tj := strings.ToLower(toStr(findings[j].meta["title"]))
		return ti < tj
	})
}

// cvssScore is a finding's numeric cvss_score, or 0 when missing/unparseable.
func cvssScore(meta map[string]any) float64 {
	score, err := strconv.ParseFloat(strings.TrimSpace(toStr(meta["cvss_score"])), 64)
	if err != nil {
		return 0
	}
	return score
}

// ── Cross-references & links ────────────────────────────────────────────────

var (
	mdImgRefRe     = regexp.MustCompile(`!\[[^\]]*\]\(\s*([^)\s]+\.md)\s*\)`)
	mdLinkRefRe    = regexp.MustCompile(`(^|[^!])\[([^\]]*)\]\(\s*([^)\s]+\.md)\s*\)`)
	httpFootnoteRe = regexp.MustCompile(`(^|[^!])\[([^\]]+)\]\(\s*(https?://[^)\s]+)\s*\)`)
	httpDirectRe   = regexp.MustCompile(`!!\[([^\]]+)\]\(\s*(https?://[^)\s]+)\s*\)`)
	tableRowRe     = regexp.MustCompile(`^[ \t]*\|.*\|[ \t]*$`)
)

type findingRef struct {
	n      int
	title  string
	anchor string
}

func resolveRef(target string, registry map[string]findingRef) (string, bool) {
	stem := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
	entry, ok := registry[stem]
	if !ok {
		entry, ok = registry[filepath.Base(target)]
	}
	if !ok {
		return "", false
	}
	return fmt.Sprintf(`[Finding \#%d — %s](#%s)`, entry.n, entry.title, entry.anchor), true
}

func transform(body string, registry map[string]findingRef) string {
	body = mdImgRefRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := mdImgRefRe.FindStringSubmatch(m)
		if resolved, ok := resolveRef(sub[1], registry); ok {
			return resolved
		}
		return m
	})

	body = mdLinkRefRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := mdLinkRefRe.FindStringSubmatch(m)
		prefix, target := sub[1], sub[3]
		if resolved, ok := resolveRef(target, registry); ok {
			return prefix + resolved
		}
		return m
	})

	// Skipped for table rows — pandoc emits table rows as a LaTeX
	// `longtable`, where a `\footnote` silently loses its text instead of
	// rendering, so links there stay plain inline links instead.
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if tableRowRe.MatchString(line) {
			continue
		}
		lines[i] = httpFootnoteRe.ReplaceAllStringFunc(line, func(m string) string {
			sub := httpFootnoteRe.FindStringSubmatch(m)
			return fmt.Sprintf("%s%s^[<%s>]", sub[1], sub[2], sub[3])
		})
	}
	body = strings.Join(lines, "\n")

	body = httpDirectRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := httpDirectRe.FindStringSubmatch(m)
		return fmt.Sprintf("[%s](%s)", sub[1], sub[2])
	})

	return body
}

// ── Chart directives ─────────────────────────────────────────────────────────

var graphDirectiveRe = regexp.MustCompile(
	`(?m)^[ \t]*:::[ \t]*(?:findings_graph|graph):([a-zA-Z]+)(?::([a-zA-Z_-]+))?[ \t]*:::[ \t]*$`,
)

var assetGraphDirectiveRe = regexp.MustCompile(
	`(?m)^[ \t]*:::[ \t]*asset_graph:([a-zA-Z]+)(?::([a-zA-Z_-]+))?[ \t]*:::[ \t]*$`,
)

var webGraphDirectiveRe = regexp.MustCompile(
	`(?m)^[ \t]*:::[ \t]*web_graph:([a-zA-Z]+)(?::([a-zA-Z_-]+))?[ \t]*:::[ \t]*$`,
)

func applyCharts(body string, findingsData []map[string]any) string {
	return graphDirectiveRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := graphDirectiveRe.FindStringSubmatch(m)
		gtype, dataset := sub[1], sub[2]
		if dataset == "" {
			dataset = "severity"
		}
		return Render(findingsData, gtype, dataset)
	})
}

func applyAssetCharts(body string, assetsData []map[string]any) string {
	return assetGraphDirectiveRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := assetGraphDirectiveRe.FindStringSubmatch(m)
		gtype, dataset := sub[1], sub[2]
		if dataset == "" {
			dataset = "os"
		}
		return RenderAssets(assetsData, gtype, dataset)
	})
}

func applyWebCharts(body string, webfindingsData []map[string]any) string {
	return webGraphDirectiveRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := webGraphDirectiveRe.FindStringSubmatch(m)
		gtype, dataset := sub[1], sub[2]
		if dataset == "" {
			dataset = "host"
		}
		return RenderWeb(webfindingsData, gtype, dataset)
	})
}

var networkDirectiveRe = regexp.MustCompile(`(?m)^[ \t]*:::[ \t]*network_diagram[ \t]*:::[ \t]*$`)

// applyNetwork replaces `:::network_diagram:::` with a diagram built from
// artifacts/assets.json (required — the build fails without it) and the
// optional artifacts/connections.json (subnet labels, pivots).
func applyNetwork(body, baseDir string) (string, error) {
	if !networkDirectiveRe.MatchString(body) {
		return body, nil
	}
	if _, err := os.Stat(filepath.Join(baseDir, "artifacts", "assets.json")); err != nil {
		return "", fmt.Errorf(":::network_diagram::: needs artifacts/assets.json — run `rex ingest` first")
	}
	var connections map[string]any
	if data, err := os.ReadFile(filepath.Join(baseDir, "artifacts", "connections.json")); err == nil {
		if err := json.Unmarshal(data, &connections); err != nil {
			return "", fmt.Errorf("network diagram: parsing artifacts/connections.json: %w", err)
		}
	}
	diagram, err := RenderNetwork(loadJSONArtifact(baseDir, "assets.json"), connections)
	if err != nil {
		return "", fmt.Errorf("network diagram: %w", err)
	}
	return networkDirectiveRe.ReplaceAllLiteralString(body, diagram), nil
}

// loadJSONArtifact reads an artifacts/<name> file written by `rex ingest`
// (assets.json / webfindings.json), returning nil if it doesn't exist yet.
func loadJSONArtifact(baseDir, name string) []map[string]any {
	data, err := os.ReadFile(filepath.Join(baseDir, "artifacts", name))
	if err != nil {
		return nil
	}
	var out []map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

// ── Heading demotion (finding bodies) ────────────────────────────────────────

var headingRe = regexp.MustCompile(`(?m)^(#{1,5})(\s)`)

func demote(body string, levels int) string {
	return headingRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := headingRe.FindStringSubmatch(m)
		n := len(sub[1]) + levels
		if n > 6 {
			n = 6
		}
		return strings.Repeat("#", n) + sub[2]
	})
}

// ── QA-gate directives (validate.go) ─────────────────────────────────────────

var darkOverrideMarkerLineRe = regexp.MustCompile(`(?m)^>\s*\[!OVERRIDE-DARK-IMG\]\s*\n?`)

func stripQADirectives(body string) string {
	return darkOverrideMarkerLineRe.ReplaceAllString(body, "")
}

// ── Image separation ──────────────────────────────────────────────────────────

// Image lines with no blank line between them are one pandoc paragraph, so the
// images render inline and squash together; an image-only line run together with
// text also renders inline, as a captionless image inside that paragraph. Give
// image-only lines a blank line on both sides.
//
// pandoc only attaches an attribute block written directly after the image. One
// split off by a space or a line break (`![..](..) {width=90%}` / `{width=90%}` on
// the next line) renders as literal text instead (#70), so re-attach it.
// Mirrors preprocessor.py's _separate_images.
const imgAttrs = `\{[ \t]*(?:[.#][^}\n]*|[^}\n]*=[^}\n]*)\}` // {width=90%} / {.class} / {#id}

var (
	imgLineRe        = regexp.MustCompile(`^[ \t]*!\[[^\]]*\]\([^)]*\)(\{[^}]*\})?[ \t]*$`)
	fenceRe          = regexp.MustCompile("^[ \t]*(```|~~~)")
	imgSpacedAttrsRe = regexp.MustCompile(`(!\[[^\]]*\]\([^)]*\))[ \t]+(` + imgAttrs + `)`)
	attrsLineRe      = regexp.MustCompile(`^[ \t]*(` + imgAttrs + `)[ \t]*`)
)

func separateImages(body string) string {
	var out []string
	inFence, prevImg := false, false
	for _, line := range strings.Split(body, "\n") {
		if fenceRe.MatchString(line) {
			inFence = !inFence
		} else if !inFence {
			line = imgSpacedAttrsRe.ReplaceAllString(line, "$1$2")
			if prevImg && strings.HasSuffix(strings.TrimRight(out[len(out)-1], " \t"), ")") {
				if m := attrsLineRe.FindStringSubmatchIndex(line); m != nil {
					out[len(out)-1] = strings.TrimRight(out[len(out)-1], " \t") + line[m[2]:m[3]]
					line = line[m[1]:]
					if line == "" {
						continue
					}
				}
			}
		}
		isImg := !inFence && imgLineRe.MatchString(line)
		if strings.TrimSpace(line) != "" && (isImg || prevImg) && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) != "" {
			out = append(out, "")
		}
		out = append(out, line)
		prevImg = isImg
	}
	return strings.Join(out, "\n")
}

// appendixPrefixRe matches "Appendix", "Appendix B —", "Appendix 2:" … — any
// lettering the author (or `rex ingest`, which writes "Appendix — Discovered
// Assets") put in the title.
var appendixPrefixRe = regexp.MustCompile(`(?i)^Appendix(?:\s+[A-Za-z0-9]{1,3})?\s*(?:[—–:-]\s*|$)`)

// appendixTitle letters appendices by position (A, B, …) so lettering is always
// consistent (#75). Mirrors preprocessor.py's _appendix_title.
func appendixTitle(index int, title string) string {
	name := appendixPrefixRe.ReplaceAllString(strings.TrimSpace(title), "")
	letter := strconv.Itoa(index + 1)
	if index < 26 {
		letter = string(rune('A' + index))
	}
	if name == "" {
		return "Appendix " + letter
	}
	return "Appendix " + letter + " — " + name
}

// ── Assembly ──────────────────────────────────────────────────────────────────

func readFrontmatterAndBody(path string) (map[string]any, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("reading %s: %w", path, err)
	}
	meta := ParseFrontmatter(string(raw))
	_, body := SplitFrontmatter(string(raw))
	return meta, body, nil
}

func timelineEntriesOf(meta map[string]any) []map[string]any {
	raw, _ := meta["timeline"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Assemble assembles the full report markdown and metadata into outputDir.
// Returns (combinedMdPath, metadataYamlPath).
func Assemble(structure Structure, outputDir string) (string, string, error) {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", outputDir, err)
	}

	assetsSrc := filepath.Join(structure.BaseDir, "assets")
	if _, err := os.Stat(assetsSrc); err == nil {
		if err := copyDir(assetsSrc, filepath.Join(outputDir, "assets")); err != nil {
			return "", "", fmt.Errorf("copying assets: %w", err)
		}
	}

	var timelineEntries []map[string]any
	timelineEntries = append(timelineEntries, timelineEntriesOf(structure.Metadata)...)

	var findings []finding
	for _, path := range structure.Reports {
		meta, _, err := readFrontmatterAndBody(path)
		if err != nil {
			return "", "", err
		}
		meta = enrich(meta)
		timelineEntries = append(timelineEntries, timelineEntriesOf(meta)...)
		findings = append(findings, finding{path: path, meta: meta})
	}
	sortFindings(findings)

	registry := map[string]findingRef{}
	findingsData := make([]map[string]any, 0, len(findings))
	for i, f := range findings {
		n := i + 1
		anchor := fmt.Sprintf("finding-%d", n)
		title := toStr(f.meta["title"])
		if title == "" {
			title = "Untitled"
		}
		stem := strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))
		ref := findingRef{n: n, title: title, anchor: anchor}
		registry[stem] = ref
		registry[filepath.Base(f.path)] = ref
		findingsData = append(findingsData, f.meta)
	}

	stats := computeStats(findingsData)
	assetsData := loadJSONArtifact(structure.BaseDir, "assets.json")
	webfindingsData := loadJSONArtifact(structure.BaseDir, "webfindings.json")

	var networkErr error // applyNetwork's error, surfaced once assembly finishes
	renderBody := func(body, path string, demoteHeadings bool) string {
		body = separateImages(stripQADirectives(body))
		body = rebasePaths(body, path, structure.BaseDir)
		if demoteHeadings {
			body = demote(body, 1)
		}
		body = applyCharts(body, findingsData)
		body = applyStats(body, stats)
		body = applyAssetCharts(body, assetsData)
		body = applyWebCharts(body, webfindingsData)
		if b, err := applyNetwork(body, structure.BaseDir); err != nil {
			networkErr = err
		} else {
			body = b
		}
		return transform(body, registry)
	}

	var chunks []string

	// --- Sections ---
	for _, path := range structure.Sections {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", "", fmt.Errorf("reading %s: %w", path, err)
		}
		meta := ParseFrontmatter(string(raw))
		timelineEntries = append(timelineEntries, timelineEntriesOf(meta)...)
		_, body := SplitFrontmatter(string(raw))
		if title := toStr(meta["title"]); title != "" {
			chunks = append(chunks, fmt.Sprintf("# %s\n\n", title))
		}
		chunks = append(chunks, strings.TrimSpace(renderBody(body, path, false)))
		chunks = append(chunks, "\n\n\\clearpage\n\n")
	}

	// --- Vulnerability Findings ---
	if len(findings) > 0 {
		chunks = append(chunks, "# Vulnerability Findings\n\n")
		for i, f := range findings {
			n := i + 1
			raw, err := os.ReadFile(f.path)
			if err != nil {
				return "", "", fmt.Errorf("reading %s: %w", f.path, err)
			}
			_, body := SplitFrontmatter(string(raw))
			title := toStr(f.meta["title"])
			if title == "" {
				title = "Untitled"
			}
			anchor := fmt.Sprintf("finding-%d", n)
			f.meta["finding_number"] = n
			chunks = append(chunks, fmt.Sprintf("## %s {#%s}\n\n", title, anchor))
			chunks = append(chunks, vulnDiv(f.meta))
			chunks = append(chunks, "\n\n")
			chunks = append(chunks, strings.TrimSpace(renderBody(body, f.path, true)))
			chunks = append(chunks, "\n\n\\clearpage\n\n")
		}
	}

	// --- Appendices ---
	for _, path := range structure.Appendices {
		meta, _, err := readFrontmatterAndBody(path)
		if err != nil {
			return "", "", err
		}
		timelineEntries = append(timelineEntries, timelineEntriesOf(meta)...)
	}
	timelineTable, err := buildTimelineTable(timelineEntries)
	if err != nil {
		return "", "", err
	}

	if len(structure.Appendices) > 0 || timelineTable != "" {
		chunks = append(chunks, "# Appendices\n\n")
		for i, path := range structure.Appendices {
			raw, err := os.ReadFile(path)
			if err != nil {
				return "", "", fmt.Errorf("reading %s: %w", path, err)
			}
			meta := ParseFrontmatter(string(raw))
			_, body := SplitFrontmatter(string(raw))
			chunks = append(chunks, fmt.Sprintf("## %s\n\n", appendixTitle(i, toStr(meta["title"]))))
			// Demoted, like finding bodies, so an appendix's own `## Scope` nests
			// under its title instead of sitting beside it (#75).
			chunks = append(chunks, strings.TrimSpace(renderBody(body, path, true)))
			chunks = append(chunks, "\n\n")
		}
		if timelineTable != "" {
			chunks = append(chunks, fmt.Sprintf("## %s\n\n", appendixTitle(len(structure.Appendices), "Timeline")))
			chunks = append(chunks, timelineTable)
			chunks = append(chunks, "\n\n")
		}
	}

	if networkErr != nil {
		return "", "", networkErr
	}

	combinedPath := filepath.Join(outputDir, "combined.md")
	if err := os.WriteFile(combinedPath, []byte(strings.Join(chunks, "\n")), 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", combinedPath, err)
	}

	outMeta := map[string]any{}
	for k, v := range structure.Metadata {
		outMeta[k] = v
	}
	if _, ok := outMeta["generator"]; !ok {
		outMeta["generator"] = "Report Rex v" + version.Version
	}
	outMeta["stats"] = stats

	metaBytes, err := yaml.Marshal(outMeta)
	if err != nil {
		return "", "", fmt.Errorf("marshaling metadata: %w", err)
	}
	metadataPath := filepath.Join(outputDir, "metadata.yaml")
	if err := os.WriteFile(metadataPath, metaBytes, 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", metadataPath, err)
	}

	return combinedPath, metadataPath, nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// statTokenRe matches inline `:::findings:total:::` / `:::findings:high:::` /
// `:::findings:highest:::` tokens, so summary prose stays in step with the
// findings (#72). Mirrors preprocessor.py's _apply_stats.
var statTokenRe = regexp.MustCompile(`:::findings:(total|critical|high|medium|low|info|highest):::`)

func applyStats(body string, stats map[string]any) string {
	highest := "None"
	for _, sev := range []string{"Critical", "High", "Medium", "Low", "Info"} {
		if stats[strings.ToLower(sev)].(int) > 0 {
			highest = sev
			break
		}
	}
	return statTokenRe.ReplaceAllStringFunc(body, func(tok string) string {
		key := statTokenRe.FindStringSubmatch(tok)[1]
		if key == "highest" {
			return highest
		}
		return strconv.Itoa(stats[key].(int))
	})
}

func computeStats(findingsData []map[string]any) map[string]any {
	counts := map[string]int{"Critical": 0, "High": 0, "Medium": 0, "Low": 0, "Info": 0}
	for _, f := range findingsData {
		sev := toStr(f["severity"])
		if sev == "" || sev == "None" {
			sev = "Info"
		}
		counts[sev]++
	}
	return map[string]any{
		"total":    len(findingsData),
		"critical": counts["Critical"],
		"high":     counts["High"],
		"medium":   counts["Medium"],
		"low":      counts["Low"],
		"info":     counts["Info"],
	}
}

func parseISODate(raw string) (time.Time, error) {
	raw = strings.ReplaceAll(raw, "Z", "+00:00")
	layouts := []string{
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timeline date %q", raw)
}

// buildTimelineTable renders included timeline entries, sorted chronologically,
// as a pandoc markdown table. Entries may come from index.md or any
// section/report/appendix file's own frontmatter; `include` defaults to true.
func buildTimelineTable(entries []map[string]any) (string, error) {
	type row struct {
		when time.Time
		desc string
	}
	var rows []row
	for _, entry := range entries {
		if include, ok := entry["include"].(bool); ok && !include {
			continue
		}
		rawDate := strings.TrimSpace(toStr(entry["date"]))
		parsed, err := parseISODate(rawDate)
		if err != nil {
			return "", fmt.Errorf("invalid timeline date %q: %w", rawDate, err)
		}
		desc := strings.ReplaceAll(toStr(entry["description"]), "|", `\|`)
		rows = append(rows, row{when: parsed, desc: desc})
	}
	if len(rows) == 0 {
		return "", nil
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].when.Before(rows[j].when) })

	lines := []string{"| Date | Description |", "|------|-------------|"}
	for _, r := range rows {
		// A 1970-01-01 date is the scaffold's unset placeholder (validate flags it
		// as placeholder_text); leave the cell blank rather than print the epoch (#73).
		when := r.when.Format("2006-01-02 15:04")
		if r.when.Format("2006-01-02") == PlaceholderEpochDate {
			when = ""
		}
		lines = append(lines, fmt.Sprintf("| %s | %s |", when, r.desc))
	}
	return strings.Join(lines, "\n"), nil
}

// vulnDiv emits a pandoc fenced div that the Lua filter converts to a styled box.
func vulnDiv(meta map[string]any) string {
	safe := func(v string) string {
		v = strings.ReplaceAll(v, `"`, "'")
		v = strings.ReplaceAll(v, "{", "(")
		v = strings.ReplaceAll(v, "}", ")")
		return v
	}

	var affectedRaw string
	if affected, ok := meta["affected_urls"].([]any); ok {
		parts := make([]string, len(affected))
		for i, a := range affected {
			parts[i] = toStr(a)
		}
		affectedRaw = strings.Join(parts, ", ")
	} else {
		affectedRaw = toStr(meta["affected_urls"])
	}

	labelJoin := func(field string) string {
		codes := SplitCodes(meta[field])
		labels := make([]string, len(codes))
		for i, c := range codes {
			labels[i] = Label(c)
		}
		return strings.Join(labels, " ; ")
	}

	owasp := labelJoin("owasp")
	mitre := labelJoin("mitre")
	mitigations := strings.Join(SplitCodes(meta["mitigations"]), ", ")
	business := strings.Join(SplitCodes(meta["business_risk"]), ", ")

	findingID := ""
	if n, ok := meta["finding_number"].(int); ok && n != 0 {
		findingID = fmt.Sprintf("F-%02d", n)
	}

	severity := toStr(meta["severity"])
	if severity == "" {
		severity = "Info"
	}
	status := toStr(meta["status"])
	if status == "" {
		status = "Open"
	}

	fields := []struct{ key, value string }{
		{"severity", severity},
		{"finding", findingID},
		{"impact", toStr(meta["impact"])},
		{"sophistication", toStr(meta["sophistication"])},
		{"cvss", toStr(meta["cvss_score"])},
		{"vector", toStr(meta["cvss_vector"])},
		{"cvssversion", toStr(meta["cvss_version"])},
		{"status", status},
		{"affected", affectedRaw},
		{"cwe", toStr(meta["cwe"])},
		{"owasp", owasp},
		{"mitre", mitre},
		{"mitigations", mitigations},
		{"material", toStr(meta["material_impact"])},
		{"business", business},
		{"component", toStr(meta["component"])},
	}

	var attrs []string
	for _, f := range fields {
		if strings.TrimSpace(f.value) != "" {
			attrs = append(attrs, fmt.Sprintf(`%s="%s"`, f.key, safe(f.value)))
		}
	}
	return fmt.Sprintf("::: {.vulnerability %s}\n:::", strings.Join(attrs, " "))
}
