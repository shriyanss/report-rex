package ingest

import (
	"path/filepath"
	"testing"
)

func TestMergeAssetsAddsNewHost(t *testing.T) {
	existing := []Asset{{IP: "10.0.0.1", Ports: []Port{{Number: "22", Proto: "tcp", Service: "ssh"}}}}
	merged := mergeAssets(existing, []Asset{{IP: "10.0.0.2", Ports: []Port{{Number: "80", Proto: "tcp", Service: "http"}}}})

	if len(merged) != 2 {
		t.Fatalf("got %d assets, want 2", len(merged))
	}
}

func TestMergeAssetsUnionsPortsOnExistingHost(t *testing.T) {
	existing := []Asset{{IP: "10.0.0.1", OS: "Linux", Ports: []Port{{Number: "22", Proto: "tcp", Service: "ssh"}}}}
	merged := mergeAssets(existing, []Asset{{IP: "10.0.0.1", Ports: []Port{{Number: "80", Proto: "tcp", Service: "http"}}}})

	if len(merged) != 1 {
		t.Fatalf("got %d assets, want 1 (merged into existing host)", len(merged))
	}
	if merged[0].OS != "Linux" {
		t.Errorf("OS = %q, want existing OS preserved when new scan has none", merged[0].OS)
	}
	if len(merged[0].Ports) != 2 {
		t.Fatalf("got %d ports, want 2 (union of both scans): %+v", len(merged[0].Ports), merged[0].Ports)
	}
}

func TestMergeAssetsUpdatesExistingPort(t *testing.T) {
	existing := []Asset{{IP: "10.0.0.1", Ports: []Port{{Number: "80", Proto: "tcp", Service: "http", Version: "1.0"}}}}
	merged := mergeAssets(existing, []Asset{{IP: "10.0.0.1", Ports: []Port{{Number: "80", Proto: "tcp", Service: "http", Version: "2.0"}}}})

	if len(merged[0].Ports) != 1 {
		t.Fatalf("got %d ports, want 1 (same port re-scanned, not duplicated)", len(merged[0].Ports))
	}
	if merged[0].Ports[0].Version != "2.0" {
		t.Errorf("Version = %q, want the newer scan's value", merged[0].Ports[0].Version)
	}
}

func TestSaveLoadAssetsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.json")
	want := []Asset{{IP: "10.0.0.1", OS: "Linux", Hostnames: []string{"host1"}, Ports: []Port{{Number: "22", Proto: "tcp"}}}}

	if err := saveAssets(path, want); err != nil {
		t.Fatalf("saveAssets: %v", err)
	}
	got, err := loadAssets(path)
	if err != nil {
		t.Fatalf("loadAssets: %v", err)
	}
	if len(got) != 1 || got[0].IP != "10.0.0.1" || got[0].OS != "Linux" {
		t.Errorf("got %+v, want round-tripped %+v", got, want)
	}
}

func TestLoadAssetsMissingFileReturnsEmpty(t *testing.T) {
	got, err := loadAssets(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err != nil {
		t.Fatalf("loadAssets on missing file: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d assets, want 0 for a missing artifacts/assets.json", len(got))
	}
}
