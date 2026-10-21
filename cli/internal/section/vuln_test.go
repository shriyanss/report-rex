package section

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupReportDir writes sampleIndexMD plus the actual files it lists in
// structure: — NextNumbered scans the real reports/appendices directories on
// disk, not index.md's listing, so the fixture needs both in sync.
func setupReportDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(sampleIndexMD), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"sections/exec_summ.md", "reports/vuln-1.md", "appendices/1.md", "appendices/2.md"} {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\ntitle: \"x\"\n---\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAddVulnRequiresTitleWithoutTemplate(t *testing.T) {
	dir := setupReportDir(t)
	if _, _, err := AddVuln(dir, VulnOptions{}); err == nil {
		t.Fatal("expected an error when neither --title nor --template is given")
	}
}

func TestAddVulnBlankWritesMinimalFrontmatter(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddVuln(dir, VulnOptions{Title: "Missing Rate Limiting"})
	if err != nil {
		t.Fatal(err)
	}
	if relPath != "reports/vuln-2.md" {
		t.Errorf("relPath = %q, want reports/vuln-2.md (sampleIndexMD already lists vuln-1.md)", relPath)
	}
	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `title: "Missing Rate Limiting"`) {
		t.Errorf("missing title:\n%s", got)
	}
	if strings.Contains(got, "cvss_vector") {
		t.Errorf("unset fields should be omitted entirely:\n%s", got)
	}
	if !strings.Contains(got, "## Replication") {
		t.Errorf("missing body skeleton:\n%s", got)
	}

	data, err = os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "    - reports/vuln-2.md") {
		t.Errorf("new vuln wasn't registered in index.md:\n%s", data)
	}
}

func TestSetFrontmatterFieldReplacesScalarAndStripsMarker(t *testing.T) {
	lines := strings.Split(`---
# > [!REX-STUB]
title: "placeholder"
status: "Open"
---`, "\n")

	got := setFrontmatterField(lines, "title", []string{`title: "Real Title"`})
	joined := strings.Join(got, "\n")

	if strings.Contains(joined, "REX-STUB") {
		t.Errorf("expected the marker to be dropped:\n%s", joined)
	}
	if !strings.Contains(joined, `title: "Real Title"`) {
		t.Errorf("expected the new title:\n%s", joined)
	}
	if !strings.Contains(joined, `status: "Open"`) {
		t.Errorf("unrelated field should be untouched:\n%s", joined)
	}
}

func TestSetFrontmatterFieldReplacesListBlock(t *testing.T) {
	lines := strings.Split(`---
# > [!REX-STUB]
business_risk:
  - Operational
  - Reputational
affected_urls:
  - "https://example.com"
---`, "\n")

	got := setFrontmatterField(lines, "business_risk", []string{"business_risk:", "  - Financial"})
	joined := strings.Join(got, "\n")

	if strings.Contains(joined, "REX-STUB") {
		t.Errorf("expected the marker to be dropped:\n%s", joined)
	}
	if strings.Contains(joined, "Operational") || strings.Contains(joined, "Reputational") {
		t.Errorf("old list items should be gone:\n%s", joined)
	}
	if !strings.Contains(joined, "  - Financial") {
		t.Errorf("expected the new list item:\n%s", joined)
	}
	if !strings.Contains(joined, `affected_urls:`) || !strings.Contains(joined, `"https://example.com"`) {
		t.Errorf("unrelated list block should be untouched:\n%s", joined)
	}
}

func TestAddVulnFromTemplateStripsOnlyOverriddenMarkers(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddVuln(dir, VulnOptions{
		Template:   "ssrf",
		Title:      "SSRF in Webhook Handler",
		CVSSVector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)

	if !strings.Contains(got, `title: "SSRF in Webhook Handler"`) {
		t.Errorf("title override missing:\n%s", got)
	}
	if !strings.Contains(got, `cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"`) {
		t.Errorf("cvss_vector override missing:\n%s", got)
	}
	// component wasn't overridden — its stub marker and placeholder value must survive
	// so `rex report validate` still flags it.
	if !strings.Contains(got, "# > [!REX-STUB]\ncomponent:") {
		t.Errorf("untouched field's stub marker should survive:\n%s", got)
	}
	if !strings.Contains(got, "> [!REX-STUB]") {
		t.Errorf("body stub markers should be untouched by frontmatter overrides:\n%s", got)
	}
}

func TestVulnTemplateNamesExcludesReadme(t *testing.T) {
	names, err := VulnTemplateNames()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		if n == "README" {
			t.Errorf("VulnTemplateNames() should exclude README.md, got %v", names)
		}
	}
	if len(names) == 0 {
		t.Error("expected at least one vuln template")
	}
}

func TestAddAppendixNumbersAndRegisters(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddAppendix(dir, "Appendix — Extra Notes")
	if err != nil {
		t.Fatal(err)
	}
	if relPath != "appendices/3.md" {
		t.Errorf("relPath = %q, want appendices/3.md (sampleIndexMD lists 1.md, 2.md)", relPath)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "    - appendices/3.md") {
		t.Errorf("new appendix wasn't registered:\n%s", data)
	}
}

func TestAddSectionRejectsExistingName(t *testing.T) {
	dir := setupReportDir(t)
	if _, _, err := AddSection(dir, "risk-summary", "Risk Summary"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := AddSection(dir, "risk-summary", "Risk Summary Again"); err == nil {
		t.Fatal("expected an error when the section file already exists")
	}
}

func TestAddVulnRequiresIndexMD(t *testing.T) {
	dir := t.TempDir() // no index.md
	if _, _, err := AddVuln(dir, VulnOptions{Title: "X"}); err == nil {
		t.Fatal("expected an error when index.md doesn't exist")
	}
}

func TestAddVulnAIStampsFlagAndCustomBody(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddVuln(dir, VulnOptions{
		Title: "AI-found IDOR",
		Body:  "## Description\n\nfound by the agent\n\n## Steps to Reproduce\n\n```\ncurl -i https://target.example/api/users/2\n```\n",
		AI:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.HasPrefix(got, "---\nai: true\n") {
		t.Errorf("expected ai: true as the first frontmatter line:\n%s", got)
	}
	if !strings.Contains(got, "found by the agent") || strings.Contains(got, "## Impact") {
		t.Errorf("expected the custom body to replace the default skeleton:\n%s", got)
	}
}

func TestAddVulnAIRequiresCurlPoC(t *testing.T) {
	dir := setupReportDir(t)
	_, _, err := AddVuln(dir, VulnOptions{
		Title: "AI-found IDOR",
		Body:  "## Description\n\nfound by the agent, no repro command included\n",
		AI:    true,
	})
	if err == nil {
		t.Fatal("expected an error when an AI-authored body has no curl PoC")
	}
}

func TestAddVulnWithoutAIOmitsFlag(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddVuln(dir, VulnOptions{Title: "Human-found bug"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ai:") {
		t.Errorf("didn't ask for AI, expected no ai: key:\n%s", data)
	}
}

func TestUpdateVulnRejectsNonAIFindingWithoutOverride(t *testing.T) {
	dir := setupReportDir(t)
	if _, err := UpdateVuln(dir, "reports/vuln-1.md", VulnOptions{Title: "Renamed"}, false); !errors.Is(err, ErrNotAIOwned) {
		t.Fatalf("expected ErrNotAIOwned, got %v", err)
	}
}

func TestUpdateVulnAllowsOverrideAndRestampsAI(t *testing.T) {
	dir := setupReportDir(t)
	note, err := UpdateVuln(dir, "reports/vuln-1.md", VulnOptions{Title: "Renamed by human request"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if note == "" {
		t.Error("expected a non-empty note")
	}
	data, err := os.ReadFile(filepath.Join(dir, "reports/vuln-1.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "title: Renamed by human request") {
		t.Errorf("title wasn't updated:\n%s", got)
	}
	if !strings.Contains(got, "ai: true") {
		t.Errorf("expected update to stamp ai: true:\n%s", got)
	}
}

func TestUpdateVulnAIOwnedAcceptsFollowUpWithoutOverride(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddVuln(dir, VulnOptions{Title: "AI finding", AI: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateVuln(dir, relPath, VulnOptions{Component: "Auth API"}, false); err != nil {
		t.Fatalf("expected an already-AI-owned finding to accept a follow-up edit without override: %v", err)
	}
}

func TestUpdateVulnAIRequiresCurlPoC(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddVuln(dir, VulnOptions{Title: "AI finding", AI: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = UpdateVuln(dir, relPath, VulnOptions{
		Body: "## Steps to Reproduce\n\nNavigate to the login page and observe the error.\n",
	}, false)
	if err == nil {
		t.Fatal("expected an error when an updated body has no curl PoC")
	}
}

func TestUpdateVulnPreservesUntouchedFields(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddVuln(dir, VulnOptions{Title: "AI finding", Component: "Payments API", AI: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateVuln(dir, relPath, VulnOptions{CWE: "CWE-79"}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, relPath))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "component: Payments API") {
		t.Errorf("untouched component field should survive:\n%s", got)
	}
	if !strings.Contains(got, "cwe: CWE-79") {
		t.Errorf("new field wasn't applied:\n%s", got)
	}
}

func TestAddVulnCustomFile(t *testing.T) {
	dir := setupReportDir(t)
	relPath, _, err := AddVuln(dir, VulnOptions{Template: "ssrf", File: "ssrf-webhook"})
	if err != nil {
		t.Fatal(err)
	}
	if relPath != "reports/ssrf-webhook.md" {
		t.Errorf("relPath = %q, want reports/ssrf-webhook.md", relPath)
	}
	data, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "    - reports/ssrf-webhook.md") {
		t.Errorf("custom-named vuln wasn't registered in index.md:\n%s", data)
	}
	for _, bad := range []string{"ssrf-webhook", "ssrf-webhook.md", "../x", "a/b"} {
		if _, _, err := AddVuln(dir, VulnOptions{Title: "x", File: bad}); err == nil {
			t.Errorf("File %q: expected an error", bad)
		}
	}
	if VulnTemplateTitle("ssrf") == "" {
		t.Error("VulnTemplateTitle(ssrf) is empty")
	}
}
