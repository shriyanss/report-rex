package section

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveReportDir resolves dir to an absolute path and confirms it's an
// existing report directory (has an index.md), the same guard `rex ingest`
// uses before writing into a report.
func resolveReportDir(dir string) (dirAbs, indexPath string, err error) {
	dirAbs, err = filepath.Abs(dir)
	if err != nil {
		return "", "", fmt.Errorf("resolving --dir: %w", err)
	}
	indexPath = filepath.Join(dirAbs, "index.md")
	if _, err := os.Stat(indexPath); err != nil {
		return "", "", fmt.Errorf("index.md not found in %s (resolved from --dir %q) — run this from inside a report directory: %w", dirAbs, dir, err)
	}
	return dirAbs, indexPath, nil
}
