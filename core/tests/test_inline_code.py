"""Inline code gets break points so long paths wrap instead of overflowing (#64)."""

from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

import pytest

TEMPLATES = Path(__file__).resolve().parents[2] / "templates"


@pytest.mark.skipif(shutil.which("pandoc") is None, reason="pandoc not installed")
@pytest.mark.parametrize("name", ["rit-cptc", "rit-cptc-comm", "rex-default", "quick-comm"])
def test_inline_code_allows_breaks(name):
    md = 'Ref: `<script src="/static/img/uploads/15"></script>` and `x`{.python}.\n'
    out = subprocess.run(
        ["pandoc", "-t", "latex", f"--lua-filter={TEMPLATES / name / 'filters' / 'vuln_filter.lua'}"],
        input=md, capture_output=True, text=True, check=True,
    ).stdout
    assert "static/\\allowbreak{}img/" in out
    assert "\\VERB|" in out  # highlighted inline code left to pandoc


@pytest.mark.parametrize("name", ["rit-cptc", "rit-cptc-comm", "rex-default", "quick-comm"])
def test_code_blocks_break_long_tokens(name):
    # #69: highlighted tokens (\NormalTok{...}) and plain/indented `verbatim`
    # blocks must both be breakable, or long tokens run past the margin.
    tex = (TEMPLATES / name / "template.tex").read_text()
    assert "breaknonspaceingroup=true" in tex
    assert "\\RecustomVerbatimEnvironment{verbatim}{Verbatim}" in tex
