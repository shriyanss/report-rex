package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ── Network-diagram artifacts (artifacts/assets.json, connections.json) ──────
//
// Lints the artifact JSON files the chart directives read, found by filename
// under artifacts/, against how the report's markdown files use them. Every
// issue carries a Line so editors can point at the offending directive or
// JSON entry. Mirrors validator.py's _check_network_artifacts.

// mdFile is a report markdown file's path and raw text (frontmatter included,
// so directive line numbers match the file on disk).
type mdFile struct{ path, raw string }

// lineAt returns the 1-based line of byte offset off in s.
func lineAt(s string, off int) int {
	return strings.Count(s[:min(max(off, 0), len(s))], "\n") + 1
}

// lineOf returns the 1-based line of needle's first occurrence in s, or 0.
func lineOf(s, needle string) int {
	if i := strings.Index(s, needle); i >= 0 {
		return lineAt(s, i)
	}
	return 0
}

// jsonLine is the line a string value/key appears on in raw JSON text.
func jsonLine(raw, value string) int {
	quoted, _ := json.Marshal(value)
	return lineOf(raw, string(quoted))
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// decodeArtifact unmarshals raw into v, returning an artifact_json_invalid
// issue (and false) if it isn't valid JSON or isn't shaped like want.
func decodeArtifact(path, raw string, v any, want string) (Issue, bool) {
	err := json.Unmarshal([]byte(raw), v)
	if err == nil {
		return Issue{}, true
	}
	name := "artifacts/" + filepath.Base(path)
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return Issue{path, "artifact_json_invalid",
			fmt.Sprintf("%s is not valid JSON: %v.", name, err), SeverityError, lineAt(raw, int(syntaxErr.Offset))}, false
	}
	return Issue{path, "artifact_json_invalid", fmt.Sprintf("%s must be %s.", name, want), SeverityError, 0}, false
}

func checkNetworkArtifacts(baseDir string, files []mdFile) []Issue {
	var issues []Issue
	artifacts := filepath.Join(baseDir, "artifacts")
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(artifacts, name))
		return err == nil
	}

	// How the report files use the artifacts.
	chartDirectives := []struct {
		re         *regexp.Regexp
		name, file string
	}{
		{assetGraphDirectiveRe, "asset_graph", "assets.json"},
		{webGraphDirectiveRe, "web_graph", "webfindings.json"},
	}
	usesNetwork := false
	for _, f := range files {
		for _, loc := range networkDirectiveRe.FindAllStringIndex(f.raw, -1) {
			usesNetwork = true
			if !exists("assets.json") {
				issues = append(issues, Issue{f.path, "network_assets_missing",
					":::network_diagram::: needs artifacts/assets.json, which doesn't exist — run `rex ingest` first, or the build fails.",
					SeverityError, lineAt(f.raw, loc[0])})
			}
		}
		for _, d := range chartDirectives {
			if exists(d.file) {
				continue
			}
			for _, loc := range d.re.FindAllStringIndex(f.raw, -1) {
				issues = append(issues, Issue{f.path, "chart_data_missing",
					fmt.Sprintf(":::%s::: has no data — artifacts/%s doesn't exist, so it renders \"No data available\".", d.name, d.file),
					SeverityWarning, lineAt(f.raw, loc[0])})
			}
		}
	}

	// assets.json: every host needs a valid IP for the diagram to place it.
	var inventory map[netip.Addr]bool // nil when assets.json is absent or unreadable
	assetsPath := filepath.Join(artifacts, "assets.json")
	if data, err := os.ReadFile(assetsPath); err == nil {
		raw := string(data)
		var assets []map[string]any
		if issue, ok := decodeArtifact(assetsPath, raw, &assets, "a JSON array of host objects"); !ok {
			issues = append(issues, issue)
		} else {
			inventory = map[netip.Addr]bool{}
			for i, a := range assets {
				ipStr := fieldStr(a, "ip")
				ip, err := netip.ParseAddr(strings.TrimSpace(ipStr))
				if err != nil {
					issues = append(issues, Issue{assetsPath, "assets_invalid",
						fmt.Sprintf("Host #%d has an invalid 'ip' %q — the network diagram can't place it.", i+1, ipStr),
						SeverityError, jsonLine(raw, ipStr)})
					continue
				}
				inventory[ip] = true
			}
		}
	}

	// connections.json: hand-written, so check every key and value.
	connPath := filepath.Join(artifacts, "connections.json")
	data, err := os.ReadFile(connPath)
	if err != nil {
		return issues
	}
	raw := string(data)
	var conn map[string]any
	if issue, ok := decodeArtifact(connPath, raw, &conn, "a JSON object"); !ok {
		return append(issues, issue)
	}
	invalid := func(at, msg string) {
		issues = append(issues, Issue{connPath, "connections_invalid", msg, SeverityError, jsonLine(raw, at)})
	}
	unknownHost := func(ip netip.Addr, s, key string) {
		if inventory != nil && !inventory[ip] {
			issues = append(issues, Issue{connPath, "connections_host_unknown",
				fmt.Sprintf("%s: %s isn't in artifacts/assets.json — it'll be drawn as an unknown host. Typo?", key, s),
				SeverityWarning, jsonLine(raw, s)})
		}
	}
	endpoint := func(s string) {
		e, err := parseEndpoint(s)
		if err != nil {
			invalid(s, fmt.Sprintf("pivots: %q is not a valid IP or CIDR.", s))
		} else if !e.isNet {
			unknownHost(e.addr, s, "pivots")
		}
	}
	for _, key := range sortedKeys(conn) {
		switch key {
		case "subnets":
			subnets, ok := conn[key].(map[string]any)
			if !ok {
				invalid(key, "'subnets' must be an object mapping CIDR → label.")
				continue
			}
			for _, cidr := range sortedKeys(subnets) {
				if _, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err != nil {
					invalid(cidr, fmt.Sprintf("subnets: %q is not a valid CIDR (e.g. 10.0.0.0/24).", cidr))
				}
			}
		case "pivots":
			pivots, ok := conn[key].(map[string]any)
			if !ok {
				invalid(key, "'pivots' must be an object mapping source → target(s).")
				continue
			}
			for _, src := range sortedKeys(pivots) {
				endpoint(src)
				switch v := pivots[src].(type) {
				case string:
					endpoint(v)
				case []any:
					for _, d := range v {
						if s, ok := d.(string); ok {
							endpoint(s)
						} else {
							invalid(src, fmt.Sprintf("pivots: every target of %q must be an IP/CIDR string.", src))
						}
					}
				default:
					invalid(src, fmt.Sprintf("pivots: the target of %q must be an IP/CIDR string or a list of them.", src))
				}
			}
		case "important":
			list, ok := conn[key].([]any)
			if !ok {
				invalid(key, "'important' must be a list of IPs.")
				continue
			}
			for _, v := range list {
				s := fmt.Sprint(v)
				if ip, err := netip.ParseAddr(strings.TrimSpace(s)); err != nil {
					invalid(s, fmt.Sprintf("important: %q is not a valid IP.", s))
				} else {
					unknownHost(ip, s, "important")
				}
			}
		default:
			issues = append(issues, Issue{connPath, "connections_key_unknown",
				fmt.Sprintf("Unknown key %q — expected 'subnets', 'pivots' or 'important'; it's ignored.", key),
				SeverityWarning, jsonLine(raw, key)})
		}
	}
	if !usesNetwork {
		issues = append(issues, Issue{connPath, "connections_unused",
			"No report file uses :::network_diagram:::, so this file has no effect.", SeverityWarning, 0})
	}
	return issues
}
