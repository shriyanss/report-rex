package report

import (
	"os"
	"path/filepath"
	"testing"
)

// networkFixture writes artifacts/<name> files (skipping empty contents) into
// a temp report dir and returns it.
func networkFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, "artifacts", name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// byCheck indexes issues by check name (last one wins — tests use one per check).
func byCheck(issues []Issue) map[string]Issue {
	out := map[string]Issue{}
	for _, i := range issues {
		out[i.Check] = i
	}
	return out
}

func TestCheckNetworkArtifactsDirectiveUsage(t *testing.T) {
	dir := networkFixture(t, nil)
	md := mdFile{"sections/m.md", "---\ntitle: M\n---\n\nIntro.\n\n:::network_diagram:::\n\n:::asset_graph:pie:os:::\n"}
	got := byCheck(checkNetworkArtifacts(dir, []mdFile{md}))

	if i := got["network_assets_missing"]; i.Severity != SeverityError || i.Line != 7 || i.File != md.path {
		t.Errorf("network_assets_missing = %+v", i)
	}
	if i := got["chart_data_missing"]; i.Severity != SeverityWarning || i.Line != 9 {
		t.Errorf("chart_data_missing = %+v", i)
	}
}

func TestCheckNetworkArtifactsCleanReportHasNoIssues(t *testing.T) {
	dir := networkFixture(t, map[string]string{
		"assets.json":      `[{"ip": "10.0.0.5"}, {"ip": "10.0.0.9"}]`,
		"connections.json": `{"subnets": {"10.0.0.0/24": "LAN"}, "pivots": {"10.0.0.5": ["10.0.0.9", "10.9.0.0/16"]}, "important": ["10.0.0.9"]}`,
	})
	issues := checkNetworkArtifacts(dir, []mdFile{{"a.md", ":::network_diagram:::\n:::asset_graph:bar:ports:::\n"}})
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %+v", issues)
	}
}

func TestCheckNetworkArtifactsConnectionsProblems(t *testing.T) {
	dir := networkFixture(t, map[string]string{
		"assets.json": "[\n  {\"ip\": \"10.0.0.5\"},\n  {\"ip\": \"not-an-ip\"}\n]",
		"connections.json": "{\n" +
			"  \"subnets\": {\"10.0.0.0/33\": \"Bad\"},\n" +
			"  \"pivots\": {\n" +
			"    \"10.0.0.5\": \"10.0.0.77\",\n" +
			"    \"10.0.0.5/24\": \"nope\"\n" +
			"  },\n" +
			"  \"pivot\": {}\n" +
			"}",
	})
	issues := checkNetworkArtifacts(dir, nil)
	got := byCheck(issues)

	if i := got["assets_invalid"]; i.Severity != SeverityError || i.Line != 3 {
		t.Errorf("assets_invalid = %+v", i)
	}
	if i := got["connections_host_unknown"]; i.Severity != SeverityWarning || i.Line != 4 {
		t.Errorf("connections_host_unknown = %+v", i)
	}
	if i := got["connections_key_unknown"]; i.Severity != SeverityWarning || i.Line != 7 {
		t.Errorf("connections_key_unknown = %+v", i)
	}
	if _, ok := got["connections_unused"]; !ok {
		t.Errorf("expected connections_unused, got %+v", issues)
	}
	invalid := 0
	for _, i := range issues {
		if i.Check == "connections_invalid" {
			invalid++
			if i.Line == 0 {
				t.Errorf("connections_invalid without a line: %+v", i)
			}
		}
	}
	if invalid != 2 { // the /33 CIDR and the "nope" target
		t.Errorf("connections_invalid count = %d, issues %+v", invalid, issues)
	}
}

func TestCheckNetworkArtifactsInvalidJSON(t *testing.T) {
	dir := networkFixture(t, map[string]string{
		"assets.json":      `{"ip": "10.0.0.5"}`,
		"connections.json": "{\n  \"subnets\": {\n    \"10.0.0.0/24\": \"LAN\",\n  }\n}",
	})
	issues := checkNetworkArtifacts(dir, []mdFile{{"a.md", ":::network_diagram:::\n"}})
	if len(issues) != 2 {
		t.Fatalf("expected 2 artifact_json_invalid issues, got %+v", issues)
	}
	for _, i := range issues {
		if i.Check != "artifact_json_invalid" || i.Severity != SeverityError {
			t.Errorf("got %+v", i)
		}
	}
	if issues[1].Line != 4 { // the trailing comma is reported at the closing brace
		t.Errorf("connections.json syntax error line = %d", issues[1].Line)
	}
}
