package ingest

import (
	"fmt"
	"sort"
	"strings"
)

// WebAppendixPath is the path (relative to the report directory) of the generated
// web-findings appendix nikto writes.
const WebAppendixPath = "appendices/web-findings.md"

// renderWebAppendixMarkdown regenerates the web-findings appendix in full from the
// merged finding list, mirroring renderAppendixMarkdown's always-overwrite approach.
func renderWebAppendixMarkdown(findings []WebFinding) string {
	sorted := make([]WebFinding, len(findings))
	copy(sorted, findings)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Host != sorted[j].Host {
			return sorted[i].Host < sorted[j].Host
		}
		return sorted[i].ID < sorted[j].ID
	})

	var b strings.Builder
	b.WriteString("---\ntitle: \"Appendix — Web Server Findings\"\n---\n\n")
	fmt.Fprintf(&b, "%d web finding(s) discovered across %d host(s).\n\n", len(sorted), countDistinctHosts(sorted))

	b.WriteString("| Host | Port | ID | Description | References |\n")
	b.WriteString("|---------------|------|--------|--------------------------------------------------|--------------------|\n")
	for _, f := range sorted {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			f.Host, f.Port, f.ID, escapeTableCell(f.Description), escapeTableCell(f.References))
	}

	b.WriteString("\n## Findings by Host\n\n:::web_graph:bar:host:::\n")
	return b.String()
}

func countDistinctHosts(findings []WebFinding) int {
	hosts := map[string]bool{}
	for _, f := range findings {
		hosts[f.Host] = true
	}
	return len(hosts)
}

func escapeTableCell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
