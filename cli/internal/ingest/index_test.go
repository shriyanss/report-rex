package ingest

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

func TestEnsureAppendixRegisteredInsertsEntry(t *testing.T) {
	path := writeIndex(t, sampleIndexMD)

	if note, err := ensureAppendixRegistered(path, AppendixPath); err != nil || note != "" {
		t.Fatalf("ensureAppendixRegistered: note=%q err=%v", note, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "    - "+AppendixPath) {
		t.Errorf("index.md missing new appendix entry:\n%s", got)
	}
	if !strings.Contains(got, "    - appendices/2.md") {
		t.Errorf("existing entries were not preserved:\n%s", got)
	}
}

func TestEnsureAppendixRegisteredIsIdempotent(t *testing.T) {
	path := writeIndex(t, sampleIndexMD)

	if _, err := ensureAppendixRegistered(path, AppendixPath); err != nil {
		t.Fatal(err)
	}
	firstPass, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if note, err := ensureAppendixRegistered(path, AppendixPath); err != nil || note != "" {
		t.Fatalf("second call: note=%q err=%v", note, err)
	}
	secondPass, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(firstPass) != string(secondPass) {
		t.Errorf("second call changed the file:\n--- after 1st ---\n%s\n--- after 2nd ---\n%s", firstPass, secondPass)
	}
}

func TestEnsureAppendixRegisteredNoAppendicesBlock(t *testing.T) {
	content := "---\ntitle: \"Test\"\nstructure:\n  sections:\n    - sections/a.md\n---\n"
	path := writeIndex(t, content)

	note, err := ensureAppendixRegistered(path, AppendixPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if note == "" {
		t.Error("expected a manual-instruction note when structure.appendices is absent")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Error("index.md should be left untouched when appendices: can't be found")
	}
}
