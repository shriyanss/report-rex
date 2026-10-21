# RIT CPTC — Quick Communication Template

A lightweight one/two-pager for mid-engagement client communications (daily
progress updates, out-of-band critical-finding notifications), carrying the same
brand as the full [`rit-cptc`](../rit-cptc/README.md) report:

- Royal-blue masthead band with a mint title (echoes the report cover)
- Magenta confidentiality banner + folded corner accent on every page
- Compact severity panel — severity chip, CVSS, optional Impact / Sophistication,
  status, and affected assets, with the title carried inline (no cover page, no TOC)
- Indigo table headers with severity color coding

Use it for the same content as `quick-comm`, when you want the communication to
match a `rit-cptc` final report.

## Usage

```yaml
---
title: "Critical Finding — SQL Injection Requiring Immediate Action"
client: "All Ports Tours"
company: "RIT CPTC Team"
date: "2025-10-18"
confidentiality: "Confidential"
template: "rit-cptc-comm"
structure:
  sections:
    - sections/body.md
---
```

```bash
report-rex build --dir reporting/communication/critical-finding \
  --template rit-cptc-comm --output alert.pdf
```

## Sharing this template

```bash
zip -r rit-cptc-comm-template.zip templates/rit-cptc-comm/
```
