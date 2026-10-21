---
sidebar_position: 4
title: Charts
---

# Charts

Report Rex can generate charts from your findings' metadata and drop them into any section, finding, or appendix. Charts are computed across **all** findings in `structure.reports` and rendered as native LaTeX (TikZ / pgf-pie / tabularray) — no external image tooling is required.

## Directive Syntax

On its own line, write:

```
:::findings_graph:<type>:<dataset>:::
```

`<dataset>` is optional and defaults to `severity`. (`:::graph:<type>:<dataset>:::` is accepted as a shorter alias for the same directive.)

```markdown
:::findings_graph:donut:severity:::
:::findings_graph:bar:owasp:::
:::findings_graph:matrix:business:::
:::findings_graph:pie:::
```

The directive must be alone on its line (leading/trailing whitespace is allowed).

## Chart Types

| Type | Aliases | Description |
|------|---------|-------------|
| `pie` | | Pie chart with legend. |
| `donut` | `doughnut` | Pie chart with a hollow center showing the total count. |
| `bar` | `chart` | Horizontal bar chart, one row per category, longest bar first. |
| `matrix` | `table` | Category × severity count grid, with color-graded cells (darker = more findings). |
| `radar` | `spider` | Spider chart, one spoke per category. With `business`, all five business-risk categories always get a spoke (zeros included). Falls back to `bar` when there are fewer than three categories. |

## Datasets

| Dataset | Aliases | Source field |
|---------|---------|--------------|
| `severity` | `sev` | Each finding's (computed) severity. This is the default. |
| `owasp` | | `owasp` codes, resolved to their names. |
| `mitre` | `attack` | `mitre` technique codes, resolved to their names. |
| `business` | `business-risk`, `business_risk`, `risk` | `business_risk` categories. |
| `material` | `material_impact`, `material-impact`, `fair-mam` | `material_impact` (FAIR-MAM) categories. |

For `pie`/`donut`/`bar`, a finding contributes once per code it carries — a finding tagged with two `business_risk` categories is counted in both. `matrix` produces a category × severity grid: one row per OWASP/MITRE/business-risk/material-impact category actually present in the findings, one column per severity level (Critical → Info), with a count in each cell.

## Examples

Severity donut in an executive summary:

```markdown
The chart below summarises the findings by severity.

:::findings_graph:donut:severity:::
```

OWASP Top 10 coverage as bars, and a category × severity matrix in an appendix:

```markdown
:::findings_graph:bar:owasp:::

:::findings_graph:matrix:owasp:::
```

## Notes

- Charts read the same metadata described in [Vulnerability Files](./vulnerability-files.md) — severity is computed from each finding's CVSS vector, and OWASP/MITRE/business-risk come from the finding frontmatter.
- If there is no data for the requested dataset (e.g. no findings tag any OWASP category), the directive renders a small "No data available" note instead of a chart.
- An unrecognised chart type renders an "Unknown chart type" note; an unrecognised dataset name silently falls back to `severity`.
- Chart colors use the palette defined by the template. Severity charts use the severity colors (`critical`/`high`/`medium`/`low`/`info`), which are overridable — see [index.md Reference](./index-md.md#color-overrides). Non-severity charts (`owasp`/`mitre`/`business`/`material`) cycle through the template's ten categorical chart colors.

## Asset Charts

`rex ingest` (see [Go CLI Reference](../reference/go-cli.md#rex-ingest)) writes a
parallel directive, computed from the scanned host/port inventory
(`artifacts/assets.json`) instead of from findings:

```
:::asset_graph:<type>:<dataset>:::
```

`<type>` accepts the same `pie`/`donut`/`bar` values as `findings_graph` above,
plus `matrix` (alias `table`): instead of a category breakdown, it renders the
full host/port inventory as a table — Host, Hostname, OS, Port, Proto, Service,
Version, one row per open port, sorted by IP (a host with no open ports still
gets one row). `<dataset>` is ignored for `table`; for `pie`/`donut`/`bar` it's
optional, defaults to `os`, and accepts:

| Dataset | Aliases | What it counts |
|---------|---------|-----------------|
| `os` | | Each host's best-guess OS string (`Unknown` if nmap couldn't determine one, or if only `naabu` — which doesn't do OS detection — has scanned that host). This is the default. |
| `ports` | `port` | Open `<port>/<proto>` pairs across every host, top 10 by count. |
| `services` | `service` | Open port service names across every host, top 10 by count. |

`rex ingest` writes `:::asset_graph:table:ports:::`, `:::asset_graph:bar:ports:::`,
and `:::asset_graph:pie:os:::` into the `appendices/assets.md` it generates —
write additional ones by hand into any section, finding, or appendix if you
want a different type/dataset combination:

```markdown
:::asset_graph:bar:services:::
```

If `artifacts/assets.json` doesn't exist (no `rex ingest` run yet), every
`asset_graph` directive renders the same "No data available" note a
`findings_graph` directive would for an empty dataset — a report with no ingested
scans still builds cleanly.

## Network Diagram

Draw a network diagram of the scanned inventory (`artifacts/assets.json`, written
by `rex ingest`) by putting this directive on its own line in any section,
finding, or appendix:

```
:::network_diagram:::
```

It renders a topology map of the network:

- A **core network** cloud at the top, linked to one region per subnet.
- Each **subnet region** has a switch, linked to its hosts by a bus line per row
  of hosts. The subnet's label and CIDR are printed under the region.
- Each **host** is an icon tile, colored by OS family (Windows, Linux, macOS,
  FreeBSD, Android, or other/unknown), with its IP and first hostname underneath.
- **Pivots** are dashed red arrows between hosts or subnets.
- A **legend** lists the OS families and line types the diagram actually uses.

The diagram is scaled down to fit the page when it's wider or taller than the
text area.

Unlike the other directives, **`:::network_diagram:::` fails the build if
`artifacts/assets.json` doesn't exist.** Run `rex ingest` first.

### Quick start

1. **Scan.** From the report directory, ingest your scans. Each run merges into
   `artifacts/assets.json`:

   ```bash
   rex ingest nmap -sV -O 10.0.0.0/24
   rex ingest existing nmap -oX dmz-scan.xml   # or import a scan you already ran
   ```

   OS icons come from nmap's OS detection (`-O`, which needs root). Hosts found only by `naabu`
   show the generic "Other / unknown" icon.

2. **Place the diagram.** Add the directive on its own line wherever it should
   appear, e.g. at the end of `sections/methodology.md` or in an appendix:

   ```markdown
   ## Network Diagram

   :::network_diagram:::
   ```

3. **Label subnets and record pivots** (optional). Create
   `artifacts/connections.json` by hand (format below). `rex ingest` never
   overwrites it. Add an entry to `pivots` each time you move from one host or
   network into another, and list hosts the reader must see in `important`.

4. **Build.** Run `rex report build` (or `./build.sh`) as usual. The diagram is
   regenerated from the JSON files on every build, so re-running `rex ingest` or
   editing `connections.json` updates it automatically.

If the build stops with `:::network_diagram::: needs artifacts/assets.json`, run
step 1 first. If it stops with `network diagram: ...`, fix the IP or CIDR the
error names in `connections.json`.

### Checking your files

`rex report validate` (and so every build) lints `assets.json` and
`connections.json`, pointing at the exact line of each problem:

- a bad IP or CIDR, or invalid JSON (errors)
- a misspelled top-level key, such as `"pivot"` (warning)
- a pivot or `important` IP that isn't in the inventory (warning). Usually a
  typo, since the host would otherwise be drawn as unknown.
- a `connections.json` that no file uses (warning)
- a `:::network_diagram:::` with no `assets.json` (error, on the directive's line)

In VS Code, the [Report Rex extension](../getting-started/ide/vscode.md) shows
these as squiggles on the offending line. It re-checks whenever either file
changes, including when `rex ingest` rewrites `assets.json`. See the
[full list of checks](../reference/cli.md#report-rex-validate).

### Subnets and pivots: `artifacts/connections.json`

This optional file names subnets and draws pivots (manual connections):

```json
{
  "subnets": {
    "10.0.0.0/24": "Corporate LAN",
    "10.14.25.0/24": "OT network"
  },
  "pivots": {
    "192.168.3.44": "10.14.25.0/24",
    "192.168.3.0/24": ["10.23.23.0/24", "10.0.0.5"]
  },
  "important": ["10.0.0.10"]
}
```

| Key | Format | Effect |
|-----|--------|--------|
| `subnets` | CIDR → label | Each host is placed in the most specific listed CIDR that contains it, and the label is shown under the CIDR. A host in no listed CIDR goes into an automatic `/24` (`/64` for IPv6). A listed subnet with no hosts is still drawn. |
| `pivots` | IP or CIDR → IP, CIDR, or a list of them | Each pivot is drawn as a dashed red arrow from the source host (or subnet switch) to the target. A target that isn't in the inventory is added: a CIDR as an empty subnet region, an IP as a host with an unknown OS. |
| `important` | list of IPs | Hosts that must always appear in the diagram (see below), drawn with a red outline. An IP not in the inventory is added with an unknown OS. |

### Crowded subnets

Each subnet region shows at most 12 hosts, in a grid up to 3 wide. In a bigger
subnet, hosts that are a pivot endpoint or listed in `important` are always
shown. The remaining slots go to the other hosts in IP order, and the rest are
summarized as "+N more hosts". If the must-show hosts alone exceed 12, they're
all drawn, and the whole diagram is scaled down to fit the page. Hidden hosts still appear in the full
`:::asset_graph:table:ports:::` inventory table.

An invalid IP or CIDR, or malformed JSON, in `connections.json` fails the build
with an error naming the bad value.

## Web Findings Charts

`rex ingest nikto` (see [Go CLI Reference](../reference/go-cli.md#rex-ingest)) writes
another parallel directive, computed from the ingested web finding list
(`artifacts/webfindings.json`) instead of from findings or the host/port inventory:

```
:::web_graph:<type>:<dataset>:::
```

`<type>` accepts `pie`/`donut`/`bar` (`matrix` is not supported for web charts).
`<dataset>` is optional and defaults to — and, for now, only accepts — `host`:

| Dataset | Aliases | What it counts |
|---------|---------|-----------------|
| `host` | `hosts` | Number of web findings per host, top 10 by count. This is the only (and default) dataset. |

`rex ingest nikto` writes `:::web_graph:bar:host:::` into the
`appendices/web-findings.md` it generates — write additional ones by hand into any
section, finding, or appendix if you want a different chart type over the same
dataset:

```markdown
:::web_graph:donut:host:::
```

If `artifacts/webfindings.json` doesn't exist (no `rex ingest nikto` run yet),
every `web_graph` directive renders the same "No data available" note the other
chart directives fall back to for an empty dataset.
