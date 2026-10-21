package ingest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// NucleiAppendixPath is the path (relative to the report directory) of the
// generated nuclei-findings appendix.
const NucleiAppendixPath = "appendices/nuclei.md"

// NucleiFinding is one nuclei match against a target.
type NucleiFinding struct {
	Host       string `json:"host"`
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	Severity   string `json:"severity"`
	MatchedAt  string `json:"matched_at"`
}

// nucleiResult is the subset of a `nuclei -jsonl` output line used here.
type nucleiResult struct {
	TemplateID string `json:"template-id"`
	Host       string `json:"host"`
	MatchedAt  string `json:"matched-at"`
	Info       struct {
		Name     string `json:"name"`
		Severity string `json:"severity"`
	} `json:"info"`
}

// parseNucleiJSONL parses `nuclei -jsonl` output (one JSON object per line) into
// NucleiFinding. Blank lines are skipped and unknown fields are ignored.
func parseNucleiJSONL(data []byte) ([]NucleiFinding, error) {
	var findings []NucleiFinding
	scanner := bufio.NewScanner(bytes.NewReader(data))
	// nuclei lines can be long (full matcher output); raise the scanner's cap.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var r nucleiResult
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		findings = append(findings, NucleiFinding{
			Host:       r.Host,
			TemplateID: r.TemplateID,
			Name:       r.Info.Name,
			Severity:   r.Info.Severity,
			MatchedAt:  r.MatchedAt,
		})
	}
	return findings, scanner.Err()
}

func loadNucleiFindings(path string) ([]NucleiFinding, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var findings []NucleiFinding
	if err := json.Unmarshal(data, &findings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return findings, nil
}

func saveNucleiFindings(path string, findings []NucleiFinding) error {
	data, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// mergeNucleiFindings folds newFindings into existing, deduping on
// (TemplateID, MatchedAt) — the same template matching the same URL in a later
// scan doesn't produce a second row.
func mergeNucleiFindings(existing, newFindings []NucleiFinding) []NucleiFinding {
	seen := make(map[[2]string]bool, len(existing))
	merged := make([]NucleiFinding, len(existing))
	copy(merged, existing)
	for _, f := range merged {
		seen[[2]string{f.TemplateID, f.MatchedAt}] = true
	}

	for _, f := range newFindings {
		key := [2]string{f.TemplateID, f.MatchedAt}
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, f)
	}
	return merged
}

// severityRank orders findings by how serious they are, most serious first.
func severityRank(sev string) int {
	switch strings.ToLower(sev) {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	case "low":
		return 3
	case "info":
		return 4
	default:
		return 5
	}
}

// renderNucleiAppendixMarkdown regenerates the nuclei appendix in full from the
// merged finding list, sorted by severity (most serious first), mirroring
// renderWebAppendixMarkdown's always-overwrite approach.
func renderNucleiAppendixMarkdown(findings []NucleiFinding) string {
	sorted := make([]NucleiFinding, len(findings))
	copy(sorted, findings)
	sort.SliceStable(sorted, func(i, j int) bool {
		if ri, rj := severityRank(sorted[i].Severity), severityRank(sorted[j].Severity); ri != rj {
			return ri < rj
		}
		if sorted[i].Host != sorted[j].Host {
			return sorted[i].Host < sorted[j].Host
		}
		return sorted[i].TemplateID < sorted[j].TemplateID
	})

	var b strings.Builder
	b.WriteString("---\ntitle: \"Appendix — Nuclei Findings\"\n---\n\n")
	fmt.Fprintf(&b, "%d nuclei finding(s) discovered across %d host(s).\n\n", len(sorted), countDistinctNucleiHosts(sorted))

	b.WriteString("| Host | Severity | Template | Name | Matched At |\n")
	b.WriteString("|---------------|----------|--------------------|--------------------------------|--------------------|\n")
	for _, f := range sorted {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			escapeTableCell(f.Host), f.Severity, escapeTableCell(f.TemplateID),
			escapeTableCell(f.Name), escapeTableCell(f.MatchedAt))
	}
	return b.String()
}

func countDistinctNucleiHosts(findings []NucleiFinding) int {
	hosts := map[string]bool{}
	for _, f := range findings {
		hosts[f.Host] = true
	}
	return len(hosts)
}

// RunNuclei runs nuclei against dir's report: shells out to the nuclei binary,
// merges the parsed findings into artifacts/nuclei.json, archives the raw JSONL
// under artifacts/nuclei/, regenerates appendices/nuclei.md, and registers that
// appendix in index.md.
func RunNuclei(dir string, args []string) error {
	dirAbs, err := resolveReportDir(dir)
	if err != nil {
		return err
	}

	bin, err := exec.LookPath("nuclei")
	if err != nil {
		return fmt.Errorf("nuclei not found in PATH: %w", err)
	}

	cmd := exec.Command(bin, append(append([]string{}, args...), "-jsonl", "-silent")...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("running nuclei: %w", err)
	}

	scanned, err := parseNucleiJSONL(out)
	if err != nil {
		return fmt.Errorf("parsing nuclei output: %w", err)
	}

	artifactsDir := filepath.Join(dirAbs, "artifacts")
	archiveDir := filepath.Join(artifactsDir, "nuclei")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", archiveDir, err)
	}
	archivePath := filepath.Join(archiveDir, fmt.Sprintf("%d.jsonl", time.Now().Unix()))
	if err := os.WriteFile(archivePath, out, 0o644); err != nil {
		return fmt.Errorf("archiving raw scan to %s: %w", archivePath, err)
	}

	findingsPath := filepath.Join(artifactsDir, "nuclei.json")
	existing, err := loadNucleiFindings(findingsPath)
	if err != nil {
		return err
	}
	merged := mergeNucleiFindings(existing, scanned)
	if err := saveNucleiFindings(findingsPath, merged); err != nil {
		return fmt.Errorf("writing %s: %w", findingsPath, err)
	}

	hosts := make([]string, len(scanned))
	for i, f := range scanned {
		hosts[i] = f.Host
	}
	if err := writeAppendix(dirAbs, NucleiAppendixPath, renderNucleiAppendixMarkdown(merged), timelineDesc("Vulnerability scan", "nuclei", "against", hosts)); err != nil {
		return err
	}

	fmt.Printf("Ingested %d finding(s) from nuclei into %s\n", len(scanned), NucleiAppendixPath)
	return nil
}
