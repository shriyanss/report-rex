package section

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleIndexMD = `---
title: "Test Report"
client: "ACME"
structure:
  sections:
    - sections/exec_summ.md
  reports:
    - reports/vuln-1.md
  appendices:
    - appendices/1.md
    - appendices/2.md
---
`

func writeIndex(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "index.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing index.md fixture: %v", err)
	}
	return path
}

func TestEnsureStructureEntryInsertsEntry(t *testing.T) {
	path := writeIndex(t, sampleIndexMD)

	if note, err := EnsureStructureEntry(path, "appendices", "appendices/3.md"); err != nil || note != "" {
		t.Fatalf("EnsureStructureEntry: note=%q err=%v", note, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "    - appendices/3.md") {
		t.Errorf("index.md missing new appendix entry:\n%s", got)
	}
	if !strings.Contains(got, "    - appendices/2.md") {
		t.Errorf("existing entries were not preserved:\n%s", got)
	}
}

func TestEnsureStructureEntryIsIdempotent(t *testing.T) {
	path := writeIndex(t, sampleIndexMD)

	if _, err := EnsureStructureEntry(path, "reports", "reports/vuln-2.md"); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureStructureEntry(path, "reports", "reports/vuln-2.md"); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("second call changed index.md:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestEnsureStructureEntryDifferentKeys(t *testing.T) {
	path := writeIndex(t, sampleIndexMD)

	if _, err := EnsureStructureEntry(path, "sections", "sections/methodology.md"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "    - sections/methodology.md") {
		t.Errorf("index.md missing new section entry:\n%s", got)
	}
	// Registering under sections: must not touch the unrelated reports:/appendices: lists.
	if !strings.Contains(got, "    - reports/vuln-1.md") || !strings.Contains(got, "    - appendices/2.md") {
		t.Errorf("unrelated structure lists were disturbed:\n%s", got)
	}
}

func TestEnsureStructureEntryMissingKey(t *testing.T) {
	path := writeIndex(t, `---
title: "Test"
structure:
  reports:
    - reports/vuln-1.md
---
`)

	note, err := EnsureStructureEntry(path, "appendices", "appendices/1.md")
	if err != nil {
		t.Fatal(err)
	}
	if note == "" {
		t.Fatal("expected a note when structure.appendices doesn't exist")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "appendices/1.md") {
		t.Errorf("index.md should have been left untouched:\n%s", data)
	}
}
