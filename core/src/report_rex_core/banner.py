"""ANSI truecolor rendering of the Report Rex logo for the CLI banner."""

from __future__ import annotations

import os
import shutil
import sys
from importlib import resources

from PIL import Image

_RESET = "\x1b[0m"
_DEFAULT_MAX_WIDTH = 44
# Terminal character cells are roughly twice as tall as they are wide.
_CHAR_ASPECT = 2.0


def _logo_path():
    return resources.files("report_rex_core") / "data" / "logo.png"


def _should_render() -> bool:
    if os.environ.get("NO_COLOR") or os.environ.get("CI"):
        return False
    return sys.stdout.isatty()


def render_banner(width: int | None = None) -> str | None:
    """Render the packaged logo as a half-block ANSI truecolor banner.

    Returns None when stdout isn't an interactive terminal (or NO_COLOR/CI is
    set), so piped or scripted invocations stay clean.
    """
    if not _should_render():
        return None

    if width is None:
        columns = shutil.get_terminal_size(fallback=(80, 24)).columns
        width = max(1, min(_DEFAULT_MAX_WIDTH, columns - 4))

    with resources.as_file(_logo_path()) as path:
        image = Image.open(path).convert("RGB")

    height = max(1, round(width * image.height / image.width / _CHAR_ASPECT))
    # Even height so every pair of source rows maps to one half-block line.
    if height % 2:
        height += 1
    image = image.resize((width, height))

    pixels = image.load()
    lines = []
    for y in range(0, height, 2):
        chars = []
        for x in range(width):
            top = pixels[x, y]
            bottom = pixels[x, y + 1]
            chars.append(
                f"\x1b[38;2;{top[0]};{top[1]};{top[2]}m"
                f"\x1b[48;2;{bottom[0]};{bottom[1]};{bottom[2]}m▀"
            )
        lines.append("".join(chars) + _RESET)

    return "\n".join(lines)
