"""Tests for renderer._encrypt_pdf's qpdf post-processing step.

Unlike render()/render_docx() (which need a real pandoc toolchain and are
untested in the fast unit-test lane, per test_docx.py's note), _encrypt_pdf's
qpdf invocation is mocked here so these run without qpdf installed.
"""

from __future__ import annotations

import subprocess

import pytest

from report_rex_core.renderer import _encrypt_pdf, render


def test_encrypt_pdf_missing_qpdf_raises(tmp_path, monkeypatch):
    monkeypatch.setattr("shutil.which", lambda name: None)
    pdf = tmp_path / "report.pdf"
    pdf.write_bytes(b"%PDF-1.4 fake")

    with pytest.raises(RuntimeError, match="qpdf"):
        _encrypt_pdf(pdf, "hunter2")

    assert pdf.exists()  # left untouched when qpdf is missing


def test_encrypt_pdf_invokes_qpdf_with_password_and_restores_name(tmp_path, monkeypatch):
    monkeypatch.setattr("shutil.which", lambda name: "/usr/bin/qpdf")
    pdf = tmp_path / "report.pdf"
    pdf.write_bytes(b"%PDF-1.4 fake")

    calls = []

    def fake_run(cmd, **kwargs):
        calls.append(cmd)
        # Simulate qpdf writing the encrypted output at its target path.
        out_path = cmd[-1]
        with open(out_path, "wb") as f:
            f.write(b"%PDF-1.4 encrypted")
        return subprocess.CompletedProcess(cmd, 0, stdout="", stderr="")

    monkeypatch.setattr("subprocess.run", fake_run)

    _encrypt_pdf(pdf, "hunter2")

    assert len(calls) == 1
    cmd = calls[0]
    assert cmd[0] == "qpdf"
    assert "hunter2" in cmd
    assert cmd[-1] == str(pdf)
    assert pdf.exists()
    assert not pdf.with_suffix(pdf.suffix + ".tmp").exists()  # temp file cleaned up


def test_encrypt_pdf_raises_on_qpdf_failure(tmp_path, monkeypatch):
    monkeypatch.setattr("shutil.which", lambda name: "/usr/bin/qpdf")
    pdf = tmp_path / "report.pdf"
    pdf.write_bytes(b"%PDF-1.4 fake")

    monkeypatch.setattr(
        "subprocess.run",
        lambda cmd, **kwargs: subprocess.CompletedProcess(cmd, 1, stdout="", stderr="bad password"),
    )

    with pytest.raises(RuntimeError, match="qpdf encryption failed"):
        _encrypt_pdf(pdf, "hunter2")

    assert pdf.exists()  # unencrypted PDF restored, not lost, when qpdf fails
    assert not pdf.with_suffix(pdf.suffix + ".tmp").exists()


def test_render_prints_pandoc_warnings_on_success(tmp_path, monkeypatch, capsys):
    # #70: a missing image is only a pandoc warning; it must reach the user.
    (tmp_path / "template.tex").write_text("")
    warning = "[WARNING] Could not fetch resource shot.png: replacing image with description\n"
    monkeypatch.setattr(
        "subprocess.run",
        lambda cmd, **kw: subprocess.CompletedProcess(cmd, 0, stdout="", stderr=warning),
    )
    render(tmp_path / "c.md", tmp_path / "m.yaml", tmp_path, tmp_path / "out.pdf")
    assert "Could not fetch resource shot.png" in capsys.readouterr().err


def test_render_accepts_lists_and_headings_without_blank_line(tmp_path, monkeypatch):
    # #71: pasted text often has no blank line before a list or heading.
    (tmp_path / "template.tex").write_text("")
    calls = []
    monkeypatch.setattr(
        "subprocess.run",
        lambda cmd, **kw: calls.append(cmd) or subprocess.CompletedProcess(cmd, 0, "", ""),
    )
    render(tmp_path / "c.md", tmp_path / "m.yaml", tmp_path, tmp_path / "out.pdf")
    from_arg = next(a for a in calls[0] if a.startswith("--from="))
    assert "+lists_without_preceding_blankline" in from_arg
    assert "-blank_before_header" in from_arg
