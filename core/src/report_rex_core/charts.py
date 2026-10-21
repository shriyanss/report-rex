"""Generate LaTeX charts from finding data.

Testers embed a directive such as ``:::findings_graph:pie:severity:::`` in any
section. The preprocessor tallies the relevant metadata across all findings and
replaces the directive with self-contained LaTeX (TikZ / pgf-pie / tabularray)
that renders a chart. No external image tooling is required.

Supported chart types : ``pie``, ``donut``, ``bar`` (alias ``chart``), ``matrix``,
                        ``radar`` (alias ``spider``)
Supported datasets     : ``severity`` (default), ``owasp``, ``mitre``, ``business``,
                        ``material`` (FAIR-MAM ``material_impact``)

A parallel ``:::asset_graph:pie:os:::`` directive (see ``render_assets``) charts the
host/port inventory an ``rex ingest`` run writes to ``artifacts/assets.json``, reusing
the same ``_pie``/``_bar`` LaTeX renderers. ``:::asset_graph:table:ports:::`` renders
the full inventory as a table instead (``_asset_matrix``, dataset ignored).
"""

from __future__ import annotations

import hashlib
import ipaddress
import math
import re
from collections import OrderedDict

from . import references

# Matches the tagged raw-LaTeX chart blocks _raw() emits below. chart_raster.py
# uses this to find and rasterize charts for the DOCX pipeline, since pandoc's
# docx writer silently drops {=latex} raw blocks (DOCX has no TikZ renderer).
CHART_BLOCK_RE = re.compile(
    r"<!--rex-chart:(?P<id>[0-9a-f]{10})-->\n"
    r"```\{=latex\}\n(?P<tikz>.*?)\n```\n"
    r"<!--/rex-chart-->",
    re.DOTALL,
)

# Severity → LaTeX colour name (defined in the template preamble).
SEVERITY_COLORS = OrderedDict(
    [
        ("Critical", "colorCritical"),
        ("High", "colorHigh"),
        ("Medium", "colorMedium"),
        ("Low", "colorLow"),
        ("Info", "colorInfo"),
    ]
)

# Categorical palette colour names (defined in the template preamble as chartcN).
_PALETTE = [f"chartc{i}" for i in range(1, 11)]

_DATASET_ALIASES = {
    "severity": "severity",
    "sev": "severity",
    "owasp": "owasp",
    "mitre": "mitre",
    "attack": "mitre",
    "business": "business",
    "business-risk": "business",
    "business_risk": "business",
    "risk": "business",
    "material": "material",
    "material_impact": "material",
    "material-impact": "material",
    "fair-mam": "material",
}

_TYPE_ALIASES = {
    "pie": "pie",
    "donut": "donut",
    "doughnut": "donut",
    "bar": "bar",
    "chart": "bar",
    "matrix": "matrix",
    "table": "matrix",
    "radar": "radar",
    "spider": "radar",
}


def _esc(s: str) -> str:
    """Escape LaTeX specials in a chart label (also drop commas for pgf-pie).

    Long unbroken tokens (hostnames, IPs, URLs) have no spaces for a
    tabularray X-column to wrap at, so without a break hint they overflow
    into neighboring cells; \\allowbreak after '.', '/', ':' gives LaTeX a
    place to break without printing anything when it doesn't. Mirrors
    cell_to_latex in the templates' vuln_filter.lua, which applies the same
    fix for pandoc-authored tables.
    """
    out = (
        s.replace("\\", r"\textbackslash{}")
        .replace("&", r"\&")
        .replace("%", r"\%")
        .replace("$", r"\$")
        .replace("#", r"\#")
        .replace("_", r"\_")
        .replace("{", r"\{")
        .replace("}", r"\}")
        .replace(".", r".\allowbreak{}")
        .replace("/", r"/\allowbreak{}")
        .replace(":", r":\allowbreak{}")
    )
    return out.replace(",", ";")


def _tally(findings: list[dict], dataset: str) -> OrderedDict[str, int]:
    counts: OrderedDict[str, int] = OrderedDict()

    if dataset == "severity":
        for sev in SEVERITY_COLORS:
            counts[sev] = 0
        for f in findings:
            sev = f.get("severity", "Info")
            if sev in ("None", ""):
                sev = "Info"
            counts[sev] = counts.get(sev, 0) + 1
        return OrderedDict((k, v) for k, v in counts.items() if v > 0)

    if dataset == "owasp":
        for f in findings:
            for code in references.split_codes(f.get("owasp")):
                key = references.label(code)
                counts[key] = counts.get(key, 0) + 1
        return counts

    if dataset in ("mitre", "material"):
        field = "material_impact" if dataset == "material" else "mitre"
        for f in findings:
            for code in references.split_codes(f.get(field)):
                key = references.label(code)
                counts[key] = counts.get(key, 0) + 1
        return counts

    if dataset == "business":
        for cat in references.BUSINESS_RISK_CATEGORIES:
            counts[cat] = 0
        for f in findings:
            for raw in references.split_codes(f.get("business_risk")):
                cat = references.normalise_business_risk(raw) or raw
                counts[cat] = counts.get(cat, 0) + 1
        return OrderedDict((k, v) for k, v in counts.items() if v > 0)

    return counts


def _colors_for(dataset: str, keys: list[str]) -> list[str]:
    if dataset == "severity":
        return [SEVERITY_COLORS.get(k, "colorInfo") for k in keys]
    return [_PALETTE[i % len(_PALETTE)] for i in range(len(keys))]


def _empty(dataset: str) -> str:
    return (
        "\\begin{center}\\textit{\\small No data available for the "
        f"{_esc(dataset)} breakdown.}}\\end{{center}}"
    )


def _raw(latex: str) -> str:
    """Wrap generated LaTeX in a pandoc raw block, tagged for chart_raster.py."""
    latex = latex.strip()
    chart_id = hashlib.sha1(latex.encode()).hexdigest()[:10]
    return f"<!--rex-chart:{chart_id}-->\n```{{=latex}}\n{latex}\n```\n<!--/rex-chart-->"


def _pie(counts, colors, *, donut: bool) -> str:
    total = sum(counts.values())
    entries = ", ".join(
        f"{v}/{_esc(k)}" for k, v in counts.items()
    )
    color_list = ", ".join(colors)
    inner = ""
    hide = ""
    if len(counts) == 1:
        # pgf-pie draws a 100% wedge as a sector: a seam from the centre to the
        # rim, with the label sitting on it. Paint a whole disc over it instead.
        hide = "hide number, "
        inner = f"\\filldraw[fill={colors[0]}, draw=black] (0,0) circle (2.3);"
        if not donut:
            inner += f"\\node[text=white, font=\\small\\bfseries] at (0,0) {{{total}}};"
    if donut:
        # pgf-pie puts numbers at r/2, right under the hole (#79): draw them
        # mid-ring ourselves, and caption the centre so it isn't read as a slice.
        hide = "hide number, "
        inner += "\\filldraw[white] (0,0) circle (1.15);"
        start = 0.0
        for v in counts.values():
            sweep = 360 * v / total
            inner += f"\\node[text=white, font=\\small\\bfseries] at ({start + sweep / 2:.1f}:1.725) {{{v}}};"
            start += sweep
        inner += (
            f"\\node[align=center] at (0,0) {{{{\\bfseries\\large {total}}}\\\\{{\\scriptsize Total}}}};"
        )
    return (
        "\\begin{center}\n"
        "\\begin{tikzpicture}\n"
        f"\\pie[radius=2.3, text=legend, sum=auto, {hide}color={{{color_list}}},\n"
        "  before number=, after number=, every only number node/.style={text=white,font=\\small\\bfseries}]\n"
        f"  {{{entries}}}\n"
        f"{inner}\n"
        "\\end{tikzpicture}\n"
        "\\end{center}"
    )


def _bar(counts, colors) -> str:
    maxv = max(counts.values()) if counts else 1
    bar_w = 7.0  # cm at full scale
    row_h = 0.62
    lines = ["\\begin{center}", "\\begin{tikzpicture}[font=\\small]"]
    n = len(counts)
    for i, ((label, v), color) in enumerate(zip(counts.items(), colors)):
        y = -(i * row_h)
        w = (v / maxv) * bar_w if maxv else 0
        lines.append(
            f"\\node[anchor=east] at (-0.15,{y:.2f}) {{{_esc(label)}}};"
        )
        lines.append(
            f"\\filldraw[{color}, rounded corners=1pt] (0,{y - 0.2:.2f}) rectangle ({w:.3f},{y + 0.2:.2f});"
        )
        lines.append(
            f"\\node[anchor=west, font=\\bfseries] at ({w + 0.1:.3f},{y:.2f}) {{{v}}};"
        )
    # baseline
    lines.append(
        f"\\draw[gray!50] (0,{0.32:.2f}) -- (0,{-(n - 1) * row_h - 0.32:.2f});"
    )
    lines.append("\\end{tikzpicture}")
    lines.append("\\end{center}")
    return "\n".join(lines)


def _radar(counts) -> str:
    """Spider chart: one spoke per category, the counts as a filled polygon."""
    keys = list(counts)
    n = len(keys)
    maxv = max(counts.values()) or 1
    rings = min(maxv, 5)
    radius = 2.4  # cm, outer ring

    def ang(i):
        return 90 - i * 360 / n

    def poly(values):
        return " -- ".join(f"({ang(i):.1f}:{radius * v / maxv:.3f})" for i, v in enumerate(values))

    lines = ["\\begin{center}", "\\begin{tikzpicture}[font=\\small]"]
    for r in range(1, rings + 1):
        lines.append(f"\\draw[gray!35] {poly([maxv * r / rings] * n)} -- cycle;")
    for i, k in enumerate(keys):
        a = ang(i)
        c, sn = math.cos(math.radians(a)), math.sin(math.radians(a))
        anchor = "west" if c > 0.2 else "east" if c < -0.2 else ("south" if sn > 0 else "north")
        lines.append(f"\\draw[gray!35] (0,0) -- ({a:.1f}:{radius:.2f});")
        lines.append(
            f"\\node[anchor={anchor}] at ({a:.1f}:{radius + 0.15:.2f}) "
            f"{{{_esc(k)} \\textbf{{({counts[k]})}}}};"
        )
    lines.append(
        f"\\filldraw[draw=chartc1, fill=chartc1!35, line width=1pt, fill opacity=0.8] "
        f"{poly(counts.values())} -- cycle;"
    )
    lines.append("\\end{tikzpicture}")
    lines.append("\\end{center}")
    return "\n".join(lines)


def _matrix(findings, dataset) -> str:
    """Category (rows) × severity (cols) count grid with colour-graded cells."""
    row_counts: OrderedDict[str, dict] = OrderedDict()
    sevs = list(SEVERITY_COLORS.keys())

    def bump(cat, sev):
        row_counts.setdefault(cat, {s: 0 for s in sevs})
        if sev in ("None", ""):
            sev = "Info"
        row_counts[cat][sev] = row_counts[cat].get(sev, 0) + 1

    field = {"owasp": "owasp", "mitre": "mitre", "business": "business_risk",
             "material": "material_impact"}.get(
        dataset, dataset
    )
    for f in findings:
        sev = f.get("severity", "Info")
        codes = references.split_codes(f.get(field))
        if dataset == "business":
            codes = [references.normalise_business_risk(c) or c for c in codes]
        elif dataset in ("owasp", "mitre"):
            codes = [references.label(c) for c in codes]
        for cat in codes:
            bump(cat, sev)

    if not row_counts:
        return _empty(dataset)

    header = " & ".join(["\\textbf{Category}"] + [f"\\textbf{{{s}}}" for s in sevs]) + " \\\\"
    body_rows = []
    for cat, cells in row_counts.items():
        cols = []
        for s in sevs:
            c = cells.get(s, 0)
            if c > 0:
                shade = min(20 + 25 * c, 85)
                cols.append(f"\\SetCell{{bg={SEVERITY_COLORS[s]}!{shade}!white}} \\textbf{{{c}}}")
            else:
                cols.append("\\SetCell{bg=gray!4} \\textcolor{gray!55}{0}")
        body_rows.append(_esc(cat) + " & " + " & ".join(cols) + " \\\\")

    colspec = "colspec={Q[l,wd=5.2cm] *{" + str(len(sevs)) + "}{Q[c,wd=1.5cm]}}"
    return (
        "\\begin{center}\n"
        f"\\begin{{tblr}}{{{colspec}, hlines={{gray!30}}, vlines={{gray!30}}, "
        "row{1}={bg=colorPrimary, fg=white, font=\\bfseries}}\n"
        + header
        + "\n"
        + "\n".join(body_rows)
        + "\n\\end{tblr}\n\\end{center}"
    )


def render(findings: list[dict], gtype: str, dataset: str) -> str:
    """Return a pandoc raw-LaTeX block for the requested chart."""
    gtype = _TYPE_ALIASES.get(gtype.strip().lower())
    dataset = _DATASET_ALIASES.get((dataset or "severity").strip().lower(), "severity")
    if gtype is None:
        return _raw(
            "\\begin{center}\\textit{\\small Unknown chart type.}\\end{center}"
        )

    if gtype == "matrix":
        return _raw(_matrix(findings, dataset))

    counts = _tally(findings, dataset)
    if not counts:
        return _raw(_empty(dataset))

    if gtype == "radar":
        if dataset == "business":
            # Every business category gets a spoke, so the shape stays comparable.
            full = OrderedDict((c, 0) for c in references.BUSINESS_RISK_CATEGORIES)
            full.update(counts)
            counts = full
        if len(counts) >= 3:
            return _raw(_radar(counts))
        gtype = "bar"  # a radar needs at least three spokes
    colors = _colors_for(dataset, list(counts.keys()))

    if gtype in ("pie", "donut"):
        return _raw(_pie(counts, colors, donut=(gtype == "donut")))
    if gtype == "bar":
        return _raw(_bar(counts, colors))
    return _raw(_empty(dataset))


# ── Asset charts (rex ingest) ───────────────────────────────────────────────

_ASSET_DATASET_ALIASES = {
    "port": "ports",
    "ports": "ports",
    "os": "os",
    "service": "services",
    "services": "services",
}

_TOP_N_ASSET_ENTRIES = 10


def _tally_assets(assets: list[dict], dataset: str) -> OrderedDict[str, int]:
    counts: OrderedDict[str, int] = OrderedDict()

    if dataset == "os":
        for a in assets:
            key = a.get("os") or "Unknown"
            counts[key] = counts.get(key, 0) + 1
    elif dataset == "ports":
        for a in assets:
            for p in a.get("ports", []):
                key = f"{p.get('number', '?')}/{p.get('proto', '?')}"
                counts[key] = counts.get(key, 0) + 1
    elif dataset == "services":
        for a in assets:
            for p in a.get("ports", []):
                key = p.get("service") or "unknown"
                counts[key] = counts.get(key, 0) + 1

    top = OrderedDict(
        sorted(counts.items(), key=lambda kv: kv[1], reverse=True)[:_TOP_N_ASSET_ENTRIES]
    )
    return top


def _asset_matrix(assets: list[dict]) -> str:
    """Full host/port inventory table — every asset, every open port, one row each.

    Unlike ``_matrix`` this doesn't tally by dataset; it's the same inventory
    ``rex ingest`` used to hand-build as a markdown pipe table (before that hit
    pandoc's dash-count column-width heuristic and rendered broken for long IPs/
    version strings). Uses ``tblr``, the same non-paginating tabularray
    environment as ``_matrix`` above — proven to compile in this template, unlike
    ``longtblr``/``longtblr*`` (tried and reverted: the former auto-numbers as
    "Table 1:" even with no caption text, the latter isn't a real environment).
    # ponytail: a single-page-only table — a very large inventory (many hosts,
    # full port-range scans) can overflow past the page footer. Upgrade to
    # tabularray's longtblr if/when that's hit in practice, with its caption
    # suppressed via `entry=none, caption={}` (untested here).
    """
    rows = sorted(assets, key=lambda a: a.get("ip", ""))
    if not rows:
        return _empty("ports")

    body_rows = []
    for a in rows:
        ip = _esc(a.get("ip", ""))
        hostname = _esc(", ".join(a.get("hostnames") or []))
        os_name = _esc(a.get("os") or "Unknown")
        ports = a.get("ports") or []
        if not ports:
            body_rows.append(f"{ip} & {hostname} & {os_name} & & & & \\\\")
            continue
        for p in ports:
            port = _esc(str(p.get("number", "")))
            proto = _esc(p.get("proto", ""))
            service = _esc(p.get("service", ""))
            version = _esc(" ".join(filter(None, [p.get("product"), p.get("version")])))
            body_rows.append(
                f"{ip} & {hostname} & {os_name} & {port} & {proto} & {service} & {version} \\\\"
            )

    header = (
        "\\textbf{Host} & \\textbf{Hostname} & \\textbf{OS} & \\textbf{Port} & "
        "\\textbf{Proto} & \\textbf{Service} & \\textbf{Version} \\\\"
    )
    colspec = "colspec={X[3,l] X[3,l] X[2,l] X[1,c] X[1,c] X[2,l] X[4,l]}"
    return (
        "\\begin{center}\n"
        f"\\begin{{tblr}}{{{colspec}, hlines={{gray!30}}, vlines={{gray!30}}, "
        "row{1}={bg=colorPrimary, fg=white, font=\\bfseries}}\n"
        + header
        + "\n"
        + "\n".join(body_rows)
        + "\n\\end{tblr}\n\\end{center}"
    )


def render_assets(assets: list[dict], gtype: str, dataset: str) -> str:
    """Return a pandoc raw-LaTeX block for an ``:::asset_graph:...:::`` directive."""
    gtype = _TYPE_ALIASES.get(gtype.strip().lower())
    dataset = _ASSET_DATASET_ALIASES.get((dataset or "os").strip().lower(), "os")
    if gtype is None:
        return _raw(
            "\\begin{center}\\textit{\\small Unknown chart type.}\\end{center}"
        )

    if gtype == "matrix":
        return _raw(_asset_matrix(assets))

    counts = _tally_assets(assets, dataset)
    if not counts:
        return _raw(_empty(dataset))
    colors = _colors_for(dataset, list(counts.keys()))

    if gtype in ("pie", "donut"):
        return _raw(_pie(counts, colors, donut=(gtype == "donut")))
    if gtype == "bar":
        return _raw(_bar(counts, colors))
    return _raw(_empty(dataset))


# ── Network diagram (rex ingest + artifacts/connections.json) ─────────────

# OS family: (name substrings, fontawesome5 glyph, tile colour, legend label).
# First match wins; anything unmatched is the last entry.
_OS_FAMILIES = [
    (("windows",), "\\faWindows", "cyan!55!blue", "Windows"),
    (("linux", "ubuntu", "debian", "centos", "red hat", "redhat", "fedora", "kali"),
     "\\faLinux", "orange!90!black", "Linux"),
    (("mac", "apple", "darwin"), "\\faApple", "gray!65!black", "macOS"),
    (("freebsd",), "\\faFreebsd", "red!65!black", "FreeBSD"),
    (("android",), "\\faAndroid", "green!55!black", "Android"),
    ((), "\\faServer", "gray!55", "Other / unknown"),
]

_NET_CELL_W = 2.4  # cm per host column inside a subnet region
_NET_CELL_H = 2.1  # cm per host row
_NET_GAP = 0.9  # cm between subnet regions
_NET_ROW0 = -2.5  # y of the first host row (region top is 0, switch at -0.9)
_NET_MAX_W = 16.0  # cm of text width before the diagram is scaled down
_NET_MAX_H = 20.0  # cm of text height before the diagram is scaled down
_NET_MAX_HOSTS = 12  # hosts per subnet before unrelated hosts are hidden

_NET_STYLES = (
    "sw/.style={rounded corners=3pt, minimum size=0.8cm, inner sep=0pt, fill=gray!70!black, "
    "text=white, font=\\large}, "
    "nd/.style={rounded corners=3pt, minimum size=0.9cm, inner sep=0pt, text=white, font=\\Large}, "
    "imp/.style={draw=colorCritical, line width=1.6pt}, "
    "lb/.style={anchor=north, align=center, text width=2.3cm, inner sep=1pt, "
    "fill=colorPrimary!4, font=\\scriptsize}"
)


def _os_family(os_name: str) -> int:
    s = os_name.lower()
    for i, (keys, *_rest) in enumerate(_OS_FAMILIES[:-1]):
        if any(k in s for k in keys):
            return i
    return len(_OS_FAMILIES) - 1


def _endpoint(s: str):
    """A connections.json pivot endpoint: an IP or a CIDR (ValueError if neither)."""
    s = str(s).strip()
    return ipaddress.ip_network(s, strict=False) if "/" in s else ipaddress.ip_address(s)


def render_network(assets: list[dict], connections: dict) -> str:
    """Return a pandoc raw-LaTeX block for a ``:::network_diagram:::`` directive.

    Draws a topology map: a core cloud linked to one region per subnet, each with
    a switch linked to its hosts (OS-coloured icon tiles). Hosts from
    ``assets.json`` go into the most specific ``subnets`` CIDR from
    ``connections.json`` that contains them (else an automatic /24, /64 for IPv6).
    Each ``pivots`` entry (IP or CIDR → IP, CIDR, or a list of them) is drawn as a
    dashed red arrow; a pivot endpoint or ``important`` IP not in the inventory is
    added. ``important`` hosts get a red outline.

    A subnet over ``_NET_MAX_HOSTS`` hosts hides hosts with no pivot and not marked
    ``important`` (shown as "+N more hosts"); pivot/important hosts are always
    drawn, and the whole diagram is scaled down if it's still too big.
    Raises ValueError on a bad IP/CIDR.
    """
    labels = {
        ipaddress.ip_network(str(k), strict=False): str(v)
        for k, v in (connections.get("subnets") or {}).items()
    }
    pivots = []
    for src, dsts in sorted((connections.get("pivots") or {}).items()):
        for dst in [dsts] if isinstance(dsts, str) else dsts:
            pivots.append((_endpoint(src), _endpoint(dst)))
    important = {ipaddress.ip_address(str(i).strip()) for i in connections.get("important") or []}

    def is_net(e):
        return isinstance(e, (ipaddress.IPv4Network, ipaddress.IPv6Network))

    hosts: dict = {}  # ip → (os, first hostname)
    for a in assets:
        ip = ipaddress.ip_address(str(a.get("ip", "")).strip())
        hosts[ip] = (a.get("os") or "Unknown", (a.get("hostnames") or [""])[0])
    endpoints = [e for p in pivots for e in p]
    must_show = {e for e in endpoints if not is_net(e)} | important
    for ip in must_show:
        hosts.setdefault(ip, ("Unknown", ""))

    def subnet_of(ip):
        best = None
        for net in labels:
            if net.version == ip.version and ip in net and (best is None or net.prefixlen > best.prefixlen):
                best = net
        return best or ipaddress.ip_network(f"{ip}/{24 if ip.version == 4 else 64}", strict=False)

    groups: dict = {net: [] for net in labels}
    for e in endpoints:
        if is_net(e):
            groups.setdefault(e, [])
    for ip in sorted(hosts, key=lambda i: (i.version, i)):
        groups.setdefault(subnet_of(ip), []).append(ip)
    if not groups:
        return _raw(_empty("network"))

    # Trim each subnet to _NET_MAX_HOSTS, never dropping a must-show host.
    hidden: dict = {}
    for net, ips in groups.items():
        free = _NET_MAX_HOSTS - sum(ip in must_show for ip in ips)
        shown = []
        for ip in ips:
            if ip in must_show:
                shown.append(ip)
            elif free > 0:
                free -= 1
                shown.append(ip)
        groups[net], hidden[net] = shown, len(ips) - len(shown)

    # ── Geometry: regions left to right, hosts in a grid under each switch ──
    ordered = sorted(groups, key=lambda n: (n.version, n.network_address, n.prefixlen))
    regions = []  # (net, left x, width, columns)
    x = 0.0
    for net in ordered:
        n = len(groups[net])
        cols = 1 if n <= 1 else 2 if n <= 4 else 3
        width = cols * _NET_CELL_W + 0.4
        regions.append((net, x, width, cols))
        x += width + _NET_GAP
    total_w = x - _NET_GAP
    max_rows = max(1, max(-(-len(groups[net]) // cols) for net, _, _, cols in regions))
    bottom = _NET_ROW0 - (max_rows - 1) * _NET_CELL_H - 1.25 - (0.45 if any(hidden.values()) else 0)
    core_x, core_y = total_w / 2, 2.1

    nodes: dict = {}  # ip/network → tikz node name
    host_xy: dict = {}  # ip → (x, y)
    for i, (net, left, width, cols) in enumerate(regions):
        nodes[net] = f"s{i}"
        for j, ip in enumerate(groups[net]):
            r, c = divmod(j, cols)
            host_xy[ip] = (left + 0.2 + (c + 0.5) * _NET_CELL_W, _NET_ROW0 - r * _NET_CELL_H)
            nodes[ip] = f"h{i}x{j}"

    # Links first (from known coordinates) so the opaque icons/labels drawn
    # afterwards cover them; pivots last, on top of everything.
    lines = []
    for i, (net, left, width, _cols) in enumerate(regions):
        lines.append(
            f"\\draw[colorPrimary!50, fill=colorPrimary!4, rounded corners=6pt] "
            f"({left:.2f},0) rectangle ({left + width:.2f},{bottom:.2f});"
        )
    for net, left, width, cols in regions:
        cx = left + width / 2
        lines.append(f"\\draw[gray!65, line width=1.2pt] ({core_x:.2f},{core_y:.2f}) -- ({cx:.2f},-0.90);")
        ips = groups[net]
        if not ips:
            continue
        # Bus layout: switch → a bus line above each host row (joined by a trunk
        # down the region's left edge) → a short drop to each host.
        nrows = -(-len(ips) // cols)
        bus = [_NET_ROW0 + 0.75 - r * _NET_CELL_H for r in range(nrows)]
        tx = left + 0.12
        lines.append(f"\\draw[gray!55] ({cx:.2f},-0.90) -- ({cx:.2f},{bus[0]:.2f});")
        if nrows > 1:
            lines.append(f"\\draw[gray!55] ({tx:.2f},{bus[0]:.2f}) -- ({tx:.2f},{bus[-1]:.2f});")
        for r in range(nrows):
            row = ips[r * cols:(r + 1) * cols]
            joins = [tx] if nrows > 1 else []  # the trunk, when there is one
            if r == 0:
                joins.append(cx)  # the switch drop
            xs = [host_xy[ip][0] for ip in row] + joins
            lines.append(f"\\draw[gray!55] ({min(xs):.2f},{bus[r]:.2f}) -- ({max(xs):.2f},{bus[r]:.2f});")
            for ip in row:
                hx, hy = host_xy[ip]
                lines.append(f"\\draw[gray!55] ({hx:.2f},{bus[r]:.2f}) -- ({hx:.2f},{hy:.2f});")
    lines.append(
        f"\\node[text=cyan!18, font=\\fontsize{{70}}{{70}}\\selectfont] at ({core_x:.2f},{core_y:.2f}) {{\\faCloud}};"
    )
    lines.append(
        f"\\node[font=\\small\\bfseries, text=colorPrimary] at ({core_x:.2f},{core_y - 0.15:.2f}) {{Core network}};"
    )
    families = set()
    host_labels = []  # drawn after the pivots, so arrows pass under the (filled) text
    for i, (net, left, width, _cols) in enumerate(regions):
        cx = left + width / 2
        lines.append(f"\\node[sw] (s{i}) at ({cx:.2f},-0.90) {{\\faNetworkWired}};")
        for j, ip in enumerate(groups[net]):
            os_name, hostname = hosts[ip]
            fam = _os_family(os_name)
            families.add(fam)
            _keys, icon, color, _name = _OS_FAMILIES[fam]
            hx, hy = host_xy[ip]
            style = "nd, imp" if ip in important else "nd"
            lines.append(f"\\node[{style}, fill={color}] (h{i}x{j}) at ({hx:.2f},{hy:.2f}) {{{icon}}};")
            text = f"\\textbf{{{_esc(str(ip))}}}"
            if hostname:
                text += f"\\\\{_esc(hostname)}"
            host_labels.append(f"\\node[lb] at ({hx:.2f},{hy - 0.55:.2f}) {{{text}}};")
        if hidden[net]:
            lines.append(
                f"\\node[font=\\scriptsize\\itshape, text=gray] at ({cx:.2f},{bottom + 0.3:.2f}) "
                f"{{+ {hidden[net]} more hosts}};"
            )
        title = f"\\textbf{{{_esc(labels[net])}}}\\\\" if net in labels else ""
        lines.append(
            f"\\node[anchor=north, align=center, font=\\small, text=colorPrimary] at ({cx:.2f},{bottom - 0.1:.2f}) "
            f"{{{title}{{\\scriptsize {_esc(str(net))}}}}};"
        )
    for src, dst in pivots:
        lines.append(
            f"\\draw[-latex, very thick, dashed, colorCritical] ({nodes[src]}) to[bend left=12] ({nodes[dst]});"
        )
    lines.extend(host_labels)

    # Legend: only the OS families / line types actually present.
    items = []
    for fam in sorted(families):
        _keys, icon, color, name = _OS_FAMILIES[fam]
        items.append(
            (f"\\node[nd, minimum size=0.5cm, font=\\footnotesize, fill={color}] at (X,Y) {{{icon}}};", name)
        )
    items.append(("\\draw[gray!55, line width=1.2pt] (X1,Y) -- (X2,Y);", "Network link"))
    if pivots:
        items.append(("\\draw[-latex, very thick, dashed, colorCritical] (X1,Y) -- (X2,Y);", "Pivot"))
    if important:
        items.append(("\\node[nd, imp, minimum size=0.5cm, fill=gray!30] at (X,Y) {};", "Important host"))
    legend_y = bottom - 1.45
    lx = 0.0
    for mark, name in items:
        mark = (mark.replace("X1", f"{lx - 0.3:.2f}").replace("X2", f"{lx + 0.3:.2f}")
                .replace("X", f"{lx:.2f}").replace("Y", f"{legend_y:.2f}"))
        lines.append(mark)
        lines.append(f"\\node[anchor=west, font=\\scriptsize] at ({lx + 0.4:.2f},{legend_y:.2f}) {{{name}}};")
        lx += 1.0 + 0.16 * len(name)  # mark + label, ~0.16cm per \scriptsize character

    width = max(total_w, lx)
    height = core_y + 1.2 - (legend_y - 0.4)
    scale = min(1.0, _NET_MAX_W / width, _NET_MAX_H / height)
    return _raw(
        "\\begin{center}\n"
        f"\\begin{{tikzpicture}}[scale={scale:.3f}, every node/.style={{transform shape}}, {_NET_STYLES}]\n"
        + "\n".join(lines)
        + "\n\\end{tikzpicture}\n\\end{center}"
    )


# ── Web findings charts (rex ingest nikto) ──────────────────────────────────

_WEB_DATASET_ALIASES = {
    "host": "host",
    "hosts": "host",
}


def _tally_web(findings: list[dict], dataset: str) -> OrderedDict[str, int]:
    counts: OrderedDict[str, int] = OrderedDict()
    if dataset == "host":
        for f in findings:
            key = f.get("host") or "Unknown"
            counts[key] = counts.get(key, 0) + 1
    top = OrderedDict(
        sorted(counts.items(), key=lambda kv: kv[1], reverse=True)[:_TOP_N_ASSET_ENTRIES]
    )
    return top


def render_web(findings: list[dict], gtype: str, dataset: str) -> str:
    """Return a pandoc raw-LaTeX block for a ``:::web_graph:...:::`` directive."""
    gtype = _TYPE_ALIASES.get(gtype.strip().lower())
    dataset = _WEB_DATASET_ALIASES.get((dataset or "host").strip().lower(), "host")
    if gtype is None or gtype == "matrix":
        return _raw(
            "\\begin{center}\\textit{\\small Unknown chart type.}\\end{center}"
        )

    counts = _tally_web(findings, dataset)
    if not counts:
        return _raw(_empty(dataset))
    colors = _colors_for(dataset, list(counts.keys()))

    if gtype in ("pie", "donut"):
        return _raw(_pie(counts, colors, donut=(gtype == "donut")))
    if gtype == "bar":
        return _raw(_bar(counts, colors))
    return _raw(_empty(dataset))
