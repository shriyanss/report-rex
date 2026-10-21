package ingest

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The BloodHound-CE/SharpHound collector JSON schema below (meta.type + a flat
// Properties map per object) matches the official public sample dataset at
// https://github.com/SpecterOps/BloodHound-Docs/blob/main/docs/assets/sample-data/ad_sampledata.zip
// — these fixtures are a hand-trimmed, schema-accurate subset rather than that
// full ~350KB archive, to keep the repo's test data small.

const sampleBHDomains = `{"meta":{"type":"domains","count":1},"data":[
  {"Properties":{"name":"CORP.LOCAL"}}
]}`

const sampleBHUsers = `{"meta":{"type":"users","count":3},"data":[
  {"Properties":{"domain":"CORP.LOCAL","name":"KRBTGT@CORP.LOCAL","enabled":false,"admincount":true,"hasspn":true,"dontreqpreauth":false,"unconstraineddelegation":false}},
  {"Properties":{"domain":"CORP.LOCAL","name":"SVC-SQL@CORP.LOCAL","enabled":true,"admincount":false,"hasspn":true,"dontreqpreauth":false,"unconstraineddelegation":false}},
  {"Properties":{"domain":"CORP.LOCAL","name":"JDOE@CORP.LOCAL","enabled":true,"admincount":false,"hasspn":false,"dontreqpreauth":true,"unconstraineddelegation":false}}
]}`

const sampleBHComputers = `{"meta":{"type":"computers","count":2},"data":[
  {"Properties":{"domain":"CORP.LOCAL","name":"DC01.CORP.LOCAL","enabled":true,"admincount":true,"unconstraineddelegation":false}},
  {"Properties":{"domain":"CORP.LOCAL","name":"LEGACYAPP01.CORP.LOCAL","enabled":true,"admincount":false,"unconstraineddelegation":true}}
]}`

const sampleBHGroups = `{"meta":{"type":"groups","count":1},"data":[
  {"Properties":{"domain":"CORP.LOCAL","name":"DOMAIN ADMINS@CORP.LOCAL","admincount":true}}
]}`

func writeBHFixtures(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"20240101000000_domains.json":   sampleBHDomains,
		"20240101000000_users.json":     sampleBHUsers,
		"20240101000000_computers.json": sampleBHComputers,
		"20240101000000_groups.json":    sampleBHGroups,
		"20240101000000_gpos.json":      `{"meta":{"type":"gpos","count":0},"data":[]}`, // unrecognized type; must be skipped, not error
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestParseBloodHoundDir(t *testing.T) {
	dir := t.TempDir()
	writeBHFixtures(t, dir)

	data, err := parseBloodHoundDir(dir)
	if err != nil {
		t.Fatalf("parseBloodHoundDir: %v", err)
	}
	if len(data.Domains) != 1 || data.Domains[0] != "CORP.LOCAL" {
		t.Errorf("Domains = %v, want [CORP.LOCAL]", data.Domains)
	}
	if len(data.Principals) != 6 { // 3 users + 2 computers + 1 group
		t.Fatalf("got %d principals, want 6: %+v", len(data.Principals), data.Principals)
	}

	var krbtgt, jdoe *ADPrincipal
	for i, p := range data.Principals {
		if p.Name == "KRBTGT@CORP.LOCAL" {
			krbtgt = &data.Principals[i]
		}
		if p.Name == "JDOE@CORP.LOCAL" {
			jdoe = &data.Principals[i]
		}
	}
	if krbtgt == nil || !krbtgt.AdminCount || !krbtgt.HasSPN {
		t.Errorf("KRBTGT principal = %+v, want AdminCount=true HasSPN=true", krbtgt)
	}
	if jdoe == nil || !jdoe.DontReqPreauth {
		t.Errorf("JDOE principal = %+v, want DontReqPreauth=true", jdoe)
	}
}

func TestParseBloodHoundDirFromZip(t *testing.T) {
	srcDir := t.TempDir()
	writeBHFixtures(t, srcDir)

	zipPath := filepath.Join(t.TempDir(), "export.zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(zf)
	entries, _ := os.ReadDir(srcDir)
	for _, e := range entries {
		content, _ := os.ReadFile(filepath.Join(srcDir, e.Name()))
		w, err := zw.Create(e.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	zw.Close()
	zf.Close()

	extractDir := t.TempDir()
	if err := extractZip(zipPath, extractDir); err != nil {
		t.Fatalf("extractZip: %v", err)
	}
	data, err := parseBloodHoundDir(extractDir)
	if err != nil {
		t.Fatalf("parseBloodHoundDir: %v", err)
	}
	if len(data.Principals) != 6 {
		t.Errorf("got %d principals from zip round-trip, want 6", len(data.Principals))
	}
}

func TestMergeADDataDedupesAndUpdates(t *testing.T) {
	existing := ADData{
		Domains:    []string{"CORP.LOCAL"},
		Principals: []ADPrincipal{{Domain: "CORP.LOCAL", Name: "JDOE@CORP.LOCAL", Type: "user", Enabled: true}},
	}
	fresh := ADData{
		Domains: []string{"CORP.LOCAL", "CHILD.CORP.LOCAL"},
		Principals: []ADPrincipal{
			{Domain: "CORP.LOCAL", Name: "JDOE@CORP.LOCAL", Type: "user", Enabled: true, AdminCount: true}, // updated
			{Domain: "CORP.LOCAL", Name: "ASMITH@CORP.LOCAL", Type: "user", Enabled: true},                 // new
		},
	}
	merged := mergeADData(existing, fresh)

	if len(merged.Domains) != 2 {
		t.Errorf("got %d domains, want 2: %v", len(merged.Domains), merged.Domains)
	}
	if len(merged.Principals) != 2 {
		t.Fatalf("got %d principals, want 2 (JDOE deduped, ASMITH added): %+v", len(merged.Principals), merged.Principals)
	}
	for _, p := range merged.Principals {
		if p.Name == "JDOE@CORP.LOCAL" && !p.AdminCount {
			t.Error("JDOE should have been updated with AdminCount=true from the fresh scan")
		}
	}
}

func TestRenderADAppendixMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeBHFixtures(t, dir)
	data, err := parseBloodHoundDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	md := renderADAppendixMarkdown(data)
	for _, want := range []string{
		`title: "Appendix — Active Directory Enumeration"`,
		"Domains collected: CORP.LOCAL.",
		"| users | 3 |",
		"| computers | 2 |",
		"| groups | 1 |",
		"High-Value Principals",
		"KRBTGT@CORP.LOCAL", // admincount
		"Kerberoastable Accounts",
		"SVC-SQL@CORP.LOCAL", // hasspn && enabled
		"AS-REP Roastable Accounts",
		"JDOE@CORP.LOCAL", // dontreqpreauth && enabled
		"Unconstrained Delegation",
		"LEGACYAPP01.CORP.LOCAL",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("appendix markdown missing %q\n---\n%s", want, md)
		}
	}
}
