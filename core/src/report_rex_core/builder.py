"""High-level build orchestration."""

from __future__ import annotations

import tempfile
from pathlib import Path

from .parser import parse_index
from .preprocessor import assemble
from .renderer import render, render_docx


def build(
    report_dir: Path,
    templates_root: Path,
    output_pdf: Path,
    template_override: str | None = None,
    draft: bool = False,
    output_docx: Path | None = None,
    password: str | None = None,
) -> None:
    """Build a PDF (and optionally a DOCX) from a report directory.

    Args:
        report_dir: Path to the directory containing index.md.
        templates_root: Root directory that contains template subdirectories.
        output_pdf: Destination path for the generated PDF.
        template_override: Template name to use instead of what index.md specifies.
        draft: If True, add a DRAFT watermark.
        output_docx: If set, also generate a DOCX clone at this path, from the
            same assembled report (see renderer.render_docx).
        password: If set, encrypt output_pdf with this password (see
            renderer.render). The DOCX clone, if any, is not encrypted.
    """
    index_path = report_dir / "index.md"
    if not index_path.exists():
        raise FileNotFoundError(f"index.md not found in {report_dir}")

    structure = parse_index(index_path)
    template_name = template_override or structure.template

    template_dir = templates_root / template_name
    if not template_dir.exists():
        raise FileNotFoundError(
            f"Template '{template_name}' not found in {templates_root}"
        )

    with tempfile.TemporaryDirectory(prefix="report-rex-") as tmp:
        tmp_path = Path(tmp)
        combined_md, metadata_yaml = assemble(structure, tmp_path)
        render(combined_md, metadata_yaml, template_dir, output_pdf, draft=draft, password=password)
        if output_docx is not None:
            render_docx(
                combined_md, metadata_yaml, template_dir, output_docx,
                report_colors=structure.metadata.get("colors"),
            )


def scaffold(target_dir: Path, template_name: str = "rit-cptc") -> None:
    """Create a new report directory with starter files."""
    target_dir.mkdir(parents=True, exist_ok=True)
    (target_dir / "sections").mkdir(exist_ok=True)
    (target_dir / "reports").mkdir(exist_ok=True)
    (target_dir / "appendices").mkdir(exist_ok=True)
    (target_dir / "assets" / "screenshots").mkdir(parents=True, exist_ok=True)

    _write_if_missing(target_dir / "index.md", _STARTER_INDEX.format(template=template_name))
    _write_if_missing(target_dir / "sections" / "exec_summ.md", _STARTER_EXEC_SUMM)
    _write_if_missing(target_dir / "sections" / "methodology.md", _STARTER_METHODOLOGY)
    _write_if_missing(target_dir / "reports" / "vuln-1.md", _STARTER_VULN)
    _write_if_missing(target_dir / "appendices" / "1.md", _STARTER_APPENDIX)


def _write_if_missing(path: Path, content: str) -> None:
    if not path.exists():
        path.write_text(content, encoding="utf-8")


_STARTER_INDEX = """\
---
title: "Penetration Test Report"
subtitle: "Final Report"
client: "Client Name"
company: "Your Security Firm"
engagement_type: "Web Application Penetration Test"
report_id: "RR-2024-0001"
date: "2024-01-01"
start_date: "2024-01-01"
end_date: "2024-01-05"
version: "1.0"
confidentiality: "Confidential"
template: "{template}"

contact_name: "Your Name"
contact_email: "you@example.com"
distribution:
  - "Client Security Team"

authors:
  - name: "Your Name"
    role: "Lead Penetration Tester"

# Branding (optional) — uncomment and point at files under assets/ to add logos.
# company_logo: "assets/company-logo-dark.png"    # dark logo, for the light page header
# company_logo_light: "assets/company-logo.png"   # light logo, for the dark cover band
# client_logo: "assets/client-logo.png"           # client logo, shown on the cover

# Timeline (optional) — a chronological log of engagement events. Entries can
# also be added to the frontmatter of any section/report/appendix file; all
# entries from every file are merged, sorted by date, and rendered as a
# Timeline appendix. Omit `include` (or set it true) to show an entry;
# `include: false` keeps it out of the rendered report.
# timeline:
#   - date: "2024-01-10T09:00:00+00:00"
#     description: "Kicked off testing"
#     include: true

structure:
  sections:
    - sections/exec_summ.md
    - sections/methodology.md
  reports:
    - reports/vuln-1.md
  appendices:
    - appendices/1.md
---
"""

# Scaffold sentinel strings — named here (rather than embedded only in the f-strings below) so
# validator.py can import them as the single source of truth for leftover-placeholder-text detection.
PLACEHOLDER_VULN_TITLE = "Example Vulnerability"
PLACEHOLDER_DESCRIPTION = "Describe the vulnerability here."
PLACEHOLDER_IMPACT = "Describe the business and technical impact."
PLACEHOLDER_RECOMMENDATION = "Describe remediation steps. Include code examples where helpful."
PLACEHOLDER_METHODOLOGY = "Describe the testing methodology, scope, and approach used during the engagement."
PLACEHOLDER_CONTACT_NAME = "Your Name"
PLACEHOLDER_CONTACT_EMAIL = "you@example.com"
# Substring match catches both "1970-01-01" (date/start_date/end_date/retest_date)
# and "1970-01-01T00:00:00+00:00" (timeline entries) — the Unix epoch, a date no
# real engagement can have, used as the "noticeable" scaffold/example date.
PLACEHOLDER_EPOCH_DATE = "1970-01-01"
# Umbrella sentinel embedded throughout the rewritten reporting/example content
# (see reporting/CLAUDE.md) so an unedited copy of that example is caught here too.
PLACEHOLDER_LOREM_IPSUM = "Lorem Ipsum"
# Template defaults in the scaffolds' index.md that render on the cover / Document
# Control page (#74): the example's report_id and ACME client logo, and
# `report-rex new`'s report_id.
PLACEHOLDER_REPORT_IDS = ("RR-2024-0142", "RR-2024-0001")
PLACEHOLDER_CLIENT_LOGO = "acme-logo.png"

_STARTER_EXEC_SUMM = """\
---
title: "Executive Summary"
---

Provide a high-level overview of the engagement, key findings, and overall risk posture.
"""

_STARTER_METHODOLOGY = f"""\
---
title: "Methodology"
---

{PLACEHOLDER_METHODOLOGY}
"""

_STARTER_VULN = f"""\
---
title: "{PLACEHOLDER_VULN_TITLE}"
# Severity and CVSS score are computed automatically from the vector below —
# just paste the CVSS v3.x vector string. (Omit it and set `severity:` manually
# for a v4.0 vector or a non-scored finding.)
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"
status: "Open"                 # Open | In Progress | Remediated
component: "Example Component"
# Optional reference tags — OWASP/MITRE codes are auto-resolved to their names
# and feed the breakdown charts.
cwe: "CWE-79"
owasp: "A03:2021"
mitre: "T1190"
business_risk:
  - Operational
affected_urls:
  - "https://example.com/endpoint"
---

## Description

{PLACEHOLDER_DESCRIPTION}

## Impact

{PLACEHOLDER_IMPACT}

## Replication

1. Step one
2. Step two

<!-- Add a screenshot once you have one:
![Screenshot](../assets/screenshots/example.png){{width=90%}} -->

## Remediation

{PLACEHOLDER_RECOMMENDATION}
"""

_STARTER_APPENDIX = """\
---
title: "Appendix A — Scope"
---

List the scope of the engagement, including IP ranges and domains tested.
"""
