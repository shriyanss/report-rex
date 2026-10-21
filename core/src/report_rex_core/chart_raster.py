"""Convert chart TikZ blocks (tagged by charts.py) into SVGs for the DOCX pipeline.

DOCX has no LaTeX/TikZ renderer, so pandoc's docx writer silently drops the raw
``{=latex}`` blocks charts.py emits for the PDF pipeline. This module locates
those tagged blocks in an assembled combined.md, compiles each one standalone
via XeLaTeX, converts it to SVG via ``pdftocairo``, and swaps the block for a
plain Markdown image reference — the rest of the DOCX pipeline then treats a
chart exactly like any other embedded image.
"""

from __future__ import annotations

import re
import subprocess
import sys
import tempfile
from pathlib import Path

import yaml

from .charts import CHART_BLOCK_RE

# Fallback categorical chart palette (rex-default's chartc1..10). Charts don't
# have the LaTeX template's preamble when rasterized standalone, so
# resolve_colors() reads the template's own chartcN \definecolor lines.
_CHART_PALETTE_HEX = [
    "0F3460", "16697A", "489FB5", "82C0CC", "E9724C",
    "C05299", "6D597A", "B56576", "937B63", "355070",
]

# Fallback severity/brand colors (rex-default's theme.yaml). resolve_colors()
# overrides these with the template's theme.yaml, then the report's colors.
_DEFAULT_COLORS = {
    "primary": "1A1A2E", "accent": "0F3460", "critical": "B71C1C",
    "high": "E65100", "medium": "F57F17", "low": "1565C0", "info": "37474F",
}

_CHARTC_RE = re.compile(r"\\definecolor\{(chartc\d+)\}\{HTML\}\{([0-9A-Fa-f]{6})\}")
# template.tex lines that change how a chart typesets (body font, table row
# height, TikZ libraries); copied into the standalone preamble so the PNG
# matches the PDF. Lines with a pandoc $variable$ are skipped.
_PREAMBLE_RE = re.compile(
    r"^\\(?:setmainfont|renewcommand\{\\arraystretch\}|SetTblrInner|usetikzlibrary)\{[^$\n]*$", re.M
)

# 11pt and the body font match every bundled template.tex's article class and
# default \setmainfont.
_TEX_TEMPLATE = """\\documentclass[11pt,border=4pt]{{standalone}}
\\usepackage{{fontspec}}
\\setmainfont{{TeX Gyre Heros}}[Scale=0.92]
\\usepackage{{tikz}}
\\usepackage{{pgf-pie}}
\\usepackage{{tabularray}}
\\usepackage{{xcolor}}
\\usepackage{{fontawesome5}}
{colors}
{preamble}
\\begin{{document}}
{body}
\\end{{document}}
"""


def resolve_colors(report_colors: dict | None, template_dir: Path | None = None) -> dict:
    """Resolve chart colors as the PDF sees them: the template's chartcN palette
    and theme.yaml colors, then the report's index.md `colors:` overrides.

    The template's chart-relevant preamble lines ride along under "preamble".
    """
    colors = dict(_DEFAULT_COLORS, preamble="")
    colors.update({f"chartc{i}": hexval for i, hexval in enumerate(_CHART_PALETTE_HEX, 1)})
    if template_dir is not None:
        tex, theme = template_dir / "template.tex", template_dir / "theme.yaml"
        if tex.exists():
            colors.update(_CHARTC_RE.findall(tex.read_text("utf-8")))
            colors["preamble"] = "\n".join(_PREAMBLE_RE.findall(tex.read_text("utf-8")))
        if theme.exists():
            theme_colors = (yaml.safe_load(theme.read_text("utf-8")) or {}).get("colors") or {}
            colors.update({k: str(v) for k, v in theme_colors.items() if v})
    if report_colors:
        colors.update({k: v for k, v in report_colors.items() if v})
    return colors


def _color_defs(colors: dict) -> str:
    named = (
        ("colorPrimary", "primary"), ("colorAccent", "accent"),
        ("colorCritical", "critical"), ("colorHigh", "high"),
        ("colorMedium", "medium"), ("colorLow", "low"), ("colorInfo", "info"),
    )
    lines = [f"\\definecolor{{{name}}}{{HTML}}{{{colors[key]}}}" for name, key in named]
    lines += [
        f"\\definecolor{{chartc{i}}}{{HTML}}{{{colors[f'chartc{i}']}}}"
        for i in range(1, len(_CHART_PALETTE_HEX) + 1)
    ]
    return "\n".join(lines)


def _strip_center(tikz: str) -> str:
    """Drop charts.py's \\begin{center}/\\end{center} wrapper.

    standalone's auto-crop only recognizes specific top-level environments
    (tikzpicture, tblr, ...); wrapped in \\center, it can't find the content's
    bounding box and falls back to a full page. Centering is meaningless for a
    standalone cropped image anyway (the DOCX paragraph centers the picture).
    """
    t = tikz.strip()
    if t.startswith("\\begin{center}") and t.endswith("\\end{center}"):
        t = t[len("\\begin{center}"):-len("\\end{center}")].strip()
    return t


def _compile_one(tikz: str, colors: dict, work_dir: Path) -> Path:
    """Compile one chart's TikZ to a cropped PNG in work_dir; return the PNG path."""
    tex_path = work_dir / "chart.tex"
    tex_path.write_text(
        _TEX_TEMPLATE.format(
            colors=_color_defs(colors), preamble=colors.get("preamble", ""), body=_strip_center(tikz),
        ),
        encoding="utf-8",
    )

    result = subprocess.run(
        ["xelatex", "-interaction=nonstopmode", "-halt-on-error", tex_path.name],
        cwd=work_dir, capture_output=True, text=True,
    )
    pdf_path = work_dir / "chart.pdf"
    if result.returncode != 0 or not pdf_path.exists():
        print(result.stdout, file=sys.stderr)
        raise RuntimeError(f"xelatex failed rasterizing a chart for DOCX (exit {result.returncode})")

    # Vector, not raster: pdftocairo turns the chart (text as glyph outlines)
    # into an SVG that draws exactly like the PDF's chart.
    svg_path = work_dir / "chart.svg"
    result = subprocess.run(["pdftocairo", "-svg", str(pdf_path), str(svg_path)], capture_output=True, text=True)
    if result.returncode != 0 or not svg_path.exists():
        print(result.stderr, file=sys.stderr)
        raise RuntimeError(f"pdftocairo failed converting a chart for DOCX (exit {result.returncode})")
    return _svg_in_points(svg_path)


def _svg_in_points(svg_path: Path) -> Path:
    """pdftocairo sizes the SVG in PostScript points but writes bare numbers,
    which pandoc reads as 96dpi pixels (a 75% chart); label them as pt."""
    svg = svg_path.read_text("utf-8")
    svg = re.sub(r'(<svg\b[^>]*?\b(?:width|height)=")([\d.]+)(")', r"\1\2pt\3", svg, count=1)
    svg = re.sub(r'(<svg\b[^>]*?\b(?:width|height)=")([\d.]+)(")', r"\1\2pt\3", svg, count=1)
    svg_path.write_text(svg, "utf-8")
    return svg_path


def rasterize(combined_md: str, out_dir: Path, colors: dict) -> str:
    """Replace tagged chart blocks in combined_md with rasterized PNG image refs.

    PNGs are written under out_dir/charts/. Returns the rewritten Markdown;
    combined_md itself is left untouched (callers pass a copy of its text).
    """
    matches = list(CHART_BLOCK_RE.finditer(combined_md))
    if not matches:
        return combined_md

    charts_dir = out_dir / "charts"
    charts_dir.mkdir(parents=True, exist_ok=True)

    replacements: dict[str, str] = {}
    for m in matches:
        chart_id = m.group("id")
        if chart_id in replacements:
            continue
        dest = charts_dir / f"chart-{chart_id}.svg"
        if not dest.exists():
            with tempfile.TemporaryDirectory(prefix="rex-chart-") as tmp:
                png = _compile_one(m.group("tikz"), colors, Path(tmp))
                dest.write_bytes(png.read_bytes())
        # Uncaptioned (an alt text would become a "Chart" caption), centered
        # via the reference.docx "Figure" paragraph style like the PDF's center.
        # .tblr marks a table chart (templates may space those differently).
        cls = "{.tblr}" if "\\begin{tblr}" in m.group("tikz") else ""
        replacements[chart_id] = f'::: {{custom-style="Figure"}}\n![]({dest.relative_to(out_dir)}){cls}\n:::'

    return CHART_BLOCK_RE.sub(lambda m: replacements[m.group("id")], combined_md)
