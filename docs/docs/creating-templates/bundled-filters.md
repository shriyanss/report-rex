---
sidebar_position: 5
---

# Bundled Lua Filters

Every bundled template ships two Lua filters in its `filters/` directory. The
renderer (`core/src/report_rex_core/renderer.py`) picks them up by file name —
there's no registration step:

| File | Used for | Passed to pandoc when |
|------|----------|-----------------------|
| `filters/vuln_filter.lua` | PDF output (emits raw LaTeX) | every PDF build, if the file exists |
| `filters/docx_vuln_filter.lua` | DOCX output (emits raw OpenXML) | `--docx` builds, if the file exists |

Either file is optional — a template without one just gets pandoc's default
rendering for that output format.

## Available Filters

| Template | Filter | Handles | What it does |
|----------|--------|---------|--------------|
| `rex-default` | `vuln_filter.lua` | `Div`, `Table`, `Code` | Vulnerability div → `vulnbox` with severity chip, CVSS score (+ version), status, and a details table. Restyles tables; makes inline code breakable. |
| `rex-default` | `docx_vuln_filter.lua` | `Pandoc` | Vulnerability div → shaded severity bar + properties table. Prepends a cover block and a TOC field. |
| `rit-cptc` | `vuln_filter.lua` | `Pandoc`, `Div`, `Table`, `Code` | Inserts a findings summary table before the first finding (`Pandoc`, run as a first pass). Vulnerability div → `vulnbox` with severity chip, finding ID, CVSS score (+ version) and a details table; affected assets nest under Component. Restyles tables with full grid lines; makes inline code breakable. |
| `rit-cptc` | `docx_vuln_filter.lua` | `Pandoc` | Rebuilds the PDF layout in OpenXML: brand-band cover (its own unnumbered section), Confidentiality / Document Control page, TOC with page numbers, numbered headings, findings summary table, the PDF's finding panel, and `\clearpage` page breaks. Page geometry, fonts and the corner/banner page art come from its `reference.docx`. |
| `quick-comm` | `vuln_filter.lua` | `Div`, `Table`, `Code` | Vulnerability div → compact panel with the title inside it (quick comms have no per-finding heading). Restyles tables; makes inline code breakable. |
| `quick-comm` | `docx_vuln_filter.lua` | `Pandoc` | Compact severity card with the title inside it. No cover page or TOC. |
| `rit-cptc-comm` | `vuln_filter.lua` | `Div`, `Table`, `Code` | Like `quick-comm`, plus Impact and Sophistication inline. Restyles tables; makes inline code breakable. |
| `rit-cptc-comm` | `docx_vuln_filter.lua` | `Pandoc` | Like `quick-comm`'s DOCX card, plus Impact and Sophistication inline. No cover page or TOC. |

## Vulnerability Attributes Read

Each filter reads attributes off the `::: {.vulnerability ...}` div the
preprocessor generates from a finding's frontmatter. Empty or missing
attributes are skipped. `severity` defaults to `Info`, `status` to `Open`.

| Attribute | `rex-default` | `rit-cptc` | `quick-comm` | `rit-cptc-comm` |
|-----------|:---:|:---:|:---:|:---:|
| `severity` | ✓ | ✓ | ✓ | ✓ |
| `cvss` | ✓ | ✓ | ✓ | ✓ |
| `cvssversion` | ✓ | ✓ | | |
| `vector` | ✓ | ✓ | | |
| `status` | ✓ | | ✓ | ✓ |
| `title` | | | ✓ | ✓ |
| `finding` | | ✓ | | |
| `affected` | ✓ | ✓ | ✓ | ✓ |
| `cwe` | ✓ | ✓ | | |
| `owasp` | ✓ | ✓ | | |
| `mitre` | ✓ | ✓ | | |
| `mitigations` | | ✓ | | |
| `business` | ✓ | ✓ | | |
| `material` | | ✓ | | |
| `component` | ✓ | ✓ | | |
| `impact` | | ✓ | | ✓ |
| `sophistication` | | ✓ | | ✓ |

The PDF and DOCX filters of a template read the same attributes.

## Shared Behavior

**`Table` (PDF filters).** Every pandoc table becomes a full-width
`tabularray` `tblr` with a `colorPrimary` header row, zebra-striped body,
and light horizontal rules. Column widths from the Markdown table are kept
as relative `X[n]` coefficients. Cell text gets `\allowbreak` after `.`,
`/` and `:` so long hostnames, IPs, and URLs wrap instead of overflowing.

**`Code` (PDF filters).** Inline code without a language class is escaped
by pandoc, then gets `\allowbreak` after `/`, `.`, `:` and `=` so long
paths don't run off the page. Inline code with a class (syntax-highlighted)
is left alone.

**Colors (DOCX filters).** OpenXML has no named colors, so the DOCX filters
hardcode defaults that match `theme.yaml`, then override them at runtime
from the report's resolved `colors:` metadata (`critical`, `high`, `medium`,
`low`, `info`, plus `primary`/`accent` for the full-report templates).

**Cover page + TOC (`rex-default`, `rit-cptc` DOCX).** Built from `title`,
`subtitle`, `client`, `company`, `engagement_type`, `report_id`, `version`,
`date`, `start_date`/`end_date`, `confidentiality`, `contact_name`,
`contact_email`, and `authors`. Skipped when `title` is empty. `rex-default`'s
TOC field is only added when the document has at least three level-1/2
headings; Word fills it in on "Update Field". `rit-cptc` writes the TOC
entries itself, with `PAGEREF` page numbers Word refreshes on open.

## LaTeX the PDF Filters Expect

The PDF filters emit raw LaTeX that relies on the template's `template.tex`
defining:

- `vulnbox` environment (all except `rit-cptc`, which builds its panel from `tblr` directly)
- `\sevchip{color}{label}` and `\metalabel{label}` macros
- `colorCritical`, `colorHigh`, `colorMedium`, `colorLow`, `colorInfo`, `colorPrimary`, `colorMute` colors
- the `tabularray` package
- `\headingfont` (`rit-cptc-comm` only)

See [LaTeX Template](./latex-template.md) and [Lua Filters](./lua-filters.md)
for writing your own.
