package ingest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shriyanss/report-rex/cli/internal/report"
)

func TestWriteAppendixAccumulatesTimelinePerAppendix(t *testing.T) {
	dir := t.TempDir()
	index := "---\ntitle: \"T\"\nstructure:\n  appendices:\n    - appendices/1.md\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "index.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	md := renderAppendixMarkdown(nil)
	for _, desc := range []string{"Port scan (nmap) against 10.0.0.1", "Port scan (naabu) against 10.0.0.2"} {
		if err := writeAppendix(dir, AppendixPath, md, desc); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAppendix(dir, WebAppendixPath, renderWebAppendixMarkdown(nil), "Web server scan (nikto) against a:80"); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string][]string{
		AppendixPath:    {"Port scan (nmap) against 10.0.0.1", "Port scan (naabu) against 10.0.0.2"},
		WebAppendixPath: {"Web server scan (nikto) against a:80"},
	} {
		raw, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		meta := report.ParseFrontmatter(string(raw))
		if meta["title"] == nil {
			t.Errorf("%s: title lost from frontmatter:\n%s", path, raw)
		}
		entries, _ := meta["timeline"].([]any)
		if len(entries) != len(want) {
			t.Fatalf("%s: got %d timeline entries, want %d:\n%s", path, len(entries), len(want), raw)
		}
		for i, e := range entries {
			m := e.(map[string]any)
			if m["description"] != want[i] || m["include"] != true || m["date"] == "" {
				t.Errorf("%s entry %d = %v, want description %q", path, i, m, want[i])
			}
		}
	}

	indexOut, _ := os.ReadFile(filepath.Join(dir, "index.md"))
	if n := strings.Count(string(indexOut), AppendixPath); n != 1 {
		t.Errorf("%s registered %d times in index.md, want 1", AppendixPath, n)
	}
}

func TestTimelineDesc(t *testing.T) {
	got := timelineDesc("Port scan", "nmap", "against", []string{"h7", "h1", "h2", "h1", "h3", "h4", "h5", "h6"})
	if want := "Port scan (nmap) against h1, h2, h3, h4, h5 (+2 more)"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := timelineDesc("Port scan", "nmap", "against", nil); got != "Port scan (nmap): nothing found" {
		t.Errorf("empty: got %q", got)
	}
}
