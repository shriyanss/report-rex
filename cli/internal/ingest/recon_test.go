package ingest

import (
	"reflect"
	"testing"
)

func TestWebTargets(t *testing.T) {
	assets := []Asset{
		{IP: "10.0.0.1", Hostnames: []string{"web.example.com"}, Ports: []Port{
			{Number: "80", Proto: "tcp", Service: "http"},
			{Number: "443", Proto: "tcp", Service: "https"},
			{Number: "22", Proto: "tcp", Service: "ssh"}, // not web
		}},
		{IP: "10.0.0.2", Ports: []Port{
			{Number: "8080", Proto: "tcp", Service: "http-proxy"}, // web by service
			{Number: "8443", Proto: "tcp", Service: "unknown"},    // web by port number
			{Number: "3306", Proto: "tcp", Service: "mysql"},      // not web
		}},
	}
	got := webTargets(assets)
	want := []string{
		"http://web.example.com",  // 80 omitted, hostname preferred
		"https://10.0.0.2:8443",   // port kept
		"https://web.example.com", // 443 omitted
		"http://10.0.0.2:8080",    // service-detected
	}
	// want is pre-sorted to match webTargets' sorted output.
	wantSorted := []string{
		"http://10.0.0.2:8080",
		"http://web.example.com",
		"https://10.0.0.2:8443",
		"https://web.example.com",
	}
	_ = want
	if !reflect.DeepEqual(got, wantSorted) {
		t.Errorf("webTargets() =\n  %#v\nwant\n  %#v", got, wantSorted)
	}
}

func TestWebTargetsDedupes(t *testing.T) {
	assets := []Asset{
		{IP: "10.0.0.1", Ports: []Port{{Number: "80", Service: "http"}}},
		{IP: "10.0.0.1", Ports: []Port{{Number: "80", Service: "http"}}},
	}
	if got := webTargets(assets); len(got) != 1 {
		t.Errorf("webTargets() = %v, want 1 deduped entry", got)
	}
}

func TestWebTargetsNoneWhenNoWebPorts(t *testing.T) {
	assets := []Asset{{IP: "10.0.0.1", Ports: []Port{{Number: "22", Service: "ssh"}}}}
	if got := webTargets(assets); len(got) != 0 {
		t.Errorf("webTargets() = %v, want empty", got)
	}
}
