package section

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var appendixNumRe = regexp.MustCompile(`^(\d+)\.md$`)

// AddAppendix creates a new numbered appendix file (appendices/N.md) in dir and
// registers it under structure.appendices in index.md. note is a non-empty
// message the caller should print when index.md couldn't be updated automatically.
func AddAppendix(dir, title string) (relPath, note string, err error) {
	dirAbs, indexPath, err := resolveReportDir(dir)
	if err != nil {
		return "", "", err
	}

	appendicesDir := filepath.Join(dirAbs, "appendices")
	n, err := NextNumbered(appendicesDir, appendixNumRe)
	if err != nil {
		return "", "", fmt.Errorf("scanning %s: %w", appendicesDir, err)
	}
	relPath = fmt.Sprintf("appendices/%d.md", n)

	if err := os.MkdirAll(appendicesDir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", appendicesDir, err)
	}
	content := fmt.Sprintf("---\ntitle: %q\n---\n\n## %s\n", title, title)
	if err := os.WriteFile(filepath.Join(dirAbs, relPath), []byte(content), 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", relPath, err)
	}

	note, err = EnsureStructureEntry(indexPath, "appendices", relPath)
	if err != nil {
		return "", "", fmt.Errorf("registering %s in index.md: %w", relPath, err)
	}
	return relPath, note, nil
}
