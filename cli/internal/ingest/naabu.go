package ingest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// naabuResult is one `naabu -json` output line — one open port per line, no service
// detection (naabu is a port-discovery tool, not a service scanner like nmap).
type naabuResult struct {
	Host  string `json:"host"`
	IP    string `json:"ip"`
	Port  int    `json:"port"`
	Proto string `json:"protocol"`
}

// parseNaabuJSON parses `naabu -json` output (one JSON object per line) into the
// tool-agnostic Asset shape. Multiple lines for the same IP (naabu commonly emits
// duplicates/retries) fold together correctly downstream: mergeAssets processes
// newAssets one at a time, so repeated (IP, port) pairs in this slice just re-union
// into the same entry instead of duplicating it.
func parseNaabuJSON(data []byte) ([]Asset, error) {
	var assets []Asset
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var r naabuResult
		if err := json.Unmarshal(line, &r); err != nil {
			return nil, err
		}
		if r.IP == "" {
			continue
		}

		asset := Asset{IP: r.IP}
		if r.Host != "" && r.Host != r.IP {
			asset.Hostnames = []string{r.Host}
		}
		asset.Ports = []Port{{Number: strconv.Itoa(r.Port), Proto: r.Proto}}
		assets = append(assets, asset)
	}
	return assets, scanner.Err()
}

// parseNaabuText parses naabu's default (non -json) output: one "host:port" line
// per open port, for a file the user already has from running naabu themselves
// (see RunNaabuExisting). Unlike -json output this carries no protocol field —
// naabu's plain-text mode only ever scans TCP, so Proto is hardcoded.
func parseNaabuText(data []byte) ([]Asset, error) {
	var assets []Asset
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		host, port, err := net.SplitHostPort(line)
		if err != nil {
			return nil, fmt.Errorf("parsing naabu output line %q: %w", line, err)
		}
		assets = append(assets, Asset{IP: host, Ports: []Port{{Number: port, Proto: "tcp"}}})
	}
	return assets, scanner.Err()
}

// RunNaabuExisting ingests naabu output the user already has on disk — the file
// named by -o in args — instead of running naabu again.
func RunNaabuExisting(dir string, args []string) error {
	return toolSpec{Name: "naabu", ArchiveExt: "txt", ExistingParse: parseNaabuText}.runExisting(dir, args, "-o")
}
