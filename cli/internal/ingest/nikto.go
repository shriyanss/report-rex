package ingest

import (
	"encoding/xml"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// niktoScans is nikto's `-Format xml` output schema (the subset used here).
type niktoScans struct {
	Scans []niktoScan `xml:"niktoscan"`
}

type niktoScan struct {
	Details []niktoScanDetails `xml:"scandetails"`
}

type niktoScanDetails struct {
	TargetIP       string      `xml:"targetip,attr"`
	TargetHostname string      `xml:"targethostname,attr"`
	TargetPort     string      `xml:"targetport,attr"`
	Items          []niktoItem `xml:"item"`
}

type niktoItem struct {
	ID          string `xml:"id,attr"`
	Description string `xml:"description"`
	URI         string `xml:"uri"`
	References  string `xml:"references"`
}

func parseNiktoXML(data []byte) ([]WebFinding, error) {
	var scans niktoScans
	if err := xml.Unmarshal(data, &scans); err != nil {
		return nil, err
	}

	var findings []WebFinding
	for _, scan := range scans.Scans {
		for _, d := range scan.Details {
			host := d.TargetHostname
			if host == "" {
				host = d.TargetIP
			}
			for _, item := range d.Items {
				findings = append(findings, WebFinding{
					Host:        host,
					Port:        d.TargetPort,
					ID:          item.ID,
					Description: strings.TrimSpace(item.Description),
					URI:         item.URI,
					References:  item.References,
				})
			}
		}
	}
	return findings, nil
}

// RunNikto runs nikto against dir's report: shells out to the nikto binary, merges
// the parsed findings into artifacts/webfindings.json, archives the raw XML under
// artifacts/nikto/, regenerates appendices/web-findings.md, and registers that
// appendix in index.md. Unlike RunNmap/RunNaabu this doesn't go through toolSpec.run
// since nikto's output (per-finding, not per-host-inventory) needs a different
// artifact file and appendix.
func RunNikto(dir string, args []string) error {
	dirAbs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolving --dir: %w", err)
	}
	indexPath := filepath.Join(dirAbs, "index.md")
	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("index.md not found in %s (resolved from --dir %q) — run this from inside a report directory: %w", dirAbs, dir, err)
	}

	bin, err := exec.LookPath("nikto")
	if err != nil {
		return fmt.Errorf("nikto not found in PATH: %w", err)
	}

	// Nikto only finalizes its XML report when writing to a real file — on an
	// early stop (e.g. -maxtime expiring), streaming to stdout (-output -)
	// produces no output at all, silently, while -output <file> still flushes
	// the partial report. Write to a temp file and read it back rather than
	// capturing stdout.
	outFile, err := os.CreateTemp("", "rex-nikto-*.xml")
	if err != nil {
		return fmt.Errorf("creating temp file for nikto output: %w", err)
	}
	outPath := outFile.Name()
	outFile.Close()
	defer os.Remove(outPath)

	cmd := exec.Command(bin, append(append([]string{}, args...), "-Format", "xml", "-output", outPath)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running nikto: %w", err)
	}

	out, err := os.ReadFile(outPath)
	if err != nil {
		return fmt.Errorf("reading nikto output: %w", err)
	}

	scanned, err := parseNiktoXML(out)
	if err != nil {
		return fmt.Errorf("parsing nikto XML output: %w", err)
	}

	artifactsDir := filepath.Join(dirAbs, "artifacts")
	archiveDir := filepath.Join(artifactsDir, "nikto")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", archiveDir, err)
	}
	archivePath := filepath.Join(archiveDir, fmt.Sprintf("%d.xml", time.Now().Unix()))
	if err := os.WriteFile(archivePath, out, 0o644); err != nil {
		return fmt.Errorf("archiving raw scan to %s: %w", archivePath, err)
	}

	findingsPath := filepath.Join(artifactsDir, "webfindings.json")
	existing, err := loadWebFindings(findingsPath)
	if err != nil {
		return err
	}
	merged := mergeWebFindings(existing, scanned)
	if err := saveWebFindings(findingsPath, merged); err != nil {
		return fmt.Errorf("writing %s: %w", findingsPath, err)
	}

	targets := make([]string, len(scanned))
	for i, f := range scanned {
		targets[i] = net.JoinHostPort(f.Host, f.Port)
	}
	if err := writeAppendix(dirAbs, WebAppendixPath, renderWebAppendixMarkdown(merged), timelineDesc("Web server scan", "nikto", "against", targets)); err != nil {
		return err
	}

	fmt.Printf("Ingested %d finding(s) from nikto into %s\n", len(scanned), WebAppendixPath)
	return nil
}
