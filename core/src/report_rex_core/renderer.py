"""Invoke pandoc to convert the assembled markdown to PDF via XeLaTeX or DOCX."""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from . import chart_raster

# Input format for every pandoc run. Pasted text often has no blank line before a
# list or heading, which plain pandoc markdown requires, so the list/heading ran
# into the paragraph above (#71); these extensions accept them.
PANDOC_FROM = (
    "markdown+raw_tex+fenced_divs+raw_attribute"
    "+lists_without_preceding_blankline-blank_before_header"
)


def render(
    combined_md: Path,
    metadata_yaml: Path,
    template_dir: Path,
    output_pdf: Path,
    draft: bool = False,
    password: str | None = None,
) -> None:
    """Run pandoc to produce a PDF from the assembled report."""
    template_tex = template_dir / "template.tex"
    if not template_tex.exists():
        raise FileNotFoundError(f"Template not found: {template_tex}")

    lua_filter = template_dir / "filters" / "vuln_filter.lua"

    cmd = [
        "pandoc",
        str(combined_md),
        f"--metadata-file={metadata_yaml}",
        f"--template={template_tex}",
        "--pdf-engine=xelatex",
        f"--resource-path={combined_md.parent}",
        f"--output={output_pdf}",
        f"--from={PANDOC_FROM}",
        "--highlight-style=tango",
        "--standalone",
        "--table-of-contents",
        "--toc-depth=2",
    ]

    if lua_filter.exists():
        cmd.append(f"--lua-filter={lua_filter}")

    if draft:
        cmd.append("--metadata=draft:true")

    theme_yaml = template_dir / "theme.yaml"
    if theme_yaml.exists():
        cmd.append(f"--metadata-file={theme_yaml}")

    result = subprocess.run(cmd, capture_output=True, text=True)
    # Surface pandoc's warnings even on success — e.g. a missing image, which
    # it silently replaces with its alt text (#70).
    if result.stderr.strip():
        print(result.stderr, file=sys.stderr, end="")
    if result.returncode != 0:
        raise RuntimeError(f"pandoc failed (exit {result.returncode})")

    if password:
        _encrypt_pdf(output_pdf, password)


def _encrypt_pdf(output_pdf: Path, password: str) -> None:
    """Password-protect output_pdf in place (AES-256, via qpdf).

    Stage the freshly-rendered file aside, encrypt it back to the original
    name, then drop the unencrypted temp copy. The password is passed as a
    plain argv element (no shell involved), so special characters are safe.
    """
    if shutil.which("qpdf") is None:
        raise RuntimeError("--password requires a qpdf install (not found in PATH)")

    tmp = output_pdf.with_suffix(output_pdf.suffix + ".tmp")
    output_pdf.rename(tmp)
    result = subprocess.run(
        ["qpdf", "--encrypt", password, password, "256", "--", str(tmp), str(output_pdf)],
        capture_output=True, text=True,
    )
    if result.returncode != 0:
        tmp.rename(output_pdf)  # restore the unencrypted PDF so a failed encrypt doesn't lose the build
        print(result.stderr, file=sys.stderr)
        raise RuntimeError(f"qpdf encryption failed (exit {result.returncode})")
    tmp.unlink(missing_ok=True)


def render_docx(
    combined_md: Path,
    metadata_yaml: Path,
    template_dir: Path,
    output_docx: Path,
    report_colors: dict | None = None,
) -> None:
    """Run pandoc to produce a DOCX from the assembled report.

    Reuses pandoc's markdown parser and its generic docx writer (paragraphs,
    headings, lists, tables, images) for everything ordinary, but a
    template-specific `docx_vuln_filter.lua` emits raw OpenXML for the pieces
    that need brand-specific structure pandoc has no notion of (vulnerability
    boxes, cover page, TOC field) — the DOCX-side counterpart to how
    `vuln_filter.lua` emits raw LaTeX for the PDF path.

    Charts are PDF-only raw LaTeX/TikZ (see charts.py) — pandoc's docx writer
    silently drops raw LaTeX it can't use, so chart_raster.py rasterizes them
    to PNGs first, in a throwaway copy of combined.md (the original, and the
    already-rendered PDF, are untouched).
    """
    colors = chart_raster.resolve_colors(report_colors, template_dir)

    with tempfile.TemporaryDirectory(prefix="report-rex-docx-") as tmp:
        tmp_path = Path(tmp)
        rewritten_md = chart_raster.rasterize(
            combined_md.read_text("utf-8"), tmp_path, colors
        )
        docx_md = tmp_path / "combined.md"
        docx_md.write_text(rewritten_md, encoding="utf-8")

        cmd = [
            "pandoc",
            str(docx_md),
            f"--metadata-file={metadata_yaml}",
            "--to=docx",
            f"--resource-path={combined_md.parent}{os.pathsep}{tmp_path}",
            f"--output={output_docx}",
            f"--from={PANDOC_FROM}",
            "--highlight-style=tango",
            "--standalone",
        ]

        docx_filter = template_dir / "filters" / "docx_vuln_filter.lua"
        if docx_filter.exists():
            cmd.append(f"--lua-filter={docx_filter}")

        reference_docx = template_dir / "reference.docx"
        if reference_docx.exists():
            cmd.append(f"--reference-doc={reference_docx}")

        theme_yaml = template_dir / "theme.yaml"
        if theme_yaml.exists():
            cmd.append(f"--metadata-file={theme_yaml}")

        result = subprocess.run(cmd, capture_output=True, text=True)
        # Surface pandoc's warnings even on success — e.g. a missing image, which
        # it silently replaces with its alt text (#70).
        if result.stderr.strip():
            print(result.stderr, file=sys.stderr, end="")
        if result.returncode != 0:
            raise RuntimeError(f"pandoc (docx) failed (exit {result.returncode})")
