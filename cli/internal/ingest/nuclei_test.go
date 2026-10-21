package ingest

import (
	"strings"
	"testing"
)

const sampleNucleiJSONL = `{"template-id":"tech-detect","info":{"name":"Technology Detection","severity":"info"},"host":"http://10.0.0.1","matched-at":"http://10.0.0.1:80"}
{"template-id":"CVE-2021-1234","info":{"name":"Critical RCE","severity":"critical"},"host":"https://10.0.0.1","matched-at":"https://10.0.0.1:443/app"}

{"template-id":"missing-header","info":{"name":"Missing Header","severity":"low"},"host":"http://10.0.0.2","matched-at":"http://10.0.0.2:8080"}`

func TestParseNucleiJSONL(t *testing.T) {
	findings, err := parseNucleiJSONL([]byte(sampleNucleiJSONL))
	if err != nil {
		t.Fatalf("parseNucleiJSONL: %v", err)
	}
	if len(findings) != 3 {
		t.Fatalf("got %d findings, want 3 (blank line skipped)", len(findings))
	}
	if findings[1].TemplateID != "CVE-2021-1234" || findings[1].Severity != "critical" {
		t.Errorf("findings[1] = %+v, want CVE-2021-1234/critical", findings[1])
	}
	if findings[1].Name != "Critical RCE" || findings[1].MatchedAt != "https://10.0.0.1:443/app" {
		t.Errorf("findings[1] name/matched-at = %+v", findings[1])
	}
}

func TestMergeNucleiFindingsDedupes(t *testing.T) {
	existing := []NucleiFinding{{TemplateID: "t1", MatchedAt: "http://a/1"}}
	merged := mergeNucleiFindings(existing, []NucleiFinding{
		{TemplateID: "t1", MatchedAt: "http://a/1"}, // dup
		{TemplateID: "t1", MatchedAt: "http://a/2"}, // same template, different URL — kept
		{TemplateID: "t2", MatchedAt: "http://a/1"}, // same URL, different template — kept
	})
	if len(merged) != 3 {
		t.Fatalf("got %d findings, want 3 after dedup: %+v", len(merged), merged)
	}
}

func TestRenderNucleiAppendixSortsBySeverity(t *testing.T) {
	md := renderNucleiAppendixMarkdown([]NucleiFinding{
		{Host: "h", TemplateID: "a", Severity: "low", MatchedAt: "u1"},
		{Host: "h", TemplateID: "b", Severity: "critical", MatchedAt: "u2"},
		{Host: "h", TemplateID: "c", Severity: "medium", MatchedAt: "u3"},
	})
	ci := strings.Index(md, "critical")
	mi := strings.Index(md, "medium")
	li := strings.Index(md, "| low |")
	if !(ci < mi && mi < li) {
		t.Errorf("rows not severity-ordered (critical<medium<low): crit=%d med=%d low=%d\n%s", ci, mi, li, md)
	}
	if !strings.Contains(md, "Appendix — Nuclei Findings") {
		t.Errorf("missing appendix title:\n%s", md)
	}
}
