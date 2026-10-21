package report

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSplitFrontmatterExtracts(t *testing.T) {
	content := "---\ntitle: Test\n---\n\nBody here."
	fm, body := SplitFrontmatter(content)
	if fm != "title: Test" {
		t.Errorf("fm = %q", fm)
	}
	if body != "Body here." {
		t.Errorf("body = %q", body)
	}
}

func TestSplitFrontmatterNone(t *testing.T) {
	content := "Just a plain markdown file."
	fm, body := SplitFrontmatter(content)
	if fm != "" || body != content {
		t.Errorf("fm=%q body=%q", fm, body)
	}
}

func TestSplitFrontmatterEmpty(t *testing.T) {
	content := "---\n---\n\nBody."
	fm, body := SplitFrontmatter(content)
	if fm != "" {
		t.Errorf("fm = %q, want empty", fm)
	}
	if body != "Body." {
		t.Errorf("body = %q", body)
	}
}

func writeSampleReport(t *testing.T, dir string) string {
	t.Helper()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "sections"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "reports"), 0o755))
	must(os.MkdirAll(filepath.Join(dir, "appendices"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "sections", "exec_summ.md"), []byte("---\ntitle: Exec\n---\nBody"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "reports", "vuln-1.md"), []byte("---\ntitle: Vuln\n---\nBody"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "appendices", "1.md"), []byte("---\ntitle: A\n---\nBody"), 0o644))
	index := filepath.Join(dir, "index.md")
	must(os.WriteFile(index, []byte(`---
title: "Test Report"
client: "ACME Corp"
structure:
  sections:
    - sections/exec_summ.md
  reports:
    - reports/vuln-1.md
  appendices:
    - appendices/1.md
---
`), 0o644))
	return index
}

func TestParseIndexValid(t *testing.T) {
	dir := t.TempDir()
	index := writeSampleReport(t, dir)
	structure, err := ParseIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	if structure.Metadata["title"] != "Test Report" {
		t.Errorf("title = %v", structure.Metadata["title"])
	}
	if structure.Metadata["client"] != "ACME Corp" {
		t.Errorf("client = %v", structure.Metadata["client"])
	}
	if structure.Template != "rit-cptc" {
		t.Errorf("template = %v", structure.Template)
	}
	if len(structure.Sections) != 1 || len(structure.Reports) != 1 || len(structure.Appendices) != 1 {
		t.Errorf("sections=%d reports=%d appendices=%d", len(structure.Sections), len(structure.Reports), len(structure.Appendices))
	}
}

func TestParseIndexMissingFileErrors(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "index.md")
	if err := os.WriteFile(index, []byte("---\ntemplate: rex-default\nstructure:\n  sections:\n    - missing.md\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ParseIndex(index)
	if err == nil {
		t.Fatal("expected error for missing referenced file")
	}
}

func TestParseIndexNoFrontmatterErrors(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "index.md")
	if err := os.WriteFile(index, []byte("No frontmatter here."), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseIndex(index); err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
}

func TestParseIndexDefaultTemplate(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "index.md")
	if err := os.WriteFile(index, []byte("---\ntitle: Test\nstructure: {}\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	structure, err := ParseIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	if structure.Template != "rit-cptc" {
		t.Errorf("template = %v", structure.Template)
	}
}
