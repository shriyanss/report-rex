"""Shared fixtures for Report Rex core tests."""

import pytest
from pathlib import Path


@pytest.fixture
def sample_report(tmp_path: Path) -> Path:
    """Create a minimal valid report directory tree."""
    (tmp_path / "sections").mkdir()
    (tmp_path / "reports").mkdir()
    (tmp_path / "appendices").mkdir()
    (tmp_path / "assets" / "screenshots").mkdir(parents=True)

    (tmp_path / "sections" / "exec_summ.md").write_text(
        "---\ntitle: Executive Summary\n---\n\nSummary text.\n"
    )
    (tmp_path / "reports" / "vuln-1.md").write_text(
        "---\ntitle: SQL Injection\nseverity: Critical\ncvss_score: 9.8\n"
        "status: Open\naffected_urls:\n  - https://example.com/login\n---\n\n"
        "## Description\n\nSQL injection found.\n"
    )
    (tmp_path / "appendices" / "1.md").write_text(
        "---\ntitle: Appendix A\n---\n\nScope list.\n"
    )

    (tmp_path / "index.md").write_text(
        "---\n"
        "title: Test Report\n"
        "client: ACME Corp\n"
        "template: rex-default\n"
        "structure:\n"
        "  sections:\n"
        "    - sections/exec_summ.md\n"
        "  reports:\n"
        "    - reports/vuln-1.md\n"
        "  appendices:\n"
        "    - appendices/1.md\n"
        "---\n"
    )
    return tmp_path
