package docker

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/shriyanss/report-rex/cli/internal/report"
)

// pandocArgs builds the pandoc invocation that turns an assembled report into
// a PDF, mirroring core/src/report_rex_core/renderer.py's subprocess call
// exactly (flag order included, since some pandoc behavior is order-sensitive).
// pandocFrom is the input format for every pandoc run. Pasted text often has no
// blank line before a list or heading, which plain pandoc markdown requires, so
// the list/heading ran into the paragraph above (#71); these extensions accept
// them. Mirrors renderer.py's PANDOC_FROM.
const pandocFrom = "markdown+raw_tex+fenced_divs+raw_attribute" +
	"+lists_without_preceding_blankline-blank_before_header"

func pandocArgs(combinedMd, metadataYaml, templateTex, resourcePath, outputPdf string, luaFilter, themeYaml string, draft bool) []string {
	args := []string{
		combinedMd,
		"--metadata-file=" + metadataYaml,
		"--template=" + templateTex,
		"--pdf-engine=xelatex",
		"--resource-path=" + resourcePath,
		"--output=" + outputPdf,
		"--from=" + pandocFrom,
		"--highlight-style=tango",
		"--standalone",
		"--table-of-contents",
		"--toc-depth=2",
	}
	if luaFilter != "" {
		args = append(args, "--lua-filter="+luaFilter)
	}
	if draft {
		args = append(args, "--metadata=draft:true")
	}
	if themeYaml != "" {
		args = append(args, "--metadata-file="+themeYaml)
	}
	return args
}

// optionalPath returns path if it exists on disk, else "".
func optionalPath(path string) string {
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}

// renderNative invokes a locally installed `pandoc` directly — no Docker, no
// Python. Used for `--no-docker` builds.
func renderNative(assembleDir, templateDir, outputPdf string, draft bool) error {
	if _, err := exec.LookPath("pandoc"); err != nil {
		return fmt.Errorf("--no-docker requires a local pandoc + XeLaTeX install (not found in PATH): %w", err)
	}

	combinedMd := filepath.Join(assembleDir, "combined.md")
	metadataYaml := filepath.Join(assembleDir, "metadata.yaml")
	templateTex := filepath.Join(templateDir, "template.tex")
	luaFilter := optionalPath(filepath.Join(templateDir, "filters", "vuln_filter.lua"))
	themeYaml := optionalPath(filepath.Join(templateDir, "theme.yaml"))

	args := pandocArgs(combinedMd, metadataYaml, templateTex, assembleDir, outputPdf, luaFilter, themeYaml, draft)
	cmd := exec.Command("pandoc", args...)
	// Logo/branding paths in metadata.yaml are relative to assembleDir (see
	// preprocessor.go's Assemble, which copies assets/ there) — XeLaTeX
	// resolves them against pandoc's own working directory, not
	// --resource-path, so it must match.
	cmd.Dir = assembleDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pandoc failed: %w", err)
	}
	return nil
}

// docxPandocArgs builds the pandoc invocation that turns a (chart-rasterized)
// assembled report into a DOCX, mirroring
// core/src/report_rex_core/renderer.py's render_docx() subprocess call.
func docxPandocArgs(combinedMd, metadataYaml, resourcePath, outputDocx string, luaFilter, themeYaml, referenceDoc string) []string {
	args := []string{
		combinedMd,
		"--metadata-file=" + metadataYaml,
		"--to=docx",
		"--resource-path=" + resourcePath,
		"--output=" + outputDocx,
		"--from=" + pandocFrom,
		"--highlight-style=tango",
		"--standalone",
	}
	if luaFilter != "" {
		args = append(args, "--lua-filter="+luaFilter)
	}
	if themeYaml != "" {
		args = append(args, "--metadata-file="+themeYaml)
	}
	if referenceDoc != "" {
		args = append(args, "--reference-doc="+referenceDoc)
	}
	return args
}

// rasterizeChartsInto reads assembleDir/combined.md, rasterizes any tagged
// chart blocks into PNGs under workDir/charts/ (DOCX has no LaTeX renderer,
// unlike the PDF pipeline's raw TikZ blocks), and writes the rewritten
// Markdown to workDir/combined.md — the original combined.md, and the
// already-rendered PDF, are left untouched.
func rasterizeChartsInto(assembleDir, workDir string, colors map[string]string, compile report.ChartCompiler) (string, error) {
	combinedText, err := os.ReadFile(filepath.Join(assembleDir, "combined.md"))
	if err != nil {
		return "", err
	}
	rewritten, err := report.RasterizeCharts(string(combinedText), workDir, colors, compile)
	if err != nil {
		return "", err
	}
	docxMd := filepath.Join(workDir, "combined.md")
	if err := os.WriteFile(docxMd, []byte(rewritten), 0o644); err != nil {
		return "", err
	}
	return docxMd, nil
}

// renderDocxNative invokes a locally installed `pandoc` (and `xelatex` +
// `pdftocairo`, for chart conversion) directly to produce a DOCX — no
// Docker, no Python. Used for `--no-docker --docx` builds.
func renderDocxNative(assembleDir, templateDir, outputDocx string, colors map[string]string) error {
	if _, err := exec.LookPath("pandoc"); err != nil {
		return fmt.Errorf("--no-docker --docx requires a local pandoc + xelatex + pdftocairo install (not found in PATH): %w", err)
	}

	docxWorkDir, err := os.MkdirTemp("", "report-rex-docx-")
	if err != nil {
		return fmt.Errorf("creating docx work dir: %w", err)
	}
	defer os.RemoveAll(docxWorkDir)

	docxMd, err := rasterizeChartsInto(assembleDir, docxWorkDir, colors, report.LocalChartCompiler)
	if err != nil {
		return fmt.Errorf("rasterizing charts for docx: %w", err)
	}

	metadataYaml := filepath.Join(assembleDir, "metadata.yaml")
	luaFilter := optionalPath(filepath.Join(templateDir, "filters", "docx_vuln_filter.lua"))
	themeYaml := optionalPath(filepath.Join(templateDir, "theme.yaml"))
	referenceDoc := optionalPath(filepath.Join(templateDir, "reference.docx"))
	resourcePath := assembleDir + string(os.PathListSeparator) + docxWorkDir

	args := docxPandocArgs(docxMd, metadataYaml, resourcePath, outputDocx, luaFilter, themeYaml, referenceDoc)
	cmd := exec.Command("pandoc", args...)
	cmd.Dir = assembleDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pandoc (docx) failed: %w", err)
	}
	return nil
}

// encryptNativePDF password-protects outputAbs in place (AES-256, via a
// locally installed `qpdf`) — rename the freshly-rendered file aside, encrypt
// it back to the original name, then drop the unencrypted temp copy. qpdf
// takes the password as a plain argv element (no shell involved), so
// passwords with special characters are safe.
func encryptNativePDF(outputAbs, password string) error {
	if _, err := exec.LookPath("qpdf"); err != nil {
		return fmt.Errorf("--password requires a local qpdf install (not found in PATH): %w", err)
	}

	tmp := outputAbs + ".tmp"
	if err := os.Rename(outputAbs, tmp); err != nil {
		return fmt.Errorf("staging %s for encryption: %w", outputAbs, err)
	}

	cmd := exec.Command("qpdf", "--encrypt", password, password, "256", "--", tmp, outputAbs)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Rename(tmp, outputAbs) // restore the unencrypted PDF so a failed encrypt doesn't lose the build
		return fmt.Errorf("qpdf encryption failed: %w", err)
	}
	os.Remove(tmp)
	return nil
}

// encryptDockerPDF is encryptNativePDF's Docker-mode counterpart: it runs
// qpdf inside the report-rex image (bypassing its Python entrypoint, the
// same way renderDocker does for pandoc) so `--password` needs no host qpdf
// install outside --no-docker mode. outputDir is already bind-mount-visible
// on the host (renderDocker/renderNative write there directly), so the
// rename happens on the host and only the encrypt step runs in-container.
func encryptDockerPDF(image, outputDir, outputName, password string) error {
	if err := Preflight(); err != nil {
		return err
	}

	outputAbs := filepath.Join(outputDir, outputName)
	tmp := outputAbs + ".tmp"
	if err := os.Rename(outputAbs, tmp); err != nil {
		return fmt.Errorf("staging %s for encryption: %w", outputAbs, err)
	}

	args := []string{
		"run", "--rm", "--entrypoint", "qpdf",
		"-v", outputDir + ":/output",
		image,
		"--encrypt", password, password, "256", "--",
		"/output/" + outputName + ".tmp", "/output/" + outputName,
	}
	if err := runReportBuild(args...); err != nil {
		os.Rename(tmp, outputAbs) // restore the unencrypted PDF so a failed encrypt doesn't lose the build
		return fmt.Errorf("qpdf encryption failed: %w", err)
	}
	os.Remove(tmp)
	return nil
}

// dockerChartCompiler returns a report.ChartCompiler that runs xelatex +
// pdftocairo inside the given report-rex image instead of on the host — the
// host isn't guaranteed to have either installed, which is the whole point
// of Docker mode. workDir (containing chart.tex, written by the caller) is
// bind-mounted read-write so the container can write chart.pdf/chart.svg
// back into it for the host process to read.
func dockerChartCompiler(image string) report.ChartCompiler {
	return func(workDir string) error {
		args := []string{
			"run", "--rm", "--entrypoint", "sh",
			"-v", workDir + ":/work",
			"-w", "/work",
			image,
			"-c", "xelatex -interaction=nonstopmode -halt-on-error chart.tex && " +
				"pdftocairo -svg chart.pdf chart.svg",
		}
		if err := runReportBuild(args...); err != nil {
			return fmt.Errorf("rasterizing chart in docker: %w", err)
		}
		return nil
	}
}

// renderDocxDocker runs pandoc inside the report-rex Docker image to produce
// a DOCX, the same --entrypoint-pandoc bypass renderDocker uses for PDF.
// Chart rasterization also runs inside that image (dockerChartCompiler),
// since the host isn't guaranteed to have xelatex/pdftocairo installed.
func renderDocxDocker(image, assembleDir, templatesDir, templateName, outputDir, outputName string, colors map[string]string) error {
	if err := Preflight(); err != nil {
		return err
	}
	if !ImageExists(image) {
		if err := BuildImage(image); err != nil {
			return fmt.Errorf("auto-building missing image %q: %w", image, err)
		}
	}

	docxWorkDir, err := os.MkdirTemp("", "report-rex-docx-")
	if err != nil {
		return fmt.Errorf("creating docx work dir: %w", err)
	}
	defer os.RemoveAll(docxWorkDir)

	if _, err := rasterizeChartsInto(assembleDir, docxWorkDir, colors, dockerChartCompiler(image)); err != nil {
		return fmt.Errorf("rasterizing charts for docx: %w", err)
	}

	const workDir = "/work"
	const docxMount = "/docx"
	const templatesMount = "/opt/report-rex/templates"
	templateDir := filepath.Join(templatesDir, templateName)
	containerTemplateDir := templatesMount + "/" + templateName

	combinedMd := docxMount + "/combined.md"
	metadataYaml := workDir + "/metadata.yaml"
	luaFilter := ""
	if optionalPath(filepath.Join(templateDir, "filters", "docx_vuln_filter.lua")) != "" {
		luaFilter = containerTemplateDir + "/filters/docx_vuln_filter.lua"
	}
	themeYaml := ""
	if optionalPath(filepath.Join(templateDir, "theme.yaml")) != "" {
		themeYaml = containerTemplateDir + "/theme.yaml"
	}
	referenceDoc := ""
	if optionalPath(filepath.Join(templateDir, "reference.docx")) != "" {
		referenceDoc = containerTemplateDir + "/reference.docx"
	}
	outputDocx := "/output/" + outputName
	// The container is always Linux regardless of host OS, so the
	// resource-path list separator is always ':'.
	resourcePath := workDir + ":" + docxMount

	pandocRenderArgs := docxPandocArgs(combinedMd, metadataYaml, resourcePath, outputDocx, luaFilter, themeYaml, referenceDoc)

	args := append([]string{
		"run", "--rm", "--entrypoint", "pandoc",
		"-w", workDir,
		"-v", assembleDir + ":" + workDir + ":ro",
		"-v", docxWorkDir + ":" + docxMount + ":ro",
		"-v", templatesDir + ":" + templatesMount + ":ro",
		"-v", outputDir + ":/output",
		image,
	}, pandocRenderArgs...)

	if err := runReportBuild(args...); err != nil {
		return fmt.Errorf("pandoc (docx) failed: %w", err)
	}
	return nil
}

// renderDocker runs pandoc inside the report-rex Docker image, bypassing its
// Python entrypoint entirely — the pandoc/latex base image already has
// pandoc + XeLaTeX on PATH, so no Python invocation is needed for rendering.
func renderDocker(image, assembleDir, templatesDir, templateName, outputDir, outputName string, draft bool) error {
	if err := Preflight(); err != nil {
		return err
	}
	if !ImageExists(image) {
		if err := BuildImage(image); err != nil {
			return fmt.Errorf("auto-building missing image %q: %w", image, err)
		}
	}

	const workDir = "/work"
	const templatesMount = "/opt/report-rex/templates"
	templateDir := filepath.Join(templatesDir, templateName)
	containerTemplateDir := templatesMount + "/" + templateName

	combinedMd := workDir + "/combined.md"
	metadataYaml := workDir + "/metadata.yaml"
	templateTex := containerTemplateDir + "/template.tex"
	luaFilter := ""
	if optionalPath(filepath.Join(templateDir, "filters", "vuln_filter.lua")) != "" {
		luaFilter = containerTemplateDir + "/filters/vuln_filter.lua"
	}
	themeYaml := ""
	if optionalPath(filepath.Join(templateDir, "theme.yaml")) != "" {
		themeYaml = containerTemplateDir + "/theme.yaml"
	}
	outputPdf := "/output/" + outputName

	pandocRenderArgs := pandocArgs(combinedMd, metadataYaml, templateTex, workDir, outputPdf, luaFilter, themeYaml, draft)

	args := append([]string{
		"run", "--rm", "--entrypoint", "pandoc",
		// Logo/branding paths in metadata.yaml are relative to workDir —
		// XeLaTeX resolves them against pandoc's own working directory
		// inside the container, which defaults to the image's WORKDIR
		// (/report, unrelated to what's mounted here) unless set explicitly.
		"-w", workDir,
		"-v", assembleDir + ":" + workDir + ":ro",
		"-v", templatesDir + ":" + templatesMount + ":ro",
		"-v", outputDir + ":/output",
		image,
	}, pandocRenderArgs...)

	if err := runReportBuild(args...); err != nil {
		return fmt.Errorf("pandoc failed: %w", err)
	}
	return nil
}
