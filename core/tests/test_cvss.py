"""Tests for the CVSS calculator."""

import pytest

from report_rex_core import cvss


@pytest.mark.parametrize(
    "vector, score, severity",
    [
        ("CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H", 9.8, "Critical"),
        ("AV:N/AC:L/PR:L/UI:R/S:C/C:H/I:H/A:N", 8.7, "High"),
        ("AV:N/AC:L/PR:L/UI:N/S:U/C:H/I:N/A:N", 6.5, "Medium"),
        ("AV:N/AC:H/PR:N/UI:R/S:U/C:L/I:N/A:N", 3.1, "Low"),
        ("AV:P/AC:H/PR:H/UI:R/S:U/C:N/I:N/A:N", 0.0, "None"),
    ],
)
def test_known_vectors(vector, score, severity):
    result = cvss.compute(vector)
    assert result.score == score
    assert result.severity == severity


def test_defaults_to_v31_without_prefix():
    assert cvss.compute("AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H").version == "3.1"


def test_reads_version_prefix():
    assert cvss.compute("CVSS:3.0/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H").version == "3.0"


def test_missing_metric_raises():
    with pytest.raises(cvss.CvssError):
        cvss.compute("AV:N/AC:L")


@pytest.mark.parametrize(
    "vector, score, severity",
    [
        ("CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N", 9.3, "Critical"),
        ("CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:H/SI:H/SA:H", 10.0, "Critical"),
        ("CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:L/VI:L/VA:L/SC:N/SI:N/SA:N", 6.9, "Medium"),
        ("CVSS:4.0/AV:N/AC:L/AT:N/PR:L/UI:N/VC:L/VI:L/VA:N/SC:N/SI:N/SA:N", 5.3, "Medium"),
        ("CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:P/VC:H/VI:N/VA:N/SC:N/SI:N/SA:N", 7.1, "High"),
        ("CVSS:4.0/AV:P/AC:H/AT:P/PR:H/UI:A/VC:N/VI:N/VA:N/SC:N/SI:N/SA:N", 0.0, "None"),
    ],
)
def test_known_v4_vectors(vector, score, severity):
    # Reference values cross-checked against the official FIRST.org
    # cvss-v4-calculator reference implementation and the RedHatProductSecurity
    # `cvss` PyPI package.
    result = cvss.compute(vector)
    assert result.score == score
    assert result.severity == severity
    assert result.version == "4.0"


def test_v4_missing_base_metric_raises():
    with pytest.raises(cvss.CvssError):
        cvss.compute("CVSS:4.0/AV:N/AC:L/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N")


def test_v4_invalid_metric_value_raises():
    with pytest.raises(cvss.CvssError):
        cvss.compute("CVSS:4.0/AV:X/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N")


def test_v4_environmental_metrics_affect_score():
    base = "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:L/VA:N/SC:N/SI:N/SA:N"
    unmodified = cvss.compute(base)
    low_requirements = cvss.compute(base + "/CR:L/IR:L/AR:L")
    assert low_requirements.score < unmodified.score


def test_unsupported_version_raises():
    with pytest.raises(cvss.CvssError):
        cvss.base_score("CVSS:2.0/AV:N/AC:L/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N")


def test_severity_boundaries():
    assert cvss.severity_from_score(0.0) == "None"
    assert cvss.severity_from_score(3.9) == "Low"
    assert cvss.severity_from_score(4.0) == "Medium"
    assert cvss.severity_from_score(6.9) == "Medium"
    assert cvss.severity_from_score(7.0) == "High"
    assert cvss.severity_from_score(9.0) == "Critical"
