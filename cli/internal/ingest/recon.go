package ingest

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// reconTools are the binaries `rex recon` needs; all must be present (fail fast).
var reconTools = []string{"nmap", "nikto", "nuclei"}

// RunRecon orchestrates a full recon sweep against targets (hosts and/or CIDRs)
// and folds everything into dir's report:
//
//  1. a two-phase nmap scan — a fast all-port discovery pass, then a
//     service/version + default-script pass against only the open ports;
//  2. nikto against each discovered web app;
//  3. nuclei against the discovered web apps.
//
// Each sub-run goes through the same ingest path as `rex ingest`, so results land
// in artifacts/*.json + the appendices and every run is stamped into the Timeline.
func RunRecon(dir string, targets []string) error {
	dirAbs, err := resolveReportDir(dir)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no targets — pass at least one --host or --cidr")
	}

	// Fail fast: every tool must be installed before we scan anything.
	var missing []string
	for _, t := range reconTools {
		if _, err := exec.LookPath(t); err != nil {
			missing = append(missing, t)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required tool(s) not found in PATH: %s", strings.Join(missing, ", "))
	}

	// Phase 1 — discovery: find open ports across all targets (no writes yet).
	fmt.Printf("recon: discovering open ports on %s ...\n", strings.Join(targets, ", "))
	ports, err := nmapDiscover(targets)
	if err != nil {
		return err
	}
	if len(ports) == 0 {
		fmt.Println("recon: no open ports found — nothing to scan further.")
		return nil
	}

	// Phase 2 — detail: service/version detection + default scripts, but only on
	// the ports phase 1 found open. One invocation keeps it to a single assets
	// merge + appendix + Timeline entry.
	fmt.Printf("recon: scanning %d open port(s) for services/scripts ...\n", len(ports))
	nmapArgs := append([]string{"-sV", "-sC", "-p", strings.Join(ports, ",")}, targets...)
	if err := RunNmap(dir, nmapArgs); err != nil {
		return err
	}

	// Detect web apps from the assembled inventory.
	assets, err := loadAssets(filepath.Join(dirAbs, "artifacts", "assets.json"))
	if err != nil {
		return err
	}
	urls := webTargets(assets)
	if len(urls) == 0 {
		fmt.Println("recon: no web services detected — skipping nikto/nuclei.")
		return nil
	}
	fmt.Printf("recon: found %d web target(s): %s\n", len(urls), strings.Join(urls, ", "))

	// nikto: one host per invocation.
	for _, u := range urls {
		if err := RunNikto(dir, []string{"-h", u}); err != nil {
			return err
		}
	}

	// nuclei: all targets in one run.
	if err := RunNuclei(dir, []string{"-u", strings.Join(urls, ",")}); err != nil {
		return err
	}

	return nil
}

// nmapDiscover runs a fast all-port sweep against targets and returns the sorted
// union of open TCP port numbers found across them. It does not touch the report;
// it only feeds phase 2's targeted scan.
func nmapDiscover(targets []string) ([]string, error) {
	bin, err := exec.LookPath("nmap")
	if err != nil {
		return nil, fmt.Errorf("nmap not found in PATH: %w", err)
	}
	args := append([]string{"-p-", "-oX", "-"}, targets...)
	cmd := exec.Command(bin, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("running nmap discovery: %w", err)
	}

	assets, err := parseNmapXML(out)
	if err != nil {
		return nil, fmt.Errorf("parsing nmap discovery output: %w", err)
	}

	seen := map[int]bool{}
	for _, a := range assets {
		for _, p := range a.Ports {
			if n, err := strconv.Atoi(p.Number); err == nil {
				seen[n] = true
			}
		}
	}
	nums := make([]int, 0, len(seen))
	for n := range seen {
		nums = append(nums, n)
	}
	sort.Ints(nums)
	ports := make([]string, len(nums))
	for i, n := range nums {
		ports[i] = strconv.Itoa(n)
	}
	return ports, nil
}

// webPortScheme reports whether an asset port looks like a web service and, if so,
// the URL scheme to reach it. A port is web when its service name mentions http or
// ssl, or its number is a well-known web port.
func webPortScheme(p Port) (scheme string, isWeb bool) {
	svc := strings.ToLower(p.Service)
	switch {
	case strings.Contains(svc, "https") || strings.Contains(svc, "ssl"):
		return "https", true
	case strings.Contains(svc, "http"): // http, http-proxy, http-alt, ...
		return "http", true
	}
	switch p.Number {
	case "443", "8443":
		return "https", true
	case "80", "8080", "8000", "8888":
		return "http", true
	}
	return "", false
}

// webTargets builds a sorted, deduped list of URLs for the web services found in
// assets, preferring a host's first hostname over its IP. Standard ports (80 for
// http, 443 for https) are left off the URL.
func webTargets(assets []Asset) []string {
	seen := map[string]bool{}
	var urls []string
	for _, a := range assets {
		host := a.IP
		if len(a.Hostnames) > 0 && a.Hostnames[0] != "" {
			host = a.Hostnames[0]
		}
		for _, p := range a.Ports {
			scheme, ok := webPortScheme(p)
			if !ok {
				continue
			}
			hostport := host
			if !((scheme == "http" && p.Number == "80") || (scheme == "https" && p.Number == "443")) {
				hostport = net.JoinHostPort(host, p.Number)
			}
			u := scheme + "://" + hostport
			if !seen[u] {
				seen[u] = true
				urls = append(urls, u)
			}
		}
	}
	sort.Strings(urls)
	return urls
}
