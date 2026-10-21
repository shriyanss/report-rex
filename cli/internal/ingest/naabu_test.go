package ingest

import "testing"

func TestParseNaabuJSON(t *testing.T) {
	data := []byte(`{"host":"scanme.nmap.org","ip":"45.33.32.156","port":22,"protocol":"tcp"}
{"host":"scanme.nmap.org","ip":"45.33.32.156","port":22,"protocol":"tcp"}
{"host":"scanme.nmap.org","ip":"45.33.32.156","port":80,"protocol":"tcp"}
`)

	assets, err := parseNaabuJSON(data)
	if err != nil {
		t.Fatalf("parseNaabuJSON: %v", err)
	}
	// Three lines in, but the first two are the same (ip, port) — mergeAssets is what
	// folds them together, so at the parse stage they're still separate entries.
	if len(assets) != 3 {
		t.Fatalf("got %d parsed entries, want 3", len(assets))
	}

	merged := mergeAssets(nil, assets)
	if len(merged) != 1 {
		t.Fatalf("got %d merged hosts, want 1", len(merged))
	}
	if len(merged[0].Ports) != 2 {
		t.Fatalf("got %d ports, want 2 (22 and 80, deduped): %+v", len(merged[0].Ports), merged[0].Ports)
	}
	if !containsStr(merged[0].Hostnames, "scanme.nmap.org") {
		t.Errorf("Hostnames = %v, want scanme.nmap.org", merged[0].Hostnames)
	}
}

func TestParseNaabuJSONSkipsBlankLines(t *testing.T) {
	data := []byte("\n{\"ip\":\"10.0.0.1\",\"port\":443,\"protocol\":\"tcp\"}\n\n")

	assets, err := parseNaabuJSON(data)
	if err != nil {
		t.Fatalf("parseNaabuJSON: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("got %d assets, want 1", len(assets))
	}
}

func TestParseNaabuJSONOmitsHostnameWhenSameAsIP(t *testing.T) {
	data := []byte(`{"host":"10.0.0.1","ip":"10.0.0.1","port":22,"protocol":"tcp"}`)

	assets, err := parseNaabuJSON(data)
	if err != nil {
		t.Fatalf("parseNaabuJSON: %v", err)
	}
	if len(assets[0].Hostnames) != 0 {
		t.Errorf("Hostnames = %v, want empty when host == ip (no real hostname resolved)", assets[0].Hostnames)
	}
}

func TestParseNaabuText(t *testing.T) {
	// The ticket's exact sample: naabu's default (non -json) output.
	data := []byte("127.0.0.1:41101\n127.0.0.1:22\n")

	assets, err := parseNaabuText(data)
	if err != nil {
		t.Fatalf("parseNaabuText: %v", err)
	}
	if len(assets) != 2 {
		t.Fatalf("got %d parsed entries, want 2", len(assets))
	}

	merged := mergeAssets(nil, assets)
	if len(merged) != 1 {
		t.Fatalf("got %d merged hosts, want 1", len(merged))
	}
	if merged[0].IP != "127.0.0.1" {
		t.Errorf("IP = %q, want 127.0.0.1", merged[0].IP)
	}
	if len(merged[0].Ports) != 2 {
		t.Fatalf("got %d ports, want 2 (22 and 41101): %+v", len(merged[0].Ports), merged[0].Ports)
	}
	for _, p := range merged[0].Ports {
		if p.Proto != "tcp" {
			t.Errorf("Port %s Proto = %q, want tcp", p.Number, p.Proto)
		}
	}
}

func TestParseNaabuTextSkipsBlankLines(t *testing.T) {
	data := []byte("\n10.0.0.1:443\n\n")

	assets, err := parseNaabuText(data)
	if err != nil {
		t.Fatalf("parseNaabuText: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("got %d assets, want 1", len(assets))
	}
}

func TestParseNaabuTextRejectsMalformedLine(t *testing.T) {
	if _, err := parseNaabuText([]byte("not-a-host-port-pair\n")); err == nil {
		t.Fatal("expected an error for a line without :port, got nil")
	}
}
