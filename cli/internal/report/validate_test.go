package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const compliantCaption = "A sufficiently long caption describing the evidence shown"

var compliantSteps = "1. Navigate to the login page and open the request in an intercepting proxy.\n" +
	"2. Replace the username parameter with a payload and forward the request.\n" +
	"3. Observe that authentication is bypassed entirely.\n"

// ── Steps to Reproduce ───────────────────────────────────────────────────────

func TestCheckStepsToReproduceMissingSectionIsError(t *testing.T) {
	body := "## Description\n\nSomething happened.\n"
	issues := checkStepsToReproduce("vuln.md", body)
	if len(issues) != 1 || issues[0].Check != "steps_to_reproduce_missing" {
		t.Errorf("got %v", issues)
	}
}

func TestCheckStepsToReproduceUnder100CharsIsError(t *testing.T) {
	body := "## Steps to Reproduce\n\nToo short.\n"
	issues := checkStepsToReproduce("vuln.md", body)
	if len(issues) != 1 || issues[0].Check != "steps_to_reproduce_length" || issues[0].Severity != SeverityError {
		t.Errorf("got %v", issues)
	}
}

func TestCheckStepsToReproduceExactly100CharsPasses(t *testing.T) {
	text := strings.Repeat("x", 100)
	body := "## Steps to Reproduce\n\n" + text + "\n\n## Evidence\n\nSomething else.\n"
	issues := checkStepsToReproduce("vuln.md", body)
	if len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

func TestCheckStepsToReproduceReplicationHeading(t *testing.T) {
	// #68: `## Replication` (steps + evidence) is the scaffolds' heading now.
	if issues := checkStepsToReproduce("v.md", "## Replication\n\n"+compliantSteps+"\n"); len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
	issues := checkStepsToReproduce("v.md", "## Replication\n\nToo short.\n")
	if len(issues) != 1 || issues[0].Check != "steps_to_reproduce_length" || !strings.Contains(issues[0].Message, "'Replication'") {
		t.Errorf("got %v", issues)
	}
}

func TestCheckStepsToReproduceOver100CharsPasses(t *testing.T) {
	body := "## Steps to Reproduce\n\n" + compliantSteps + "\n"
	issues := checkStepsToReproduce("vuln.md", body)
	if len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

// ── Screenshot caption ───────────────────────────────────────────────────────

func writeCompliantScreenshot(t *testing.T, path string) {
	t.Helper()
	makePNG(t, path, withAnnotation(lightBackground), 200, 200)
}

func TestCheckScreenshotsCaptionUnder35CharsIsError(t *testing.T) {
	dir := t.TempDir()
	writeCompliantScreenshot(t, filepath.Join(dir, "assets", "screenshots", "x.png"))
	body := "![Short caption](../assets/screenshots/x.png)\n"
	issues := checkScreenshots(filepath.Join(dir, "reports", "vuln.md"), body, dir)
	if !anyCheck(issues, "screenshot_caption_length") {
		t.Errorf("got %v", issues)
	}
}

func TestCheckScreenshotsCaption35CharsOrMorePasses(t *testing.T) {
	dir := t.TempDir()
	writeCompliantScreenshot(t, filepath.Join(dir, "assets", "screenshots", "x.png"))
	body := "![" + compliantCaption + "](../assets/screenshots/x.png)\n"
	issues := checkScreenshots(filepath.Join(dir, "reports", "vuln.md"), body, dir)
	if anyCheck(issues, "screenshot_caption_length") {
		t.Errorf("got %v", issues)
	}
}

func TestCheckScreenshotsCaptionWithPandocWidthAttrStillParsed(t *testing.T) {
	dir := t.TempDir()
	writeCompliantScreenshot(t, filepath.Join(dir, "assets", "screenshots", "x.png"))
	body := "![" + compliantCaption + "](../assets/screenshots/x.png){width=90%}\n"
	issues := checkScreenshots(filepath.Join(dir, "reports", "vuln.md"), body, dir)
	if anyCheck(issues, "screenshot_caption_length") || anyCheck(issues, "screenshot_missing") {
		t.Errorf("got %v", issues)
	}
}

func TestCheckScreenshotPresent(t *testing.T) {
	if issues := checkScreenshotPresent("vuln.md", "## Description\n\nNo image here.\n"); !anyCheck(issues, "screenshot_none") {
		t.Errorf("got %v", issues)
	}
	if issues := checkScreenshotPresent("vuln.md", "!["+compliantCaption+"](../assets/screenshots/x.png)\n"); len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

func TestCheckRequiredMetadata(t *testing.T) {
	issues := checkRequiredMetadata("index.md", map[string]any{"client": "Acme", "date": ""})
	if len(issues) != 2 || !anyCheck(issues, "metadata_missing") {
		t.Errorf("got %v", issues)
	}
	if issues := checkRequiredMetadata("index.md", map[string]any{"title": "T", "client": "Acme", "date": "1970-01-01"}); len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

// ── CVSS presence ────────────────────────────────────────────────────────────

func TestCheckCvssPresentMissingBothIsError(t *testing.T) {
	issues := checkCvssPresent("vuln.md", map[string]any{})
	if len(issues) != 1 || !strings.Contains(issues[0].Check, "cvss") {
		t.Errorf("got %v", issues)
	}
}

func TestCheckCvssPresentVectorPresentPasses(t *testing.T) {
	issues := checkCvssPresent("vuln.md", map[string]any{"cvss_vector": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"})
	if len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

func TestCheckCvssPresentManualSeverityPasses(t *testing.T) {
	issues := checkCvssPresent("vuln.md", map[string]any{"severity": "High"})
	if len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

func TestCheckCvssPresentManualScorePasses(t *testing.T) {
	issues := checkCvssPresent("vuln.md", map[string]any{"cvss_score": 7.5})
	if len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

// ── Timeline presence ────────────────────────────────────────────────────────

func TestCheckTimelinePresentMissingIsError(t *testing.T) {
	issues := checkTimelinePresent("vuln.md", map[string]any{})
	if len(issues) != 1 || issues[0].Check != "timeline_missing" {
		t.Errorf("got %v", issues)
	}
}

func TestCheckTimelinePresentEmptyListIsError(t *testing.T) {
	issues := checkTimelinePresent("vuln.md", map[string]any{"timeline": []any{}})
	if len(issues) != 1 {
		t.Errorf("got %v", issues)
	}
}

func TestCheckTimelinePresentPresentPasses(t *testing.T) {
	issues := checkTimelinePresent("vuln.md", map[string]any{
		"timeline": []any{map[string]any{"date": "2024-01-01T00:00:00+00:00", "description": "x"}},
	})
	if len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

// ── Placeholder text ─────────────────────────────────────────────────────────

func TestCheckPlaceholderTextEditedReportPasses(t *testing.T) {
	meta := map[string]any{"title": "Real Finding Title"}
	body := "## Description\n\nA real, specific description of the issue.\n"
	issues := checkPlaceholderText("vuln.md", meta, body)
	if len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

func TestCheckPlaceholderTextPartialLeftoverIsCaught(t *testing.T) {
	meta := map[string]any{"title": "Real Finding Title"}
	body := "## Description\n\nA real, specific description.\n\n" +
		"## Recommendation\n\n" + PlaceholderRecommendation + "\n"
	issues := checkPlaceholderText("vuln.md", meta, body)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, PlaceholderRecommendation) {
		t.Errorf("got %v", issues)
	}
}

// TestCheckPlaceholderTextNonTitleMetadataFieldIsCaught proves the haystack
// covers the whole metadata map, not just meta["title"] — this is what makes
// index.md's own frontmatter (client/contact_name/dates, none of which are
// "title") checkable at all.
func TestCheckPlaceholderTextNonTitleMetadataFieldIsCaught(t *testing.T) {
	meta := map[string]any{"title": "Real Finding Title", "contact_email": PlaceholderContactEmail}
	issues := checkPlaceholderText("index.md", meta, "")
	if len(issues) != 1 || !strings.Contains(issues[0].Message, PlaceholderContactEmail) {
		t.Errorf("got %v", issues)
	}
}

func TestCheckPlaceholderTextEpochDateAndLoremIpsumAreRegistered(t *testing.T) {
	meta := map[string]any{"title": "Real Finding Title", "date": PlaceholderEpochDate + "T00:00:00+00:00"}
	body := "Some real text. " + PlaceholderLoremIpsum + " dolor sit amet."
	issues := checkPlaceholderText("index.md", meta, body)
	joined := ""
	for _, i := range issues {
		joined += i.Message
	}
	if !strings.Contains(joined, PlaceholderEpochDate) || !strings.Contains(joined, PlaceholderLoremIpsum) {
		t.Errorf("got %v", issues)
	}
}

func TestCheckPlaceholderTextCoverTemplateDefaults(t *testing.T) {
	// #74: the example's report_id and ACME logo shipped on a real cover.
	meta := map[string]any{"report_id": "RR-2024-0142", "client_logo": "assets/acme-logo.png"}
	joined := ""
	for _, i := range checkPlaceholderText("index.md", meta, "") {
		joined += i.Message
	}
	if !strings.Contains(joined, "RR-2024-0142") || !strings.Contains(joined, PlaceholderClientLogo) {
		t.Errorf("cover defaults not flagged: %s", joined)
	}
}

func TestCheckTestingWindow(t *testing.T) {
	issues := checkTestingWindow("index.md", map[string]any{"start_date": "2026-09-27"})
	if len(issues) != 1 || issues[0].Check != "testing_window_incomplete" || issues[0].Severity != SeverityWarning ||
		!strings.Contains(issues[0].Message, "end_date") {
		t.Errorf("got %v", issues)
	}
	for _, meta := range []map[string]any{{}, {"start_date": "2026-09-27", "end_date": "2026-09-28"}} {
		if issues := checkTestingWindow("index.md", meta); len(issues) != 0 {
			t.Errorf("%v: unexpected %v", meta, issues)
		}
	}
}

// ── Stub markers ─────────────────────────────────────────────────────────────

func TestCheckStubMarkersNoneFound(t *testing.T) {
	raw := "---\ntitle: Real\n---\n\n## Description\n\nReal content.\n"
	if issues := checkStubMarkers("vuln.md", raw); len(issues) != 0 {
		t.Errorf("got %v", issues)
	}
}

func TestCheckStubMarkersInBodyIsError(t *testing.T) {
	raw := "## Description\n\n> [!REX-STUB]\n[Generic description.]\n"
	issues := checkStubMarkers("vuln.md", raw)
	if len(issues) != 1 || issues[0].Check != "stub_marker_present" {
		t.Errorf("got %v", issues)
	}
}

func TestCheckStubMarkersInFrontmatterCommentIsError(t *testing.T) {
	raw := "---\n# > [!REX-STUB]\ntitle: \"[STUB] Generic Title\"\n---\n\nBody.\n"
	issues := checkStubMarkers("vuln.md", raw)
	if len(issues) != 1 || issues[0].Check != "stub_marker_present" {
		t.Errorf("got %v", issues)
	}
}

func TestCheckStubMarkersMultipleCounted(t *testing.T) {
	raw := "---\n# > [!REX-STUB]\ntitle: \"[STUB]\"\n---\n\n" +
		"> [!REX-STUB]\n[Generic body.]\n"
	issues := checkStubMarkers("vuln.md", raw)
	if len(issues) != 1 {
		t.Errorf("got %v", issues)
	}
	if !strings.Contains(issues[0].Message, "2 unresolved") {
		t.Errorf("got %q", issues[0].Message)
	}
}

// ── Validate (end-to-end) ────────────────────────────────────────────────────

func TestValidateScaffoldedReportFailsValidate(t *testing.T) {
	dir := buildSampleReportDir(t)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "reports", "vuln-1.md"), []byte(
		"---\ntitle: \""+PlaceholderVulnTitle+"\"\n---\n\n"+
			"## Description\n\n"+PlaceholderDescription+"\n\n"+
			"## Impact\n\n"+PlaceholderImpact+"\n\n"+
			"## Recommendation\n\n"+PlaceholderRecommendation+"\n"), 0o644))
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Validate(structure)
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	for _, i := range result.Issues {
		if i.Check == "placeholder_text" {
			messages = append(messages, i.Message)
		}
	}
	joined := strings.Join(messages, " ")
	for _, want := range []string{PlaceholderVulnTitle, PlaceholderDescription, PlaceholderImpact, PlaceholderRecommendation} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %q", want, joined)
		}
	}
}

// TestValidateIndexMetadataPlaceholderIsCaught proves index.md's own
// frontmatter is now scanned — before this, it was parsed but never opened as
// a target file by Validate() at all.
func TestValidateIndexMetadataPlaceholderIsCaught(t *testing.T) {
	dir := buildSampleReportDir(t)
	content, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(content), "client: ACME Corp", "client: "+PlaceholderLoremIpsum+" Holdings", 1)
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	structure, err := ParseIndex(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := Validate(structure)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range result.Issues {
		if i.Check == "placeholder_text" && strings.HasSuffix(i.File, "index.md") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a placeholder_text issue on index.md, got %v", result.Issues)
	}
}

// ── Frontmatter (#58) ────────────────────────────────────────────────────────

func TestCheckFrontmatterUnclosedQuotePointsAtItsLine(t *testing.T) {
	// Shaped like #50's missing-https-carissa.md: more keys and a list after
	// the unclosed quote, where yaml.v3 itself blames line 1.
	content := "---\ntitle: Missing HTTPS\nmaterial_impact: \"Information Privacy\ncvss_vector: AV:N\n" +
		"business_risk:\n  - Operational\naffected_urls:\n  - \"http://a\"\n---\n\nBody\n"
	issue := checkFrontmatter("vuln.md", content)
	if issue == nil || issue.Check != "frontmatter_invalid" || issue.Severity != SeverityError {
		t.Fatalf("want frontmatter_invalid error, got %+v", issue)
	}
	if !strings.Contains(issue.Message, "unclosed quote") {
		t.Errorf("want an unclosed-quote message, got %q", issue.Message)
	}
	if issue.Line != 3 {
		t.Errorf("want line 3 (the unclosed quote), got %d: %s", issue.Line, issue.Message)
	}
}

func TestCheckFrontmatterDuplicateKeyPointsAtSecondOne(t *testing.T) {
	issue := checkFrontmatter("vuln.md", "---\ntitle: A\nseverity: High\ntitle: B\n---\n")
	if issue == nil || issue.Line != 4 || strings.Contains(issue.Message, "line 4:") {
		t.Fatalf("want frontmatter_invalid on line 4 without a stale line prefix, got %+v", issue)
	}
}

func TestCheckFrontmatterNotAtTop(t *testing.T) {
	for name, content := range map[string]string{
		"blank line": "\n---\ntitle: X\n---\nBody\n",
		"BOM":        "\xef\xbb\xbf---\ntitle: X\n---\nBody\n",
	} {
		issue := checkFrontmatter("vuln.md", content)
		if issue == nil || issue.Check != "frontmatter_not_at_top" {
			t.Errorf("%s: want frontmatter_not_at_top, got %+v", name, issue)
			continue
		}
		if want := map[string]int{"blank line": 2, "BOM": 1}[name]; issue.Line != want {
			t.Errorf("%s: want line %d, got %d", name, want, issue.Line)
		}
	}
}

func TestCheckFrontmatterValidAndHorizontalRulePass(t *testing.T) {
	for _, content := range []string{
		"---\ntitle: X\n---\nBody\n",
		"# Heading\n\n---\n\nText\n",
		"No frontmatter at all\n",
	} {
		if issue := checkFrontmatter("x.md", content); issue != nil {
			t.Errorf("%q: unexpected %+v", content, issue)
		}
	}
}

func TestCheckTitlePresent(t *testing.T) {
	if issues := checkTitlePresent("v.md", "---\ntitle: X\n---\n", map[string]any{"title": "X"}); len(issues) != 0 {
		t.Errorf("unexpected %+v", issues)
	}
	issues := checkTitlePresent("v.md", "---\nseverity: High\n---\n", map[string]any{"severity": "High"})
	if len(issues) != 1 || issues[0].Check != "title_missing" || strings.Contains(issues[0].Message, "no frontmatter") {
		t.Errorf("want plain title_missing, got %+v", issues)
	}
	issues = checkTitlePresent("v.md", "Just a body\n", map[string]any{})
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "no frontmatter") {
		t.Errorf("want title_missing naming the missing block, got %+v", issues)
	}
}

func TestValidateOverlayReplacesDiskContentAndSkipsBrokenFile(t *testing.T) {
	dir := t.TempDir()
	vuln := filepath.Join(dir, "vuln.md")
	if err := os.WriteFile(vuln, []byte("---\ntitle: Fine on disk\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(vuln)
	result, err := Validate(Structure{
		Metadata: map[string]any{"title": "T", "client": "C", "date": "D"},
		Reports:  []string{vuln},
		BaseDir:  dir,
		Overlay:  map[string]string{abs: "---\ntitle: \"unclosed\n---\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var checks []string
	for _, i := range result.Issues {
		if i.File == vuln {
			checks = append(checks, i.Check)
		}
	}
	if len(checks) != 1 || checks[0] != "frontmatter_invalid" {
		t.Errorf("want only frontmatter_invalid from the overlay, got %v", checks)
	}
}

// ── Check 8 — Markdown structure (#71) ───────────────────────────────────────

const pastedFinding = `---
title: "XSS"
---

## Recommendation

Untrusted data.
1. HTML-encode the note

TODO: confirm the cookie ID trick

Evidence
![shot](x.png)

Verify whether the note is rendered in:

    Shop/order views.

    Checkout pages.

1. Step one

    ` + "`" + `` + "`" + `` + "`" + `bash
    curl x
    ` + "`" + `` + "`" + `` + "`" + `

Mention ` + "`" + `TODO` + "`" + ` in code is fine.

` + "`" + `` + "`" + `` + "`" + `
TODO inside a fence is fine
    and so is indentation
` + "`" + `` + "`" + `` + "`" + `
`

func TestCheckMarkdownStructureFlagsPastedText(t *testing.T) {
	var got []string
	for _, i := range checkMarkdownStructure("v.md", pastedFinding, true) {
		got = append(got, fmt.Sprintf("%s:%d:%s", i.Check, i.Line, i.Severity))
	}
	want := "todo_marker:10:warning,section_heading_plain_text:12:warning,indented_code_block:17:warning"
	if strings.Join(got, ",") != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

func TestCheckMarkdownStructurePlainHeadingOnlyCheckedInFindings(t *testing.T) {
	if issues := checkMarkdownStructure("s.md", "Evidence\n", false); len(issues) != 0 {
		t.Errorf("unexpected issues: %v", issues)
	}
}

// ── Check 9 — Dangling finding links (#72) ───────────────────────────────────

func TestCheckDanglingRefs(t *testing.T) {
	raw := "See [SQLi](./vuln-1.md) and [XSS](vuln-9.md).\n![](vuln-8.md)\n```\n[x](vuln-7.md)\n```\n"
	issues := checkDanglingRefs("s.md", raw, map[string]bool{"vuln-1": true, "vuln-1.md": true})
	var got []string
	for _, i := range issues {
		got = append(got, fmt.Sprintf("%s:%d:%s", i.Check, i.Line, i.Severity))
	}
	if strings.Join(got, ",") != "dangling_reference:1:error,dangling_reference:2:error" ||
		!strings.Contains(issues[0].Message, "vuln-9.md") {
		t.Errorf("got %v", issues)
	}
}

func TestCheckPlaceholderTextLoremIpsumAnyCase(t *testing.T) {
	for _, body := range []string{"Lorem ipsum dolor", "portal.loremipsum.example"} {
		if issues := checkPlaceholderText("s.md", map[string]any{}, body); len(issues) != 1 {
			t.Errorf("%q: got %v", body, issues)
		}
	}
}
