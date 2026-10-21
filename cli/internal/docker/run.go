package docker

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/shriyanss/report-rex/cli/internal/buildscript"
	"github.com/shriyanss/report-rex/cli/internal/embedded"
	"github.com/shriyanss/report-rex/cli/internal/report"
)

// RunReportBuild parses and assembles the report natively (no Docker, no
// Python), then renders the assembled combined.md/metadata.yaml to a PDF via
// pandoc — inside the report-rex Docker image by default, or via a local
// `pandoc` install when flags.NoDocker is set.
func RunReportBuild(flags buildscript.BuildFlags) error {
	reportDirAbs, err := filepath.Abs(flags.Dir)
	if err != nil {
		return fmt.Errorf("resolving --dir: %w", err)
	}
	indexPath := filepath.Join(reportDirAbs, "index.md")
	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("index.md not found in %s (resolved from --dir %q): %w", reportDirAbs, flags.Dir, err)
	}

	structure, err := report.ParseIndex(indexPath)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", indexPath, err)
	}

	if !flags.Force {
		result, err := report.Validate(structure)
		if err != nil {
			return fmt.Errorf("validating %s: %w", indexPath, err)
		}
		if !result.OK() {
			var b strings.Builder
			fmt.Fprintf(&b, "%d validation error(s) — fix them or pass --force to build anyway "+
				"(run `rex report validate --dir %s` for details):\n", len(result.Errors()), flags.Dir)
			for _, issue := range result.Errors() {
				fmt.Fprintf(&b, "  [%s] %s: %s\n", issue.File, issue.Check, issue.Message)
			}
			return errors.New(b.String())
		}
	}

	templateName := flags.Template
	if templateName == "" {
		templateName = structure.Template
	}

	templatesDir, cleanupTemplates, err := materializeTemplates()
	if err != nil {
		return fmt.Errorf("materializing embedded templates: %w", err)
	}
	defer cleanupTemplates()

	templateDir := filepath.Join(templatesDir, templateName)
	if _, err := os.Stat(filepath.Join(templateDir, "template.tex")); err != nil {
		return fmt.Errorf("template %q not found in %s", templateName, templatesDir)
	}

	assembleDir, err := os.MkdirTemp("", "report-rex-assemble-")
	if err != nil {
		return fmt.Errorf("creating assemble dir: %w", err)
	}
	defer os.RemoveAll(assembleDir)

	if _, _, err := report.Assemble(structure, assembleDir); err != nil {
		return fmt.Errorf("assembling report: %w", err)
	}

	outputFile := flags.OutputFile()
	outputAbs, err := filepath.Abs(outputFile)
	if err != nil {
		return fmt.Errorf("resolving --output: %w", err)
	}
	outputDir := filepath.Dir(outputAbs)
	outputName := filepath.Base(outputAbs)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	renderPDF := func() error {
		return renderDocker(flags.Image, assembleDir, templatesDir, templateName, outputDir, outputName, !flags.Final)
	}
	if flags.NoDocker {
		renderPDF = func() error { return renderNative(assembleDir, templateDir, outputAbs, !flags.Final) }
	}
	if err := withProgress("Rendering PDF", flags.NoTerminalBS, renderPDF); err != nil {
		return err
	}

	if flags.Password != "" {
		encryptPDF := func() error { return encryptDockerPDF(flags.Image, outputDir, outputName, flags.Password) }
		if flags.NoDocker {
			encryptPDF = func() error { return encryptNativePDF(outputAbs, flags.Password) }
		}
		if err := withProgress("Encrypting PDF", flags.NoTerminalBS, encryptPDF); err != nil {
			return err
		}
	}

	if !flags.Docx {
		return nil
	}

	docxAbs, err := filepath.Abs(flags.OutputDocxFile())
	if err != nil {
		return fmt.Errorf("resolving docx output path: %w", err)
	}
	docxDir := filepath.Dir(docxAbs)
	docxName := filepath.Base(docxAbs)
	if err := os.MkdirAll(docxDir, 0o755); err != nil {
		return fmt.Errorf("creating docx output directory: %w", err)
	}

	colors := report.ResolveChartColors(asMap(structure.Metadata["colors"]), templateDir)
	renderDocx := func() error {
		return renderDocxDocker(flags.Image, assembleDir, templatesDir, templateName, docxDir, docxName, colors)
	}
	if flags.NoDocker {
		renderDocx = func() error { return renderDocxNative(assembleDir, templateDir, docxAbs, colors) }
	}
	return withProgress("Rendering DOCX", flags.NoTerminalBS, renderDocx)
}

// asMap type-asserts a decoded-YAML `any` value (e.g. index.md's `colors:`
// key) to a string-keyed map, returning nil if it isn't one (unset, or the
// user set it to something else — ResolveChartColors treats nil as "no
// overrides").
func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// runReportBuild is like run(), but also captures stderr so it can detect the
// specific case where the container's bind-mounted /work directory appears
// empty even though the host-side assemble step just populated it. That
// combination means the bind mount itself came up empty — commonly Docker
// Desktop's file-sharing config not covering the mounted path (seen with
// directories under /tmp) — so we surface an actionable hint instead of the
// confusing pandoc "file does not exist" error alone.
func runReportBuild(args ...string) error {
	cmd := exec.Command("docker", args...)
	var stderrBuf bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	if err != nil && strings.Contains(stderrBuf.String(), "/work/combined.md") {
		return fmt.Errorf(`%w

The assembled report exists on your machine, but appeared empty inside the
container — this usually means Docker Desktop isn't sharing that path with its
VM. Things to try:
  - Docker Desktop > Settings > Resources > File sharing: confirm your $TMPDIR
    (or a parent of it) is listed
  - Restart Docker Desktop and try again`, err)
	}
	return err
}

// materializeTemplates extracts the embedded templates/ tree to a temp dir, returning
// a cleanup func the caller must defer.
func materializeTemplates() (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "report-rex-templates-")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	templatesFS, err := fs.Sub(embedded.BuildContextFS(), "templates")
	if err != nil {
		cleanup()
		return "", func() {}, err
	}
	if err := extractFS(templatesFS, tmpDir); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return tmpDir, cleanup, nil
}
