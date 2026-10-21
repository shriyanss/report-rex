---
sidebar_position: 3
---

# Sections and Appendices

## Section Files (`sections/`)

Section files are regular markdown with an optional YAML frontmatter `title` key.

```markdown
---
title: "Executive Summary"
---

Write your content here using standard Markdown.

Tables, code blocks, lists, and images all work as expected.
```

If `title` is present in frontmatter, it becomes a top-level `# Heading` in the output. If absent, the content is included as-is (useful when you want to control heading levels yourself).

Images are referenced relative to the section file itself:

```markdown
![Risk matrix](../assets/screenshots/risk-matrix.png)
```

A section file's own frontmatter can also carry `timeline` entries — see [index.md Reference](./index-md.md#timeline) — which get merged with `index.md`'s and every other content file's timeline into a single sorted "Timeline" appendix.

Sections and appendices can also embed auto-generated [charts](./charts.md) (e.g. `:::findings_graph:donut:severity:::`) or a [network diagram](./charts.md#network-diagram) (`:::network_diagram:::`), and use the extra [Markdown features](./markdown-features.md) — link footnotes, finding cross-references, and syntax highlighting.

They are listed, in render order, under `structure.sections` in `index.md`:

```yaml
structure:
  sections:
    - sections/exec_summ.md
    - sections/methodology.md
```

## Appendix Files (`appendices/`)

Appendix files work identically to section files. They are rendered after the vulnerability findings, under an "Appendices" section heading, and are listed under `structure.appendices`. The `title` frontmatter key becomes a `## Heading` (one level below sections), lettered by position: `Appendix A — …`, `Appendix B — …`, and so on. Any lettering already in the title (`Appendix B —`, `Appendix —`, `Appendix 2:`) is replaced, so the order in `structure.appendices` decides the letters. Headings inside the appendix body are demoted one level, so they nest under its title.

```markdown
---
title: "Appendix A — Scope"
---

| Asset | In Scope |
|-------|----------|
| example.com | Yes |
```

If a report has a merged timeline (from `index.md` and/or any section/report/appendix file), it is rendered as the final, lettered appendix ("Appendix X — Timeline") even if `structure.appendices` is empty.

### Generated Asset Appendix

`rex ingest nmap`/`rex ingest naabu` (see [Go CLI Reference](../reference/go-cli.md#rex-ingest))
generate `appendices/assets.md` for you — a host/port table plus [asset
charts](./charts.md#asset-charts) — from a real scan, and add it to
`structure.appendices` automatically. It's a regular appendix file once
generated (same frontmatter shape, same render position), but it's fully
overwritten on every `ingest` run, so treat it as generated output rather than
something to hand-edit.

### Generated Web Findings Appendix

`rex ingest nikto` generates `appendices/web-findings.md` the same way — a
host/port/finding table (with an ID, description, and references column) plus
a [findings-by-host chart](./charts.md#web-findings-charts), regenerated from
`artifacts/webfindings.json` on every run and added to `structure.appendices`
automatically. Same rules as the asset appendix: it's a normal appendix file
once written, but fully overwritten on the next `ingest nikto` run, so don't
hand-edit it.

### Generated AD Enumeration Appendix

`rex ingest bloodhound <path>` generates `appendices/ad-enum.md` from an
imported SharpHound/BloodHound-CE export (see [Go CLI
Reference](../reference/go-cli.md#rex-ingest) — this one imports an existing
collector export rather than running a live collector). It lists the domains
collected, an object-counts table (users/computers/groups), and four
bullet-list sections pulled straight from the collector's raw AD attributes:
High-Value Principals (`admincount`-set, i.e. AdminSDHolder-protected),
Kerberoastable Accounts (enabled users with `hasspn`), AS-REP Roastable
Accounts (enabled users with `dontreqpreauth`), and Unconstrained Delegation
(computers with `unconstraineddelegation`). This is deliberately not a
BloodHound-style graph/attack-path analysis — just the handful of raw AD
properties pentesters commonly flag directly — and, like the other two
generated appendices, it's fully overwritten on every `ingest bloodhound` run.

## Communication Reports

For quick, mid-engagement PDFs (daily updates, critical finding notifications), use a communication template (`rit-cptc-comm` or `quick-comm`) instead of a full-report one. A communication report's `index.md` is much smaller: no `structure.reports` or `structure.appendices`, usually a single `structure.sections` entry, and typically no `authors`/`distribution`/`timeline` block since these reports are short-lived. Findings aren't listed under `structure.reports` at all — instead, an inline `.vulnerability` fenced div (see [Vulnerability Files](./vulnerability-files.md#inline-vulnerability-boxes)) is embedded directly in the body section.

`reporting/communication/critical-finding/index.md`:

```yaml
---
title: "Critical Finding — SQL Injection Requiring Immediate Action"
client: "ACME Corporation"
company: "Report Rex"
date: "2024-01-12"
confidentiality: "Confidential"
template: "quick-comm"
company_logo: "assets/company-logo-dark.png"
client_logo: "assets/acme-logo.png"
structure:
  sections:
    - sections/body.md
---
```

`reporting/communication/critical-finding/sections/body.md` (excerpt):

```markdown
## Summary

During today's testing session (Day 1), a **Critical** severity SQL
Injection vulnerability was identified in the primary authentication
endpoint.

::: {.vulnerability severity="Critical" title="SQL Injection in /api/login" cvss="9.8" status="Open" affected="https://portal.acme.com/api/login"}
:::

## What Was Found
...

## Immediate Actions Requested
...
```

`reporting/communication/progress-update/` follows the same pattern for a routine daily-status PDF with no findings box at all — just plain `## Summary` / `## Progress` / `## Next Steps` sections in `sections/body.md`.

Build either with `--dir` pointed at the specific report directory:

```bash
./build.sh --dir reporting/communication/critical-finding --template quick-comm --output alert.pdf
```

The `--template` flag is optional if `template: "quick-comm"` is already set in `index.md`, as shown above — it's mainly useful to preview a different template against the same content.
