"""Tests for the CLI logo banner rendering."""

from report_rex_core import banner


def test_render_banner_none_when_not_a_tty(monkeypatch):
    monkeypatch.setattr(banner.sys.stdout, "isatty", lambda: False)
    assert banner.render_banner() is None


def test_render_banner_none_when_no_color(monkeypatch):
    monkeypatch.setattr(banner.sys.stdout, "isatty", lambda: True)
    monkeypatch.setenv("NO_COLOR", "1")
    assert banner.render_banner() is None


def test_render_banner_renders_ansi_art(monkeypatch):
    monkeypatch.setattr(banner.sys.stdout, "isatty", lambda: True)
    monkeypatch.delenv("NO_COLOR", raising=False)
    monkeypatch.delenv("CI", raising=False)

    result = banner.render_banner(width=10)

    assert result is not None
    assert "\x1b[38;2;" in result
    assert "\x1b[48;2;" in result
    assert result.endswith("\x1b[0m")
