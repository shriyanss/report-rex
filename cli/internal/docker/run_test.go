package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/buildscript"
)

// writeInvalidReportDir writes a minimal report directory whose single
// finding fails multiple QA-gate checks (no CVSS, no timeline, no Steps to
// Reproduce), so RunReportBuild's new validation guard has something to catch
// well before it ever reaches template/Docker/pandoc concerns.
func writeInvalidReportDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(dir, "reports"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "reports", "vuln-1.md"), []byte(
		"---\ntitle: Finding\n---\n\n## Description\n\nToo short.\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "index.md"), []byte(
		"---\ntitle: Test Report\nclient: Test Client\ntemplate: rex-default\n"+
			"structure:\n  reports:\n    - reports/vuln-1.md\n---\n"), 0o644))
	return dir
}

// TestRunDockerMissingIndexFailsFast confirms the index.md pre-check runs (and
// errors clearly) before any Docker interaction, so this passes even without a
// docker binary/daemon available in the test environment.
func TestRunDockerMissingIndexFailsFast(t *testing.T) {
	dir := t.TempDir()

	err := RunReportBuild(buildscript.BuildFlags{Dir: dir, Output: "report.pdf", Image: "report-rex:latest"})
	if err == nil {
		t.Fatal("expected an error for a report dir with no index.md, got nil")
	}
	if !strings.Contains(err.Error(), "index.md not found") {
		t.Errorf("error = %q, want it to mention a missing index.md", err.Error())
	}
	if !strings.Contains(err.Error(), filepath.Clean(dir)) {
		t.Errorf("error = %q, want it to reference the resolved dir %q", err.Error(), dir)
	}
}

// TestRunReportBuildFailsValidationWithoutForce proves RunReportBuild itself
// enforces the QA gate (not just its cmd/report_build.go caller) — this is
// what closes the gap where `rex watch`'s rebuild loop called RunReportBuild
// directly with no validation at all.
func TestRunReportBuildFailsValidationWithoutForce(t *testing.T) {
	dir := writeInvalidReportDir(t)

	err := RunReportBuild(buildscript.BuildFlags{Dir: dir, Output: "report.pdf"})
	if err == nil {
		t.Fatal("expected a validation error for an invalid report dir, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("error = %q, want it to mention validation errors", err.Error())
	}
}

// TestRunReportBuildForceSkipsValidation proves --force bypasses the guard —
// the build then proceeds past validation and fails later for an unrelated
// reason (no docker/pandoc available in this test environment), not for a
// validation-shaped error.
func TestRunReportBuildForceSkipsValidation(t *testing.T) {
	dir := writeInvalidReportDir(t)

	err := RunReportBuild(buildscript.BuildFlags{Dir: dir, Output: "report.pdf", Force: true})
	if err == nil {
		t.Fatal("expected an error (no docker/pandoc in test env), got nil")
	}
	if strings.Contains(err.Error(), "validation error") {
		t.Errorf("error = %q, --force should have skipped validation entirely", err.Error())
	}
}
