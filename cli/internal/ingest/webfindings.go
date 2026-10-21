package ingest

import (
	"encoding/json"
	"fmt"
	"os"
)

// WebFinding is one nikto finding against a host/port.
type WebFinding struct {
	Host        string `json:"host"`
	Port        string `json:"port"`
	ID          string `json:"id"`
	Description string `json:"description"`
	URI         string `json:"uri"`
	References  string `json:"references"`
}

func loadWebFindings(path string) ([]WebFinding, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var findings []WebFinding
	if err := json.Unmarshal(data, &findings); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return findings, nil
}

func saveWebFindings(path string, findings []WebFinding) error {
	data, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// mergeWebFindings folds newFindings into existing, deduping on
// (Host, Port, ID, URI, Description) — the same finding re-reported by a later
// scan doesn't produce a second row. Description is part of the key because
// nikto reuses one ID across multiple distinct checks for the same URI (e.g. id
// 013587 covers several different "missing security header" findings,
// differentiated only by their description text) — keying on ID+URI alone would
// silently collapse those into one.
func mergeWebFindings(existing, newFindings []WebFinding) []WebFinding {
	seen := make(map[[5]string]bool, len(existing))
	merged := make([]WebFinding, len(existing))
	copy(merged, existing)
	for _, f := range merged {
		seen[[5]string{f.Host, f.Port, f.ID, f.URI, f.Description}] = true
	}

	for _, f := range newFindings {
		key := [5]string{f.Host, f.Port, f.ID, f.URI, f.Description}
		if seen[key] {
			continue
		}
		seen[key] = true
		merged = append(merged, f)
	}
	return merged
}
