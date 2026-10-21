package ingest

import (
	"os"
	"testing"
)

func TestParseNmapXML(t *testing.T) {
	data, err := os.ReadFile("testdata/scanme-sample.xml")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	assets, err := parseNmapXML(data)
	if err != nil {
		t.Fatalf("parseNmapXML: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("got %d assets, want 1", len(assets))
	}

	a := assets[0]
	if a.IP != "45.33.32.156" {
		t.Errorf("IP = %q, want 45.33.32.156", a.IP)
	}
	if !containsStr(a.Hostnames, "scanme.nmap.org") {
		t.Errorf("Hostnames = %v, want to include scanme.nmap.org", a.Hostnames)
	}

	// Fixture has ports 22, 80, 9929 open and 139, 646, 1434, 12345, 31337 filtered —
	// only open ports should survive.
	if len(a.Ports) != 3 {
		t.Fatalf("got %d open ports, want 3: %+v", len(a.Ports), a.Ports)
	}
	for _, p := range a.Ports {
		if p.Number == "139" {
			t.Errorf("filtered port 139 leaked into open ports: %+v", p)
		}
	}

	found := false
	for _, p := range a.Ports {
		if p.Number == "22" {
			found = true
			if p.Service != "ssh" || p.Product != "OpenSSH" {
				t.Errorf("port 22 = %+v, want service=ssh product=OpenSSH", p)
			}
		}
	}
	if !found {
		t.Error("expected open port 22 in results")
	}
}

func TestParseNmapXMLSkipsDownHosts(t *testing.T) {
	data := []byte(`<?xml version="1.0"?><nmaprun>
<host><status state="down"/><address addr="10.0.0.1" addrtype="ipv4"/></host>
</nmaprun>`)

	assets, err := parseNmapXML(data)
	if err != nil {
		t.Fatalf("parseNmapXML: %v", err)
	}
	if len(assets) != 0 {
		t.Errorf("got %d assets, want 0 for a down host", len(assets))
	}
}
