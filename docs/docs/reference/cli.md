---
sidebar_position: 2
---

# CLI Reference

The `report-rex` command is installed by the `report-rex-core` Python package.

:::note
There's also a standalone [Go CLI](./go-cli) named `rex` that wraps the Docker
build pipeline (no repo checkout required). It has different, namespaced commands
(`report build`, `report init`, `image build`, `watch`) — see that page if you're
looking for a downloadable binary rather than a pip-installed console script.
:::

Running `report-rex` with no subcommand in an interactive terminal prints a banner
(the logo, rendered as ANSI truecolor art) followed by the usage help; piped or
non-interactive invocations skip the banner and print help text only.

## Commands

### `report-rex build`

Build a PDF report from a report directory. Runs the QA-gate checks (same as
`report-rex validate`) before building unless `--skip-validation` is given.

```
report-rex build [OPTIONS]

Options:
  --dir PATH               Report directory containing index.md
  --output, -o FILE        Output PDF filename
  --template TEXT          Override template (default: from index.md)
  --templates-root PATH    Root directory containing templates
  --final                  Produce the final report, no DRAFT watermark
  --docx                   Also generate a .docx clone alongside the PDF
  --skip-validation        Skip the QA-gate checks that normally run before building
  --password TEXT          Encrypt the output PDF with this password (requires qpdf)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dir PATH` | `.` | Report directory containing `index.md`. Must already exist. |
| `--output, -o FILE` | `report.pdf` | Output PDF filename. |
| `--template TEXT` | from `index.md` | Override the template name/path. |
| `--templates-root PATH` | auto-detected | Root directory containing templates. If omitted, walks up from the current directory looking for a `templates/` folder. |
| `--final` | off | Produces the final report with no DRAFT watermark. Default is draft. |
| `--docx` | off | Also generates a `.docx` clone next to the PDF (same basename, `.docx` extension) — see [Output Formats](./output-formats.md). |
| `--skip-validation` | off | Skips the QA-gate checks and builds even if they'd fail. |
| `--password TEXT` | — | Encrypts the output PDF (AES-256, via `qpdf`) with this password. Requires `qpdf` on `PATH`. Omit for an unencrypted PDF (default, unchanged). The `.docx` clone, if any, is never encrypted. |

```bash
report-rex build --dir reporting/acme-corp --output acme-report.pdf

report-rex build --dir reporting/acme-corp --template branded --final

report-rex build --dir reporting/acme-corp --output acme-report.pdf --docx

report-rex build --dir reporting/acme-corp --templates-root /opt/report-rex/templates \
  --output /tmp/acme-report.pdf --skip-validation

report-rex build --dir reporting/acme-corp --password 'hunter2'
```

### `report-rex new`

Scaffold a new report directory with starter files.

```
report-rex new [TARGET_DIR] [OPTIONS]

Arguments:
  TARGET_DIR  Directory to create

Options:
  --template TEXT  Template to scaffold for
```

| Argument/Flag | Default | Description |
|------|---------|-------------|
| `TARGET_DIR` | `reporting` | Directory to create. Fails if it already exists and is non-empty. |
| `--template TEXT` | `rit-cptc` | Template to scaffold starter content for. |

```bash
report-rex new reporting/acme-corp

report-rex new reporting/acme-corp --template branded
```

### `report-rex validate`

A QA gate for report content, not just structure. First checks that `index.md` parses and
every file referenced in `structure:` exists (unchanged from before), then runs a set of
content checks against every section, finding, and appendix and reports **every** problem
found, not just the first:

```
report-rex validate [OPTIONS]

Options:
  --dir PATH  Report directory to validate
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dir PATH` | `.` | Report directory to validate. |

```bash
report-rex validate --dir reporting/acme-corp
```

Checks performed on each `reports/*.md` finding:

- **Title present** (`title_missing`) — the finding needs a `title:`. Without one, or with no frontmatter block at all, it renders as "Untitled".
- **Valid frontmatter** (`frontmatter_invalid`, also for sections/appendices) — the YAML must parse. An unclosed quote, for example, drops all of the finding's metadata. In `rex report validate`, the issue points at the first line that breaks the YAML; for an unclosed quote, that's where the quote opens. The file's other checks are skipped.
- **Frontmatter at the top** (`frontmatter_not_at_top`, every file) — the opening `---` must be the very first line. A blank line or BOM before it makes Rex ignore the whole block.
- **CVSS present** — either `cvss_vector` or a manual `severity`/`cvss_score` must be set.
- **Timeline present** — the finding's own frontmatter must have at least one `timeline:` entry.
- **Replication length** — the `## Replication` section (or the older `## Steps to Reproduce`) must be at least 100 characters.
- **Leftover placeholder text** — flags scaffold sentinel text (from `report-rex new`) left unedited in a finding, section, or appendix.

Checks performed on every screenshot (`![caption](path.png)`) referenced from any section, finding, or appendix:

- **Caption length** — the image's alt text must be at least 35 characters.
- **Referenced file exists** — the image path must resolve to a real file.
- **Light mode** — the screenshot must not be a dark-themed capture (checked via average border luminance). Legitimately dark screenshots (terminal output, dark-mode tools) can be exempted per-image with an override marker — see below.
- **Annotation present** — the screenshot must contain a visible highlight (arrow, box, circle) calling out the evidence, detected via a saturated red/orange/yellow color-cluster heuristic.
- **Text legibility** *(warning only)* — a best-effort estimate of on-screen text size; flagged as a warning, not an error, since it's an approximation rather than a precise measurement.

Checks performed on the chart-data files under `artifacts/`, found by filename and checked against how the report's files use them:

- **Network diagram data present** (`network_assets_missing`) — a file uses `:::network_diagram:::` but `artifacts/assets.json` doesn't exist. This is an error, because the build would fail.
- **Chart data present** (`chart_data_missing`, *warning*) — a file uses `:::asset_graph:::` or `:::web_graph:::` but its data file (`assets.json` / `webfindings.json`) doesn't exist, so the chart renders "No data available".
- **Valid JSON** (`artifact_json_invalid`) — `assets.json` must be a JSON array of host objects, and `connections.json` must be a JSON object.
- **Valid host IPs** (`assets_invalid`) — every host in `assets.json` needs a valid `ip`.
- **Valid connections** (`connections_invalid`) — every `subnets` key must be a CIDR, every `pivots` endpoint an IP or CIDR, and every `important` entry an IP.
- **Unknown key** (`connections_key_unknown`, *warning*) — a top-level key other than `subnets`/`pivots`/`important`, usually a typo.
- **Unknown host** (`connections_host_unknown`, *warning*) — a pivot or `important` IP that isn't in `assets.json`. It's still drawn, as an unknown host, but is often a typo.
- **Unused** (`connections_unused`, *warning*) — `connections.json` exists but no file uses `:::network_diagram:::`.

These issues include the line they point at (the directive's line, or the offending JSON entry).

Errors fail the command (`exit 1`); warnings are printed but do not. Output is grouped by
file:

```
reports/vuln-3.md:
  [error] cvss_missing: No CVSS data: missing both 'cvss_vector' and 'severity'/'cvss_score'.
  [warning] screenshot_text_too_small: Estimated text height ~4.2px is below the legibility threshold (10px)...
artifacts/connections.json:
  [error] line 5: connections_invalid: pivots: "10.0.0.0/33" is not a valid IP or CIDR.

2 error(s), 1 warning(s) across 2 file(s)
```

#### Overriding the dark-mode check

Some screenshots are legitimately dark — terminal captures, dark-themed proxy tools — and
shouldn't fail the light-mode check. Add a `> [!OVERRIDE-DARK-IMG]` line directly above the
image reference to skip the dark-mode check for that one image (the annotation and
legibility checks still run):

```markdown
> [!OVERRIDE-DARK-IMG]
![Reverse shell caught on the attacker listener](../assets/screenshots/shell.png)
```

The marker is a validator directive, not report content — it's stripped automatically and
never appears in the rendered PDF.

See also: [Overrides reference](./overrides.md) for every override mechanism the tool has, in one place.

## Installing the CLI

From a repository checkout:

```bash
pip install core/
report-rex --version
```

Or from your own package registry (see [Install & Build Your First Report](../getting-started/cli.md)):

```bash
pip install report-rex-core \
  --index-url https://pypi.example.internal/simple \
  --trusted-host pypi.example.internal
```

When running outside a repository checkout, pass `--templates-root` so the CLI can
locate the bundled templates (they are not part of the wheel).
