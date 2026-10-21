---
sidebar_position: 1
---

# build.sh Reference

`build.sh` is the user-facing entry point for rendering a report: it wraps
`docker run` (or a native `report-rex build` invocation with `--no-docker`)
and resolves the report/output paths to absolute paths for the volume mounts.

```
Usage: ./build.sh [options]

Options:
  --dir PATH        Report directory containing index.md
  --template NAME   Template name or path
  --output FILE     Output PDF path
  --final           Produce the final report, no DRAFT watermark (default: draft, appends -draft to filename)
  --docx            Also generate a .docx clone alongside the PDF
  --force           Skip QA-gate checks (validation errors) and build anyway
  --no-docker       Run natively (requires pandoc, xelatex, python in PATH)
  --image TAG       Docker image tag
  --password PASS   Encrypt the output PDF with this password (requires qpdf)
  -h, --help        Show this help
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dir PATH` | `reporting/example` | Report directory containing `index.md`. |
| `--template NAME` | from `index.md` | Template name, path, or `.zip` file — overrides whatever `index.md`'s frontmatter specifies. |
| `--output FILE` | `report.pdf` | Output PDF path. |
| `--final` | off | Produces the final report with no DRAFT watermark and the plain `<name>.pdf` filename. Default is draft, which renames the output to `<name>-draft.pdf`. |
| `--docx` | off | Also generates a `.docx` clone next to the PDF (same basename, `.docx` extension) — see [Output Formats](./output-formats.md). |
| `--force` | off | Skips the QA-gate validation checks (see [`report-rex validate`](./cli#report-rex-validate)) and builds even if they'd fail. Passed through as `--skip-validation` to the Python CLI. |
| `--no-docker` | off | Runs `report-rex` natively instead of via Docker. Requires `pandoc`, `xelatex`, and `python` on `PATH`. |
| `--image TAG` | `report-rex:latest` | Docker image tag to run (Docker mode only). |
| `--password PASS` | — | Encrypts the output PDF (AES-256, via `qpdf`) with this password. Omit for an unencrypted PDF (default, unchanged). The `.docx` clone, if any, is never encrypted. |
| `-h`, `--help` | — | Print the usage summary and exit. |

## Examples

```bash
# Build the example report using Docker (draft by default)
./build.sh --dir reporting/example --output report.pdf
# → produces report-draft.pdf

# Build the final report, no watermark
./build.sh --dir reporting/my-engagement --output report.pdf --final

# Override the template
./build.sh --dir reporting/my-engagement --template my-company-template

# Use a zipped template
./build.sh --dir reporting/my-engagement --template templates/branded.zip

# Build even though validation reports errors
./build.sh --dir reporting/my-engagement --force

# Native (no Docker)
./build.sh --dir reporting/my-engagement --no-docker

# Use a custom-tagged image
./build.sh --dir reporting/my-engagement --image report-rex:v2

# Quick communication PDF
./build.sh --dir reporting/communication --template quick-comm --output update.pdf
```

## Docker Volume Mounts

In Docker mode, `build.sh` maps three directories:

| Mount | Container Path | Mode |
|-------|----------------|------|
| `$REPORT_DIR` (abs) | `/report` | Read-only |
| `templates/` (abs) | `/opt/report-rex/templates` | Read-only |
| `$(dirname $OUTPUT)` | `/output` | Read-write |

The output directory is mounted separately from the filename because Docker cannot bind-mount a file that does not yet exist — only the parent directory is mounted.
