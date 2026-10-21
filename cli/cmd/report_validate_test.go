package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func writeValidateFixture(t *testing.T, reports []string) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	reportsYAML := "[]"
	if len(reports) > 0 {
		reportsYAML = "\n"
		for _, r := range reports {
			reportsYAML += "      - " + r + "\n"
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "reports"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "index.md"), []byte(
		"---\ntitle: Test Report\nclient: ACME\ndate: \"2024-01-01\"\nstructure:\n  sections: []\n  appendices: []\n  reports: "+reportsYAML+"\n---\n"), 0o644))
	for _, r := range reports {
		must(os.WriteFile(filepath.Join(dir, r), []byte(
			"---\ntitle: A Finding\n---\n\n## Description\n\nSomething happened.\n"), 0o644))
	}
	return dir
}

func TestRunValidationJSONNoIssues(t *testing.T) {
	dir := writeValidateFixture(t, nil)
	_, result, err := runValidation(dir, true, nil)
	if err != nil {
		t.Fatalf("runValidation: %v", err)
	}
	if len(result.Issues) != 0 {
		t.Errorf("expected no issues, got %v", result.Issues)
	}
}

func TestRunValidationJSONRelativizesIssuePaths(t *testing.T) {
	dir := writeValidateFixture(t, []string{"reports/vuln-1.md"})
	_, result, err := runValidation(dir, true, nil)
	if err != nil {
		t.Fatalf("runValidation: %v", err)
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected the incomplete finding to produce issues")
	}
	for _, issue := range result.Issues {
		if issue.File != "reports/vuln-1.md" {
			t.Errorf("expected relative path %q, got %q", "reports/vuln-1.md", issue.File)
		}
	}
	if result.OK() {
		t.Error("expected the incomplete finding to fail validation")
	}
}

func TestRunValidationJSONMissingIndexReturnsError(t *testing.T) {
	dir := t.TempDir()
	_, _, err := runValidation(dir, true, nil)
	if err == nil {
		t.Fatal("expected an error for a dir with no index.md")
	}
}

func TestRunValidationTextModeUnaffectedByJSONPathRelativization(t *testing.T) {
	dir := writeValidateFixture(t, []string{"reports/vuln-1.md"})
	_, result, err := runValidation(dir, false, nil)
	if err != nil {
		t.Fatalf("runValidation: %v", err)
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues in text mode too")
	}
	if result.Issues[0].File != "reports/vuln-1.md" {
		t.Errorf("expected relative path in text mode as well, got %q", result.Issues[0].File)
	}
}

func TestRunValidationOverlayUsesBufferContent(t *testing.T) {
	dir := writeValidateFixture(t, []string{"reports/vuln-1.md"})
	abs, _ := filepath.Abs(filepath.Join(dir, "reports/vuln-1.md"))
	_, result, err := runValidation(dir, true, map[string]string{abs: "No frontmatter in the unsaved buffer\n"})
	if err != nil {
		t.Fatalf("runValidation: %v", err)
	}
	for _, i := range result.Issues {
		if i.Check == "title_missing" && i.File == filepath.Join("reports", "vuln-1.md") {
			return
		}
	}
	t.Errorf("want title_missing from the overlay, got %+v", result.Issues)
}
