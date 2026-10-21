package ingest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// toolSpec describes how to invoke and parse one scan tool for `rex ingest`.
type toolSpec struct {
	Name          string   // binary name; also the artifacts/<name>/ archive subdir
	ExtraArgs     []string // appended to the user's args to force machine-readable output
	ArchiveExt    string   // file extension for the raw archived output
	Parse         func([]byte) ([]Asset, error)
	ExistingParse func([]byte) ([]Asset, error) // parser for a pre-existing output file, if the tool's default (non-forced) format differs from Parse
}

// resolveReportDir resolves dir to an absolute path and confirms it's a report
// directory (i.e. contains index.md), returning an error worded for --dir if not.
func resolveReportDir(dir string) (string, error) {
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving --dir: %w", err)
	}
	indexPath := filepath.Join(dirAbs, "index.md")
	if _, err := os.Stat(indexPath); err != nil {
		return "", fmt.Errorf("index.md not found in %s (resolved from --dir %q) — run this from inside a report directory: %w", dirAbs, dir, err)
	}
	return dirAbs, nil
}

// findFlagValue scans args for flag and returns the value immediately following it
// (space-separated only, e.g. "-o" "naabu.txt" — not "-o=naabu.txt").
func findFlagValue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// ingestAssets merges raw's parsed assets into artifacts/assets.json, archives raw
// under artifacts/<toolName>/, regenerates appendices/assets.md, and registers that
// appendix in index.md. Shared by toolSpec.run (live tool output) and
// toolSpec.runExisting (a file the user already has).
func ingestAssets(dirAbs, toolName, archiveExt string, raw []byte, parse func([]byte) ([]Asset, error)) error {
	scanned, err := parse(raw)
	if err != nil {
		return fmt.Errorf("parsing %s output: %w", toolName, err)
	}
	// Some tools (naabu) emit one line per port, possibly with duplicates for the
	// same host — collapse this run's own results by IP before merging into the
	// existing inventory, so the reported host count and the merge below are both
	// counting unique hosts, not raw output lines.
	scanned = mergeAssets(nil, scanned)

	artifactsDir := filepath.Join(dirAbs, "artifacts")
	archiveDir := filepath.Join(artifactsDir, toolName)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", archiveDir, err)
	}
	archivePath := filepath.Join(archiveDir, fmt.Sprintf("%d.%s", time.Now().Unix(), archiveExt))
	if err := os.WriteFile(archivePath, raw, 0o644); err != nil {
		return fmt.Errorf("archiving raw scan to %s: %w", archivePath, err)
	}

	assetsPath := filepath.Join(artifactsDir, "assets.json")
	existing, err := loadAssets(assetsPath)
	if err != nil {
		return err
	}
	merged := mergeAssets(existing, scanned)
	if err := saveAssets(assetsPath, merged); err != nil {
		return fmt.Errorf("writing %s: %w", assetsPath, err)
	}

	ips := make([]string, len(scanned))
	for i, a := range scanned {
		ips[i] = a.IP
	}
	if err := writeAppendix(dirAbs, AppendixPath, renderAppendixMarkdown(merged), timelineDesc("Port scan", toolName, "against", ips)); err != nil {
		return err
	}

	fmt.Printf("Ingested %d host(s) from %s into %s\n", len(scanned), toolName, AppendixPath)
	return nil
}

// run shells out to spec's tool, merges the parsed results into
// artifacts/assets.json, archives the raw output under artifacts/<tool>/,
// regenerates appendices/assets.md, and registers that appendix in index.md.
func (spec toolSpec) run(dir string, args []string) error {
	dirAbs, err := resolveReportDir(dir)
	if err != nil {
		return err
	}

	bin, err := exec.LookPath(spec.Name)
	if err != nil {
		return fmt.Errorf("%s not found in PATH: %w", spec.Name, err)
	}

	cmd := exec.Command(bin, append(append([]string{}, args...), spec.ExtraArgs...)...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("running %s: %w", spec.Name, err)
	}

	return ingestAssets(dirAbs, spec.Name, spec.ArchiveExt, out, spec.Parse)
}

// runExisting ingests a file the user already has (named by outputFlag's value in
// args, e.g. "-o naabu.txt") instead of running spec's tool — for output the user
// produced themselves before ever invoking `rex`.
func (spec toolSpec) runExisting(dir string, args []string, outputFlag string) error {
	dirAbs, err := resolveReportDir(dir)
	if err != nil {
		return err
	}

	path, ok := findFlagValue(args, outputFlag)
	if !ok {
		return fmt.Errorf("no %s <file> in args — pass the file %s already wrote its output to, e.g. %s %s results.txt", outputFlag, spec.Name, spec.Name, outputFlag)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	return ingestAssets(dirAbs, spec.Name, spec.ArchiveExt, out, spec.ExistingParse)
}

// RunNmap runs nmap against dir's report (see toolSpec.run).
func RunNmap(dir string, args []string) error {
	return toolSpec{Name: "nmap", ExtraArgs: []string{"-oX", "-"}, ArchiveExt: "xml", Parse: parseNmapXML}.run(dir, args)
}

// RunNmapExisting ingests an nmap XML report the user already has on disk — the
// file named by -oX in args — instead of running nmap again. -oX <file> and the
// -oX - RunNmap forces produce the same XML schema, so this reuses parseNmapXML
// as-is rather than needing a separate parser (unlike naabu's live -json vs.
// default text output).
func RunNmapExisting(dir string, args []string) error {
	return toolSpec{Name: "nmap", ArchiveExt: "xml", ExistingParse: parseNmapXML}.runExisting(dir, args, "-oX")
}

// RunNaabu runs naabu against dir's report (see toolSpec.run).
func RunNaabu(dir string, args []string) error {
	return toolSpec{Name: "naabu", ExtraArgs: []string{"-json", "-silent"}, ArchiveExt: "json", Parse: parseNaabuJSON}.run(dir, args)
}
