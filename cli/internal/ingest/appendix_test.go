package ingest

import (
	"strings"
	"testing"
)

func TestRenderAppendixMarkdown(t *testing.T) {
	assets := []Asset{
		{IP: "10.0.0.2", Hostnames: []string{"b.example.com"}, OS: "Linux", Ports: []Port{{Number: "80", Proto: "tcp", Service: "http"}}},
		{IP: "10.0.0.1", Hostnames: []string{"a.example.com"}, Ports: []Port{{Number: "22", Proto: "tcp", Service: "ssh"}}},
	}

	md := renderAppendixMarkdown(assets)

	// The inventory table itself is now built by the :::asset_graph:table:ports:::
	// directive at PDF-assembly time (see charts.RenderAssets/_asset_matrix), not
	// here — this just needs to emit the summary line and the three directives.
	for _, want := range []string{
		`title: "Appendix — Discovered Assets"`,
		":::asset_graph:table:ports:::",
		":::asset_graph:bar:ports:::",
		":::asset_graph:pie:os:::",
		"2 host(s) scanned, 2 open port(s) discovered.",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("appendix markdown missing %q\n---\n%s", want, md)
		}
	}
}

func TestRenderAppendixMarkdownEmptyAssetsSummary(t *testing.T) {
	md := renderAppendixMarkdown(nil)
	if !strings.Contains(md, "0 host(s) scanned, 0 open port(s) discovered.") {
		t.Errorf("expected a zeroed summary line for no assets:\n%s", md)
	}
}
