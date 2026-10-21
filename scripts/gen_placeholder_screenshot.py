#!/usr/bin/env python3
"""Dev-time generator for the reporting/example placeholder screenshot.

The Go/Python validators flag this exact image (by SHA-256) as a leftover
demo screenshot (see checkPlaceholderScreenshot / _check_placeholder_screenshot).
If you regenerate this image, paste the printed hash into both:
  - cli/internal/report/placeholders.go: PlaceholderScreenshotSHA256
  - core/src/report_rex_core/validator.py: _PLACEHOLDER_SCREENSHOT_SHA256

Run: .venv/bin/python scripts/gen_placeholder_screenshot.py
"""

import hashlib
from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

SCREENSHOTS = Path(__file__).resolve().parent.parent / "reporting/example/assets/screenshots"
NAMES = [
    "sqli-auth-bypass.png",
    "idor-invoice.png",
    "missing-headers.png",
    "xss-exfil.png",
    "xss-payload.png",
]


def main() -> None:
    img = Image.new("RGB", (1200, 800), (245, 245, 245))  # light bg -> passes dark-mode check
    d = ImageDraw.Draw(img)
    d.rectangle([100, 100, 560, 260], outline=(255, 120, 0), width=14)  # saturated orange -> passes annotation check
    d.text((100, 320), "LOREM IPSUM PLACEHOLDER", fill=(20, 20, 20), font=ImageFont.load_default(size=72))
    d.text((100, 420), "Replace this screenshot before building.", fill=(20, 20, 20), font=ImageFont.load_default(size=48))

    digest = None
    for name in NAMES:
        path = SCREENSHOTS / name
        img.save(path)
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
    print("sha256:", digest)


if __name__ == "__main__":
    main()
