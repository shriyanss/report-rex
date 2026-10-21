"""Tests for the `report-rex validate` CLI command."""

from pathlib import Path

import numpy as np
from click.testing import CliRunner
from PIL import Image

from report_rex_core.cli import main


def _make_png(path: Path, arr: np.ndarray) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    Image.fromarray(arr.astype(np.uint8), "RGB").save(path)
    return path


def _light_background(h=200, w=200) -> np.ndarray:
    return np.full((h, w, 3), 240, dtype=np.uint8)


def _with_annotation(arr: np.ndarray) -> np.ndarray:
    arr = arr.copy()
    arr[50:80, 50:130] = [255, 60, 0]
    return arr


def _write_compliant_report(tmp_path: Path) -> Path:
    (tmp_path / "sections").mkdir()
    (tmp_path / "reports").mkdir()
    (tmp_path / "appendices").mkdir()
    (tmp_path / "assets" / "screenshots").mkdir(parents=True)

    _make_png(
        tmp_path / "assets" / "screenshots" / "evidence.png",
        _with_annotation(_light_background()),
    )

    (tmp_path / "sections" / "exec_summ.md").write_text(
        "---\ntitle: Executive Summary\n---\n\nA real summary of the engagement.\n"
    )
    (tmp_path / "reports" / "vuln-1.md").write_text(
        "---\n"
        "title: SQL Injection\n"
        'cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"\n'
        "timeline:\n"
        '  - date: "2024-01-12T14:20:00+00:00"\n'
        '    description: "Found it"\n'
        "    include: true\n"
        "---\n\n"
        "## Description\n\nA real, specific description of the SQL injection issue found "
        "in the login endpoint.\n\n"
        "## Steps to Reproduce\n\n"
        "1. Navigate to the login page and open the request in an intercepting proxy.\n"
        "2. Replace the username parameter with a payload and forward the request.\n"
        "3. Observe that authentication is bypassed entirely.\n\n"
        "## Evidence\n\n"
        "![A sufficiently long caption describing the evidence shown]"
        "(../assets/screenshots/evidence.png)\n\n"
        "## Recommendation\n\nUse parameterized queries for all database access.\n"
    )
    (tmp_path / "appendices" / "1.md").write_text(
        "---\ntitle: Appendix A\n---\n\nScope list.\n"
    )
    (tmp_path / "index.md").write_text(
        "---\n"
        "title: Test Report\n"
        "client: ACME Corp\n"
        "date: \"2024-01-01\"\n"
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


class TestValidateCommand:
    def test_clean_report_exits_zero(self, tmp_path: Path):
        report_dir = _write_compliant_report(tmp_path)
        runner = CliRunner()
        result = runner.invoke(main, ["validate", "--dir", str(report_dir)])
        assert result.exit_code == 0
        assert "Valid" in result.output

    def test_report_with_errors_exits_one(self, sample_report: Path):
        runner = CliRunner()
        result = runner.invoke(main, ["validate", "--dir", str(sample_report)])
        assert result.exit_code == 1

    def test_output_prints_all_errors_not_first_only(self, sample_report: Path):
        runner = CliRunner()
        result = runner.invoke(main, ["validate", "--dir", str(sample_report)])
        assert "timeline_missing" in result.output
        assert "steps_to_reproduce_missing" in result.output

    def test_warnings_alone_do_not_fail_exit_code(self):
        # A ValidationResult containing only warnings must report ok=True — the actual
        # cli.py exit-code branch (`if n_errors: sys.exit(1)`) relies on this property,
        # exercised end-to-end by test_clean_report_exits_zero above where the compliant
        # fixture produces zero issues at all.
        from report_rex_core.validator import ValidationIssue, ValidationResult

        result = ValidationResult(issues=[
            ValidationIssue(Path("x.md"), "screenshot_text_too_small", "small text", "warning"),
        ])
        assert result.ok
        assert result.errors == []
        assert len(result.warnings) == 1

    def test_missing_index_still_exits_one(self, tmp_path: Path):
        runner = CliRunner()
        result = runner.invoke(main, ["validate", "--dir", str(tmp_path)])
        assert result.exit_code == 1
        assert "index.md not found" in result.output

    def test_parse_index_failure_still_exits_one_with_original_message_style(self, tmp_path: Path):
        (tmp_path / "index.md").write_text(
            "---\ntemplate: rex-default\nstructure:\n  sections:\n    - missing.md\n---\n"
        )
        runner = CliRunner()
        result = runner.invoke(main, ["validate", "--dir", str(tmp_path)])
        assert result.exit_code == 1
        assert "Invalid:" in result.output


class TestBuildCommand:
    """`build` runs the same QA-gate checks as `validate` before rendering — a report
    with validation errors must not silently produce a PDF. `build()` itself is
    monkeypatched to a stub so these tests don't need pandoc/xelatex, matching the
    rest of this test suite's no-pandoc-required unit-test philosophy."""

    def test_build_aborts_on_validation_errors(self, sample_report: Path, monkeypatch, tmp_path: Path):
        called = []
        monkeypatch.setattr("report_rex_core.cli.build", lambda **kw: called.append(kw))
        runner = CliRunner()
        result = runner.invoke(main, [
            "build", "--dir", str(sample_report),
            "--templates-root", str(tmp_path),
            "--output", str(tmp_path / "out.pdf"),
        ])
        assert result.exit_code == 1
        assert "Build aborted" in result.output
        assert called == []  # build() must never run when validation fails

    def test_build_skip_validation_flag_bypasses_gate(self, sample_report: Path, monkeypatch, tmp_path: Path):
        called = []
        monkeypatch.setattr("report_rex_core.cli.build", lambda **kw: called.append(kw))
        runner = CliRunner()
        result = runner.invoke(main, [
            "build", "--dir", str(sample_report),
            "--templates-root", str(tmp_path),
            "--output", str(tmp_path / "out.pdf"),
            "--skip-validation",
        ])
        assert result.exit_code == 0
        assert len(called) == 1

    def test_clean_report_build_proceeds_normally(self, tmp_path: Path, monkeypatch):
        report_dir = tmp_path / "report"
        report_dir.mkdir()
        _write_compliant_report(report_dir)
        called = []
        monkeypatch.setattr("report_rex_core.cli.build", lambda **kw: called.append(kw))
        runner = CliRunner()
        result = runner.invoke(main, [
            "build", "--dir", str(report_dir),
            "--templates-root", str(tmp_path),
            "--output", str(tmp_path / "out.pdf"),
        ])
        assert result.exit_code == 0
        assert len(called) == 1
