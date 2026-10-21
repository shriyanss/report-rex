---
sidebar_position: 7
title: Output Formats
---

# Output Formats

Report Rex builds a PDF by default. Passing `--docx` (both the Python
`report-rex build` and the Go `rex report build`/`rex watch`) also generates a
`.docx` clone next to it — same basename, `.docx` extension:

```bash
report-rex build --dir reporting/acme-corp --output acme-report.pdf --docx
# → acme-report.pdf and acme-report.docx

rex report build --dir ./acme-engagement --docx
# → report.pdf and report.docx
```

The DRAFT watermark (default unless `--final` is passed) only applies to the
PDF — the DOCX clone is never renamed with a `-draft` suffix, since it isn't
watermarked either.

## How it's built

The DOCX isn't a PDF-to-Word conversion — an earlier attempt at that lost
most of the report's formatting (custom fonts, severity boxes, headers).
Instead, the same assembled
`combined.md`/`metadata.yaml` the PDF pipeline produces is fed to pandoc a
second time with `--to=docx`, using a template-specific
`filters/docx_vuln_filter.lua` — the DOCX-side counterpart to
`filters/vuln_filter.lua` — that emits raw WordprocessingML (OpenXML)
wherever the PDF path would reach for a `tcolorbox`: the branded cover
page, the table of contents field, and each finding's severity panel.
Everything else (headings, paragraphs, lists, plain tables, embedded
images) goes through pandoc's ordinary DOCX writer, styled by the
template's `reference.docx` (passed via `--reference-doc`): brand-colored
headings, bordered tables with a shaded header row, shaded inline code, and
fonts every OS ships (Arial/Courier New). After changing a template's
`theme.yaml` colors, regenerate it:

```bash
python3 core/scripts/make_reference_docx.py
```

Charts (`:::findings_graph:...:::` and friends, including `:::network_diagram:::`) are PDF-only raw
LaTeX/TikZ — DOCX has no LaTeX renderer, and pandoc's DOCX writer silently
drops raw LaTeX blocks it can't use. A chart-rasterization step compiles
each chart standalone via `xelatex` and converts it to a vector SVG via
`pdftocairo` before the DOCX render, so charts appear in the `.docx` as
ordinary embedded vector images, drawn exactly like the PDF's chart.

## Template support

All four bundled templates (`rex-default`, `rit-cptc`, `rit-cptc-comm`,
`quick-comm`) support `--docx`. `rex-default` and `rit-cptc` get a branded
cover page and a table of contents; `rit-cptc-comm` and `quick-comm` don't,
matching their one-pager PDF layout.

`rit-cptc`'s DOCX mirrors its PDF page for page: same A4 geometry, cover,
Document Control page, numbered headings, page breaks, summary table, finding
panels, and the corner/classification-banner page art, with the classification
and page number as Word fields. Lines break at the same words: the template's
LaTeX sticks to typesetting rules Word can reproduce (whole half-point sizes,
no kerning, ligatures, hyphenation or shrinking spaces, rigid vertical
spacing), and the DOCX filter pre-wraps code blocks and draws the TOC's dotted
leaders on LaTeX's grid. The TOC's page numbers are `PAGEREF` fields that Word
refreshes when it opens the file (it asks to update fields).

## Prerequisites

- **Docker mode** (default): nothing extra — the runtime image already
  bundles `pandoc`, `xelatex`, and `pdftocairo`.
- **`--no-docker`**: in addition to the existing `pandoc`/`xelatex`
  requirement, `--docx` needs `pdftocairo` (part of `poppler-utils`/`poppler`)
  on `PATH` for chart rasterization.

## Known limitations

- Heading and table colors come from the template's `theme.yaml`, baked into
  its `reference.docx` — a `colors:` override in `index.md` recolors the
  cover and finding panels but not headings or plain tables. `rit-cptc`
  embeds its Liberation Sans/Mono fonts in the `.docx` (about 1.5MB), so it
  renders with the PDF's exact glyphs; the other templates use Arial/Courier
  New.
- Positions can differ by a point or two between the PDF and a word
  processor's layout of the `.docx` (verified against LibreOffice).
- Freeform raw LaTeX typed directly into report content (outside of chart
  directives and vulnerability frontmatter) has no DOCX equivalent and is
  dropped, same as pandoc's default behavior for any output format that
  isn't LaTeX.

## Verifying a DOCX build

To confirm a `.docx` build matches its PDF sibling, convert it back to PDF
with LibreOffice and compare page-by-page:

```bash
soffice --headless --convert-to pdf acme-report.docx
```
