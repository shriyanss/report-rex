# reporting/ — Claude Guidance

## What This Is

Example report directories. Users copy these as a starting point for each engagement. Each subdirectory is a self-contained report that can be built independently.

## Directory Layout

```
reporting/
├── example/                   Full pentest report example (4 findings, 2 sections, 3 appendices)
│   ├── index.md               YAML frontmatter only — wires up everything
│   ├── sections/
│   │   ├── exec_summ.md       Executive summary
│   │   └── methodology.md     Testing methodology
│   ├── reports/
│   │   ├── vuln-1.md          Critical: SQL Injection
│   │   ├── vuln-2.md          High: Stored XSS
│   │   ├── vuln-3.md          Medium: IDOR
│   │   └── vuln-4.md          Low: Missing security headers
│   ├── appendices/
│   │   ├── 1.md               Scope and tools
│   │   ├── 2.md               Findings coverage (OWASP / MITRE / business-risk)
│   │   └── 3.md               FAIR-MAM material impact
│   └── assets/screenshots/    PNG evidence screenshots (committed to repo)
└── communication/
    ├── critical-finding/       Critical finding out-of-band alert
    │   ├── index.md
    │   └── sections/body.md
    └── progress-update/        End-of-day progress update
        ├── index.md
        └── sections/body.md
```

## index.md Schema

All report metadata lives in index.md YAML frontmatter:

```yaml
---
title: "..."          # required
client: "..."         # required
date: "YYYY-MM-DD"    # required
version: "1.0"        # optional
confidentiality: "Confidential"  # optional
template: "rit-cptc"  # optional (default: "rit-cptc")
authors:
  - name: "..."
    role: "..."
logo:
  path: "assets/logo.png"  # relative to this directory
  width: "4cm"
colors:              # override theme.yaml defaults
  primary: "1A1A2E"
timeline:            # optional — also allowed in section/report/appendix frontmatter; merged and sorted
  - date: "2024-01-10T09:00:00+00:00"
    description: "..."
    include: true    # optional, defaults to true; false keeps it out of the PDF
structure:
  sections: [...]    # paths relative to this directory
  reports:  [...]
  appendices: [...]
---
```

## Default Finding Templates

`vuln-templates/` (repo root, sibling of `reporting/`) has copy-pasteable
starter write-ups for common finding classes (CSRF, SSRF, broken access
control, XXE, SSTI, etc.). Every stub value/section in those templates is
marked with a `> [!REX-STUB]` line — `report-rex build` fails
(`stub_marker_present`) until every marker and its stub content is replaced.
See `vuln-templates/README.md` for the full workflow.

## Vulnerability File Frontmatter

```yaml
---
title: "..."           # required — shown in severity box
severity: "Critical"   # required — Critical|High|Medium|Low|Info
cvss_score: 9.8        # optional — 0.0–10.0
cvss_vector: "..."     # optional
status: "Open"         # optional — Open|In Progress|Remediated
affected_urls:         # optional — list
  - "https://..."
---
```

## Image Paths

All image paths in markdown are relative to the file containing them:

```markdown
# In reports/vuln-1.md:
![Screenshot](../assets/screenshots/sqli.png)

# In sections/exec_summ.md:
![Risk matrix](../assets/screenshots/risk-matrix.png)
```

The preprocessor copies `assets/` into the build temp directory, so pandoc resolves them correctly.

## Building

```bash
# Full report
./build.sh --dir reporting/example --output report.pdf

# Quick communication
./build.sh --dir reporting/communication/critical-finding --template quick-comm --output alert.pdf

# Draft (default)
./build.sh --dir reporting/example --output report.pdf

# Final
./build.sh --dir reporting/example --output report.pdf --final
```

## Per-Engagement Usage

For each new engagement, copy the example directory:

```bash
cp -r reporting/example reporting/client-acme-2024
# or use the scaffold command:
report-rex new reporting/client-acme-2024
```
