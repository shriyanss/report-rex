#!/usr/bin/env python3
"""Structural lint for vuln-templates/*.md.

Unlike `report-rex validate` (which checks a *filled-in* engagement report —
real CVSS data, a real timeline, real screenshots), this checks that the
templates themselves are well-formed: required frontmatter keys are present,
required section headings exist, and each stubbed section actually carries a
`[!REX-STUB]` marker (so nothing was left un-marked and could slip through
`validator.py`'s build-time stub check unnoticed).

Run from the repo root: `python scripts/check_vuln_templates.py`
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

from report_rex_core.parser import parse_frontmatter, split_frontmatter

TEMPLATES_DIR = Path(__file__).resolve().parent.parent / "vuln-templates"

REQUIRED_KEYS = (
    "title", "cvss_vector", "status", "component", "cwe", "owasp", "mitre",
    "mitigations", "impact", "sophistication", "material_impact",
    "business_risk", "affected_urls", "timeline",
)

REQUIRED_SECTIONS = ("Description", "Impact", "Replication", "Remediation")

_HEADING = re.compile(r"^##\s+(.+?)\s*$", re.MULTILINE)
_STUB_MARKER = re.compile(r"^[ \t]*#?[ \t]*>\s*\[!REX-STUB\]\s*$", re.MULTILINE)


def _section_body(body: str, heading: str) -> str | None:
    matches = list(_HEADING.finditer(body))
    for idx, m in enumerate(matches):
        if m.group(1).strip().lower() != heading.lower():
            continue
        start = m.end() + 1
        end = matches[idx + 1].start() if idx + 1 < len(matches) else len(body)
        return body[start:end]
    return None


def check_file(path: Path) -> list[str]:
    errors: list[str] = []
    raw = path.read_text("utf-8")

    try:
        meta = parse_frontmatter(raw)
    except Exception as exc:  # noqa: BLE001 - report any parse failure as a lint error
        return [f"frontmatter failed to parse: {exc}"]

    for key in REQUIRED_KEYS:
        if key not in meta:
            errors.append(f"missing required frontmatter key: {key!r}")

    _, body = split_frontmatter(raw)
    for heading in REQUIRED_SECTIONS:
        section = _section_body(body, heading)
        if section is None:
            errors.append(f"missing required section: '## {heading}'")
            continue
        if not _STUB_MARKER.search(section):
            errors.append(f"'## {heading}' section has no '[!REX-STUB]' marker")

    return errors


def main() -> int:
    if not TEMPLATES_DIR.is_dir():
        print(f"error: {TEMPLATES_DIR} not found", file=sys.stderr)
        return 1

    template_files = sorted(p for p in TEMPLATES_DIR.glob("*.md") if p.name != "README.md")
    if not template_files:
        print(f"error: no .md files found in {TEMPLATES_DIR}", file=sys.stderr)
        return 1

    had_errors = False
    for path in template_files:
        errors = check_file(path)
        if errors:
            had_errors = True
            print(f"{path.relative_to(TEMPLATES_DIR.parent)}:")
            for err in errors:
                print(f"  [error] {err}")

    if had_errors:
        print("\nvuln-templates structural check FAILED", file=sys.stderr)
        return 1

    print(f"vuln-templates structural check OK ({len(template_files)} file(s))")
    return 0


if __name__ == "__main__":
    sys.exit(main())
