package report

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── vulnDiv ──────────────────────────────────────────────────────────────────

func TestVulnDivEmitsFencedDiv(t *testing.T) {
	meta := map[string]any{
		"title": "SQL Injection", "severity": "Critical", "cvss_score": 9.8,
		"status": "Open", "affected_urls": []any{"https://example.com"},
	}
	result := vulnDiv(meta)
	if !strings.Contains(result, "::: {.vulnerability") {
		t.Errorf("missing fenced div open: %s", result)
	}
	if !strings.Contains(result, `severity="Critical"`) {
		t.Errorf("missing severity: %s", result)
	}
	if !strings.Contains(result, `cvss="9.8"`) {
		t.Errorf("missing cvss: %s", result)
	}
	if !strings.Contains(result, `affected="https://example.com"`) {
		t.Errorf("missing affected: %s", result)
	}
	if strings.Contains(result, "title=") {
		t.Errorf("title should not be in the div: %s", result)
	}
}

func TestVulnDivDefaultsForMissingFields(t *testing.T) {
	result := vulnDiv(map[string]any{})
	if !strings.Contains(result, `severity="Info"`) || !strings.Contains(result, `status="Open"`) {
		t.Errorf("got %s", result)
	}
}

func TestVulnDivEmitsReferenceFields(t *testing.T) {
	meta := map[string]any{
		"severity": "High", "cwe": "CWE-79", "owasp": "A03:2021",
		"mitre": "T1059", "component": "Profile",
	}
	result := vulnDiv(meta)
	if !strings.Contains(result, `cwe="CWE-79"`) {
		t.Errorf("got %s", result)
	}
	if !strings.Contains(result, "A03:2021") || !strings.Contains(result, "Injection") {
		t.Errorf("owasp label not resolved: %s", result)
	}
	if !strings.Contains(result, "T1059") {
		t.Errorf("got %s", result)
	}
}

func TestVulnDivSanitizesBracesInAffectedURLs(t *testing.T) {
	meta := map[string]any{"severity": "High", "affected_urls": []any{"https://example.com/api/{id}"}}
	result := vulnDiv(meta)
	if !strings.Contains(result, `affected="https://example.com/api/(id)"`) {
		t.Errorf("got %s", result)
	}
}

func TestVulnDivEmitsEvaluationAndMaterialFields(t *testing.T) {
	meta := map[string]any{
		"severity": "Critical", "impact": "Critical", "sophistication": "Low",
		"material_impact": "Proprietary Data Loss", "mitigations": "M1032, M1038",
	}
	result := vulnDiv(meta)
	for _, want := range []string{`impact="Critical"`, `sophistication="Low"`, `material="Proprietary Data Loss"`, `mitigations="M1032, M1038"`} {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in %s", want, result)
		}
	}
}

func TestVulnDivEmitsFindingIDFromNumber(t *testing.T) {
	result := vulnDiv(map[string]any{"severity": "High", "finding_number": 3})
	if !strings.Contains(result, `finding="F-03"`) {
		t.Errorf("got %s", result)
	}
}

func TestVulnDivOmitsOptionalFieldsWhenAbsent(t *testing.T) {
	result := vulnDiv(map[string]any{"severity": "Low"})
	for _, absent := range []string{"impact=", "sophistication=", "material=", "mitigations=", "finding="} {
		if strings.Contains(result, absent) {
			t.Errorf("should not contain %q: %s", absent, result)
		}
	}
}

// ── timeline table ───────────────────────────────────────────────────────────

func TestTimelineTableIncludedAndOmittedEntriesAppear(t *testing.T) {
	entries := []map[string]any{
		{"date": "2024-01-01T00:00:00+00:00", "description": "A"},
		{"date": "2024-01-02T00:00:00+00:00", "description": "B", "include": true},
	}
	result, err := buildTimelineTable(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "A") || !strings.Contains(result, "B") {
		t.Errorf("got %s", result)
	}
}

func TestTimelineTableExcludedEntriesAreDropped(t *testing.T) {
	entries := []map[string]any{
		{"date": "2024-01-01T00:00:00+00:00", "description": "Keep", "include": true},
		{"date": "2024-01-02T00:00:00+00:00", "description": "Drop", "include": false},
	}
	result, err := buildTimelineTable(entries)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Keep") {
		t.Errorf("missing Keep: %s", result)
	}
	if strings.Contains(result, "Drop") {
		t.Errorf("should not contain Drop: %s", result)
	}
}

func TestTimelineTableSortedChronologically(t *testing.T) {
	entries := []map[string]any{
		{"date": "2024-01-03T00:00:00+00:00", "description": "Third"},
		{"date": "2024-01-01T00:00:00+00:00", "description": "First"},
		{"date": "2024-01-02T00:00:00+00:00", "description": "Second"},
	}
	result, err := buildTimelineTable(entries)
	if err != nil {
		t.Fatal(err)
	}
	iFirst := strings.Index(result, "First")
	iSecond := strings.Index(result, "Second")
	iThird := strings.Index(result, "Third")
	if !(iFirst < iSecond && iSecond < iThird) {
		t.Errorf("not sorted chronologically: %s", result)
	}
}

func TestTimelineTableEmptyInputReturnsEmptyString(t *testing.T) {
	result, err := buildTimelineTable(nil)
	if err != nil || result != "" {
		t.Errorf("got %q, %v", result, err)
	}
}

func TestTimelineTableAllExcludedReturnsEmptyString(t *testing.T) {
	entries := []map[string]any{{"date": "2024-01-01T00:00:00+00:00", "description": "x", "include": false}}
	result, err := buildTimelineTable(entries)
	if err != nil || result != "" {
		t.Errorf("got %q, %v", result, err)
	}
}

func TestTimelineTableMalformedDateErrors(t *testing.T) {
	if _, err := buildTimelineTable([]map[string]any{{"date": "not-a-date", "description": "x"}}); err == nil {
		t.Error("expected error for malformed date")
	}
}

func TestTimelineTableEpochPlaceholderDateRendersBlank(t *testing.T) {
	result, err := buildTimelineTable([]map[string]any{{"date": "1970-01-01T10:15:00+00:00", "description": "Recon"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result, "1970") || !strings.Contains(result, "|  | Recon |") {
		t.Errorf("epoch date not blanked (#73): %s", result)
	}
}

// ── sortFindings ─────────────────────────────────────────────────────────────

func TestSortFindingsByScoreWithinSeverity(t *testing.T) {
	findings := []finding{
		{meta: map[string]any{"title": "A", "severity": "High", "cvss_score": "7.7"}},
		{meta: map[string]any{"title": "Z", "severity": "High", "cvss_score": "8.0"}},
		{meta: map[string]any{"title": "M", "severity": "Medium", "cvss_score": "n/a"}},
		{meta: map[string]any{"title": "C", "severity": "Critical", "cvss_score": "9.0"}},
		{meta: map[string]any{"title": "B", "severity": "Medium", "cvss_score": "5.0"}},
	}
	sortFindings(findings)
	var got []string
	for _, f := range findings {
		got = append(got, toStr(f.meta["title"]))
	}
	if strings.Join(got, ",") != "C,Z,A,B,M" {
		t.Errorf("order = %v, want C,Z,A,B,M (#67)", got)
	}
}

// ── separateImages ───────────────────────────────────────────────────────────

func TestSeparateImages(t *testing.T) {
	cases := map[string]string{
		"![a](a.png)\n![b](b.png){width=90%}":      "![a](a.png)\n\n![b](b.png){width=90%}",
		"![a](a.png)\n\n![b](b.png)":               "![a](a.png)\n\n![b](b.png)",
		"```\n![a](a.png)\n![b](b.png)\n```":       "```\n![a](a.png)\n![b](b.png)\n```",
		"Evidence\n![a](a.png)\n{width=90%}\nNext": "Evidence\n\n![a](a.png){width=90%}\n\nNext",
		"![a](a.png) {width=90%}":                  "![a](a.png){width=90%}",
		"![a](a.png)\n{width=90%} Next":            "![a](a.png){width=90%}\n\nNext",
		"![a](a.png) {.wide #fig}":                 "![a](a.png){.wide #fig}",
		"![a](a.png)\n{not attributes}":            "![a](a.png)\n\n{not attributes}",
		"```\n![a](a.png)\n{width=90%}\n```":       "```\n![a](a.png)\n{width=90%}\n```",
	}
	for in, want := range cases {
		if got := separateImages(in); got != want {
			t.Errorf("separateImages(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── enrich ───────────────────────────────────────────────────────────────────

func TestEnrichComputesSeverityAndScoreFromVector(t *testing.T) {
	meta := enrich(map[string]any{"cvss_vector": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"})
	if meta["severity"] != "Critical" || meta["cvss_score"] != "9.8" || meta["cvss_version"] != "3.1" {
		t.Errorf("got %v", meta)
	}
}

func TestEnrichFallsBackToManualSeverityWithoutVector(t *testing.T) {
	meta := enrich(map[string]any{"severity": "Medium", "cvss_score": 5.0})
	if meta["severity"] != "Medium" || meta["cvss_score"] != "5.0" {
		t.Errorf("got %v", meta)
	}
}

// ── transform ────────────────────────────────────────────────────────────────

func TestTransformExternalLinkBecomesFootnote(t *testing.T) {
	out := transform("See [the docs](https://example.com/x).", nil)
	if !strings.Contains(out, "the docs^[<https://example.com/x>]") {
		t.Errorf("got %s", out)
	}
}

func TestTransformDoubleBangLinkStaysInline(t *testing.T) {
	out := transform("See !![the docs](https://example.com/x).", nil)
	if !strings.Contains(out, "[the docs](https://example.com/x)") {
		t.Errorf("got %s", out)
	}
	if strings.Contains(out, "^[") {
		t.Errorf("should not contain footnote marker: %s", out)
	}
}

func TestTransformLinkInTableRowStaysInline(t *testing.T) {
	// A footnote (`^[...]`) inside a pandoc-generated `longtable` silently
	// loses its text in the rendered PDF, so table-row links must stay
	// plain inline links instead (issue #27).
	out := transform("| [Burp Suite](https://portswigger.net/burp) | scanner |", nil)
	if !strings.Contains(out, "[Burp Suite](https://portswigger.net/burp)") {
		t.Errorf("got %s", out)
	}
	if strings.Contains(out, "^[") {
		t.Errorf("should not contain footnote marker: %s", out)
	}
}

func TestTransformCrossReferenceResolved(t *testing.T) {
	registry := map[string]findingRef{"vuln-2": {n: 2, title: "Stored XSS", anchor: "finding-2"}}
	out := transform("Amplifies [the XSS](./vuln-2.md).", registry)
	if !strings.Contains(out, `Finding \#2 — Stored XSS`) {
		t.Errorf("got %s", out)
	}
	if !strings.Contains(out, "(#finding-2)") {
		t.Errorf("got %s", out)
	}
}

func TestTransformImageCrossReferenceResolved(t *testing.T) {
	registry := map[string]findingRef{"vuln-1": {n: 1, title: "SQLi", anchor: "finding-1"}}
	out := transform("![](./vuln-1.md)", registry)
	if !strings.Contains(out, `Finding \#1 — SQLi`) {
		t.Errorf("got %s", out)
	}
}

// ── assemble ─────────────────────────────────────────────────────────────────

func buildSampleReportDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "sections"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "reports"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "appendices"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "assets", "screenshots"), 0o755))

	must(os.WriteFile(filepath.Join(dir, "sections", "exec_summ.md"), []byte(
		"---\ntitle: Executive Summary\n---\n\nSummary text.\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "reports", "vuln-1.md"), []byte(
		"---\ntitle: SQL Injection\nseverity: Critical\ncvss_score: 9.8\n"+
			"status: Open\naffected_urls:\n  - https://example.com/login\n---\n\n"+
			"## Description\n\nSQL injection found.\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "appendices", "1.md"), []byte(
		"---\ntitle: Appendix A\n---\n\nScope list.\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "index.md"), []byte(
		"---\n"+
			"title: Test Report\n"+
			"client: ACME Corp\n"+
			"template: rex-default\n"+
			"structure:\n"+
			"  sections:\n"+
			"    - sections/exec_summ.md\n"+
			"  reports:\n"+
			"    - reports/vuln-1.md\n"+
			"  appendices:\n"+
			"    - appendices/1.md\n"+
			"---\n"), 0o644))
	return dir
}

func TestAssembleCreatesCombinedMd(t *testing.T) {
	dir := buildSampleReportDir(t)
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	combined, metadata, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(combined); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(metadata); err != nil {
		t.Error(err)
	}
}

func TestAssembleCombinedContainsSectionContent(t *testing.T) {
	dir := buildSampleReportDir(t)
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	combinedPath, _, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(combinedPath)
	if !strings.Contains(string(content), "Summary text.") || !strings.Contains(string(content), "Executive Summary") {
		t.Errorf("got %s", content)
	}
}

func TestAssembleCombinedContainsVulnDivAndHeading(t *testing.T) {
	dir := buildSampleReportDir(t)
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	combinedPath, _, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(combinedPath)
	s := string(content)
	if !strings.Contains(s, ".vulnerability") || !strings.Contains(s, `severity="Critical"`) {
		t.Errorf("got %s", s)
	}
	if !strings.Contains(s, "## SQL Injection {#finding-1}") {
		t.Errorf("got %s", s)
	}
}

func TestAssembleCombinedContainsAppendix(t *testing.T) {
	dir := buildSampleReportDir(t)
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	combinedPath, _, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(combinedPath)
	if !strings.Contains(string(content), "Appendix A") || !strings.Contains(string(content), "Scope list.") {
		t.Errorf("got %s", content)
	}
}

func TestAssembleMetadataHasReportMetaAndStats(t *testing.T) {
	dir := buildSampleReportDir(t)
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, metadataPath, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(metadataPath)
	meta := ParseFrontmatter("---\n" + string(raw) + "---\n")
	if meta["title"] != "Test Report" || meta["client"] != "ACME Corp" {
		t.Errorf("got %v", meta)
	}
	stats, _ := meta["stats"].(map[string]any)
	if stats["total"] != 1 || stats["critical"] != 1 {
		t.Errorf("got stats %v", stats)
	}
	if gen, _ := meta["generator"].(string); !strings.HasPrefix(gen, "Report Rex v") {
		t.Errorf("got generator %q", meta["generator"])
	}
}

func TestAssembleKeepsLogoPathsRelative(t *testing.T) {
	// Regression test: logo paths must stay relative to the assemble dir
	// (where Assemble copies assets/) rather than being rewritten to the
	// original report dir's absolute host path — the Docker render step
	// only mounts the assemble dir, not the original report dir, so an
	// absolutized host path would be unresolvable inside the container.
	dir := buildSampleReportDir(t)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "assets", "company-logo.png"), []byte("fake-png"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "index.md"), []byte(
		"---\n"+
			"title: Test Report\n"+
			"client: ACME Corp\n"+
			"template: rex-default\n"+
			"company_logo: assets/company-logo.png\n"+
			"structure:\n"+
			"  sections:\n"+
			"    - sections/exec_summ.md\n"+
			"  reports:\n"+
			"    - reports/vuln-1.md\n"+
			"  appendices:\n"+
			"    - appendices/1.md\n"+
			"---\n"), 0o644))

	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	_, metadataPath, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(metadataPath)
	meta := ParseFrontmatter("---\n" + string(raw) + "---\n")
	if meta["company_logo"] != "assets/company-logo.png" {
		t.Errorf("got company_logo %v, want unchanged relative path", meta["company_logo"])
	}
}

func TestAssembleDarkOverrideMarkerIsStrippedNotRendered(t *testing.T) {
	dir := buildSampleReportDir(t)
	vulnPath := filepath.Join(dir, "reports", "vuln-1.md")
	content, _ := os.ReadFile(vulnPath)
	updated := strings.Replace(string(content),
		"## Description\n\nSQL injection found.\n",
		"## Description\n\nSQL injection found.\n\n"+
			"> [!OVERRIDE-DARK-IMG]\n"+
			"![A screenshot](../assets/screenshots/x.png)\n", 1)
	if err := os.WriteFile(vulnPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	combinedPath, _, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	rendered, _ := os.ReadFile(combinedPath)
	if strings.Contains(string(rendered), "OVERRIDE-DARK-IMG") {
		t.Errorf("marker should have been stripped: %s", rendered)
	}
}

func TestAssembleAssetsCopied(t *testing.T) {
	dir := buildSampleReportDir(t)
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(t.TempDir(), "out")
	if _, _, err := Assemble(structure, outDir); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(outDir, "assets")); err != nil || !info.IsDir() {
		t.Errorf("assets dir missing: %v", err)
	}
}

func TestAssembleTimelineAbsentWhenNotDefined(t *testing.T) {
	dir := buildSampleReportDir(t)
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	combinedPath, _, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(combinedPath)
	if strings.Contains(string(content), "— Timeline") {
		t.Errorf("should not contain Timeline section: %s", content)
	}
}

func TestAssembleTimelineMergedFromIndexAndContentFiles(t *testing.T) {
	dir := buildSampleReportDir(t)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "index.md"), []byte(
		"---\n"+
			"title: Test Report\n"+
			"client: ACME Corp\n"+
			"template: rex-default\n"+
			"timeline:\n"+
			`  - date: "2024-01-01T00:00:00+00:00"`+"\n"+
			`    description: "From index"`+"\n"+
			"structure:\n"+
			"  sections:\n"+
			"    - sections/exec_summ.md\n"+
			"  reports:\n"+
			"    - reports/vuln-1.md\n"+
			"  appendices:\n"+
			"    - appendices/1.md\n"+
			"---\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "reports", "vuln-1.md"), []byte(
		"---\ntitle: SQL Injection\nseverity: Critical\ncvss_score: 9.8\n"+
			"status: Open\naffected_urls:\n  - https://example.com/login\n"+
			"timeline:\n"+
			`  - date: "2024-01-02T00:00:00+00:00"`+"\n"+
			`    description: "From vuln file"`+"\n"+
			"    include: true\n"+
			`  - date: "2024-01-03T00:00:00+00:00"`+"\n"+
			`    description: "Excluded note"`+"\n"+
			"    include: false\n"+
			"---\n\n## Description\n\nSQL injection found.\n"), 0o644))

	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	combinedPath, _, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(combinedPath)
	s := string(content)
	if !strings.Contains(s, "— Timeline") {
		t.Errorf("missing Timeline section: %s", s)
	}
	if !strings.Contains(s, "From index") || !strings.Contains(s, "From vuln file") {
		t.Errorf("got %s", s)
	}
	if strings.Contains(s, "Excluded note") {
		t.Errorf("excluded note should be dropped: %s", s)
	}
}

func TestAssembleNetworkDiagram(t *testing.T) {
	dir := buildSampleReportDir(t)
	appendix := filepath.Join(dir, "appendices", "1.md")
	if err := os.WriteFile(appendix, []byte("---\ntitle: Appendix A\n---\n\n:::network_diagram:::\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Assemble(structure, filepath.Join(t.TempDir(), "out")); err == nil || !strings.Contains(err.Error(), "assets.json") {
		t.Fatalf("expected missing assets.json error, got %v", err)
	}

	artifacts := filepath.Join(dir, "artifacts")
	if err := os.MkdirAll(artifacts, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(artifacts, "assets.json"), []byte(`[{"ip": "10.0.0.1", "os": "Linux"}]`), 0o644)
	os.WriteFile(filepath.Join(artifacts, "connections.json"), []byte(`{"subnets": {"10.0.0.0/24": "Corp"}, "pivots": {"10.0.0.1": "10.9.9.0/24"}}`), 0o644)
	combinedPath, _, err := Assemble(structure, filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(combinedPath)
	if strings.Contains(string(content), "network_diagram") || !strings.Contains(string(content), "colorCritical") {
		t.Errorf("network diagram not rendered:\n%s", content)
	}
}

func TestAppendixTitleLettersByPosition(t *testing.T) {
	// #75: "Appendix A", "Appendix B", then an unlettered ingest appendix.
	cases := []struct {
		index       int
		title, want string
	}{
		{0, "Appendix A — Scope and Rules of Engagement", "Appendix A — Scope and Rules of Engagement"},
		{2, "Appendix — Discovered Assets", "Appendix C — Discovered Assets"},
		{1, "Appendix 2: Tools", "Appendix B — Tools"},
		{1, "appendix b - Coverage", "Appendix B — Coverage"},
		{3, "Glossary", "Appendix D — Glossary"},
		{0, "", "Appendix A"},
		{0, "Appendix", "Appendix A"},
		{0, "Appendixes of note", "Appendix A — Appendixes of note"},
	}
	for _, c := range cases {
		if got := appendixTitle(c.index, c.title); got != c.want {
			t.Errorf("appendixTitle(%d, %q) = %q, want %q", c.index, c.title, got, c.want)
		}
	}
}

func TestApplyStatsTokens(t *testing.T) {
	// #72: summary prose derived from the findings, not hard-coded.
	stats := map[string]any{"total": 3, "critical": 0, "high": 3, "medium": 0, "low": 0, "info": 0}
	body := "Found :::findings:total::: issues (:::findings:high::: High); top: :::findings:highest:::."
	if got := applyStats(body, stats); got != "Found 3 issues (3 High); top: High." {
		t.Errorf("got %q", got)
	}
	zero := map[string]any{"total": 0, "critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0}
	if got := applyStats(":::findings:highest:::", zero); got != "None" {
		t.Errorf("got %q", got)
	}
	if got := applyStats(":::findings:bogus:::", stats); got != ":::findings:bogus:::" {
		t.Errorf("got %q", got)
	}
}
