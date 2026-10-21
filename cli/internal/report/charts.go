package report

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"sort"
	"strings"
)

// ChartBlockRE matches the tagged raw-LaTeX chart blocks rawLatex() emits
// below. chartraster.go uses this to find and rasterize charts for the DOCX
// pipeline, since pandoc's docx writer silently drops {=latex} raw blocks
// (DOCX has no TikZ renderer).
var ChartBlockRE = regexp.MustCompile(
	"(?s)<!--rex-chart:([0-9a-f]{10})-->\n```\\{=latex}\n(.*?)\n```\n<!--/rex-chart-->",
)

// severityColors maps severity -> LaTeX colour name (defined in the template preamble).
var severityColors = []struct{ Key, Color string }{
	{"Critical", "colorCritical"},
	{"High", "colorHigh"},
	{"Medium", "colorMedium"},
	{"Low", "colorLow"},
	{"Info", "colorInfo"},
}

var chartPalette = func() []string {
	out := make([]string, 10)
	for i := range out {
		out[i] = fmt.Sprintf("chartc%d", i+1)
	}
	return out
}()

var datasetAliases = map[string]string{
	"severity": "severity", "sev": "severity",
	"owasp": "owasp", "mitre": "mitre", "attack": "mitre",
	"business": "business", "business-risk": "business", "business_risk": "business", "risk": "business",
	"material": "material", "material_impact": "material", "material-impact": "material", "fair-mam": "material",
}

var chartTypeAliases = map[string]string{
	"pie": "pie", "donut": "donut", "doughnut": "donut",
	"bar": "bar", "chart": "bar",
	"matrix": "matrix", "table": "matrix",
	"radar": "radar", "spider": "radar",
}

// orderedCounts preserves insertion order, mirroring Python's OrderedDict.
type orderedCounts struct {
	keys   []string
	counts map[string]int
}

func newOrderedCounts() *orderedCounts {
	return &orderedCounts{counts: map[string]int{}}
}

func (o *orderedCounts) bump(key string) {
	if _, ok := o.counts[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.counts[key]++
}

func (o *orderedCounts) nonZero() *orderedCounts {
	out := newOrderedCounts()
	for _, k := range o.keys {
		if o.counts[k] > 0 {
			out.keys = append(out.keys, k)
			out.counts[k] = o.counts[k]
		}
	}
	return out
}

func fieldStr(f map[string]any, key string) string {
	v, ok := f[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func fieldStrDefault(f map[string]any, key, def string) string {
	if s := fieldStr(f, key); s != "" {
		return s
	}
	return def
}

// fieldSlice reads a []any field (e.g. an asset's "ports" or "hostnames") out of a
// map decoded from JSON, where nested objects/arrays come through as `any`.
func fieldSlice(f map[string]any, key string) []any {
	v, _ := f[key].([]any)
	return v
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func tallySeverity(findings []map[string]any) *orderedCounts {
	counts := newOrderedCounts()
	for _, c := range severityColors {
		counts.keys = append(counts.keys, c.Key)
		counts.counts[c.Key] = 0
	}
	for _, f := range findings {
		sev := fieldStr(f, "severity")
		if sev == "" || sev == "None" {
			sev = "Info"
		}
		if _, ok := counts.counts[sev]; !ok {
			counts.keys = append(counts.keys, sev)
		}
		counts.counts[sev]++
	}
	return counts.nonZero()
}

func tallyByCodes(findings []map[string]any, field string) *orderedCounts {
	counts := newOrderedCounts()
	for _, f := range findings {
		for _, code := range SplitCodes(f[field]) {
			counts.bump(Label(code))
		}
	}
	return counts
}

func tallyBusiness(findings []map[string]any) *orderedCounts {
	counts := newOrderedCounts()
	for _, cat := range BusinessRiskCategories {
		counts.keys = append(counts.keys, cat)
		counts.counts[cat] = 0
	}
	for _, f := range findings {
		for _, raw := range SplitCodes(f["business_risk"]) {
			cat, ok := NormaliseBusinessRisk(raw)
			if !ok {
				cat = raw
			}
			if _, exists := counts.counts[cat]; !exists {
				counts.keys = append(counts.keys, cat)
			}
			counts.counts[cat]++
		}
	}
	return counts.nonZero()
}

func tally(findings []map[string]any, dataset string) *orderedCounts {
	switch dataset {
	case "severity":
		return tallySeverity(findings)
	case "owasp":
		return tallyByCodes(findings, "owasp")
	case "mitre":
		return tallyByCodes(findings, "mitre")
	case "material":
		return tallyByCodes(findings, "material_impact")
	case "business":
		return tallyBusiness(findings)
	default:
		return newOrderedCounts()
	}
}

func colorsFor(dataset string, keys []string) []string {
	if dataset == "severity" {
		byKey := map[string]string{}
		for _, c := range severityColors {
			byKey[c.Key] = c.Color
		}
		out := make([]string, len(keys))
		for i, k := range keys {
			if c, ok := byKey[k]; ok {
				out[i] = c
			} else {
				out[i] = "colorInfo"
			}
		}
		return out
	}
	out := make([]string, len(keys))
	for i := range keys {
		out[i] = chartPalette[i%len(chartPalette)]
	}
	return out
}

// escLatex escapes LaTeX specials in a chart label (also drops commas for pgf-pie).
// Long unbroken tokens (hostnames, IPs, URLs) have no spaces for a tabularray
// X-column to wrap at, so without a break hint they overflow into neighboring
// cells; \allowbreak after '.', '/', ':' gives LaTeX a place to break without
// printing anything when it doesn't. Mirrors cell_to_latex in the templates'
// vuln_filter.lua, which applies the same fix for pandoc-authored tables.
func escLatex(s string) string {
	replacer := strings.NewReplacer(
		"\\", `\textbackslash{}`,
		"&", `\&`,
		"%", `\%`,
		"$", `\$`,
		"#", `\#`,
		"_", `\_`,
		"{", `\{`,
		"}", `\}`,
		".", `.\allowbreak{}`,
		"/", `/\allowbreak{}`,
		":", `:\allowbreak{}`,
	)
	out := replacer.Replace(s)
	return strings.ReplaceAll(out, ",", ";")
}

func emptyChart(dataset string) string {
	return "\\begin{center}\\textit{\\small No data available for the " +
		escLatex(dataset) + " breakdown.}\\end{center}"
}

func rawLatex(latex string) string {
	latex = strings.TrimSpace(latex)
	sum := sha1.Sum([]byte(latex))
	chartID := hex.EncodeToString(sum[:])[:10]
	return "<!--rex-chart:" + chartID + "-->\n```{=latex}\n" + latex + "\n```\n<!--/rex-chart-->"
}

func pieChart(counts *orderedCounts, colors []string, donut bool) string {
	total := 0
	entries := make([]string, len(counts.keys))
	for i, k := range counts.keys {
		v := counts.counts[k]
		total += v
		entries[i] = fmt.Sprintf("%d/%s", v, escLatex(k))
	}
	colorList := strings.Join(colors, ", ")
	inner, hide := "", ""
	if len(counts.keys) == 1 {
		// pgf-pie draws a 100% wedge as a sector: a seam from the centre to the
		// rim, with the label sitting on it. Paint a whole disc over it instead (#76).
		hide = "hide number, "
		inner = fmt.Sprintf("\\filldraw[fill=%s, draw=black] (0,0) circle (2.3);", colors[0])
		if !donut {
			inner += fmt.Sprintf("\\node[text=white, font=\\small\\bfseries] at (0,0) {%d};", total)
		}
	}
	if donut {
		// pgf-pie puts numbers at r/2, right under the hole (#79): draw them
		// mid-ring ourselves, and caption the centre so it isn't read as a slice.
		hide = "hide number, "
		inner += "\\filldraw[white] (0,0) circle (1.15);"
		start := 0.0
		for _, k := range counts.keys {
			v := counts.counts[k]
			sweep := 360 * float64(v) / float64(total)
			inner += fmt.Sprintf("\\node[text=white, font=\\small\\bfseries] at (%.1f:1.725) {%d};", start+sweep/2, v)
			start += sweep
		}
		inner += fmt.Sprintf("\\node[align=center] at (0,0) {{\\bfseries\\large %d}\\\\{\\scriptsize Total}};", total)
	}
	return "\\begin{center}\n" +
		"\\begin{tikzpicture}\n" +
		fmt.Sprintf("\\pie[radius=2.3, text=legend, sum=auto, %scolor={%s},\n", hide, colorList) +
		"  before number=, after number=, every only number node/.style={text=white,font=\\small\\bfseries}]\n" +
		fmt.Sprintf("  {%s}\n", strings.Join(entries, ", ")) +
		inner + "\n" +
		"\\end{tikzpicture}\n" +
		"\\end{center}"
}

func barChart(counts *orderedCounts, colors []string) string {
	maxv := 1
	for _, k := range counts.keys {
		if v := counts.counts[k]; v > maxv {
			maxv = v
		}
	}
	const barW = 7.0
	const rowH = 0.62
	var b strings.Builder
	b.WriteString("\\begin{center}\n\\begin{tikzpicture}[font=\\small]\n")
	n := len(counts.keys)
	for i, k := range counts.keys {
		v := counts.counts[k]
		color := colors[i]
		y := -(float64(i) * rowH)
		w := (float64(v) / float64(maxv)) * barW
		fmt.Fprintf(&b, "\\node[anchor=east] at (-0.15,%.2f) {%s};\n", y, escLatex(k))
		fmt.Fprintf(&b, "\\filldraw[%s, rounded corners=1pt] (0,%.2f) rectangle (%.3f,%.2f);\n", color, y-0.2, w, y+0.2)
		fmt.Fprintf(&b, "\\node[anchor=west, font=\\bfseries] at (%.3f,%.2f) {%d};\n", w+0.1, y, v)
	}
	fmt.Fprintf(&b, "\\draw[gray!50] (0,%.2f) -- (0,%.2f);\n", 0.32, -(float64(n-1))*rowH-0.32)
	b.WriteString("\\end{tikzpicture}\n\\end{center}")
	return b.String()
}

// radarChart draws a spider chart: one spoke per category, the counts as a
// filled polygon. Mirrors charts.py's _radar.
func radarChart(counts *orderedCounts) string {
	n := len(counts.keys)
	maxv := 0
	for _, k := range counts.keys {
		if v := counts.counts[k]; v > maxv {
			maxv = v
		}
	}
	if maxv == 0 {
		maxv = 1
	}
	rings := maxv
	if rings > 5 {
		rings = 5
	}
	const radius = 2.4 // cm, outer ring
	ang := func(i int) float64 { return 90 - float64(i)*360/float64(n) }
	poly := func(values []float64) string {
		pts := make([]string, len(values))
		for i, v := range values {
			pts[i] = fmt.Sprintf("(%.1f:%.3f)", ang(i), radius*v/float64(maxv))
		}
		return strings.Join(pts, " -- ")
	}

	var b strings.Builder
	b.WriteString("\\begin{center}\n\\begin{tikzpicture}[font=\\small]\n")
	for r := 1; r <= rings; r++ {
		ring := make([]float64, n)
		for i := range ring {
			ring[i] = float64(maxv) * float64(r) / float64(rings)
		}
		fmt.Fprintf(&b, "\\draw[gray!35] %s -- cycle;\n", poly(ring))
	}
	values := make([]float64, n)
	for i, k := range counts.keys {
		a := ang(i)
		values[i] = float64(counts.counts[k])
		c, sn := math.Cos(a*math.Pi/180), math.Sin(a*math.Pi/180)
		anchor := "north"
		switch {
		case c > 0.2:
			anchor = "west"
		case c < -0.2:
			anchor = "east"
		case sn > 0:
			anchor = "south"
		}
		fmt.Fprintf(&b, "\\draw[gray!35] (0,0) -- (%.1f:%.2f);\n", a, radius)
		fmt.Fprintf(&b, "\\node[anchor=%s] at (%.1f:%.2f) {%s \\textbf{(%d)}};\n", anchor, a, radius+0.15, escLatex(k), counts.counts[k])
	}
	fmt.Fprintf(&b, "\\filldraw[draw=chartc1, fill=chartc1!35, line width=1pt, fill opacity=0.8] %s -- cycle;\n", poly(values))
	b.WriteString("\\end{tikzpicture}\n\\end{center}")
	return b.String()
}

func matrixChart(findings []map[string]any, dataset string) string {
	sevs := make([]string, len(severityColors))
	for i, c := range severityColors {
		sevs[i] = c.Key
	}

	rowOrder := []string{}
	rowCounts := map[string]map[string]int{}
	bump := func(cat, sev string) {
		if _, ok := rowCounts[cat]; !ok {
			rowCounts[cat] = map[string]int{}
			for _, s := range sevs {
				rowCounts[cat][s] = 0
			}
			rowOrder = append(rowOrder, cat)
		}
		if sev == "" || sev == "None" {
			sev = "Info"
		}
		rowCounts[cat][sev]++
	}

	field := dataset
	switch dataset {
	case "owasp":
		field = "owasp"
	case "mitre":
		field = "mitre"
	case "business":
		field = "business_risk"
	case "material":
		field = "material_impact"
	}

	for _, f := range findings {
		sev := fieldStr(f, "severity")
		if sev == "" {
			sev = "Info"
		}
		codes := SplitCodes(f[field])
		normalised := make([]string, 0, len(codes))
		switch dataset {
		case "business":
			for _, c := range codes {
				if cat, ok := NormaliseBusinessRisk(c); ok {
					normalised = append(normalised, cat)
				} else {
					normalised = append(normalised, c)
				}
			}
		case "owasp", "mitre":
			for _, c := range codes {
				normalised = append(normalised, Label(c))
			}
		default:
			normalised = codes
		}
		for _, cat := range normalised {
			bump(cat, sev)
		}
	}

	if len(rowOrder) == 0 {
		return emptyChart(dataset)
	}

	headerCols := []string{"\\textbf{Category}"}
	for _, s := range sevs {
		headerCols = append(headerCols, fmt.Sprintf("\\textbf{%s}", s))
	}
	header := strings.Join(headerCols, " & ") + " \\\\"

	bodyRows := make([]string, 0, len(rowOrder))
	for _, cat := range rowOrder {
		cells := rowCounts[cat]
		cols := make([]string, 0, len(sevs))
		for _, s := range sevs {
			c := cells[s]
			if c > 0 {
				shade := 20 + 25*c
				if shade > 85 {
					shade = 85
				}
				byKey := map[string]string{}
				for _, sc := range severityColors {
					byKey[sc.Key] = sc.Color
				}
				cols = append(cols, fmt.Sprintf("\\SetCell{bg=%s!%d!white} \\textbf{%d}", byKey[s], shade, c))
			} else {
				cols = append(cols, "\\SetCell{bg=gray!4} \\textcolor{gray!55}{0}")
			}
		}
		bodyRows = append(bodyRows, escLatex(cat)+" & "+strings.Join(cols, " & ")+" \\\\")
	}

	colspec := fmt.Sprintf("colspec={Q[l,wd=5.2cm] *{%d}{Q[c,wd=1.5cm]}}", len(sevs))
	return "\\begin{center}\n" +
		fmt.Sprintf("\\begin{tblr}{%s, hlines={gray!30}, vlines={gray!30}, row{1}={bg=colorPrimary, fg=white, font=\\bfseries}}\n", colspec) +
		header + "\n" +
		strings.Join(bodyRows, "\n") +
		"\n\\end{tblr}\n\\end{center}"
}

// Render returns a pandoc raw-LaTeX block for a `:::findings_graph:TYPE:DATASET:::` directive.
func Render(findings []map[string]any, gtype, dataset string) string {
	gt, ok := chartTypeAliases[strings.ToLower(strings.TrimSpace(gtype))]
	if !ok {
		return rawLatex("\\begin{center}\\textit{\\small Unknown chart type.}\\end{center}")
	}
	ds, ok := datasetAliases[strings.ToLower(strings.TrimSpace(dataset))]
	if !ok {
		ds = "severity"
	}

	if gt == "matrix" {
		return rawLatex(matrixChart(findings, ds))
	}

	counts := tally(findings, ds)
	if len(counts.keys) == 0 {
		return rawLatex(emptyChart(ds))
	}

	if gt == "radar" {
		if ds == "business" {
			// Every business category gets a spoke, so the shape stays comparable.
			full := newOrderedCounts()
			for _, cat := range BusinessRiskCategories {
				full.keys = append(full.keys, cat)
				full.counts[cat] = 0
			}
			for _, k := range counts.keys {
				if _, ok := full.counts[k]; !ok {
					full.keys = append(full.keys, k)
				}
				full.counts[k] = counts.counts[k]
			}
			counts = full
		}
		if len(counts.keys) >= 3 {
			return rawLatex(radarChart(counts))
		}
		gt = "bar" // a radar needs at least three spokes
	}
	colors := colorsFor(ds, counts.keys)

	switch gt {
	case "pie", "donut":
		return rawLatex(pieChart(counts, colors, gt == "donut"))
	case "bar":
		return rawLatex(barChart(counts, colors))
	default:
		return rawLatex(emptyChart(ds))
	}
}

// ── Asset charts (rex ingest) ────────────────────────────────────────────────

var assetDatasetAliases = map[string]string{
	"port": "ports", "ports": "ports",
	"os":      "os",
	"service": "services", "services": "services",
}

const topNAssetEntries = 10

// topN sorts by count descending (stable, so equal counts keep insertion order —
// mirrors Python's `sorted(counts.items(), key=lambda kv: kv[1], reverse=True)`)
// and caps at n entries.
func topN(counts *orderedCounts, n int) *orderedCounts {
	keys := append([]string{}, counts.keys...)
	sort.SliceStable(keys, func(i, j int) bool { return counts.counts[keys[i]] > counts.counts[keys[j]] })
	if len(keys) > n {
		keys = keys[:n]
	}
	out := newOrderedCounts()
	for _, k := range keys {
		out.keys = append(out.keys, k)
		out.counts[k] = counts.counts[k]
	}
	return out
}

func tallyAssets(assets []map[string]any, dataset string) *orderedCounts {
	counts := newOrderedCounts()
	switch dataset {
	case "os":
		for _, a := range assets {
			counts.bump(fieldStrDefault(a, "os", "Unknown"))
		}
	case "ports":
		for _, a := range assets {
			for _, pv := range fieldSlice(a, "ports") {
				p := asMap(pv)
				counts.bump(fmt.Sprintf("%s/%s", fieldStrDefault(p, "number", "?"), fieldStrDefault(p, "proto", "?")))
			}
		}
	case "services":
		for _, a := range assets {
			for _, pv := range fieldSlice(a, "ports") {
				p := asMap(pv)
				counts.bump(fieldStrDefault(p, "service", "unknown"))
			}
		}
	}
	return topN(counts, topNAssetEntries)
}

// assetMatrixChart renders the full host/port inventory as a table — every asset,
// every open port, one row each — for a `:::asset_graph:table:...:::` directive.
// Uses `tblr`, the same non-paginating tabularray environment matrixChart above
// uses — proven to compile in this template, unlike `longtblr`/`longtblr*` (tried
// and reverted: the former auto-numbers as "Table 1:" even with no caption text,
// the latter isn't a real environment). Explicit X-column weights replace the
// dash-count heuristic that made the old hand-built markdown table overlap on
// long IPs/version strings.
// ponytail: single-page-only — a very large inventory (many hosts, full
// port-range scans) can overflow past the page footer. Upgrade to tabularray's
// longtblr if/when that's hit in practice, with its caption suppressed via
// `entry=none, caption={}` (untested here).
func assetMatrixChart(assets []map[string]any) string {
	rows := append([]map[string]any{}, assets...)
	sort.Slice(rows, func(i, j int) bool { return fieldStr(rows[i], "ip") < fieldStr(rows[j], "ip") })
	if len(rows) == 0 {
		return emptyChart("ports")
	}

	var bodyRows []string
	for _, a := range rows {
		ip := escLatex(fieldStr(a, "ip"))
		var hostnames []string
		for _, h := range fieldSlice(a, "hostnames") {
			if s, ok := h.(string); ok {
				hostnames = append(hostnames, s)
			}
		}
		hostname := escLatex(strings.Join(hostnames, ", "))
		osName := escLatex(fieldStrDefault(a, "os", "Unknown"))

		ports := fieldSlice(a, "ports")
		if len(ports) == 0 {
			bodyRows = append(bodyRows, fmt.Sprintf("%s & %s & %s & & & & \\\\", ip, hostname, osName))
			continue
		}
		for _, pv := range ports {
			p := asMap(pv)
			port := escLatex(fieldStr(p, "number"))
			proto := escLatex(fieldStr(p, "proto"))
			service := escLatex(fieldStr(p, "service"))
			version := escLatex(strings.TrimSpace(fieldStr(p, "product") + " " + fieldStr(p, "version")))
			bodyRows = append(bodyRows, fmt.Sprintf("%s & %s & %s & %s & %s & %s & %s \\\\", ip, hostname, osName, port, proto, service, version))
		}
	}

	header := "\\textbf{Host} & \\textbf{Hostname} & \\textbf{OS} & \\textbf{Port} & " +
		"\\textbf{Proto} & \\textbf{Service} & \\textbf{Version} \\\\"
	colspec := "colspec={X[3,l] X[3,l] X[2,l] X[1,c] X[1,c] X[2,l] X[4,l]}"
	return "\\begin{center}\n" +
		fmt.Sprintf("\\begin{tblr}{%s, hlines={gray!30}, vlines={gray!30}, row{1}={bg=colorPrimary, fg=white, font=\\bfseries}}\n", colspec) +
		header + "\n" +
		strings.Join(bodyRows, "\n") +
		"\n\\end{tblr}\n\\end{center}"
}

// RenderAssets returns a pandoc raw-LaTeX block for an `:::asset_graph:TYPE:DATASET:::` directive.
func RenderAssets(assets []map[string]any, gtype, dataset string) string {
	gt, ok := chartTypeAliases[strings.ToLower(strings.TrimSpace(gtype))]
	if !ok {
		return rawLatex("\\begin{center}\\textit{\\small Unknown chart type.}\\end{center}")
	}
	ds, ok := assetDatasetAliases[strings.ToLower(strings.TrimSpace(dataset))]
	if !ok {
		ds = "os"
	}

	if gt == "matrix" {
		return rawLatex(assetMatrixChart(assets))
	}

	counts := tallyAssets(assets, ds)
	if len(counts.keys) == 0 {
		return rawLatex(emptyChart(ds))
	}
	colors := colorsFor(ds, counts.keys)

	switch gt {
	case "pie", "donut":
		return rawLatex(pieChart(counts, colors, gt == "donut"))
	case "bar":
		return rawLatex(barChart(counts, colors))
	default:
		return rawLatex(emptyChart(ds))
	}
}

// ── Network diagram (rex ingest + artifacts/connections.json) ───────────────

// osFamilies: name substrings, fontawesome5 glyph, tile colour, legend label.
// First match wins; anything unmatched is the last entry.
var osFamilies = []struct {
	keys               []string
	icon, color, label string
}{
	{[]string{"windows"}, `\faWindows`, "cyan!55!blue", "Windows"},
	{[]string{"linux", "ubuntu", "debian", "centos", "red hat", "redhat", "fedora", "kali"}, `\faLinux`, "orange!90!black", "Linux"},
	{[]string{"mac", "apple", "darwin"}, `\faApple`, "gray!65!black", "macOS"},
	{[]string{"freebsd"}, `\faFreebsd`, "red!65!black", "FreeBSD"},
	{[]string{"android"}, `\faAndroid`, "green!55!black", "Android"},
	{nil, `\faServer`, "gray!55", "Other / unknown"},
}

const (
	netCellW    = 2.4  // cm per host column inside a subnet region
	netCellH    = 2.1  // cm per host row
	netGap      = 0.9  // cm between subnet regions
	netRow0     = -2.5 // y of the first host row (region top is 0, switch at -0.9)
	netMaxW     = 16.0 // cm of text width before the diagram is scaled down
	netMaxH     = 20.0 // cm of text height before the diagram is scaled down
	netMaxHosts = 12   // hosts per subnet before unrelated hosts are hidden
)

const netStyles = `sw/.style={rounded corners=3pt, minimum size=0.8cm, inner sep=0pt, fill=gray!70!black, ` +
	`text=white, font=\large}, ` +
	`nd/.style={rounded corners=3pt, minimum size=0.9cm, inner sep=0pt, text=white, font=\Large}, ` +
	`imp/.style={draw=colorCritical, line width=1.6pt}, ` +
	`lb/.style={anchor=north, align=center, text width=2.3cm, inner sep=1pt, ` +
	`fill=colorPrimary!4, font=\scriptsize}`

func osFamily(osName string) int {
	s := strings.ToLower(osName)
	for i, f := range osFamilies[:len(osFamilies)-1] {
		for _, k := range f.keys {
			if strings.Contains(s, k) {
				return i
			}
		}
	}
	return len(osFamilies) - 1
}

// netEndpoint is a connections.json pivot endpoint: an IP or a CIDR.
type netEndpoint struct {
	addr   netip.Addr
	prefix netip.Prefix
	isNet  bool
}

func (e netEndpoint) key() string {
	if e.isNet {
		return "net:" + e.prefix.String()
	}
	return "ip:" + e.addr.String()
}

func parseEndpoint(s string) (netEndpoint, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		return netEndpoint{prefix: p.Masked(), isNet: true}, err
	}
	a, err := netip.ParseAddr(s)
	return netEndpoint{addr: a}, err
}

// RenderNetwork returns a pandoc raw-LaTeX block for a `:::network_diagram:::`
// directive: a topology map with a core cloud linked to one region per subnet,
// each with a switch linked to its hosts (OS-coloured icon tiles). Hosts from
// assets.json go into the most specific `subnets` CIDR from connections.json that
// contains them (else an automatic /24, /64 for IPv6). Each `pivots` entry (IP or
// CIDR → IP, CIDR, or a list of them) is drawn as a dashed red arrow; a pivot
// endpoint or `important` IP not in the inventory is added. `important` hosts get
// a red outline.
//
// A subnet over netMaxHosts hosts hides hosts with no pivot and not marked
// `important` (shown as "+N more hosts"); pivot/important hosts are always drawn,
// and the whole diagram is scaled down if it's still too big. Mirrors charts.py's
// render_network byte for byte.
func RenderNetwork(assets []map[string]any, connections map[string]any) (string, error) {
	labels := map[netip.Prefix]string{}
	for k, v := range asMap(connections["subnets"]) {
		p, err := netip.ParsePrefix(strings.TrimSpace(k))
		if err != nil {
			return "", err
		}
		labels[p.Masked()] = fmt.Sprint(v)
	}

	type pivot struct{ src, dst netEndpoint }
	var pivots []pivot
	pv := asMap(connections["pivots"])
	srcs := make([]string, 0, len(pv))
	for k := range pv {
		srcs = append(srcs, k)
	}
	sort.Strings(srcs)
	for _, src := range srcs {
		var dsts []string
		switch v := pv[src].(type) {
		case string:
			dsts = []string{v}
		case []any:
			for _, d := range v {
				dsts = append(dsts, fmt.Sprint(d))
			}
		default:
			return "", fmt.Errorf("pivot %q: target must be a string or a list", src)
		}
		s, err := parseEndpoint(src)
		if err != nil {
			return "", err
		}
		for _, d := range dsts {
			e, err := parseEndpoint(d)
			if err != nil {
				return "", err
			}
			pivots = append(pivots, pivot{s, e})
		}
	}

	important := map[netip.Addr]bool{}
	for _, i := range fieldSlice(connections, "important") {
		ip, err := netip.ParseAddr(strings.TrimSpace(fmt.Sprint(i)))
		if err != nil {
			return "", err
		}
		important[ip] = true
	}
	mustShow := map[netip.Addr]bool{}
	for ip := range important {
		mustShow[ip] = true
	}
	for _, pv := range pivots {
		for _, e := range []netEndpoint{pv.src, pv.dst} {
			if !e.isNet {
				mustShow[e.addr] = true
			}
		}
	}

	type host struct{ os, hostname string }
	hosts := map[netip.Addr]host{}
	for _, a := range assets {
		ip, err := netip.ParseAddr(strings.TrimSpace(fieldStr(a, "ip")))
		if err != nil {
			return "", err
		}
		h := host{os: fieldStrDefault(a, "os", "Unknown")}
		if hn := fieldSlice(a, "hostnames"); len(hn) > 0 {
			h.hostname = fmt.Sprint(hn[0])
		}
		hosts[ip] = h
	}
	for ip := range mustShow {
		if _, ok := hosts[ip]; !ok {
			hosts[ip] = host{os: "Unknown"}
		}
	}

	groups := map[netip.Prefix][]netip.Addr{}
	for p := range labels {
		groups[p] = nil
	}
	for _, pv := range pivots {
		for _, e := range []netEndpoint{pv.src, pv.dst} {
			if _, ok := groups[e.prefix]; e.isNet && !ok {
				groups[e.prefix] = nil
			}
		}
	}
	subnetOf := func(ip netip.Addr) netip.Prefix {
		var best netip.Prefix
		for p := range labels {
			if p.Contains(ip) && (!best.IsValid() || p.Bits() > best.Bits()) {
				best = p
			}
		}
		if best.IsValid() {
			return best
		}
		bits := 64
		if ip.Is4() {
			bits = 24
		}
		return netip.PrefixFrom(ip, bits).Masked()
	}
	ips := make([]netip.Addr, 0, len(hosts))
	for ip := range hosts {
		ips = append(ips, ip)
	}
	sort.Slice(ips, func(i, j int) bool { return ips[i].Less(ips[j]) })
	for _, ip := range ips {
		p := subnetOf(ip)
		groups[p] = append(groups[p], ip)
	}
	if len(groups) == 0 {
		return rawLatex(emptyChart("network")), nil
	}

	// Trim each subnet to netMaxHosts, never dropping a must-show host.
	hidden := map[netip.Prefix]int{}
	anyHidden := false
	for p, ips := range groups {
		free := netMaxHosts
		for _, ip := range ips {
			if mustShow[ip] {
				free--
			}
		}
		var shown []netip.Addr
		for _, ip := range ips {
			switch {
			case mustShow[ip]:
				shown = append(shown, ip)
			case free > 0:
				free--
				shown = append(shown, ip)
			}
		}
		groups[p], hidden[p] = shown, len(ips)-len(shown)
		anyHidden = anyHidden || hidden[p] > 0
	}

	// ── Geometry: regions left to right, hosts in a grid under each switch ──
	ordered := make([]netip.Prefix, 0, len(groups))
	for p := range groups {
		ordered = append(ordered, p)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if c := ordered[i].Addr().Compare(ordered[j].Addr()); c != 0 {
			return c < 0
		}
		return ordered[i].Bits() < ordered[j].Bits()
	})
	type region struct {
		net         netip.Prefix
		left, width float64
		cols        int
	}
	var regions []region
	x := 0.0
	maxRows := 1
	for _, p := range ordered {
		n := len(groups[p])
		cols := 3
		switch {
		case n <= 1:
			cols = 1
		case n <= 4:
			cols = 2
		}
		width := float64(cols)*netCellW + 0.4
		regions = append(regions, region{p, x, width, cols})
		x += width + netGap
		maxRows = max(maxRows, (n+cols-1)/cols)
	}
	totalW := x - netGap
	extra := 0.0
	if anyHidden {
		extra = 0.45
	}
	bottom := netRow0 - float64(maxRows-1)*netCellH - 1.25 - extra
	coreX, coreY := totalW/2, 2.1

	nodes := map[string]string{} // endpoint key → tikz node name
	type xy struct{ x, y float64 }
	hostXY := map[netip.Addr]xy{}
	for i, r := range regions {
		nodes[netEndpoint{prefix: r.net, isNet: true}.key()] = fmt.Sprintf("s%d", i)
		for j, ip := range groups[r.net] {
			row, col := j/r.cols, j%r.cols
			hostXY[ip] = xy{r.left + 0.2 + (float64(col)+0.5)*netCellW, netRow0 - float64(row)*netCellH}
			nodes[netEndpoint{addr: ip}.key()] = fmt.Sprintf("h%dx%d", i, j)
		}
	}

	// Links first (from known coordinates) so the opaque icons/labels drawn
	// afterwards cover them; pivots last, on top of everything.
	var lines []string
	for _, r := range regions {
		lines = append(lines, fmt.Sprintf(
			`\draw[colorPrimary!50, fill=colorPrimary!4, rounded corners=6pt] (%.2f,0) rectangle (%.2f,%.2f);`,
			r.left, r.left+r.width, bottom))
	}
	for _, r := range regions {
		cx := r.left + r.width/2
		lines = append(lines, fmt.Sprintf(`\draw[gray!65, line width=1.2pt] (%.2f,%.2f) -- (%.2f,-0.90);`, coreX, coreY, cx))
		ips := groups[r.net]
		if len(ips) == 0 {
			continue
		}
		// Bus layout: switch → a bus line above each host row (joined by a trunk
		// down the region's left edge) → a short drop to each host.
		nrows := (len(ips) + r.cols - 1) / r.cols
		bus := make([]float64, nrows)
		for row := range bus {
			bus[row] = netRow0 + 0.75 - float64(row)*netCellH
		}
		tx := r.left + 0.12
		lines = append(lines, fmt.Sprintf(`\draw[gray!55] (%.2f,-0.90) -- (%.2f,%.2f);`, cx, cx, bus[0]))
		if nrows > 1 {
			lines = append(lines, fmt.Sprintf(`\draw[gray!55] (%.2f,%.2f) -- (%.2f,%.2f);`, tx, bus[0], tx, bus[nrows-1]))
		}
		for row := 0; row < nrows; row++ {
			rowIPs := ips[row*r.cols : min((row+1)*r.cols, len(ips))]
			lo, hi := hostXY[rowIPs[0]].x, hostXY[rowIPs[0]].x
			if nrows > 1 { // the trunk
				lo, hi = math.Min(lo, tx), math.Max(hi, tx)
			}
			if row == 0 { // the switch drop
				lo, hi = math.Min(lo, cx), math.Max(hi, cx)
			}
			for _, ip := range rowIPs {
				lo, hi = math.Min(lo, hostXY[ip].x), math.Max(hi, hostXY[ip].x)
			}
			lines = append(lines, fmt.Sprintf(`\draw[gray!55] (%.2f,%.2f) -- (%.2f,%.2f);`, lo, bus[row], hi, bus[row]))
			for _, ip := range rowIPs {
				h := hostXY[ip]
				lines = append(lines, fmt.Sprintf(`\draw[gray!55] (%.2f,%.2f) -- (%.2f,%.2f);`, h.x, bus[row], h.x, h.y))
			}
		}
	}
	lines = append(lines,
		fmt.Sprintf(`\node[text=cyan!18, font=\fontsize{70}{70}\selectfont] at (%.2f,%.2f) {\faCloud};`, coreX, coreY),
		fmt.Sprintf(`\node[font=\small\bfseries, text=colorPrimary] at (%.2f,%.2f) {Core network};`, coreX, coreY-0.15))
	families := map[int]bool{}
	var hostLabels []string // drawn after the pivots, so arrows pass under the (filled) text
	for i, r := range regions {
		cx := r.left + r.width/2
		lines = append(lines, fmt.Sprintf(`\node[sw] (s%d) at (%.2f,-0.90) {\faNetworkWired};`, i, cx))
		for j, ip := range groups[r.net] {
			h, pt := hosts[ip], hostXY[ip]
			fam := osFamily(h.os)
			families[fam] = true
			style := "nd"
			if important[ip] {
				style = "nd, imp"
			}
			lines = append(lines, fmt.Sprintf(`\node[%s, fill=%s] (h%dx%d) at (%.2f,%.2f) {%s};`,
				style, osFamilies[fam].color, i, j, pt.x, pt.y, osFamilies[fam].icon))
			text := `\textbf{` + escLatex(ip.String()) + "}"
			if h.hostname != "" {
				text += `\\` + escLatex(h.hostname)
			}
			hostLabels = append(hostLabels, fmt.Sprintf(`\node[lb] at (%.2f,%.2f) {%s};`, pt.x, pt.y-0.55, text))
		}
		if hidden[r.net] > 0 {
			lines = append(lines, fmt.Sprintf(`\node[font=\scriptsize\itshape, text=gray] at (%.2f,%.2f) {+ %d more hosts};`,
				cx, bottom+0.3, hidden[r.net]))
		}
		title := ""
		if l, ok := labels[r.net]; ok {
			title = `\textbf{` + escLatex(l) + `}\\`
		}
		lines = append(lines, fmt.Sprintf(`\node[anchor=north, align=center, font=\small, text=colorPrimary] at (%.2f,%.2f) {%s{\scriptsize %s}};`,
			cx, bottom-0.1, title, escLatex(r.net.String())))
	}
	for _, pv := range pivots {
		lines = append(lines, fmt.Sprintf(
			`\draw[-latex, very thick, dashed, colorCritical] (%s) to[bend left=12] (%s);`,
			nodes[pv.src.key()], nodes[pv.dst.key()]))
	}
	lines = append(lines, hostLabels...)

	// Legend: only the OS families / line types actually present.
	type legendItem struct{ mark, name string }
	var items []legendItem
	for fam := range osFamilies {
		if families[fam] {
			f := osFamilies[fam]
			items = append(items, legendItem{
				fmt.Sprintf(`\node[nd, minimum size=0.5cm, font=\footnotesize, fill=%s] at (X,Y) {%s};`, f.color, f.icon), f.label})
		}
	}
	items = append(items, legendItem{`\draw[gray!55, line width=1.2pt] (X1,Y) -- (X2,Y);`, "Network link"})
	if len(pivots) > 0 {
		items = append(items, legendItem{`\draw[-latex, very thick, dashed, colorCritical] (X1,Y) -- (X2,Y);`, "Pivot"})
	}
	if len(important) > 0 {
		items = append(items, legendItem{`\node[nd, imp, minimum size=0.5cm, fill=gray!30] at (X,Y) {};`, "Important host"})
	}
	legendY := bottom - 1.45
	lx := 0.0
	f := func(v float64) string { return fmt.Sprintf("%.2f", v) }
	for _, it := range items {
		lines = append(lines,
			strings.NewReplacer("X1", f(lx-0.3), "X2", f(lx+0.3), "X", f(lx), "Y", f(legendY)).Replace(it.mark),
			fmt.Sprintf(`\node[anchor=west, font=\scriptsize] at (%.2f,%.2f) {%s};`, lx+0.4, legendY, it.name))
		lx += 1.0 + 0.16*float64(len(it.name)) // mark + label, ~0.16cm per \scriptsize character
	}

	width := math.Max(totalW, lx)
	height := coreY + 1.2 - (legendY - 0.4)
	scale := math.Min(1.0, math.Min(netMaxW/width, netMaxH/height))
	return rawLatex("\\begin{center}\n" +
		fmt.Sprintf("\\begin{tikzpicture}[scale=%.3f, every node/.style={transform shape}, %s]\n", scale, netStyles) +
		strings.Join(lines, "\n") +
		"\n\\end{tikzpicture}\n\\end{center}"), nil
}

// ── Web findings charts (rex ingest nikto) ───────────────────────────────────

var webDatasetAliases = map[string]string{
	"host": "host", "hosts": "host",
}

func tallyWeb(findings []map[string]any, dataset string) *orderedCounts {
	counts := newOrderedCounts()
	if dataset == "host" {
		for _, f := range findings {
			counts.bump(fieldStrDefault(f, "host", "Unknown"))
		}
	}
	return topN(counts, topNAssetEntries)
}

// RenderWeb returns a pandoc raw-LaTeX block for a `:::web_graph:TYPE:DATASET:::` directive.
func RenderWeb(findings []map[string]any, gtype, dataset string) string {
	gt, ok := chartTypeAliases[strings.ToLower(strings.TrimSpace(gtype))]
	if !ok || gt == "matrix" {
		return rawLatex("\\begin{center}\\textit{\\small Unknown chart type.}\\end{center}")
	}
	ds, ok := webDatasetAliases[strings.ToLower(strings.TrimSpace(dataset))]
	if !ok {
		ds = "host"
	}

	counts := tallyWeb(findings, ds)
	if len(counts.keys) == 0 {
		return rawLatex(emptyChart(ds))
	}
	colors := colorsFor(ds, counts.keys)

	switch gt {
	case "pie", "donut":
		return rawLatex(pieChart(counts, colors, gt == "donut"))
	case "bar":
		return rawLatex(barChart(counts, colors))
	default:
		return rawLatex(emptyChart(ds))
	}
}
