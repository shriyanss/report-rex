package ingest

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/shriyanss/report-rex/cli/internal/fsutil"
)

// ADPrincipal is one AD object (user/computer/group) collected by a SharpHound/
// BloodHound-compatible collector, reduced to the fields the report appendix needs.
type ADPrincipal struct {
	Domain                  string `json:"domain"`
	Name                    string `json:"name"`
	Type                    string `json:"type"` // "user", "computer", or "group"
	Enabled                 bool   `json:"enabled"`
	AdminCount              bool   `json:"admincount"`
	HasSPN                  bool   `json:"hasspn"`
	DontReqPreauth          bool   `json:"dontreqpreauth"`
	UnconstrainedDelegation bool   `json:"unconstraineddelegation"`
}

// ADData is the merged, tool-agnostic AD dataset persisted to
// artifacts/bloodhound.json.
type ADData struct {
	Domains    []string      `json:"domains"`
	Principals []ADPrincipal `json:"principals"`
}

// bhFile is the shape of one SharpHound/BloodHound-CE collector JSON file
// (<timestamp>_<type>.json): a "meta" header plus a "data" array of objects, each
// with a flat "Properties" map. Only Properties is used here — Aces/edges are for
// BloodHound's own graph analysis, out of scope for a report appendix.
type bhFile struct {
	Meta struct {
		Type string `json:"type"`
	} `json:"meta"`
	Data []struct {
		Properties map[string]any `json:"Properties"`
	} `json:"data"`
}

func bhProp(props map[string]any, key string) string {
	v, _ := props[key].(string)
	return v
}

func bhBool(props map[string]any, key string) bool {
	v, _ := props[key].(bool)
	return v
}

// parseBloodHoundDir parses every *_<type>.json file in dir (as extracted from a
// collector zip, or a directory the user already extracted one into) into an
// ADData. Unrecognized/unrelated files are skipped rather than erroring, since a
// collector export commonly includes files this report tool doesn't use
// (certtemplates, ous, gpos, containers, ntauthstores, ...).
func parseBloodHoundDir(dir string) (ADData, error) {
	var data ADData
	domains := map[string]bool{}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return data, fmt.Errorf("reading %s: %w", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return data, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		var f bhFile
		if err := json.Unmarshal(raw, &f); err != nil {
			continue // not a collector JSON file we recognize; skip it
		}

		switch f.Meta.Type {
		case "domains":
			for _, obj := range f.Data {
				if name := bhProp(obj.Properties, "name"); name != "" {
					domains[name] = true
				}
			}
		case "users", "computers", "groups":
			ptype := strings.TrimSuffix(f.Meta.Type, "s")
			for _, obj := range f.Data {
				name := bhProp(obj.Properties, "name")
				if name == "" {
					continue
				}
				data.Principals = append(data.Principals, ADPrincipal{
					Domain:                  bhProp(obj.Properties, "domain"),
					Name:                    name,
					Type:                    ptype,
					Enabled:                 bhBool(obj.Properties, "enabled"),
					AdminCount:              bhBool(obj.Properties, "admincount"),
					HasSPN:                  bhBool(obj.Properties, "hasspn"),
					DontReqPreauth:          bhBool(obj.Properties, "dontreqpreauth"),
					UnconstrainedDelegation: bhBool(obj.Properties, "unconstraineddelegation"),
				})
			}
		}
	}

	for d := range domains {
		data.Domains = append(data.Domains, d)
	}
	return data, nil
}

// extractZip extracts a zip file's flat contents into destDir (no subdirectories
// expected in a collector export).
func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		src, err := f.Open()
		if err != nil {
			return err
		}
		dest, err := os.Create(filepath.Join(destDir, filepath.Base(f.Name)))
		if err != nil {
			src.Close()
			return err
		}
		_, copyErr := io.Copy(dest, src)
		src.Close()
		dest.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func loadADData(path string) (ADData, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ADData{}, nil
	}
	if err != nil {
		return ADData{}, fmt.Errorf("reading %s: %w", path, err)
	}
	var d ADData
	if err := json.Unmarshal(data, &d); err != nil {
		return ADData{}, fmt.Errorf("parsing %s: %w", path, err)
	}
	return d, nil
}

func saveADData(path string, d ADData) error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// mergeADData folds a freshly parsed collection into existing, deduping principals
// by (Domain, Type, Name) and domains by name.
func mergeADData(existing, fresh ADData) ADData {
	domains := map[string]bool{}
	for _, d := range existing.Domains {
		domains[d] = true
	}
	for _, d := range fresh.Domains {
		domains[d] = true
	}

	type key struct{ domain, ptype, name string }
	seen := map[key]int{}
	merged := make([]ADPrincipal, len(existing.Principals))
	copy(merged, existing.Principals)
	for i, p := range merged {
		seen[key{p.Domain, p.Type, p.Name}] = i
	}
	for _, p := range fresh.Principals {
		k := key{p.Domain, p.Type, p.Name}
		if i, ok := seen[k]; ok {
			merged[i] = p // newer collection wins for this principal
			continue
		}
		seen[k] = len(merged)
		merged = append(merged, p)
	}

	out := ADData{Principals: merged}
	for d := range domains {
		out.Domains = append(out.Domains, d)
	}
	return out
}

// RunBloodHound imports an already-collected SharpHound/BloodHound-CE export (a
// .zip, or a directory of its extracted *_<type>.json files) into dir's report.
// Unlike nmap/naabu/nikto, this never runs a live collector itself — BloodHound
// collection needs real domain credentials and network access to the target
// Active Directory environment, which is a separate, out-of-band step (SharpHound
// on Windows, or bloodhound-python) the user runs themselves before handing the
// export to `rex ingest bloodhound`.
func RunBloodHound(dir string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("rex ingest bloodhound requires a path to a collector .zip or extracted directory")
	}
	srcPath := args[0]

	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving --dir: %w", err)
	}
	indexPath := filepath.Join(dirAbs, "index.md")
	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("index.md not found in %s (resolved from --dir %q) — run this from inside a report directory: %w", dirAbs, dir, err)
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", srcPath, err)
	}

	extractDir := srcPath
	if !info.IsDir() {
		tmpDir, err := os.MkdirTemp("", "rex-bloodhound-")
		if err != nil {
			return fmt.Errorf("creating temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)
		if err := extractZip(srcPath, tmpDir); err != nil {
			return fmt.Errorf("extracting %s: %w", srcPath, err)
		}
		extractDir = tmpDir
	}

	fresh, err := parseBloodHoundDir(extractDir)
	if err != nil {
		return fmt.Errorf("parsing BloodHound export: %w", err)
	}

	artifactsDir := filepath.Join(dirAbs, "artifacts")
	archiveDir := filepath.Join(artifactsDir, "bloodhound")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", archiveDir, err)
	}
	if !info.IsDir() {
		if err := copyFile(srcPath, filepath.Join(archiveDir, filepath.Base(srcPath))); err != nil {
			return fmt.Errorf("archiving %s: %w", srcPath, err)
		}
	} else if err := fsutil.CopyFS(os.DirFS(extractDir), archiveDir); err != nil {
		return fmt.Errorf("archiving %s: %w", srcPath, err)
	}

	adPath := filepath.Join(artifactsDir, "bloodhound.json")
	existing, err := loadADData(adPath)
	if err != nil {
		return err
	}
	merged := mergeADData(existing, fresh)
	if err := saveADData(adPath, merged); err != nil {
		return fmt.Errorf("writing %s: %w", adPath, err)
	}

	if err := writeAppendix(dirAbs, ADAppendixPath, renderADAppendixMarkdown(merged), timelineDesc("Active Directory enumeration", "BloodHound", "of", fresh.Domains)); err != nil {
		return err
	}

	fmt.Printf("Ingested %d AD principal(s) across %d domain(s) from BloodHound export into %s\n",
		len(fresh.Principals), len(fresh.Domains), ADAppendixPath)
	return nil
}

func copyFile(src, dest string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644)
}
