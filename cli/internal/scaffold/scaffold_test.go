package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/report"
)

func TestInitReportFullReport(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "report")

	if err := InitReport(target, TypeFullReport, "", Meta{}); err != nil {
		t.Fatalf("InitReport() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "index.md")); err != nil {
		t.Errorf("expected index.md to be extracted: %v", err)
	}
}

func TestInitReportQuickComm(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "comm")

	if err := InitReport(target, TypeQuickComm, "critical-finding", Meta{}); err != nil {
		t.Fatalf("InitReport() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "index.md")); err != nil {
		t.Errorf("expected index.md to be extracted: %v", err)
	}
}

func TestInitReportRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "report")

	if err := InitReport(target, TypeFullReport, "", Meta{}); err != nil {
		t.Fatalf("first InitReport() error = %v", err)
	}
	if err := InitReport(target, TypeFullReport, "", Meta{}); err == nil {
		t.Fatal("expected second InitReport() into a non-empty dir to fail, got nil")
	}
}

func TestInitReportIgnoresHiddenEntries(t *testing.T) {
	target := t.TempDir()
	for _, d := range []string{".git", ".vscode"} {
		if err := os.Mkdir(filepath.Join(target, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := InitReport(target, TypeFullReport, "", Meta{}); err != nil {
		t.Fatalf("InitReport() into a dir with only hidden entries: %v", err)
	}

	visible := t.TempDir()
	if err := os.WriteFile(filepath.Join(visible, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := InitReport(visible, TypeFullReport, "", Meta{}); err == nil {
		t.Fatal("expected InitReport() into a dir with a visible file to fail, got nil")
	}
}

func TestInitReportUnknownType(t *testing.T) {
	dir := t.TempDir()
	if err := InitReport(filepath.Join(dir, "report"), "bogus", "", Meta{}); err == nil {
		t.Fatal("expected unknown report type to error, got nil")
	}
}

func TestInitReportPatchesMeta(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "report")

	meta := Meta{Title: "Acme Web App Pentest", Client: "Acme Corp", Date: "2026-09-01"}
	if err := InitReport(target, TypeFullReport, "", meta); err != nil {
		t.Fatalf("InitReport() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(target, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		`title: "Acme Web App Pentest"`,
		`client: "Acme Corp"`,
		`date: "2026-09-01"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("index.md missing %q:\n%s", want, got)
		}
	}
	// structure: is a nested key, not a top-level title/client/date field — make
	// sure the line-based patch didn't clobber it.
	if !strings.Contains(got, "structure:") {
		t.Errorf("index.md lost its structure: block:\n%s", got)
	}
}

// TestInitReportFullReportFailsValidateOutOfTheBox is the regression test for
// issue #22: `rex report init` used to extract the ACME-populated example
// tree verbatim, with no stub markers or placeholder sentinels of any kind —
// so an unedited copy silently passed `rex report validate`. It must not
// anymore.
func TestInitReportFullReportFailsValidateOutOfTheBox(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "report")
	if err := InitReport(target, TypeFullReport, "", Meta{}); err != nil {
		t.Fatalf("InitReport() error = %v", err)
	}
	structure, err := report.ParseIndex(filepath.Join(target, "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := report.Validate(structure)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK() {
		t.Fatal("expected an unedited rex report init tree to fail validation, got no errors")
	}
	found := false
	for _, i := range result.Errors() {
		if i.Check == "placeholder_text" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected at least one placeholder_text error, got %v", result.Errors())
	}
}
