---
slug: /
sidebar_position: 1
---

# Introduction

<p align="center">

![Report Rex logo](../static/img/logo.png)

</p>

**Report Rex** is an open-source Markdown-to-PDF report generation tool built for penetration testers.

Write your findings in Markdown. Get a professional PDF.

## Why Report Rex?

- **Markdown-first** — Write in your editor of choice, commit to git, diff like code.
- **Professional output** — Polished LaTeX-rendered PDFs with cover pages, TOC, and colored severity boxes.
- **Automatic CVSS** — Paste a CVSS v3.x vector; the base score and severity are computed for you. Findings are auto-sorted by severity and numbered.
- **Charts from your findings** — Drop a directive like `:::findings_graph:donut:severity:::` to render severity, OWASP, MITRE, and business-risk breakdowns as native LaTeX.
- **Network diagrams** — `:::network_diagram:::` turns your `rex ingest` scan inventory into a topology map with OS icons, subnet labels, and the pivots you took. See [Charts → Network Diagram](./reporting-structure/charts.md#network-diagram).
- **Screenshot to text** — `rex ocr shot.png --lang http` turns a screenshot of code or a Caido request into a code block you can paste, offline. See [Go CLI → `rex ocr`](./reference/go-cli.md#rex-ocr).
- **Modular templates** — Ship and share self-contained template packages (zip them up, drop them in).
- **Docker-native** — No LaTeX installation headaches; run `./build.sh` (or `rex report build`) and get a PDF.
- **Custom branding** — Reusable across firms: swap logos, colors, and fonts per client via `index.md` or `theme.yaml`, and inject per-report LaTeX with `header-includes`.
- **CI/CD friendly** — Build reports in pipelines; the `--no-docker` flag targets native environments.
- **Real-time collaboration** — `rex-svr` + `rex sync` keep a team's report workspace in sync, with divergent edits flagged as conflicts instead of silently overwritten.

## What it looks like

The example report renders a branded cover, a Document Control page, an executive
summary with charts generated from your findings, per-finding severity panels, and
an OWASP/MITRE coverage matrix.

| | |
|---|---|
| ![Cover page](/img/screenshots/cover.png) | ![Document Control](/img/screenshots/document-control.png) |
| ![Executive summary with charts](/img/screenshots/executive-summary.png) | ![Finding detail](/img/screenshots/finding-detail.png) |

![OWASP coverage matrix](/img/screenshots/coverage-matrix.png)

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Report directory** | A directory with `index.md` + `sections/`, `reports/`, `appendices/` |
| **index.md** | Wires together the structure; holds all report metadata |
| **Template** | A self-contained LaTeX + Lua filter package in `templates/<name>/` |
| **build.sh** | User-facing build script; delegates to Docker or native Python |
| **rex** | Standalone Go CLI; parses/assembles/validates natively, uses Docker (or a local `pandoc`) only for the final PDF render — no repo checkout needed |
| **rex-svr / rex sync** | Optional sync server + client for real-time, multi-person report workspaces |
| **Handoff** | A `handoffs/<teammate>/<title>.md` file passing a thread of investigation to someone else, versioned server-side |

## Quick Start

```bash
# Clone this repository
git clone <your-fork-or-repo-url> && cd report-rex

# Build the example report (Docker required)
./build.sh --dir reporting/example --output example-report.pdf

# Or natively (requires pandoc, xelatex, python)
./build.sh --dir reporting/example --output example-report.pdf --no-docker
```

See [Getting Started](./getting-started/cli.md) for installing the standalone `rex`
binary, pulling the prebuilt Docker image, or installing from your own package
registry.
