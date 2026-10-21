// Package report ports report-rex-core's parser/preprocessor/validator to
// native Go, so `rex report build`/`rex report validate` no longer need to
// shell out to Python for anything but the final pandoc/xelatex render.
package report

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Structure is a parsed index.md: metadata plus every referenced section,
// finding, and appendix file resolved to an absolute path.
type Structure struct {
	Metadata   map[string]any
	Sections   []string
	Reports    []string
	Appendices []string
	BaseDir    string
	Template   string
	// Overlay maps an absolute file path to content that Validate uses in
	// place of the file on disk: an editor's unsaved buffers (#58).
	Overlay map[string]string
}

// ParseIndex parses index.md, validates every referenced file exists, and
// returns the resolved report structure.
func ParseIndex(indexPath string) (Structure, error) {
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return Structure{}, fmt.Errorf("reading %s: %w", indexPath, err)
	}

	frontmatter, _ := SplitFrontmatter(string(content))
	if frontmatter == "" {
		return Structure{}, fmt.Errorf("%s has no YAML frontmatter", indexPath)
	}

	var meta map[string]any
	if err := yaml.Unmarshal([]byte(frontmatter), &meta); err != nil {
		return Structure{}, fmt.Errorf("parsing frontmatter of %s: %w", indexPath, err)
	}
	if meta == nil {
		meta = map[string]any{}
	}

	baseDir := filepath.Dir(indexPath)
	structureField, _ := meta["structure"].(map[string]any)

	resolve := func(key string) ([]string, error) {
		raw, _ := structureField[key].([]any)
		resolved := make([]string, 0, len(raw))
		for _, item := range raw {
			p, _ := item.(string)
			full := filepath.Join(baseDir, p)
			if _, err := os.Stat(full); err != nil {
				return nil, fmt.Errorf("referenced file not found: %s", full)
			}
			resolved = append(resolved, full)
		}
		return resolved, nil
	}

	sections, err := resolve("sections")
	if err != nil {
		return Structure{}, err
	}
	reports, err := resolve("reports")
	if err != nil {
		return Structure{}, err
	}
	appendices, err := resolve("appendices")
	if err != nil {
		return Structure{}, err
	}

	template := "rit-cptc"
	if t, ok := meta["template"].(string); ok && t != "" {
		template = t
	}

	return Structure{
		Metadata:   meta,
		Sections:   sections,
		Reports:    reports,
		Appendices: appendices,
		BaseDir:    baseDir,
		Template:   template,
	}, nil
}

// SplitFrontmatter splits YAML frontmatter from a markdown body. Returns
// ("", content) when content has no leading "---" delimiter.
func SplitFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(content, "---") {
		return "", content
	}
	end := strings.Index(content[3:], "\n---")
	if end == -1 {
		return "", content
	}
	end += 3
	return strings.TrimSpace(content[3:end]), strings.TrimSpace(content[end+4:])
}

// ParseFrontmatter parses just the YAML frontmatter of a markdown file into a map.
func ParseFrontmatter(content string) map[string]any {
	fm, _ := SplitFrontmatter(content)
	if fm == "" {
		return map[string]any{}
	}
	var meta map[string]any
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil || meta == nil {
		return map[string]any{}
	}
	return meta
}
