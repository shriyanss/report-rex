---
sidebar_position: 1
---

# Install & Build Your First Report

This page gets you from nothing installed to a rendered PDF. It covers three
ways to install Report Rex — pick one — then walks through scaffolding,
editing, and building a report.

## Installing

### Option A — Standalone binary (recommended)

The [Go CLI](../reference/go-cli.md) (`rex`) is a single downloadable binary —
no repository checkout needed. It parses, assembles, and validates your report
natively; Docker only comes into play for the final pandoc/XeLaTeX render (and
is built automatically the first time it's needed).

Download the archive for your platform from the
[Releases page](https://github.com/shriyanss/report-rex/releases), extract it, and put `rex` on your `PATH`:

```bash
tar -xzf report-rex_<version>_<os>_<arch>.tar.gz
sudo mv rex /usr/local/bin/
rex version
```

Then scaffold and build:

```bash
rex report init --dir my-engagement   # scaffold from a full example
cd my-engagement
rex report build                      # builds the Docker image on first run
```

See the [Go CLI Reference](../reference/go-cli.md) for the full command list —
the same archive also contains `rex-svr`, the sync server used for
[team collaboration](./server.md).

### Option B — Docker

The runtime image bundles pandoc, XeLaTeX, all required LaTeX packages, and the
`report-rex` CLI, so there are no local dependencies beyond Docker.

```bash
# Pull the published image (substitute your own registry)
docker pull <your-registry>/report-rex:latest

# …or pin a specific release
docker pull <your-registry>/report-rex:v0.1.0
```

`build.sh` wraps Docker for you. Point it at the pulled image with `--image`:

```bash
./build.sh --image <your-registry>/report-rex:latest \
           --dir reporting/example --output report.pdf
```

### Option C — Native (from source)

Native rendering skips Docker but needs a local pandoc + XeLaTeX toolchain.

Requirements:
- Python 3.11+
- [pandoc](https://pandoc.org/installing.html) ≥ 3.0
- A TeX distribution with XeLaTeX — [TeX Live](https://tug.org/texlive/) (Linux/macOS) or [MiKTeX](https://miktex.org/) (Windows)

```bash
# Clone this repository (brings the templates/ and reporting/ examples with it)
git clone <your-fork-or-repo-url> && cd report-rex

# Install the core package (provides the report-rex CLI)
pip install core/

# Build a report natively
./build.sh --dir reporting/example --output report.pdf --no-docker
```

#### Required LaTeX packages

The Docker image already includes these. For a native install on a minimal TeX
Live, install them with `tlmgr` (the same set the runtime image ships):

```bash
tlmgr install \
  tcolorbox fontawesome5 titlesec fancyhdr geometry xcolor pgf \
  environ trimspaces draftwatermark everypage listings pgfplots \
  booktabs microtype unicode-math enumitem parskip tex-gyre \
  pdfcol tikzfill tabularray ninecolors pgf-pie framed fvextra \
  caption float lineno upquote
```

#### Installing just the Python CLI (CI / standalone)

To get the `report-rex` command without cloning — for example inside a
pipeline — install the package from your own package registry:

```bash
pip install report-rex-core \
  --index-url https://<your-registry>/api/v4/projects/<id>/packages/pypi/simple \
  --trusted-host <your-registry>
report-rex --version
```

Note: templates live in the repository (and in the Docker image under
`/opt/report-rex/templates`). When you run the CLI outside a repo checkout, pass
`--templates-root /path/to/templates` so it can find them.

## Your First Report

### 1. Scaffold a new report

With the Go CLI:

```bash
rex report init --dir my-engagement
```

Or, with the Python CLI (`report-rex-core`), scaffold with `new` instead:

```bash
report-rex new my-engagement
```

Either way, this creates a ready-to-build starter tree:

```
my-engagement/
├── index.md              # metadata + document structure
├── sections/
│   ├── exec_summ.md
│   └── methodology.md
├── reports/
│   └── vuln-1.md         # one finding per file
├── appendices/
│   └── 1.md
└── assets/
    └── screenshots/      # drop evidence images here
```

### 2. Edit `index.md`

`index.md` holds all report metadata and the document structure. The scaffold
starts you off with:

```yaml
---
title: "Penetration Test Report"
subtitle: "Final Report"
client: "Client Name"
company: "Your Security Firm"
engagement_type: "Web Application Penetration Test"
report_id: "RR-2024-0001"
date: "2024-01-01"
start_date: "2024-01-01"
end_date: "2024-01-05"
version: "1.0"
confidentiality: "Confidential"
template: "rit-cptc"

contact_name: "Your Name"
contact_email: "you@example.com"
distribution:
  - "Client Security Team"

authors:
  - name: "Your Name"
    role: "Lead Penetration Tester"

# Branding (optional) — uncomment and point at files under assets/ to add logos.
# company_logo: "assets/company-logo-dark.png"    # dark logo, for the light header
# company_logo_light: "assets/company-logo.png"   # light logo, for the dark cover band
# client_logo: "assets/client-logo.png"           # client logo, shown on the cover

structure:
  sections:
    - sections/exec_summ.md
    - sections/methodology.md
  reports:
    - reports/vuln-1.md
  appendices:
    - appendices/1.md
---
```

Every field except `title` and `client` is optional. See the full
[index.md reference](../reporting-structure/index-md.md) for where each field appears
on the cover, Document Control page, and header/footer, plus color overrides.

### 3. Write your findings

Each file in `reports/` is one finding: YAML frontmatter followed by the write-up.
**You don't set the severity by hand** — paste the CVSS vector and Report Rex
computes the score and severity for you:

```markdown
---
title: "SQL Injection in Login"
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"  # → 9.8, Critical
status: "Open"
component: "Authentication API"
cwe: "CWE-89"
owasp: "A03:2021"        # auto-resolved to "Injection"
mitre: "T1190"           # auto-resolved to the technique name
business_risk:
  - Operational
  - Financial
affected_urls:
  - "https://target.com/login"
---

## Description
...
```

Findings are **automatically sorted by severity** (Critical → Info), then by CVSS
score (highest first), tie-broken by title, and numbered sequentially — the order
you list them in `index.md` doesn't matter. See
[Vulnerability Files](../reporting-structure/vulnerability-files.md) for the complete schema, manual severity for v4.0 vectors, and the recommended body
structure.

### 4. Build

With the Go CLI:

```bash
rex report build --dir my-engagement --output report.pdf
```

Or with `build.sh` directly:

```bash
# Docker (recommended)
./build.sh --dir my-engagement --output report.pdf

# …or natively
./build.sh --dir my-engagement --output report.pdf --no-docker
```

Every build is a watermarked draft by default:

```bash
./build.sh --dir my-engagement --output report.pdf
# → produces report-draft.pdf
```

Pass `--final` to produce the plain, unwatermarked report instead:

```bash
./build.sh --dir my-engagement --output report.pdf --final
# → produces report.pdf
```

(`rex report build --final` does the same thing.)

`rex report build` runs the same QA-gate checks as `rex report validate` first
and aborts on any error unless you pass `--force`. Run validation on its own —
no Docker or Python needed — to iterate on content without a full render:

```bash
rex report validate --dir my-engagement
```

### 5. Add charts (optional)

Drop a directive into any section to render a breakdown chart generated from your
findings — no manual data entry:

```markdown
:::findings_graph:donut:severity:::
```

See [Charts](../reporting-structure/charts.md) for all chart types (pie, donut, bar,
matrix) and datasets (severity, OWASP, MITRE, business risk).

## What's next

Working solo, you're done — keep editing and re-running the build command.
Working with a team, see [Real-Time Collaboration](./server.md) to set up
`rex-svr` and keep everyone's copy in sync.
