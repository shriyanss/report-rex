package section

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/shriyanss/report-rex/cli/internal/embedded"
	"github.com/shriyanss/report-rex/cli/internal/report"
)

// ErrNotAIOwned is returned by UpdateVuln when the target finding's
// frontmatter doesn't already carry ai: true and allowOverride wasn't set —
// it's what stops an AI-driven caller from silently rewriting a
// human-authored finding.
var ErrNotAIOwned = errors.New("finding is not AI-owned (missing ai: true); pass allowOverride to change it anyway")

var vulnNumRe = regexp.MustCompile(`^vuln-(\d+)\.md$`)

// curlPoCRe matches a one-liner `curl` command, optionally preceded by a
// shell prompt (`$ `).
var curlPoCRe = regexp.MustCompile(`(?m)^\s*\$?\s*curl\s+\S`)

// requireCurlPoC returns an error if body has no one-liner `curl` PoC
// command — every AI-created/updated finding needs one so a human triager
// can reproduce it immediately instead of re-deriving repro steps from prose.
func requireCurlPoC(body string) error {
	if !curlPoCRe.MatchString(body) {
		return fmt.Errorf("AI-authored findings must include a one-liner `curl` PoC command in the body (e.g. under Replication) so a human triager can reproduce it immediately")
	}
	return nil
}

// VulnOptions covers the reports/vuln-N.md frontmatter fields (see
// reporting/example/reports/vuln-1.md). Every field is optional except Title,
// which is required unless Template is set (a template already ships its own
// [!REX-STUB]-marked title placeholder).
type VulnOptions struct {
	Title          string
	Template       string // vuln-templates/<Template>.md, without extension
	File           string // reports/<File>.md; empty means the next vuln-N.md
	CVSSVector     string
	Component      string
	CWE            string
	OWASP          string
	MITRE          string
	Mitigations    string
	Impact         string
	Sophistication string
	MaterialImpact string
	BusinessRisk   []string
	AffectedURLs   []string
	TimelineDate   string
	TimelineDesc   string

	// Body, if set, replaces the fixed Description/Impact/Replication/
	// Remediation skeleton with this markdown verbatim.
	Body string
	// AI marks the finding as AI-authored by stamping `ai: true` into its
	// frontmatter. Only the MCP server sets this.
	AI bool
}

// AddVuln creates a new numbered finding file (reports/vuln-N.md) in dir —
// either from a vuln-templates/<Template>.md stub with the supplied fields
// overlaid onto it, or from scratch when no template is given — and registers
// it under structure.reports in index.md.
func AddVuln(dir string, opts VulnOptions) (relPath, note string, err error) {
	if opts.Title == "" && opts.Template == "" {
		return "", "", fmt.Errorf("--title is required unless --template is given")
	}

	dirAbs, indexPath, err := resolveReportDir(dir)
	if err != nil {
		return "", "", err
	}

	reportsDir := filepath.Join(dirAbs, "reports")
	if opts.File != "" {
		name := strings.TrimSuffix(opts.File, ".md")
		if name == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
			return "", "", fmt.Errorf("invalid --file %q: use a plain name like ssrf-webhook", opts.File)
		}
		relPath = "reports/" + name + ".md"
		if _, err := os.Stat(filepath.Join(dirAbs, relPath)); err == nil {
			return "", "", fmt.Errorf("%s already exists", relPath)
		}
	} else {
		n, err := NextNumbered(reportsDir, vulnNumRe)
		if err != nil {
			return "", "", fmt.Errorf("scanning %s: %w", reportsDir, err)
		}
		relPath = fmt.Sprintf("reports/vuln-%d.md", n)
	}

	if opts.AI && opts.Body != "" {
		if err := requireCurlPoC(opts.Body); err != nil {
			return "", "", err
		}
	}

	var content string
	if opts.Template != "" {
		content, err = renderFromTemplate(opts)
		if err != nil {
			return "", "", err
		}
	} else {
		content = renderBlankVuln(opts)
	}
	if opts.Body != "" {
		content = replaceBody(content, opts.Body)
	}
	if opts.AI {
		content = stampAI(content)
	}

	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", reportsDir, err)
	}
	if err := os.WriteFile(filepath.Join(dirAbs, relPath), []byte(content), 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", relPath, err)
	}

	note, err = EnsureStructureEntry(indexPath, "reports", relPath)
	if err != nil {
		return "", "", fmt.Errorf("registering %s in index.md: %w", relPath, err)
	}
	return relPath, note, nil
}

// replaceBody swaps content's markdown body (everything after the closing
// frontmatter ---) for body verbatim, keeping the frontmatter untouched.
func replaceBody(content, body string) string {
	fm, _ := report.SplitFrontmatter(content)
	return "---\n" + fm + "\n---\n\n" + strings.TrimSpace(body) + "\n"
}

// stampAI inserts `ai: true` as the first frontmatter line.
func stampAI(content string) string {
	const marker = "---\n"
	if !strings.HasPrefix(content, marker) {
		return content
	}
	return marker + "ai: true\n" + content[len(marker):]
}

// UpdateVuln patches an existing finding's frontmatter/body in place. Only
// the fields set on opts are changed — everything else in the finding's
// current frontmatter is left as-is. Every update re-stamps ai: true, so
// once a finding has been touched by AI it stays flagged.
//
// If the finding isn't already AI-owned (ai: true), UpdateVuln refuses with
// ErrNotAIOwned unless allowOverride is set.
func UpdateVuln(dir, relPath string, opts VulnOptions, allowOverride bool) (note string, err error) {
	dirAbs, _, err := resolveReportDir(dir)
	if err != nil {
		return "", err
	}

	path := filepath.Join(dirAbs, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", relPath, err)
	}

	_, body := report.SplitFrontmatter(string(data))
	meta := report.ParseFrontmatter(string(data))

	if aiOwned, _ := meta["ai"].(bool); !aiOwned && !allowOverride {
		return "", fmt.Errorf("%s: %w", relPath, ErrNotAIOwned)
	}

	if opts.Body != "" {
		if err := requireCurlPoC(opts.Body); err != nil {
			return "", err
		}
	}

	applyVulnFields(meta, opts)
	meta["ai"] = true

	fmBytes, err := yaml.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshaling frontmatter for %s: %w", relPath, err)
	}

	newBody := body
	if opts.Body != "" {
		newBody = strings.TrimSpace(opts.Body)
	}
	content := "---\n" + string(fmBytes) + "---\n\n" + newBody + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", relPath, err)
	}
	return "updated " + relPath, nil
}

// applyVulnFields merges every non-empty field of opts into meta, leaving
// fields opts doesn't set untouched. business_risk/affected_urls are
// replaced wholesale when given; a timeline entry is appended (an engagement
// accumulates timeline events over multiple updates, rather than restating
// its whole history each time).
func applyVulnFields(meta map[string]any, opts VulnOptions) {
	set := func(key, value string) {
		if value != "" {
			meta[key] = value
		}
	}
	set("title", opts.Title)
	set("cvss_vector", opts.CVSSVector)
	set("component", opts.Component)
	set("cwe", opts.CWE)
	set("owasp", opts.OWASP)
	set("mitre", opts.MITRE)
	set("mitigations", opts.Mitigations)
	set("impact", opts.Impact)
	set("sophistication", opts.Sophistication)
	set("material_impact", opts.MaterialImpact)
	if len(opts.BusinessRisk) > 0 {
		meta["business_risk"] = opts.BusinessRisk
	}
	if len(opts.AffectedURLs) > 0 {
		meta["affected_urls"] = opts.AffectedURLs
	}
	if opts.TimelineDate != "" && opts.TimelineDesc != "" {
		timeline, _ := meta["timeline"].([]any)
		meta["timeline"] = append(timeline, map[string]any{
			"date":        opts.TimelineDate,
			"description": opts.TimelineDesc,
			"include":     true,
		})
	}
}

// VulnTemplateNames lists the available vuln-templates/*.md stubs, without
// extension, for --template's error message / shell completion.
func VulnTemplateNames() ([]string, error) {
	entries, err := fs.ReadDir(embedded.VulnTemplatesFS(), ".")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == "README.md" || !strings.HasSuffix(name, ".md") {
			continue
		}
		names = append(names, strings.TrimSuffix(name, ".md"))
	}
	sort.Strings(names)
	return names, nil
}

// VulnTemplateTitle returns the stub title in a vuln-templates/<name>.md
// frontmatter, or "" if the template has none.
func VulnTemplateTitle(name string) string {
	data, err := fs.ReadFile(embedded.VulnTemplatesFS(), name+".md")
	if err != nil {
		return ""
	}
	title, _ := report.ParseFrontmatter(string(data))["title"].(string)
	return title
}

func renderFromTemplate(opts VulnOptions) (string, error) {
	data, err := fs.ReadFile(embedded.VulnTemplatesFS(), opts.Template+".md")
	if err != nil {
		names, _ := VulnTemplateNames()
		return "", fmt.Errorf("unknown vuln template %q (available: %s)", opts.Template, strings.Join(names, ", "))
	}
	lines := strings.Split(string(data), "\n")

	set := func(key, value string) {
		if value != "" {
			lines = setFrontmatterField(lines, key, []string{key + `: "` + value + `"`})
		}
	}
	set("title", opts.Title)
	set("cvss_vector", opts.CVSSVector)
	set("component", opts.Component)
	set("cwe", opts.CWE)
	set("owasp", opts.OWASP)
	set("mitre", opts.MITRE)
	set("mitigations", opts.Mitigations)
	set("impact", opts.Impact)
	set("sophistication", opts.Sophistication)
	set("material_impact", opts.MaterialImpact)

	if len(opts.BusinessRisk) > 0 {
		block := append([]string{"business_risk:"}, indentedList(opts.BusinessRisk, false)...)
		lines = setFrontmatterField(lines, "business_risk", block)
	}
	if len(opts.AffectedURLs) > 0 {
		block := append([]string{"affected_urls:"}, indentedList(opts.AffectedURLs, true)...)
		lines = setFrontmatterField(lines, "affected_urls", block)
	}
	if opts.TimelineDate != "" && opts.TimelineDesc != "" {
		block := []string{
			"timeline:",
			`  - date: "` + opts.TimelineDate + `"`,
			`    description: "` + opts.TimelineDesc + `"`,
			"    include: true",
		}
		lines = setFrontmatterField(lines, "timeline", block)
	}

	return strings.Join(lines, "\n"), nil
}

func indentedList(items []string, quoted bool) []string {
	out := make([]string, len(items))
	for i, item := range items {
		if quoted {
			out[i] = `  - "` + item + `"`
		} else {
			out[i] = "  - " + item
		}
	}
	return out
}

// setFrontmatterField replaces the line-run for a top-level frontmatter key
// (a single "key: value" line, or a "key:" line plus its indented list/map
// continuation lines) with newBlock. If the field is preceded by a "> [!REX-STUB]"
// comment marker, that marker is dropped too, since a real value now fills it.
// If key isn't found, lines is returned unchanged.
func setFrontmatterField(lines []string, key string, newBlock []string) []string {
	start := -1
	for i, line := range lines {
		if line == key+":" || strings.HasPrefix(line, key+": ") {
			start = i
			break
		}
	}
	if start == -1 {
		return lines
	}

	end := start
	for end+1 < len(lines) && len(lines[end+1]) > 0 && lines[end+1][0] == ' ' {
		end++
	}

	if start > 0 && strings.TrimSpace(lines[start-1]) == "# > [!REX-STUB]" {
		start--
	}

	out := make([]string, 0, len(lines)-(end-start+1)+len(newBlock))
	out = append(out, lines[:start]...)
	out = append(out, newBlock...)
	out = append(out, lines[end+1:]...)
	return out
}

func renderBlankVuln(opts VulnOptions) string {
	var fm strings.Builder
	fm.WriteString("---\n")
	fmt.Fprintf(&fm, "title: %q\n", opts.Title)
	writeIf := func(key, value string) {
		if value != "" {
			fmt.Fprintf(&fm, "%s: %q\n", key, value)
		}
	}
	writeIf("cvss_vector", opts.CVSSVector)
	writeIf("component", opts.Component)
	writeIf("cwe", opts.CWE)
	writeIf("owasp", opts.OWASP)
	writeIf("mitre", opts.MITRE)
	writeIf("mitigations", opts.Mitigations)
	writeIf("impact", opts.Impact)
	writeIf("sophistication", opts.Sophistication)
	writeIf("material_impact", opts.MaterialImpact)
	if len(opts.BusinessRisk) > 0 {
		fm.WriteString("business_risk:\n")
		for _, l := range indentedList(opts.BusinessRisk, false) {
			fm.WriteString(l + "\n")
		}
	}
	if len(opts.AffectedURLs) > 0 {
		fm.WriteString("affected_urls:\n")
		for _, l := range indentedList(opts.AffectedURLs, true) {
			fm.WriteString(l + "\n")
		}
	}
	if opts.TimelineDate != "" && opts.TimelineDesc != "" {
		fmt.Fprintf(&fm, "timeline:\n  - date: %q\n    description: %q\n    include: true\n", opts.TimelineDate, opts.TimelineDesc)
	}
	fm.WriteString("---\n")

	fm.WriteString("\n## Description\n\n\n## Impact\n\n\n## Replication\n\n\n## Remediation\n\n")
	return fm.String()
}
