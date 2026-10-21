package ingest

import (
	"encoding/json"
	"fmt"
	"os"
)

// Port is one open port on an Asset.
type Port struct {
	Number  string `json:"number"`
	Proto   string `json:"proto"`
	Service string `json:"service"`
	Product string `json:"product"`
	Version string `json:"version"`
}

// Asset is one scanned host, merged across every ingest run that has touched it.
type Asset struct {
	IP        string   `json:"ip"`
	Hostnames []string `json:"hostnames"`
	OS        string   `json:"os"`
	Ports     []Port   `json:"ports"`
}

// loadAssets reads artifacts/assets.json, returning an empty slice if it doesn't exist yet.
func loadAssets(path string) ([]Asset, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var assets []Asset
	if err := json.Unmarshal(data, &assets); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return assets, nil
}

// saveAssets writes assets back to artifacts/assets.json, pretty-printed.
func saveAssets(path string, assets []Asset) error {
	data, err := json.MarshalIndent(assets, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// mergeAssets folds newAssets into existing, keyed by IP: an existing host's OS and
// hostnames are replaced by the newer scan's values (when present), and ports are
// unioned by (number, proto). A host not seen before is appended.
func mergeAssets(existing, newAssets []Asset) []Asset {
	byIP := make(map[string]int, len(existing))
	merged := make([]Asset, len(existing))
	copy(merged, existing)
	for i, a := range merged {
		byIP[a.IP] = i
	}

	for _, na := range newAssets {
		idx, ok := byIP[na.IP]
		if !ok {
			byIP[na.IP] = len(merged)
			merged = append(merged, na)
			continue
		}

		cur := &merged[idx]
		if na.OS != "" {
			cur.OS = na.OS
		}
		for _, h := range na.Hostnames {
			if !containsStr(cur.Hostnames, h) {
				cur.Hostnames = append(cur.Hostnames, h)
			}
		}
		for _, p := range na.Ports {
			if i := findPort(cur.Ports, p.Number, p.Proto); i >= 0 {
				cur.Ports[i] = p
			} else {
				cur.Ports = append(cur.Ports, p)
			}
		}
	}
	return merged
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func findPort(ports []Port, number, proto string) int {
	for i, p := range ports {
		if p.Number == number && p.Proto == proto {
			return i
		}
	}
	return -1
}
