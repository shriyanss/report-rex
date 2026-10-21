# Quick Communication Template

A lightweight one-to-two-page template for mid-engagement communications:
client progress updates, critical finding notifications, and daily briefings.

No table of contents, no separate cover page — just a clean header block and content.

## Usage

```bash
./build.sh --dir reporting/communication --template quick-comm --output daily-update.pdf
```

## Example index.md

```yaml
---
title: "Critical Finding — Immediate Action Required"
client: "ACME Corp"
date: "2024-01-15"
confidentiality: "Confidential"
template: "quick-comm"
structure:
  sections:
    - incident-1.md
---
```
