// Package section implements `rex report section <vuln|appendix|narrative>`:
// generating a new report content file from scratch (or from a vuln-templates/
// stub) and wiring it into index.md's structure: block, so adding a finding,
// appendix, or narrative section no longer requires copying an existing file
// by hand and hand-editing index.md.
package section

import (
	"fmt"
	"os"
	"strings"
)

// EnsureStructureEntry makes sure path is listed under structure.<key> in
// index.md, inserting it if missing. This is a line-based text edit, not a full
// YAML parse/marshal round-trip: re-marshaling index.md would destroy the
// hand-written comments it commonly contains (e.g. branding notes). It's a
// no-op if the entry is already present, and if the structure.<key> list can't
// be found it leaves index.md untouched and returns a message for the caller
// to print instead of guessing at the YAML structure to fabricate.
func EnsureStructureEntry(indexPath, key, path string) (string, error) {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", indexPath, err)
	}
	lines := strings.Split(string(data), "\n")

	entry := "    - " + path
	keyLine := -1
	lastEntryLine := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == strings.TrimSpace(entry) {
			return "", nil // already registered
		}
		if keyLine == -1 && trimmed == key+":" {
			keyLine = i
			continue
		}
		if keyLine != -1 {
			if strings.HasPrefix(strings.TrimLeft(line, " "), "- ") {
				lastEntryLine = i
				continue
			}
			// first non-list-item line after key: ends the block
			break
		}
	}

	if keyLine == -1 {
		return fmt.Sprintf("could not find structure.%s in %s — add %q to it manually", key, indexPath, path), nil
	}

	insertAt := keyLine + 1
	if lastEntryLine != -1 {
		insertAt = lastEntryLine + 1
	}
	lines = append(lines[:insertAt], append([]string{entry}, lines[insertAt:]...)...)

	return "", os.WriteFile(indexPath, []byte(strings.Join(lines, "\n")), 0o644)
}
