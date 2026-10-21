package section

import (
	"fmt"
	"os"
	"path/filepath"
)

// AddSection creates a new narrative section file (sections/<name>.md) in dir
// and registers it under structure.sections in index.md. Unlike vulns and
// appendices, narrative sections aren't numbered (see sections/exec_summ.md,
// sections/methodology.md), so name is a required slug.
func AddSection(dir, name, title string) (relPath, note string, err error) {
	dirAbs, indexPath, err := resolveReportDir(dir)
	if err != nil {
		return "", "", err
	}

	sectionsDir := filepath.Join(dirAbs, "sections")
	relPath = fmt.Sprintf("sections/%s.md", name)
	if _, err := os.Stat(filepath.Join(dirAbs, relPath)); err == nil {
		return "", "", fmt.Errorf("%s already exists", relPath)
	}

	if err := os.MkdirAll(sectionsDir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating %s: %w", sectionsDir, err)
	}
	content := fmt.Sprintf("---\ntitle: %q\n---\n\n", title)
	if err := os.WriteFile(filepath.Join(dirAbs, relPath), []byte(content), 0o644); err != nil {
		return "", "", fmt.Errorf("writing %s: %w", relPath, err)
	}

	note, err = EnsureStructureEntry(indexPath, "sections", relPath)
	if err != nil {
		return "", "", fmt.Errorf("registering %s in index.md: %w", relPath, err)
	}
	return relPath, note, nil
}
