package syncclient

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

func TestScanWorkspaceMatchesExampleShape(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{
		"index.md":                 "# Report",
		"appendices/1.md":          "appendix 1",
		"assets/logo.png":          "PNGDATA",
		"assets/screenshots/a.png": "PNGDATA2",
		"reports/vuln-1.md":        "finding",
		"sections/exec_summ.md":    "summary",
	})

	files, err := ScanWorkspace(dir)
	if err != nil {
		t.Fatalf("ScanWorkspace: %v", err)
	}
	if len(files) != 6 {
		t.Fatalf("len(files) = %d, want 6", len(files))
	}
	if _, ok := files["assets/screenshots/a.png"]; !ok {
		t.Error("expected nested asset path to be scanned")
	}
}

func TestScanWorkspaceExcludesReservedPaths(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, map[string]string{
		"index.md":            "# Report",
		".git/HEAD":           "ref: refs/heads/main",
		".report-rex/sync.db": "binary junk",
		"assets/.DS_Store":    "junk",
		"draft.md.swp":        "vim swap",
	})

	files, err := ScanWorkspace(dir)
	if err != nil {
		t.Fatalf("ScanWorkspace: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1 (only index.md); got %v", len(files), files)
	}
	if _, ok := files["index.md"]; !ok {
		t.Error("expected index.md to be scanned")
	}
}

func TestWriteReadRemoveFile(t *testing.T) {
	dir := t.TempDir()
	if err := WriteFile(dir, "nested/dir/file.md", []byte("hello")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := ReadFile(dir, "nested/dir/file.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want hello", got)
	}
	if err := RemoveFile(dir, "nested/dir/file.md"); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "nested/dir/file.md")); !os.IsNotExist(err) {
		t.Errorf("expected file removed, stat err = %v", err)
	}
	// Removing an already-absent file must not error.
	if err := RemoveFile(dir, "nested/dir/file.md"); err != nil {
		t.Errorf("RemoveFile on absent file: %v", err)
	}
	// Emptied parents are pruned (#44), but never the workspace root.
	if _, err := os.Stat(filepath.Join(dir, "nested")); !os.IsNotExist(err) {
		t.Errorf("expected empty nested/ pruned, stat err = %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("workspace root removed: %v", err)
	}
	// A parent that still has other contents is kept.
	writeFixture(t, dir, map[string]string{"a/b/x.md": "x", "a/y.md": "y"})
	if err := RemoveFile(dir, "a/b/x.md"); err != nil {
		t.Fatalf("RemoveFile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a/b")); !os.IsNotExist(err) {
		t.Errorf("expected empty a/b pruned, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a/y.md")); err != nil {
		t.Errorf("sibling a/y.md lost: %v", err)
	}
}

// A moved-away workspace must fail the scan, not look like every file was
// deleted (issue #46).
func TestScanWorkspaceMissingDirIsError(t *testing.T) {
	if _, err := ScanWorkspace(filepath.Join(t.TempDir(), "gone")); err == nil {
		t.Fatal("ScanWorkspace on a missing dir: want error, got nil")
	}
}
