// Package scaffold extracts the embedded example report-rex report trees for
// `rex report init`. Unlike core/src/report_rex_core/builder.py's `scaffold()`
// (which writes minimal starter-text files), this extracts the full example-populated
// reporting/example / reporting/communication/<kind> trees, giving new users a
// complete, realistic starting point. No Docker required.
package scaffold

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/shriyanss/report-rex/cli/internal/embedded"
	"github.com/shriyanss/report-rex/cli/internal/fsutil"
)

const (
	TypeFullReport = "full-report"
	TypeQuickComm  = "quick-comm"
)

// Meta holds the engagement values InitReport patches into the scaffold's index.md,
// overriding its placeholder title/client/date.
type Meta struct {
	Title  string
	Client string
	Date   string
}

// InitReport extracts the matching embedded scaffold tree into targetDir.
//
//   - reportType == TypeFullReport: extracts scaffolds/full-report.
//   - reportType == TypeQuickComm: extracts scaffolds/quick-comm/<kind>, kind being
//     "critical-finding" or "progress-update".
//
// targetDir must not already exist or must be empty (hidden entries like .git/ or
// .vscode/ don't count); InitReport refuses to overwrite an in-progress report. Any non-empty field in meta overwrites the corresponding
// placeholder value in the scaffold's index.md.
func InitReport(targetDir, reportType, kind string, meta Meta) error {
	entries, err := os.ReadDir(targetDir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			return fmt.Errorf("target directory %q is not empty; refusing to overwrite", targetDir)
		}
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("checking target directory: %w", err)
	}

	var srcPath string
	switch reportType {
	case TypeFullReport:
		srcPath = "full-report"
	case TypeQuickComm:
		if kind == "" {
			kind = "progress-update"
		}
		srcPath = filepath.Join("quick-comm", kind)
	default:
		return fmt.Errorf("unknown report type %q (expected %q or %q)", reportType, TypeFullReport, TypeQuickComm)
	}

	src, err := fs.Sub(embedded.ScaffoldFS(), srcPath)
	if err != nil {
		return fmt.Errorf("unknown scaffold %q: %w", srcPath, err)
	}

	if err := fsutil.CopyFS(src, targetDir); err != nil {
		return err
	}

	return patchIndexMeta(filepath.Join(targetDir, "index.md"), meta)
}

// patchIndexMeta overwrites index.md's top-level title/client/date frontmatter
// values with any non-empty fields in meta. It's a line-based edit (anchored at
// column 0, so indented keys like structure:'s list entries are never touched)
// rather than a full YAML re-marshal, which would destroy hand-written comments.
func patchIndexMeta(indexPath string, meta Meta) error {
	fields := map[string]string{
		"title":  meta.Title,
		"client": meta.Client,
		"date":   meta.Date,
	}
	hasValue := false
	for _, v := range fields {
		if v != "" {
			hasValue = true
			break
		}
	}
	if !hasValue {
		return nil
	}

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", indexPath, err)
	}

	for key, value := range fields {
		if value == "" {
			continue
		}
		re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:.*$`)
		data = re.ReplaceAllLiteral(data, []byte(key+`: "`+value+`"`))
	}

	return os.WriteFile(indexPath, data, 0o644)
}
