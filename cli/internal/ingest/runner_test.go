package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunMissingIndexFailsFast confirms the index.md pre-check runs (and errors
// clearly) before the scan tool is ever invoked, mirroring docker.RunReportBuild's
// index.md preflight in cli/internal/docker/run.go. Covers every wired-up tool —
// nmap/naabu share toolSpec.run, nikto/bloodhound have their own Run functions but
// replicate the same preflight check.
func TestRunMissingIndexFailsFast(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(dir string, args []string) error
	}{
		{"nmap", RunNmap},
		{"naabu", RunNaabu},
		{"nikto", RunNikto},
		{"bloodhound", RunBloodHound},
		{"naabu-existing", RunNaabuExisting},
		{"nmap-existing", RunNmapExisting},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()

			err := tc.run(dir, []string{"-o", "naabu.txt"})
			if err == nil {
				t.Fatal("expected an error for a report dir with no index.md, got nil")
			}
			if !strings.Contains(err.Error(), "index.md not found") {
				t.Errorf("error = %q, want it to mention a missing index.md", err.Error())
			}
		})
	}
}

// TestRunNaabuExistingEndToEnd exercises RunNaabuExisting against a real temp report
// directory, unlike RunNaabu/RunNikto which need the real binary on PATH — proof that
// it never shells out.
func TestRunNaabuExistingEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("---\ntitle: t\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	naabuFile := filepath.Join(dir, "naabu.txt")
	if err := os.WriteFile(naabuFile, []byte("127.0.0.1:41101\n127.0.0.1:22\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RunNaabuExisting(dir, []string{"-o", naabuFile}); err != nil {
		t.Fatalf("RunNaabuExisting: %v", err)
	}

	assets, err := loadAssets(filepath.Join(dir, "artifacts", "assets.json"))
	if err != nil {
		t.Fatalf("loadAssets: %v", err)
	}
	if len(assets) != 1 || assets[0].IP != "127.0.0.1" {
		t.Fatalf("assets = %+v, want one host 127.0.0.1", assets)
	}
	if len(assets[0].Ports) != 2 {
		t.Fatalf("got %d ports, want 2: %+v", len(assets[0].Ports), assets[0].Ports)
	}

	appendix, err := os.ReadFile(filepath.Join(dir, AppendixPath))
	if err != nil {
		t.Fatalf("reading appendix: %v", err)
	}
	// The inventory table itself is built later, at PDF-assembly time, from
	// assets.json (already asserted above) via :::asset_graph:table:ports:::  —
	// the generated appendix markdown only needs to contain that directive.
	if !strings.Contains(string(appendix), ":::asset_graph:table:ports:::") {
		t.Errorf("appendix missing the asset_graph table directive:\n%s", appendix)
	}

	archiveDir := filepath.Join(dir, "artifacts", "naabu")
	entries, err := os.ReadDir(archiveDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected 1 archived file in %s, got %v (err=%v)", archiveDir, entries, err)
	}
}

func TestRunNaabuExistingMissingOutputFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("---\ntitle: t\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunNaabuExisting(dir, []string{"-host", "127.0.0.1"})
	if err == nil {
		t.Fatal("expected an error when -o is missing from args, got nil")
	}
	if !strings.Contains(err.Error(), "-o") {
		t.Errorf("error = %q, want it to mention the missing -o flag", err.Error())
	}
}

// TestRunNmapExistingEndToEnd exercises RunNmapExisting against a real temp report
// directory using the same fixture xml_test.go's TestParseNmapXML parses — proof
// that -oX <file> reuses parseNmapXML unchanged and that RunNmapExisting never
// shells out to nmap.
func TestRunNmapExistingEndToEnd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("---\ntitle: t\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile("testdata/scanme-sample.xml")
	if err != nil {
		t.Fatal(err)
	}
	nmapFile := filepath.Join(dir, "nmap.xml")
	if err := os.WriteFile(nmapFile, fixture, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RunNmapExisting(dir, []string{"-oX", nmapFile}); err != nil {
		t.Fatalf("RunNmapExisting: %v", err)
	}

	assets, err := loadAssets(filepath.Join(dir, "artifacts", "assets.json"))
	if err != nil {
		t.Fatalf("loadAssets: %v", err)
	}
	if len(assets) != 1 || assets[0].IP != "45.33.32.156" {
		t.Fatalf("assets = %+v, want one host 45.33.32.156", assets)
	}
	if len(assets[0].Ports) != 3 {
		t.Fatalf("got %d ports, want 3: %+v", len(assets[0].Ports), assets[0].Ports)
	}

	appendix, err := os.ReadFile(filepath.Join(dir, AppendixPath))
	if err != nil {
		t.Fatalf("reading appendix: %v", err)
	}
	if !strings.Contains(string(appendix), ":::asset_graph:table:ports:::") {
		t.Errorf("appendix missing the asset_graph table directive:\n%s", appendix)
	}

	archiveDir := filepath.Join(dir, "artifacts", "nmap")
	entries, err := os.ReadDir(archiveDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected 1 archived file in %s, got %v (err=%v)", archiveDir, entries, err)
	}
}

func TestRunNmapExistingMissingOutputFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte("---\ntitle: t\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := RunNmapExisting(dir, []string{"-sV", "127.0.0.1"})
	if err == nil {
		t.Fatal("expected an error when -oX is missing from args, got nil")
	}
	if !strings.Contains(err.Error(), "-oX") {
		t.Errorf("error = %q, want it to mention the missing -oX flag", err.Error())
	}
}
