package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/shriyanss/report-rex/cli/internal/report"
	"github.com/spf13/cobra"
)

var reportValidateDir string
var reportValidateJSON bool
var reportValidateStdin bool

var reportValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate a report directory: structure, required fields, and screenshot QA",
	RunE: func(cmd *cobra.Command, args []string) error {
		var overlay map[string]string
		if reportValidateStdin {
			if err := json.NewDecoder(os.Stdin).Decode(&overlay); err != nil {
				return fmt.Errorf("--stdin: expected a JSON object of {absolute path: content}: %w", err)
			}
		}
		structure, result, err := runValidation(reportValidateDir, reportValidateJSON, overlay)
		if err != nil {
			if reportValidateJSON {
				_ = json.NewEncoder(os.Stdout).Encode(struct {
					Error string `json:"error"`
				}{err.Error()})
				os.Exit(1)
			}
			return err
		}
		if !reportValidateJSON && len(result.Issues) == 0 {
			fmt.Printf("Valid — %d sections, %d findings, %d appendices\n",
				len(structure.Sections), len(structure.Reports), len(structure.Appendices))
		}
		if !result.OK() {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	reportValidateCmd.Flags().StringVar(&reportValidateDir, "dir", ".", "report directory to validate")
	reportValidateCmd.Flags().BoolVar(&reportValidateJSON, "json", false, "output validation issues as JSON instead of human-readable text")
	reportValidateCmd.Flags().BoolVar(&reportValidateStdin, "stdin", false,
		"read a JSON object of {absolute path: content} from stdin and validate that content in place of those files on disk (an editor's unsaved buffers)")
	reportCmd.AddCommand(reportValidateCmd)
}

// runValidation parses index.md and runs the QA-gate checks against dir. In
// text mode (jsonOutput false — the default `report validate` UX, and always
// used by `report build`'s pre-render gate) it prints every issue grouped by
// file, mirroring report_rex_core.cli's _run_validation. In JSON mode (the
// editor extensions' lint feature) it instead encodes a single {issues,
// errors, warnings} envelope to stdout so callers get structured, parseable
// diagnostics. overlay (may be nil) replaces files' on-disk content; see
// report.Structure.Overlay.
func runValidation(dir string, jsonOutput bool, overlay map[string]string) (report.Structure, report.Result, error) {
	indexPath := filepath.Join(dir, "index.md")
	if _, err := os.Stat(indexPath); err != nil {
		return report.Structure{}, report.Result{}, fmt.Errorf("index.md not found in %s", dir)
	}

	structure, err := report.ParseIndex(indexPath)
	if err != nil {
		return report.Structure{}, report.Result{}, fmt.Errorf("invalid: %w", err)
	}

	structure.Overlay = overlay
	result, err := report.Validate(structure)
	if err != nil {
		return structure, report.Result{}, err
	}

	if resolvedDir, absErr := filepath.Abs(dir); absErr == nil {
		for i := range result.Issues {
			if rel, relErr := filepath.Rel(resolvedDir, result.Issues[i].File); relErr == nil {
				result.Issues[i].File = rel
			}
		}
	}

	if jsonOutput {
		issues := result.Issues
		if issues == nil {
			issues = []report.Issue{}
		}
		err := json.NewEncoder(os.Stdout).Encode(struct {
			Issues   []report.Issue `json:"issues"`
			Errors   int            `json:"errors"`
			Warnings int            `json:"warnings"`
		}{issues, len(result.Errors()), len(result.Warnings())})
		return structure, result, err
	}

	if len(result.Issues) == 0 {
		return structure, result, nil
	}

	byFile := map[string][]report.Issue{}
	for _, issue := range result.Issues {
		byFile[issue.File] = append(byFile[issue.File], issue)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	sort.Strings(files)

	for _, f := range files {
		fmt.Println(f + ":")
		for _, issue := range byFile[f] {
			label := "warning"
			if issue.Severity == report.SeverityError {
				label = "error"
			}
			line := fmt.Sprintf("  [%s] %s: %s", label, issue.Check, issue.Message)
			if issue.Line > 0 {
				line = fmt.Sprintf("  [%s] line %d: %s: %s", label, issue.Line, issue.Check, issue.Message)
			}
			if issue.Severity == report.SeverityError {
				fmt.Fprintln(os.Stderr, line)
			} else {
				fmt.Println(line)
			}
		}
	}

	nErrors, nWarnings := len(result.Errors()), len(result.Warnings())
	summary := fmt.Sprintf("\n%d error(s), %d warning(s) across %d file(s)", nErrors, nWarnings, len(byFile))
	if nErrors > 0 {
		fmt.Fprintln(os.Stderr, summary)
	} else {
		fmt.Println(summary)
	}
	return structure, result, nil
}
