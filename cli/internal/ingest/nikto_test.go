package ingest

import (
	"strings"
	"testing"
)

const sampleNiktoXML = `<?xml version="1.0"?>
<niktoscans>
  <niktoscan>
    <scandetails targetip="10.0.0.1" targethostname="example.com" targetport="80">
      <item id="600050" method="HEAD">
        <description>Apache/2.4.7 appears to be outdated.</description>
        <uri>/</uri>
        <namelink></namelink>
        <iplink></iplink>
        <references></references>
      </item>
      <item id="013587" method="GET">
        <description>Suggested security header missing: x-content-type-options.</description>
        <uri>/</uri>
        <namelink></namelink>
        <iplink></iplink>
        <references>https://example.com/ref</references>
      </item>
    </scandetails>
  </niktoscan>
</niktoscans>`

func TestParseNiktoXML(t *testing.T) {
	findings, err := parseNiktoXML([]byte(sampleNiktoXML))
	if err != nil {
		t.Fatalf("parseNiktoXML: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("got %d findings, want 2", len(findings))
	}
	if findings[0].Host != "example.com" || findings[0].Port != "80" {
		t.Errorf("findings[0] = %+v, want host=example.com port=80", findings[0])
	}
	if findings[1].References != "https://example.com/ref" {
		t.Errorf("References = %q, want the ref URL", findings[1].References)
	}
}

func TestParseNiktoXMLFallsBackToIPWithoutHostname(t *testing.T) {
	data := `<niktoscans><niktoscan><scandetails targetip="10.0.0.5" targethostname="" targetport="80">
      <item id="1"><description>d</description><uri>/</uri></item>
    </scandetails></niktoscan></niktoscans>`
	findings, err := parseNiktoXML([]byte(data))
	if err != nil {
		t.Fatalf("parseNiktoXML: %v", err)
	}
	if findings[0].Host != "10.0.0.5" {
		t.Errorf("Host = %q, want 10.0.0.5 fallback", findings[0].Host)
	}
}

func TestMergeWebFindingsDedupes(t *testing.T) {
	existing := []WebFinding{{Host: "a.com", Port: "80", ID: "1", URI: "/"}}
	fresh := []WebFinding{
		{Host: "a.com", Port: "80", ID: "1", URI: "/"},      // duplicate
		{Host: "a.com", Port: "80", ID: "2", URI: "/admin"}, // new
	}
	merged := mergeWebFindings(existing, fresh)
	if len(merged) != 2 {
		t.Fatalf("got %d findings, want 2 (dedup on Host/Port/ID/URI/Description)", len(merged))
	}
}

// nikto reuses one finding ID across several distinct checks against the same
// URI (e.g. id 013587 covers multiple different missing-security-header
// findings) — only their Description differs, so Description must be part of
// the dedup key or these collapse into a single row.
func TestMergeWebFindingsKeepsSameIDDistinctDescriptions(t *testing.T) {
	fresh := []WebFinding{
		{Host: "a.com", Port: "80", ID: "013587", URI: "/", Description: "missing x-content-type-options"},
		{Host: "a.com", Port: "80", ID: "013587", URI: "/", Description: "missing content-security-policy"},
	}
	merged := mergeWebFindings(nil, fresh)
	if len(merged) != 2 {
		t.Fatalf("got %d findings, want 2 distinct findings kept despite sharing an ID: %+v", len(merged), merged)
	}
}

func TestRenderWebAppendixMarkdown(t *testing.T) {
	findings := []WebFinding{
		{Host: "a.com", Port: "80", ID: "1", Description: "issue one", References: "http://ref"},
		{Host: "b.com", Port: "443", ID: "2", Description: "issue two"},
	}
	md := renderWebAppendixMarkdown(findings)
	for _, want := range []string{
		`title: "Appendix — Web Server Findings"`,
		"| Host | Port | ID | Description | References |",
		"a.com", "b.com", "issue one", "issue two",
		":::web_graph:bar:host:::",
		"2 web finding(s) discovered across 2 host(s).",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("appendix markdown missing %q\n---\n%s", want, md)
		}
	}
}
