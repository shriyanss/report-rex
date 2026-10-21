package section

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNextNumberedEmptyDir(t *testing.T) {
	n, err := NextNumbered(filepath.Join(t.TempDir(), "missing"), vulnNumRe)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("NextNumbered() on a missing dir = %d, want 1", n)
	}
}

func TestNextNumberedSkipsGaps(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"vuln-1.md", "vuln-2.md", "vuln-4.md", "notes.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, err := NextNumbered(dir, vulnNumRe)
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("NextNumbered() = %d, want 5 (one past the highest, ignoring the gap at 3)", n)
	}
}

func TestNextNumberedAppendix(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"1.md", "2.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, err := NextNumbered(dir, appendixNumRe)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("NextNumbered() = %d, want 3", n)
	}
}
