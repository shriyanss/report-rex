# core/ — Claude Guidance

## What This Is

The `report-rex-core` Python package. Installable via `pip install .`. Provides the `report-rex` CLI and all build logic.

## Structure

```
core/
├── pyproject.toml              Hatchling build system; defines report-rex CLI entry point
├── ruff.toml                   Linter config
└── src/report_rex_core/
    ├── __init__.py             Version string only
    ├── parser.py               Parse index.md → ReportStructure dataclass
    ├── preprocessor.py         Assemble multiple .md files → combined.md + metadata.yaml
    ├── validator.py            QA-gate checks for `validate`: required fields, prose length, screenshot heuristics
    ├── renderer.py             Call pandoc subprocess → PDF
    ├── builder.py              Orchestrate parse→assemble→render; scaffold for `new` command
    └── cli.py                  Click CLI: build / new / validate subcommands
```

## Running Tests

```bash
# From repo root
python3 -m venv .venv && .venv/bin/pip install -e "core/[dev]"
.venv/bin/pytest core/tests/ -v
```

Tests live in `core/tests/`. The `conftest.py` provides a `sample_report` fixture (a `tmp_path`-based directory tree). There are no integration tests requiring pandoc — those are in the `render-test` job in `.github/workflows/ci.yml`.

## Module Responsibilities

### `parser.py`

- `parse_index(index_path)` → `ReportStructure` — reads YAML frontmatter, resolves all paths, raises `FileNotFoundError` for missing files
- `split_frontmatter(content)` → `(str, str)` — splits `---...\n---` from the body
- `parse_frontmatter(content)` → `dict` — convenience wrapper

### `preprocessor.py`

- `assemble(structure, output_dir)` → `(combined_md, metadata_yaml)` — main assembler
- Calls `split_frontmatter` on each file, strips frontmatter from content
- For vuln files: calls `_vuln_div(meta)` to emit a pandoc fenced div
- Collects `timeline` entries from index.md's frontmatter and from every section/report/appendix file's own frontmatter, merges them, and calls `_timeline_table(entries)` to emit a sorted, `include`-filtered Markdown table rendered as a "Timeline" appendix
- Copies `assets/` from the report dir into `output_dir`
- Writes `combined.md` and `metadata.yaml` to `output_dir`

The fenced div format for vulns:
```
::: {.vulnerability severity="Critical" title="..." cvss="9.8" status="Open" affected="..."}
:::
```
The Lua filter in the template converts this to a tcolorbox LaTeX block.

### `validator.py`

- `validate_report(structure)` → `ValidationResult` — runs every QA-gate check and collects
  every problem into `ValidationResult.issues` (never fails fast on the first issue)
- `ValidationIssue(file, check, message, severity)` — `severity` is `"error"` (fails the
  `validate` command) or `"warning"` (printed but does not fail it)
- `index.md` check: `title`, `client`, `date` required (`metadata_missing`, `REQUIRED_METADATA`)
- Per-finding (`reports/*.md`) checks: unparseable YAML frontmatter (`frontmatter_invalid`,
  also for sections/appendices; skips that file's other checks), a `---` block not on the
  very first line (`frontmatter_not_at_top`, every file), title presence
  (`title_missing`), CVSS presence (`cvss_missing`), timeline presence
  (`timeline_missing`), at least one screenshot (`screenshot_none`), Replication (or legacy Steps to Reproduce) length (`steps_to_reproduce_length`, min
  `MIN_STEPS_TO_REPRODUCE_CHARS = 100`)
- Applies to every section/finding/appendix: leftover scaffold placeholder text
  (`placeholder_text`, sourced from `builder.py`'s `PLACEHOLDER_*` constants) and screenshot
  checks on every `![caption](path.png)` reference
- Screenshot checks: caption length (`screenshot_caption_length`, min
  `MIN_CAPTION_CHARS = 35`), referenced file exists (`screenshot_missing`), light-mode
  luminance (`screenshot_dark_mode`, min `MIN_LUMINANCE_LIGHT_MODE = 128`), annotation
  presence via saturated-color clustering (`screenshot_no_annotation`, min
  `MIN_SATURATED_CLUSTER_PX = 200`), and a best-effort text-legibility estimate via a
  row-density ink profile (`screenshot_text_too_small`, `severity="error"`, min
  `MIN_TEXT_HEIGHT_PX = 10`)
- A `> [!OVERRIDE-DARK-IMG]` line immediately above an image reference skips the
  `screenshot_dark_mode` check for that one image (other checks still run) — the marker is
  stripped by `preprocessor._strip_qa_directives` before rendering, so it never appears in
  the PDF
- A `> [!REX-STUB]` line (or `# > [!REX-STUB]` as a YAML comment in frontmatter) marks an
  unfilled stub value/section left over from a `vuln-templates/` template. Any occurrence
  is an unconditional error (`stub_marker_present`) — since `build` runs `validate_report`
  before rendering, a copied-but-unedited template blocks `report-rex build` until every
  marker and its stub content is replaced
- Uses Pillow + numpy for image analysis; both are hard dependencies in `pyproject.toml`

### `renderer.py`

- `render(combined_md, metadata_yaml, template_dir, output_pdf, draft)` — builds the pandoc command and runs it
- Key pandoc flags: `--from markdown+raw_tex+fenced_divs`, `--pdf-engine=xelatex`, `--lua-filter`, `--metadata-file`
- Always passes the template's `filters/vuln_filter.lua` if it exists
- Passes `--metadata=draft:true` when `draft=True`

### `builder.py`

- `build(report_dir, templates_root, output_pdf, template_override, draft)` — full pipeline
- `scaffold(target_dir, template_name)` — creates starter directory tree for `report-rex new`

### `cli.py`

- Click group with three subcommands: `build`, `new`, `validate`
- `_find_templates_root()` walks up from `cwd` to find a `templates/` directory

## Adding a New CLI Command

```python
@main.command("my-command")
@click.option("--foo", default="bar")
def my_command(foo: str) -> None:
    """One-line description shown in --help."""
    ...
```

## Common Pitfalls

- **YAML frontmatter delimiter**: must be `---` on its own line at the very start of the file (no leading whitespace). The `split_frontmatter` function checks `content.startswith("---")`.
- **Pandoc `--from` flag**: must include `+fenced_divs` for the `.vulnerability` divs to parse correctly.
- **`realpath -m`** is used for the output path because the file may not exist yet.
