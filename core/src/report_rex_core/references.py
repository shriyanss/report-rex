"""Lookup helpers for OWASP / MITRE reference data.

The bundled ``data/references.yaml`` maps category and technique codes to their
official names (fetched from owasp.org and attack.mitre.org). These names are
used to label findings and the auto-generated breakdown charts so testers only
need to write a short code (e.g. ``owasp: A03:2021``) in a finding.
"""

from __future__ import annotations

import functools
import re
from pathlib import Path

import yaml

_DATA_FILE = Path(__file__).parent / "data" / "references.yaml"


@functools.lru_cache(maxsize=1)
def _data() -> dict:
    if not _DATA_FILE.exists():
        return {}
    return yaml.safe_load(_DATA_FILE.read_text("utf-8")) or {}


def _normalise_code(code: str) -> str:
    """Uppercase and strip a code, dropping any trailing ``:year`` for lookup."""
    return code.strip().upper()


def owasp_name(code: str) -> str | None:
    """Resolve an OWASP code (web/API/LLM/mobile) to its official name.

    Accepts forms like ``A03``, ``A03:2021``, ``API1``, ``LLM01``, ``M1``.
    """
    raw = _normalise_code(code)
    bare = raw.split(":")[0]
    data = _data()
    for table in ("owasp_web_2021", "owasp_api_2023", "owasp_llm_2025", "owasp_mobile_2024"):
        mapping = {k.upper(): v for k, v in (data.get(table) or {}).items()}
        if bare in mapping:
            return mapping[bare]
    return None


def mitre_name(code: str) -> str | None:
    """Resolve a MITRE ATT&CK technique (Txxxx) or tactic (TAxxxx) ID to its name."""
    raw = _normalise_code(code)
    # sub-technique T1059.001 -> fall back to parent technique name if unknown
    data = _data()
    techniques = {k.upper(): v for k, v in (data.get("mitre_techniques") or {}).items()}
    tactics = {k.upper(): v for k, v in (data.get("mitre_tactics") or {}).items()}
    if raw in tactics:
        return tactics[raw]
    if raw in techniques:
        return techniques[raw]
    parent = raw.split(".")[0]
    return techniques.get(parent)


def label(code: str) -> str:
    """Return ``CODE — Name`` when a name is known, else just the code."""
    code = code.strip()
    name = owasp_name(code) or mitre_name(code)
    return f"{code} — {name}" if name else code


#: The five standard business-risk impact categories (fixed, not fetched).
BUSINESS_RISK_CATEGORIES = [
    "Strategic",
    "Operational",
    "Financial",
    "Compliance",
    "Reputational",
]


def normalise_business_risk(value: str) -> str | None:
    """Match a free-text business-risk value to a canonical category."""
    v = value.strip().lower()
    for cat in BUSINESS_RISK_CATEGORIES:
        if cat.lower().startswith(v) or v.startswith(cat.lower()):
            return cat
    return None


def split_codes(value) -> list[str]:
    """Split a frontmatter field that may be a list or a delimited string."""
    if value is None:
        return []
    if isinstance(value, list):
        items = [str(v) for v in value]
    else:
        items = re.split(r"[,;]", str(value))
    return [i.strip() for i in items if i.strip()]
