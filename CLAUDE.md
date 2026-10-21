# Report Rex — Claude Guidance

## Project Overview

Report Rex converts Markdown → LaTeX → PDF for pentest reports. The pipeline is:

```
index.md + section .md files
  → core/parser.py       (parse structure)
  → core/preprocessor.py (assemble combined.md + vuln fenced divs)
  → pandoc               (Markdown + Lua filter → LaTeX)
  → XeLaTeX              (LaTeX → PDF)
```

## Directory Responsibilities

| Directory | Purpose |
|-----------|---------|
| `core/` | Python package (`report-rex-core`). All transformation logic. |
| `templates/` | Self-contained LaTeX template directories. Shareable as zip. |
| `reporting/` | Example report content. Users copy this per engagement. |
| `docs/` | Docusaurus 3.x documentation site. |
| `build.sh` | User-facing build script (Docker or `--no-docker` native). |
| `Dockerfile` | Runtime image based on `pandoc/latex:3.6` + Python. |
| `.github/workflows/` | GitHub Actions: `ci.yml` (lint + test), `docs.yml` (Pages), `release.yml` (tag publishing). |
| `cli/` | Go CLI (`rex` standalone binary, plus the `rex-svr` sync server). Embeds `Dockerfile`/`core/`/`templates/` via `go:embed`, drives Docker on the user's behalf. See `cli/CLAUDE.md`. |
| `version.txt` | Source of truth for the Go CLI's release version (independent of `core/pyproject.toml`'s Python package version). |
| `CHANGELOG.md` | Keep-a-Changelog history. Update it alongside any significant change. |
| `.goreleaser.yml` | GoReleaser config: cross-compiles `rex`/`rex-svr` for 5 platforms, publishes GitHub Releases on version tags. |

## Development Commands

```bash
# Install Python package
python3 -m venv .venv && .venv/bin/pip install -e "core/[dev]"

# Run tests
.venv/bin/pytest core/tests/ -v

# Lint
.venv/bin/ruff check core/src/

# Build Docker image
docker build -t report-rex:latest .

# Render the example report (Docker)
./build.sh --dir reporting/example --output example.pdf

# Render as a final report (default is draft, with a watermark)
./build.sh --dir reporting/example --output example.pdf --final

# Render a quick-comm
./build.sh --dir reporting/communication/critical-finding --template quick-comm --output alert.pdf

# Validate report structure
.venv/bin/report-rex validate --dir reporting/example

# Scaffold a new report
.venv/bin/report-rex new my-engagement

# Build the Go CLI locally (materializes embedded assets first)
cd cli && make build

# Run Go CLI tests
cd cli && make test

# Try the Go CLI against the example report
cli/bin/rex report build --dir reporting/example --output example.pdf
```

## Key Design Decisions

- **Vulnerability metadata** lives in YAML frontmatter of `reports/vuln-N.md`. The preprocessor converts it to pandoc fenced divs (`.vulnerability severity="Critical" ...`), and the Lua filter converts those to tcolorbox LaTeX blocks.

- **Template discovery** order: directory path → `.zip` file → bare name under `templates/`. Templates are self-contained and shareable as a zip.

- **Docker volume mounts**: report dir as `/report` (read-only), templates as `/opt/report-rex/templates` (read-only), output *directory* as `/output` (read-write). The file itself cannot be pre-mounted because it doesn't exist yet.

- **Draft by default**: every build adds `--metadata=draft:true` to pandoc (activates `\usepackage{draftwatermark}` in the template) and renames the output from `report.pdf` → `report-draft.pdf`, unless `--final` is passed.

- **`theme.yaml`** provides default pandoc variable values per template; any key can be overridden in `index.md` frontmatter.

## Adding a New Template

1. Create `templates/<name>/` with `report-rex.yaml`, `template.tex`, `theme.yaml`, `filters/vuln_filter.lua`
2. Reference it in `index.md` via `template: "<name>"` or pass `--template <name>` at build time
3. See `core/CLAUDE.md` and `templates/CLAUDE.md` for details

## Adding New Core Features

- New CLI commands → `core/src/report_rex_core/cli.py`
- New preprocessing steps → `core/src/report_rex_core/preprocessor.py`
- Renderer changes → `core/src/report_rex_core/renderer.py`
- Always add tests in `core/tests/`

## Go CLI (`cli/`)

The Go CLI (`cli/`) is a standalone `rex` binary, distributed separately from
the Python `report-rex-core` package, that wraps the Markdown→LaTeX→PDF pipeline
described at the top of this file. See `cli/CLAUDE.md` for Go-specific conventions.

- **Embed-at-build architecture**: `cli/scripts/prepare-embed.sh` copies the
  Dockerfile, `core/`, `templates/`, `styles/`, `.vale.ini`, and the
  `reporting/example`/`reporting/communication/*` scaffolds into a gitignored
  `cli/internal/embedded/data/` before every `go build`/`go test` (both locally via
  `make` and in CI). `go:embed` cannot reference paths outside its own module
  directory, so this sync step is required — never edit `internal/embedded/data/`
  directly, and never commit it.
- **Commands**: `rex image build` (builds the Docker image from embedded
  source), `rex report build` (drop-in replacement for `./build.sh`'s Docker
  mode, defaulting `--dir` to the current directory instead of `reporting/example`;
  auto-builds the image if it's missing), `rex report init` (extracts the
  embedded full-report/quick-comm example trees — no Docker needed), `rex
  watch` (fsnotify-based rebuild loop), `rex handoffs` (work handoffs under
  `handoffs/<assignee>/<title>.md`, synchronized in scoped queues against
  `rex-svr`), `rex mcp` (stdio MCP server so an AI harness can add/update
  vulnerability findings without touching report files directly — every
  finding it creates is stamped `ai: true`, and it refuses to modify a
  finding that isn't already AI-owned unless told to; see `docs/docs/
  reference/mcp.md`). `rex ingest <tool>` folds a scan tool's output
  (nmap/naabu/nikto/bloodhound) into the report's appendices + Timeline, and
  `rex recon --host/--cidr` chains those: a two-phase nmap scan, then nikto and
  nuclei against the web apps it discovers (fails fast if any of nmap/nikto/
  nuclei is missing).
- No Docker SDK dependency — `internal/docker` shells out to the `docker` CLI the
  same way `build.sh` does, keeping the binary small and CGO-free.

## Dev & Release Workflow

- All development (Python core, Go CLI, templates, docs) happens on the `dev`
  branch (branched from `main`), not directly on `main`.
- Before branching new work off `dev`, verify `dev` and `main` haven't
  diverged — a fix occasionally lands straight on `main` (e.g. via a direct
  merge or hotfix), leaving `dev` behind it. If `main` has commits `dev`
  lacks, merge `main` into `dev` directly first (no separate branch/PR needed
  for that catch-up merge) so new work always starts from parity.
- To release: open a PR from `dev` → `main`; once CI is green and it merges, bump
  `version.txt`, then push a `vX.Y.Z` tag from `main`. The tag runs
  `release.yml`: GoReleaser (GitHub Release), `.vsix` upload, GHCR image, PyPI.
- Update `CHANGELOG.md` alongside any significant change — don't batch it up later.

## CI/CD

- GitHub Actions `ci.yml` runs on every push to `main`/`dev` and every PR: lint,
  Python/Go/VS Code tests, e2e (`e2e-cli`, `e2e-ingest`, synctest), and `render-test`
  (every template rendered inside the runtime image via `scripts/render-all-templates.sh`)
- `release.yml` publishes on version tags: `git tag v1.0.0 && git push origin v1.0.0`
- `docs.yml` deploys the docs to GitHub Pages on `main`

## Do Not

- Do not hard-code absolute paths anywhere — all paths must be relative or resolved at runtime
- Do not mix pandoc template syntax (`$variable$`) with Jinja2 syntax (`{{ variable }}`) in the same file
- Do not skip tests when modifying `parser.py` or `preprocessor.py` — these are the core logic files
