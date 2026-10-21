package ingest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// timelineEntry is one ingest run, persisted to artifacts/timeline.json. The
// generated appendices are overwritten wholesale on every run, so this file —
// not their frontmatter — is what accumulates the history (and where an entry
// is durably hidden by setting "include": false).
type timelineEntry struct {
	Appendix    string `json:"appendix"`
	Date        string `json:"date"`
	Description string `json:"description"`
	Include     *bool  `json:"include,omitempty"`
}

// writeAppendix records a timeline entry for this ingest run, writes md to
// relPath with every timeline entry recorded for that appendix injected into its
// frontmatter (the preprocessor folds appendix timelines into the report's
// Timeline table), and registers relPath in index.md.
func writeAppendix(dirAbs, relPath, md, desc string) error {
	timelinePath := filepath.Join(dirAbs, "artifacts", "timeline.json")
	var entries []timelineEntry
	if data, err := os.ReadFile(timelinePath); err == nil {
		if err := json.Unmarshal(data, &entries); err != nil {
			return fmt.Errorf("parsing %s: %w", timelinePath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", timelinePath, err)
	}
	entries = append(entries, timelineEntry{Appendix: relPath, Date: time.Now().Format(time.RFC3339), Description: desc})
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(timelinePath), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(timelinePath), err)
	}
	if err := os.WriteFile(timelinePath, data, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", timelinePath, err)
	}

	var block strings.Builder
	block.WriteString("timeline:\n")
	for _, e := range entries {
		if e.Appendix != relPath {
			continue
		}
		fmt.Fprintf(&block, "  - date: %q\n    description: %q\n    include: %t\n", e.Date, e.Description, e.Include == nil || *e.Include)
	}
	// Every generated appendix starts "---\ntitle: ...\n---\n": insert before the closing fence.
	if end := strings.Index(md[3:], "\n---\n"); strings.HasPrefix(md, "---\n") && end >= 0 {
		md = md[:3+end+1] + block.String() + md[3+end+1:]
	}

	appendixAbs := filepath.Join(dirAbs, relPath)
	if err := os.MkdirAll(filepath.Dir(appendixAbs), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(appendixAbs), err)
	}
	if err := os.WriteFile(appendixAbs, []byte(md), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", appendixAbs, err)
	}

	note, err := ensureAppendixRegistered(filepath.Join(dirAbs, "index.md"), relPath)
	if err != nil {
		return fmt.Errorf("registering %s in index.md: %w", relPath, err)
	}
	if note != "" {
		fmt.Println(note)
	}
	return nil
}

// timelineDesc builds an entry description like "Port scan (nmap) against
// 10.0.0.1, 10.0.0.2 (+3 more)" from the hosts/domains a run actually found.
func timelineDesc(kind, tool, prep string, hosts []string) string {
	seen := map[string]bool{}
	var uniq []string
	for _, h := range hosts {
		if h != "" && !seen[h] {
			seen[h] = true
			uniq = append(uniq, h)
		}
	}
	if len(uniq) == 0 {
		return fmt.Sprintf("%s (%s): nothing found", kind, tool)
	}
	sort.Strings(uniq)
	const maxShown = 5
	list := strings.Join(uniq[:min(len(uniq), maxShown)], ", ")
	if len(uniq) > maxShown {
		list += fmt.Sprintf(" (+%d more)", len(uniq)-maxShown)
	}
	return fmt.Sprintf("%s (%s) %s %s", kind, tool, prep, list)
}
