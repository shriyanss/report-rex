package ingest

import "github.com/shriyanss/report-rex/cli/internal/section"

// ensureAppendixRegistered makes sure appendixPath is listed under
// structure.appendices in index.md, inserting it if missing. See
// section.EnsureStructureEntry for the implementation, shared with
// `rex report section`.
func ensureAppendixRegistered(indexPath, appendixPath string) (string, error) {
	return section.EnsureStructureEntry(indexPath, "appendices", appendixPath)
}
