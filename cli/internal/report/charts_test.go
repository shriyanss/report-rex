package report

import (
	"fmt"
	"strings"
	"testing"
)

// unwrapChart strips rawLatex()'s <!--rex-chart:id-->/```{=latex}/``` wrapper,
// returning just the TikZ body, so tests can assert on chart content without
// hardcoding the id (a content hash) or the tagging mechanism itself —
// CHART_BLOCK_RE's own round-trip is covered separately in chartraster_test.go.
func unwrapChart(t *testing.T, block string) string {
	t.Helper()
	m := ChartBlockRE.FindStringSubmatch(block)
	if m == nil {
		t.Fatalf("chart block missing rex-chart tag:\n%s", block)
	}
	return m[2]
}

func sampleFindings() []map[string]any {
	return []map[string]any{
		{"severity": "Critical", "owasp": "A03:2021", "mitre": "T1190", "business_risk": []any{"Operational"}},
		{"severity": "High", "owasp": "A01", "mitre": "T1059.001", "business_risk": "Financial"},
		{"severity": "High", "owasp": []any{"A01", "A03"}, "mitre": "T1190", "business_risk": "Financial, Operational"},
		{"severity": "Medium"},
		{"severity": "None"},
	}
}

// Expected outputs captured verbatim from `report_rex_core.charts.render` on the
// same input (core/src/report_rex_core/charts.py), to keep the Go port byte-compatible.
func TestRenderSeverityPie(t *testing.T) {
	want := "\\begin{center}\n" +
		"\\begin{tikzpicture}\n" +
		"\\pie[radius=2.3, text=legend, sum=auto, color={colorCritical, colorHigh, colorMedium, colorInfo},\n" +
		"  before number=, after number=, every only number node/.style={text=white,font=\\small\\bfseries}]\n" +
		"  {1/Critical, 2/High, 1/Medium, 1/Info}\n" +
		"\n" +
		"\\end{tikzpicture}\n" +
		"\\end{center}"
	got := unwrapChart(t, Render(sampleFindings(), "pie", "severity"))
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderOwaspBar(t *testing.T) {
	want := "\\begin{center}\n" +
		"\\begin{tikzpicture}[font=\\small]\n" +
		"\\node[anchor=east] at (-0.15,-0.00) {A03:\\allowbreak{}2021 — Injection};\n" +
		"\\filldraw[chartc1, rounded corners=1pt] (0,-0.20) rectangle (3.500,0.20);\n" +
		"\\node[anchor=west, font=\\bfseries] at (3.600,-0.00) {1};\n" +
		"\\node[anchor=east] at (-0.15,-0.62) {A01 — Broken Access Control};\n" +
		"\\filldraw[chartc2, rounded corners=1pt] (0,-0.82) rectangle (7.000,-0.42);\n" +
		"\\node[anchor=west, font=\\bfseries] at (7.100,-0.62) {2};\n" +
		"\\node[anchor=east] at (-0.15,-1.24) {A03 — Injection};\n" +
		"\\filldraw[chartc3, rounded corners=1pt] (0,-1.44) rectangle (3.500,-1.04);\n" +
		"\\node[anchor=west, font=\\bfseries] at (3.600,-1.24) {1};\n" +
		"\\draw[gray!50] (0,0.32) -- (0,-1.56);\n" +
		"\\end{tikzpicture}\n" +
		"\\end{center}"
	got := unwrapChart(t, Render(sampleFindings(), "bar", "owasp"))
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderMitreDonut(t *testing.T) {
	want := "\\begin{center}\n" +
		"\\begin{tikzpicture}\n" +
		"\\pie[radius=2.3, text=legend, sum=auto, hide number, color={chartc1, chartc2},\n" +
		"  before number=, after number=, every only number node/.style={text=white,font=\\small\\bfseries}]\n" +
		"  {2/T1190 — Exploit Public-Facing Application, 1/T1059.\\allowbreak{}001 — Command and Scripting Interpreter}\n" +
		"\\filldraw[white] (0,0) circle (1.15);" +
		"\\node[text=white, font=\\small\\bfseries] at (120.0:1.725) {2};" +
		"\\node[text=white, font=\\small\\bfseries] at (300.0:1.725) {1};" +
		"\\node[align=center] at (0,0) {{\\bfseries\\large 3}\\\\{\\scriptsize Total}};\n" +
		"\\end{tikzpicture}\n" +
		"\\end{center}"
	got := unwrapChart(t, Render(sampleFindings(), "donut", "mitre"))
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderBusinessMatrix(t *testing.T) {
	want := "\\begin{center}\n" +
		"\\begin{tblr}{colspec={Q[l,wd=5.2cm] *{5}{Q[c,wd=1.5cm]}}, hlines={gray!30}, vlines={gray!30}, row{1}={bg=colorPrimary, fg=white, font=\\bfseries}}\n" +
		"\\textbf{Category} & \\textbf{Critical} & \\textbf{High} & \\textbf{Medium} & \\textbf{Low} & \\textbf{Info} \\\\\n" +
		"Operational & \\SetCell{bg=colorCritical!45!white} \\textbf{1} & \\SetCell{bg=colorHigh!45!white} \\textbf{1} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} \\\\\n" +
		"Financial & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=colorHigh!70!white} \\textbf{2} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} \\\\\n" +
		"\\end{tblr}\n" +
		"\\end{center}"
	got := unwrapChart(t, Render(sampleFindings(), "matrix", "business"))
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderOwaspTableAlias(t *testing.T) {
	want := "\\begin{center}\n" +
		"\\begin{tblr}{colspec={Q[l,wd=5.2cm] *{5}{Q[c,wd=1.5cm]}}, hlines={gray!30}, vlines={gray!30}, row{1}={bg=colorPrimary, fg=white, font=\\bfseries}}\n" +
		"\\textbf{Category} & \\textbf{Critical} & \\textbf{High} & \\textbf{Medium} & \\textbf{Low} & \\textbf{Info} \\\\\n" +
		"A03:\\allowbreak{}2021 — Injection & \\SetCell{bg=colorCritical!45!white} \\textbf{1} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} \\\\\n" +
		"A01 — Broken Access Control & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=colorHigh!70!white} \\textbf{2} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} \\\\\n" +
		"A03 — Injection & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=colorHigh!45!white} \\textbf{1} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} & \\SetCell{bg=gray!4} \\textcolor{gray!55}{0} \\\\\n" +
		"\\end{tblr}\n" +
		"\\end{center}"
	got := unwrapChart(t, Render(sampleFindings(), "table", "owasp"))
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderUnknownType(t *testing.T) {
	want := "\\begin{center}\\textit{\\small Unknown chart type.}\\end{center}"
	got := unwrapChart(t, Render(sampleFindings(), "bogus", "severity"))
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderEmptyDataset(t *testing.T) {
	want := "\\begin{center}\\textit{\\small No data available for the owasp breakdown.}\\end{center}"
	got := unwrapChart(t, Render(nil, "pie", "owasp"))
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func sampleAssets() []map[string]any {
	return []map[string]any{
		{"ip": "10.0.0.2", "hostnames": []any{"b.example.com"}, "os": "Linux",
			"ports": []any{map[string]any{"number": "80", "proto": "tcp", "service": "http"}}},
		{"ip": "10.0.0.1", "hostnames": []any{"a.example.com"},
			"ports": []any{map[string]any{"number": "22", "proto": "tcp", "service": "ssh"}}},
	}
}

func TestRenderAssetsPieByOS(t *testing.T) {
	assets := []map[string]any{{"os": "Linux"}, {"os": "Linux"}, {"os": "Windows"}, {}}
	got := RenderAssets(assets, "pie", "os")
	for _, want := range []string{"tikzpicture", "Linux", "Unknown"} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderAssets pie/os missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAssetsSingleSlicePieDrawsFullDisc(t *testing.T) {
	got := RenderAssets([]map[string]any{{}}, "pie", "os")
	for _, want := range []string{"hide number", "circle (2.3)", "at (0,0) {1}"} {
		if !strings.Contains(got, want) {
			t.Errorf("single-slice pie missing %q (#76):\n%s", want, got)
		}
	}
	if multi := RenderAssets([]map[string]any{{"os": "Linux"}, {"os": "Windows"}}, "pie", "os"); strings.Contains(multi, "hide number") {
		t.Errorf("multi-slice pie should keep pgf-pie labels:\n%s", multi)
	}
}

func TestRenderAssetsBarByPorts(t *testing.T) {
	assets := []map[string]any{
		{"ports": []any{map[string]any{"number": "22", "proto": "tcp"}, map[string]any{"number": "80", "proto": "tcp"}}},
		{"ports": []any{map[string]any{"number": "22", "proto": "tcp"}}},
	}
	got := RenderAssets(assets, "bar", "ports")
	for _, want := range []string{`22/\allowbreak{}tcp`, `80/\allowbreak{}tcp`} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderAssets bar/ports missing %q:\n%s", want, got)
		}
	}
}

func TestRenderAssetsEmptyPlaceholder(t *testing.T) {
	got := RenderAssets(nil, "pie", "os")
	if !strings.Contains(got, "No data available") {
		t.Errorf("expected placeholder, got:\n%s", got)
	}
}

func TestRenderAssetsTableSortedAndUnknownOS(t *testing.T) {
	got := RenderAssets(sampleAssets(), "table", "ports")
	if !strings.Contains(got, "tblr") {
		t.Errorf("expected a tblr table, got:\n%s", got)
	}
	if !strings.Contains(got, "Unknown") {
		t.Errorf("expected missing os to render as Unknown:\n%s", got)
	}
	if strings.Index(got, `10.\allowbreak{}0.\allowbreak{}0.\allowbreak{}1`) > strings.Index(got, `10.\allowbreak{}0.\allowbreak{}0.\allowbreak{}2`) {
		t.Errorf("expected hosts sorted by IP:\n%s", got)
	}
}

func TestRenderAssetsTableHostnameBreakable(t *testing.T) {
	got := RenderAssets(sampleAssets(), "table", "ports")
	if !strings.Contains(got, `a.\allowbreak{}example.\allowbreak{}com`) {
		t.Errorf("expected dotted hostname to have \\allowbreak break hints so it wraps instead of overflowing:\n%s", got)
	}
}

func TestRenderAssetsTableHostWithNoOpenPorts(t *testing.T) {
	got := RenderAssets([]map[string]any{{"ip": "10.0.0.1"}}, "table", "ports")
	if !strings.Contains(got, `10.\allowbreak{}0.\allowbreak{}0.\allowbreak{}1`) {
		t.Errorf("expected a row for a host with no open ports:\n%s", got)
	}
}

func TestRenderAssetsTableEmptyPlaceholder(t *testing.T) {
	got := RenderAssets(nil, "table", "ports")
	if !strings.Contains(got, "No data available") {
		t.Errorf("expected placeholder, got:\n%s", got)
	}
}

func TestRenderWebBarByHost(t *testing.T) {
	findings := []map[string]any{{"host": "a.com"}, {"host": "a.com"}, {"host": "b.com"}}
	got := RenderWeb(findings, "bar", "host")
	for _, want := range []string{`a.\allowbreak{}com`, `b.\allowbreak{}com`} {
		if !strings.Contains(got, want) {
			t.Errorf("RenderWeb bar/host missing %q:\n%s", want, got)
		}
	}
}

func TestRenderWebMissingHostFallsBackToUnknown(t *testing.T) {
	got := RenderWeb([]map[string]any{{}}, "pie", "host")
	if !strings.Contains(got, "Unknown") {
		t.Errorf("expected Unknown fallback, got:\n%s", got)
	}
}

func TestRenderWebEmptyPlaceholder(t *testing.T) {
	got := RenderWeb(nil, "bar", "host")
	if !strings.Contains(got, "No data available") {
		t.Errorf("expected placeholder, got:\n%s", got)
	}
}

func TestRenderWebUnknownChartType(t *testing.T) {
	got := RenderWeb([]map[string]any{{"host": "a.com"}}, "matrix", "host")
	if !strings.Contains(got, "Unknown chart type.") {
		t.Errorf("expected web_graph:matrix to stay unsupported, got:\n%s", got)
	}
}

// Expected output captured verbatim from charts.py's render_network on the same
// input, to keep the Go port byte-compatible.
func TestRenderNetworkMatchesPython(t *testing.T) {
	assets := []map[string]any{
		{"ip": "10.0.0.5", "os": "Microsoft Windows Server 2019", "hostnames": []any{"dc01.corp.local"}},
		{"ip": "10.0.0.9", "os": "Linux 5.4"},
		{"ip": "10.0.1.3"},
	}
	for i := 1; i < 15; i++ {
		assets = append(assets, map[string]any{"ip": fmt.Sprintf("10.0.2.%d", i)})
	}
	conn := map[string]any{
		"subnets":   map[string]any{"10.0.0.0/24": "Corp LAN"},
		"pivots":    map[string]any{"10.0.0.5": []any{"172.16.5.0/24", "192.168.1.7"}, "10.0.1.0/24": "10.0.0.9", "10.0.2.13": "10.0.0.5"},
		"important": []any{"10.0.2.11"},
	}
	got, err := RenderNetwork(assets, conn)
	if err != nil {
		t.Fatal(err)
	}
	want := "<!--rex-chart:4306bac39a-->\n```{=latex}\n\\begin{center}\n\\begin{tikzpicture}[scale=0.645, every node/.style={transform shape}, sw/.style={rounded corners=3pt, minimum size=0.8cm, inner sep=0pt, fill=gray!70!black, text=white, font=\\large}, nd/.style={rounded corners=3pt, minimum size=0.9cm, inner sep=0pt, text=white, font=\\Large}, imp/.style={draw=colorCritical, line width=1.6pt}, lb/.style={anchor=north, align=center, text width=2.3cm, inner sep=1pt, fill=colorPrimary!4, font=\\scriptsize}]\n\\draw[colorPrimary!50, fill=colorPrimary!4, rounded corners=6pt] (0.00,0) rectangle (5.20,-10.50);\n\\draw[colorPrimary!50, fill=colorPrimary!4, rounded corners=6pt] (6.10,0) rectangle (8.90,-10.50);\n\\draw[colorPrimary!50, fill=colorPrimary!4, rounded corners=6pt] (9.80,0) rectangle (17.40,-10.50);\n\\draw[colorPrimary!50, fill=colorPrimary!4, rounded corners=6pt] (18.30,0) rectangle (21.10,-10.50);\n\\draw[colorPrimary!50, fill=colorPrimary!4, rounded corners=6pt] (22.00,0) rectangle (24.80,-10.50);\n\\draw[gray!65, line width=1.2pt] (12.40,2.10) -- (2.60,-0.90);\n\\draw[gray!55] (2.60,-0.90) -- (2.60,-1.75);\n\\draw[gray!55] (1.40,-1.75) -- (3.80,-1.75);\n\\draw[gray!55] (1.40,-1.75) -- (1.40,-2.50);\n\\draw[gray!55] (3.80,-1.75) -- (3.80,-2.50);\n\\draw[gray!65, line width=1.2pt] (12.40,2.10) -- (7.50,-0.90);\n\\draw[gray!55] (7.50,-0.90) -- (7.50,-1.75);\n\\draw[gray!55] (7.50,-1.75) -- (7.50,-1.75);\n\\draw[gray!55] (7.50,-1.75) -- (7.50,-2.50);\n\\draw[gray!65, line width=1.2pt] (12.40,2.10) -- (13.60,-0.90);\n\\draw[gray!55] (13.60,-0.90) -- (13.60,-1.75);\n\\draw[gray!55] (9.92,-1.75) -- (9.92,-8.05);\n\\draw[gray!55] (9.92,-1.75) -- (16.00,-1.75);\n\\draw[gray!55] (11.20,-1.75) -- (11.20,-2.50);\n\\draw[gray!55] (13.60,-1.75) -- (13.60,-2.50);\n\\draw[gray!55] (16.00,-1.75) -- (16.00,-2.50);\n\\draw[gray!55] (9.92,-3.85) -- (16.00,-3.85);\n\\draw[gray!55] (11.20,-3.85) -- (11.20,-4.60);\n\\draw[gray!55] (13.60,-3.85) -- (13.60,-4.60);\n\\draw[gray!55] (16.00,-3.85) -- (16.00,-4.60);\n\\draw[gray!55] (9.92,-5.95) -- (16.00,-5.95);\n\\draw[gray!55] (11.20,-5.95) -- (11.20,-6.70);\n\\draw[gray!55] (13.60,-5.95) -- (13.60,-6.70);\n\\draw[gray!55] (16.00,-5.95) -- (16.00,-6.70);\n\\draw[gray!55] (9.92,-8.05) -- (16.00,-8.05);\n\\draw[gray!55] (11.20,-8.05) -- (11.20,-8.80);\n\\draw[gray!55] (13.60,-8.05) -- (13.60,-8.80);\n\\draw[gray!55] (16.00,-8.05) -- (16.00,-8.80);\n\\draw[gray!65, line width=1.2pt] (12.40,2.10) -- (19.70,-0.90);\n\\draw[gray!65, line width=1.2pt] (12.40,2.10) -- (23.40,-0.90);\n\\draw[gray!55] (23.40,-0.90) -- (23.40,-1.75);\n\\draw[gray!55] (23.40,-1.75) -- (23.40,-1.75);\n\\draw[gray!55] (23.40,-1.75) -- (23.40,-2.50);\n\\node[text=cyan!18, font=\\fontsize{70}{70}\\selectfont] at (12.40,2.10) {\\faCloud};\n\\node[font=\\small\\bfseries, text=colorPrimary] at (12.40,1.95) {Core network};\n\\node[sw] (s0) at (2.60,-0.90) {\\faNetworkWired};\n\\node[nd, fill=cyan!55!blue] (h0x0) at (1.40,-2.50) {\\faWindows};\n\\node[nd, fill=orange!90!black] (h0x1) at (3.80,-2.50) {\\faLinux};\n\\node[anchor=north, align=center, font=\\small, text=colorPrimary] at (2.60,-10.60) {\\textbf{Corp LAN}\\\\{\\scriptsize 10.\\allowbreak{}0.\\allowbreak{}0.\\allowbreak{}0/\\allowbreak{}24}};\n\\node[sw] (s1) at (7.50,-0.90) {\\faNetworkWired};\n\\node[nd, fill=gray!55] (h1x0) at (7.50,-2.50) {\\faServer};\n\\node[anchor=north, align=center, font=\\small, text=colorPrimary] at (7.50,-10.60) {{\\scriptsize 10.\\allowbreak{}0.\\allowbreak{}1.\\allowbreak{}0/\\allowbreak{}24}};\n\\node[sw] (s2) at (13.60,-0.90) {\\faNetworkWired};\n\\node[nd, fill=gray!55] (h2x0) at (11.20,-2.50) {\\faServer};\n\\node[nd, fill=gray!55] (h2x1) at (13.60,-2.50) {\\faServer};\n\\node[nd, fill=gray!55] (h2x2) at (16.00,-2.50) {\\faServer};\n\\node[nd, fill=gray!55] (h2x3) at (11.20,-4.60) {\\faServer};\n\\node[nd, fill=gray!55] (h2x4) at (13.60,-4.60) {\\faServer};\n\\node[nd, fill=gray!55] (h2x5) at (16.00,-4.60) {\\faServer};\n\\node[nd, fill=gray!55] (h2x6) at (11.20,-6.70) {\\faServer};\n\\node[nd, fill=gray!55] (h2x7) at (13.60,-6.70) {\\faServer};\n\\node[nd, fill=gray!55] (h2x8) at (16.00,-6.70) {\\faServer};\n\\node[nd, fill=gray!55] (h2x9) at (11.20,-8.80) {\\faServer};\n\\node[nd, imp, fill=gray!55] (h2x10) at (13.60,-8.80) {\\faServer};\n\\node[nd, fill=gray!55] (h2x11) at (16.00,-8.80) {\\faServer};\n\\node[font=\\scriptsize\\itshape, text=gray] at (13.60,-10.20) {+ 2 more hosts};\n\\node[anchor=north, align=center, font=\\small, text=colorPrimary] at (13.60,-10.60) {{\\scriptsize 10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}0/\\allowbreak{}24}};\n\\node[sw] (s3) at (19.70,-0.90) {\\faNetworkWired};\n\\node[anchor=north, align=center, font=\\small, text=colorPrimary] at (19.70,-10.60) {{\\scriptsize 172.\\allowbreak{}16.\\allowbreak{}5.\\allowbreak{}0/\\allowbreak{}24}};\n\\node[sw] (s4) at (23.40,-0.90) {\\faNetworkWired};\n\\node[nd, fill=gray!55] (h4x0) at (23.40,-2.50) {\\faServer};\n\\node[anchor=north, align=center, font=\\small, text=colorPrimary] at (23.40,-10.60) {{\\scriptsize 192.\\allowbreak{}168.\\allowbreak{}1.\\allowbreak{}0/\\allowbreak{}24}};\n\\draw[-latex, very thick, dashed, colorCritical] (h0x0) to[bend left=12] (s3);\n\\draw[-latex, very thick, dashed, colorCritical] (h0x0) to[bend left=12] (h4x0);\n\\draw[-latex, very thick, dashed, colorCritical] (s1) to[bend left=12] (h0x1);\n\\draw[-latex, very thick, dashed, colorCritical] (h2x11) to[bend left=12] (h0x0);\n\\node[lb] at (1.40,-3.05) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}0.\\allowbreak{}5}\\\\dc01.\\allowbreak{}corp.\\allowbreak{}local};\n\\node[lb] at (3.80,-3.05) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}0.\\allowbreak{}9}};\n\\node[lb] at (7.50,-3.05) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}1.\\allowbreak{}3}};\n\\node[lb] at (11.20,-3.05) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}1}};\n\\node[lb] at (13.60,-3.05) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}2}};\n\\node[lb] at (16.00,-3.05) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}3}};\n\\node[lb] at (11.20,-5.15) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}4}};\n\\node[lb] at (13.60,-5.15) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}5}};\n\\node[lb] at (16.00,-5.15) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}6}};\n\\node[lb] at (11.20,-7.25) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}7}};\n\\node[lb] at (13.60,-7.25) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}8}};\n\\node[lb] at (16.00,-7.25) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}9}};\n\\node[lb] at (11.20,-9.35) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}10}};\n\\node[lb] at (13.60,-9.35) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}11}};\n\\node[lb] at (16.00,-9.35) {\\textbf{10.\\allowbreak{}0.\\allowbreak{}2.\\allowbreak{}13}};\n\\node[lb] at (23.40,-3.05) {\\textbf{192.\\allowbreak{}168.\\allowbreak{}1.\\allowbreak{}7}};\n\\node[nd, minimum size=0.5cm, font=\\footnotesize, fill=cyan!55!blue] at (0.00,-11.95) {\\faWindows};\n\\node[anchor=west, font=\\scriptsize] at (0.40,-11.95) {Windows};\n\\node[nd, minimum size=0.5cm, font=\\footnotesize, fill=orange!90!black] at (2.12,-11.95) {\\faLinux};\n\\node[anchor=west, font=\\scriptsize] at (2.52,-11.95) {Linux};\n\\node[nd, minimum size=0.5cm, font=\\footnotesize, fill=gray!55] at (3.92,-11.95) {\\faServer};\n\\node[anchor=west, font=\\scriptsize] at (4.32,-11.95) {Other / unknown};\n\\draw[gray!55, line width=1.2pt] (7.02,-11.95) -- (7.62,-11.95);\n\\node[anchor=west, font=\\scriptsize] at (7.72,-11.95) {Network link};\n\\draw[-latex, very thick, dashed, colorCritical] (9.94,-11.95) -- (10.54,-11.95);\n\\node[anchor=west, font=\\scriptsize] at (10.64,-11.95) {Pivot};\n\\node[nd, imp, minimum size=0.5cm, fill=gray!30] at (12.04,-11.95) {};\n\\node[anchor=west, font=\\scriptsize] at (12.44,-11.95) {Important host};\n\\end{tikzpicture}\n\\end{center}\n```\n<!--/rex-chart-->"
	if got != want {
		t.Errorf("RenderNetwork mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderNetworkBadEndpoint(t *testing.T) {
	_, err := RenderNetwork([]map[string]any{{"ip": "10.0.0.1"}},
		map[string]any{"pivots": map[string]any{"10.0.0.1": "nope/24"}})
	if err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestRenderNetworkEmpty(t *testing.T) {
	got, err := RenderNetwork(nil, nil)
	if err != nil || !strings.Contains(got, "No data available") {
		t.Errorf("got %q, %v", got, err)
	}
}

// Captured from charts.render(sampleFindings, "radar", "business").
func TestRenderBusinessRadarMatchesPython(t *testing.T) {
	want := "\\begin{center}\n" +
		"\\begin{tikzpicture}[font=\\small]\n" +
		"\\draw[gray!35] (90.0:1.200) -- (18.0:1.200) -- (-54.0:1.200) -- (-126.0:1.200) -- (-198.0:1.200) -- cycle;\n" +
		"\\draw[gray!35] (90.0:2.400) -- (18.0:2.400) -- (-54.0:2.400) -- (-126.0:2.400) -- (-198.0:2.400) -- cycle;\n" +
		"\\draw[gray!35] (0,0) -- (90.0:2.40);\n" +
		"\\node[anchor=south] at (90.0:2.55) {Strategic \\textbf{(0)}};\n" +
		"\\draw[gray!35] (0,0) -- (18.0:2.40);\n" +
		"\\node[anchor=west] at (18.0:2.55) {Operational \\textbf{(2)}};\n" +
		"\\draw[gray!35] (0,0) -- (-54.0:2.40);\n" +
		"\\node[anchor=west] at (-54.0:2.55) {Financial \\textbf{(2)}};\n" +
		"\\draw[gray!35] (0,0) -- (-126.0:2.40);\n" +
		"\\node[anchor=east] at (-126.0:2.55) {Compliance \\textbf{(0)}};\n" +
		"\\draw[gray!35] (0,0) -- (-198.0:2.40);\n" +
		"\\node[anchor=east] at (-198.0:2.55) {Reputational \\textbf{(0)}};\n" +
		"\\filldraw[draw=chartc1, fill=chartc1!35, line width=1pt, fill opacity=0.8] (90.0:0.000) -- (18.0:2.400) -- (-54.0:2.400) -- (-126.0:0.000) -- (-198.0:0.000) -- cycle;\n" +
		"\\end{tikzpicture}\n" +
		"\\end{center}"
	if got := unwrapChart(t, Render(sampleFindings(), "radar", "business")); got != want {
		t.Errorf("radar mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderRadarFallsBackToBarBelowThreeSpokes(t *testing.T) {
	findings := []map[string]any{{"severity": "High", "material_impact": "Information Privacy"}}
	got := unwrapChart(t, Render(findings, "spider", "material"))
	if !strings.Contains(got, "rounded corners") {
		t.Errorf("expected bar fallback for <3 categories, got:\n%s", got)
	}
}

func TestRenderMaterialMatrix(t *testing.T) {
	findings := []map[string]any{
		{"severity": "High", "material_impact": "Proprietary Data Loss"},
		{"severity": "Low", "material_impact": "Proprietary Data Loss"},
	}
	got := unwrapChart(t, Render(findings, "matrix", "fair-mam"))
	if !strings.Contains(got, "Proprietary Data Loss") || strings.Count(got, "\\textbf{1}") != 2 {
		t.Errorf("material matrix missing category/counts:\n%s", got)
	}
}
