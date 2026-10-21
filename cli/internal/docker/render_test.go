package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEncryptNativePDFMissingQpdfFailsFast confirms encryptNativePDF reports
// a clear error when qpdf isn't on PATH, before touching the PDF at all —
// mirrors renderNative's missing-pandoc check. PATH is overridden to an empty
// dir so this is deterministic regardless of whether the host running the
// test happens to have qpdf installed.
func TestEncryptNativePDFMissingQpdfFailsFast(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	pdf := filepath.Join(t.TempDir(), "report.pdf")
	if err := os.WriteFile(pdf, []byte("%PDF-1.4 fake"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := encryptNativePDF(pdf, "hunter2")
	if err == nil {
		t.Fatal("expected an error when qpdf is not on PATH, got nil")
	}
	if !strings.Contains(err.Error(), "qpdf") {
		t.Errorf("error = %q, want it to mention qpdf", err.Error())
	}

	if _, statErr := os.Stat(pdf); statErr != nil {
		t.Errorf("original PDF should be left in place when qpdf is missing, but stat failed: %v", statErr)
	}
}
