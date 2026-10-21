"""CVSS vector parsing and base-score computation.

The tester only needs to paste a CVSS vector string (e.g.
``CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H``) into a finding's
frontmatter. Report Rex computes the numeric base score and the qualitative
severity automatically.

Full support is provided for CVSS **v3.0**, **v3.1**, and **v4.0**. Vectors
without a ``CVSS:x.y`` prefix are treated as v3.1. v4.0 base-score
computation follows the official MacroVector lookup + mean-severity-distance
algorithm (see :mod:`.cvss4`); Threat and Environmental metrics are accepted
in the vector but only the 11 Base metrics affect the score Report Rex
reports, since testers only supply a base vector.
"""

from __future__ import annotations

import math
from dataclasses import dataclass

from . import cvss4

# ── Metric weight tables (CVSS v3.x base metrics) ───────────────────────────
_AV = {"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.20}
_AC = {"L": 0.77, "H": 0.44}
_UI = {"N": 0.85, "R": 0.62}
_CIA = {"H": 0.56, "L": 0.22, "N": 0.00}
# Privileges Required is scope-dependent.
_PR_UNCHANGED = {"N": 0.85, "L": 0.62, "H": 0.27}
_PR_CHANGED = {"N": 0.85, "L": 0.68, "H": 0.50}

_REQUIRED_V3 = ("AV", "AC", "PR", "UI", "S", "C", "I", "A")
_REQUIRED_V4 = cvss4.BASE_METRICS

SEVERITY_ORDER = ["None", "Low", "Medium", "High", "Critical"]
#: Numeric rank used for sorting findings (higher = more severe).
SEVERITY_RANK = {"Critical": 4, "High": 3, "Medium": 2, "Low": 1, "None": 0, "Info": 0}


class CvssError(ValueError):
    """Raised when a CVSS vector string cannot be parsed."""


@dataclass
class CvssResult:
    version: str
    score: float
    severity: str
    vector: str


def parse_vector(vector: str) -> tuple[str, dict[str, str]]:
    """Split a CVSS vector into (version, {metric: value}).

    Accepts an optional ``CVSS:x.y/`` prefix. Raises :class:`CvssError` on
    malformed input or missing base metrics.
    """
    if not vector or not isinstance(vector, str):
        raise CvssError("empty CVSS vector")

    parts = [p for p in vector.strip().split("/") if p]
    version = "3.1"
    metrics: dict[str, str] = {}

    for part in parts:
        if ":" not in part:
            raise CvssError(f"malformed CVSS component: {part!r}")
        key, _, value = part.partition(":")
        key, value = key.strip().upper(), value.strip().upper()
        if key == "CVSS":
            version = value
            continue
        metrics[key] = value

    required = _REQUIRED_V4 if version.split(".")[0] == "4" else _REQUIRED_V3
    missing = [m for m in required if m not in metrics]
    if missing:
        raise CvssError(f"CVSS vector missing base metrics: {', '.join(missing)}")

    return version, metrics


def _roundup(value: float, version: str) -> float:
    """CVSS Roundup. v3.1 uses the integer-arithmetic definition; v3.0 uses ceil."""
    if version == "3.0":
        return math.ceil(value * 10) / 10
    # CVSS v3.1 official Roundup
    int_input = round(value * 100_000)
    if int_input % 10_000 == 0:
        return int_input / 100_000
    return (math.floor(int_input / 10_000) + 1) / 10


def base_score(vector: str) -> tuple[str, float]:
    """Compute the CVSS base score. Returns (version, score)."""
    version, m = parse_vector(vector)
    major = version.split(".")[0]
    if major == "4":
        return version, _base_score_v4(m)
    if major != "3":
        raise CvssError(
            f"base-score computation only supports CVSS v3.x and v4.0 (got {version})"
        )

    scope_changed = m["S"] == "C"
    try:
        av = _AV[m["AV"]]
        ac = _AC[m["AC"]]
        ui = _UI[m["UI"]]
        pr = (_PR_CHANGED if scope_changed else _PR_UNCHANGED)[m["PR"]]
        c, i, a = _CIA[m["C"]], _CIA[m["I"]], _CIA[m["A"]]
    except KeyError as exc:
        raise CvssError(f"invalid CVSS metric value: {exc}") from exc

    iss = 1 - (1 - c) * (1 - i) * (1 - a)
    if scope_changed:
        impact = 7.52 * (iss - 0.029) - 3.25 * (iss - 0.02) ** 15
    else:
        impact = 6.42 * iss

    exploitability = 8.22 * av * ac * pr * ui

    if impact <= 0:
        score = 0.0
    elif scope_changed:
        score = _roundup(min(1.08 * (impact + exploitability), 10), version)
    else:
        score = _roundup(min(impact + exploitability, 10), version)

    return version, round(score, 1)


def _base_score_v4(metrics: dict[str, str]) -> float:
    """Compute the CVSS v4.0 base score from a parsed metric mapping."""
    for key, value in metrics.items():
        allowed = cvss4.METRIC_VALUES.get(key)
        if allowed is not None and value not in allowed:
            raise CvssError(f"invalid CVSS metric value: {key}:{value}")

    selected = {key: metrics.get(key, "X") for key in cvss4.METRIC_VALUES}
    return cvss4.base_score(selected)


def severity_from_score(score: float) -> str:
    """Map a numeric base score to the CVSS qualitative severity rating."""
    if score <= 0:
        return "None"
    if score < 4.0:
        return "Low"
    if score < 7.0:
        return "Medium"
    if score < 9.0:
        return "High"
    return "Critical"


def compute(vector: str) -> CvssResult:
    """Parse a vector and return version, score and qualitative severity."""
    version, score = base_score(vector)
    return CvssResult(
        version=version,
        score=score,
        severity=severity_from_score(score),
        vector=vector.strip(),
    )
