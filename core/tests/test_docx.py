"""Tests for the DOCX pipeline: chart rasterization and render_docx().

Skipped automatically when xelatex/pdftocairo/pandoc aren't on PATH (CI's
integration-tests job has the full toolchain; the fast unit-test lane doesn't
need it, same policy as the existing PDF renderer has no direct unit tests).
"""

from __future__ import annotations

import re
import shutil
import zipfile
from xml.etree import ElementTree as ET

import pytest

from report_rex_core import chart_raster, charts
from report_rex_core.builder import build

pytestmark = pytest.mark.skipif(
    not (shutil.which("pandoc") and shutil.which("xelatex") and shutil.which("pdftocairo")),
    reason="requires pandoc, xelatex, and pdftocairo on PATH",
)


class TestChartTagging:
    def test_raw_block_is_tagged_and_matchable(self):
        block = charts.render([{"severity": "Critical"}], "pie", "severity")
        m = charts.CHART_BLOCK_RE.search(block)
        assert m is not None
        assert "tikzpicture" in m.group("tikz")

    def test_identical_charts_share_one_id(self):
        # Same findings + args -> same TikZ -> same content hash -> same id,
        # so rasterize() only compiles it once.
        block = charts.render([{"severity": "Critical"}], "pie", "severity")
        ids = {m.group("id") for m in charts.CHART_BLOCK_RE.finditer(block + "\n" + block)}
        assert len(ids) == 1


class TestRasterize:
    def test_rasterize_replaces_chart_block_with_image_ref(self, tmp_path):
        block = charts.render([{"severity": "Critical"}, {"severity": "Low"}], "pie", "severity")
        colors = chart_raster.resolve_colors(None)
        rewritten = chart_raster.rasterize(block, tmp_path, colors)

        assert charts.CHART_BLOCK_RE.search(rewritten) is None
        assert rewritten.startswith('::: {custom-style="Figure"}\n![](charts/chart-')
        svg_files = list((tmp_path / "charts").glob("*.svg"))
        assert len(svg_files) == 1
        # sized in points, as the PDF draws it (bare numbers read as pixels)
        assert re.search(r'<svg[^>]* width="[\d.]+pt"', svg_files[0].read_text())

    def test_non_chart_text_is_left_untouched(self, tmp_path):
        colors = chart_raster.resolve_colors(None)
        text = "Just some ordinary report prose.\n"
        assert chart_raster.rasterize(text, tmp_path, colors) == text


class TestRenderDocx:
    def test_build_produces_valid_docx(self, sample_report, tmp_path):
        templates_root = _repo_root() / "templates"
        output_pdf = tmp_path / "report.pdf"
        output_docx = tmp_path / "report.docx"

        build(
            report_dir=sample_report,
            templates_root=templates_root,
            output_pdf=output_pdf,
            output_docx=output_docx,
        )

        assert output_docx.exists()
        with zipfile.ZipFile(output_docx) as z:
            names = z.namelist()
            assert "word/document.xml" in names
            document_xml = z.read("word/document.xml")
            ET.fromstring(document_xml)  # raises if malformed
            styles_xml = z.read("word/styles.xml")

        # Branded reference.docx applied: rex-default primary color, no
        # Office-only Aptos theme font, and no stray pandoc Date paragraph
        # above the cover.
        assert b"1A1A2E" in styles_xml
        assert b"Aptos" not in styles_xml
        assert b'w:val="Date"' not in document_xml

        assert output_pdf.exists()

    def test_rit_cptc_docx_mirrors_pdf_layout(self, sample_report, tmp_path):
        output_docx = tmp_path / "report.docx"
        build(
            report_dir=sample_report,
            templates_root=_repo_root() / "templates",
            output_pdf=tmp_path / "report.pdf",
            template_override="rit-cptc",
            output_docx=output_docx,
        )
        with zipfile.ZipFile(output_docx) as z:
            document_xml = z.read("word/document.xml").decode()
            footer_xml = z.read("word/footer1.xml").decode()
            ET.fromstring(document_xml)

        assert "<w:sectPr><w:pgSz" in document_xml  # cover closes its own section
        assert "<w:pageBreakBefore/>" in document_xml  # \clearpage honored
        assert "PAGEREF _RexToc1" in document_xml  # TOC with page numbers
        assert "DOCPROPERTY confidentiality" in footer_xml  # classification banner


def _repo_root():
    from pathlib import Path

    here = Path(__file__).resolve()
    for parent in here.parents:
        if (parent / "templates").is_dir():
            return parent
    raise RuntimeError("could not locate repo root from test file")
