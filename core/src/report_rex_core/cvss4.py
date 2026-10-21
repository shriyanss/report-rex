"""CVSS v4.0 base-score computation.

Ported from the official FIRST.org reference implementation
(``FIRSTdotorg/cvss-v4-calculator``, BSD-2-Clause), which is the
normative implementation of the CVSS v4.0 specification's scoring
algorithm (MacroVector lookup + mean-severity-distance interpolation).
Only the base-metric scoring path is used here; Threat and Environmental
metrics default to "not defined" (``X``) since Report Rex only takes a
base vector from the tester.
"""

from __future__ import annotations

import itertools
from decimal import ROUND_HALF_UP, Decimal

# Compensates for float error (e.g. 8.6 - 7.15 == 1.4499999999999993) before
# rounding half away from zero, matching the reference implementation.
_EPSILON = 10**-6

# ── Metric value domains (CVSS v4.0 spec) ───────────────────────────────────
METRIC_VALUES: dict[str, tuple[str, ...]] = {
    "AV": ("N", "A", "L", "P"),
    "AC": ("L", "H"),
    "AT": ("N", "P"),
    "PR": ("N", "L", "H"),
    "UI": ("N", "P", "A"),
    "VC": ("H", "L", "N"),
    "VI": ("H", "L", "N"),
    "VA": ("H", "L", "N"),
    "SC": ("H", "L", "N"),
    "SI": ("H", "L", "N"),
    "SA": ("H", "L", "N"),
    "E": ("X", "A", "P", "U"),
    "CR": ("X", "H", "M", "L"),
    "IR": ("X", "H", "M", "L"),
    "AR": ("X", "H", "M", "L"),
    "MAV": ("X", "N", "A", "L", "P"),
    "MAC": ("X", "L", "H"),
    "MAT": ("X", "N", "P"),
    "MPR": ("X", "N", "L", "H"),
    "MUI": ("X", "N", "P", "A"),
    "MVC": ("X", "H", "L", "N"),
    "MVI": ("X", "H", "L", "N"),
    "MVA": ("X", "H", "L", "N"),
    "MSC": ("X", "H", "L", "N"),
    "MSI": ("X", "S", "H", "L", "N"),
    "MSA": ("X", "S", "H", "L", "N"),
}

BASE_METRICS: tuple[str, ...] = (
    "AV", "AC", "AT", "PR", "UI", "VC", "VI", "VA", "SC", "SI", "SA",
)

# Metrics that may be "not defined" (X) — everything besides the 11 base ones.
_OPTIONAL_DEFAULT = "X"

# ── MacroVector -> score lookup table (270 entries) ─────────────────────────
LOOKUP: dict[str, float] = {
    "000000": 10, "000001": 9.9, "000010": 9.8, "000011": 9.5, "000020": 9.5, "000021": 9.2,
    "000100": 10, "000101": 9.6, "000110": 9.3, "000111": 8.7, "000120": 9.1, "000121": 8.1,
    "000200": 9.3, "000201": 9, "000210": 8.9, "000211": 8, "000220": 8.1, "000221": 6.8,
    "001000": 9.8, "001001": 9.5, "001010": 9.5, "001011": 9.2, "001020": 9, "001021": 8.4,
    "001100": 9.3, "001101": 9.2, "001110": 8.9, "001111": 8.1, "001120": 8.1, "001121": 6.5,
    "001200": 8.8, "001201": 8, "001210": 7.8, "001211": 7, "001220": 6.9, "001221": 4.8,
    "002001": 9.2, "002011": 8.2, "002021": 7.2, "002101": 7.9, "002111": 6.9, "002121": 5,
    "002201": 6.9, "002211": 5.5, "002221": 2.7, "010000": 9.9, "010001": 9.7, "010010": 9.5,
    "010011": 9.2, "010020": 9.2, "010021": 8.5, "010100": 9.5, "010101": 9.1, "010110": 9,
    "010111": 8.3, "010120": 8.4, "010121": 7.1, "010200": 9.2, "010201": 8.1, "010210": 8.2,
    "010211": 7.1, "010220": 7.2, "010221": 5.3, "011000": 9.5, "011001": 9.3, "011010": 9.2,
    "011011": 8.5, "011020": 8.5, "011021": 7.3, "011100": 9.2, "011101": 8.2, "011110": 8,
    "011111": 7.2, "011120": 7, "011121": 5.9, "011200": 8.4, "011201": 7, "011210": 7.1,
    "011211": 5.2, "011220": 5, "011221": 3, "012001": 8.6, "012011": 7.5, "012021": 5.2,
    "012101": 7.1, "012111": 5.2, "012121": 2.9, "012201": 6.3, "012211": 2.9, "012221": 1.7,
    "100000": 9.8, "100001": 9.5, "100010": 9.4, "100011": 8.7, "100020": 9.1, "100021": 8.1,
    "100100": 9.4, "100101": 8.9, "100110": 8.6, "100111": 7.4, "100120": 7.7, "100121": 6.4,
    "100200": 8.7, "100201": 7.5, "100210": 7.4, "100211": 6.3, "100220": 6.3, "100221": 4.9,
    "101000": 9.4, "101001": 8.9, "101010": 8.8, "101011": 7.7, "101020": 7.6, "101021": 6.7,
    "101100": 8.6, "101101": 7.6, "101110": 7.4, "101111": 5.8, "101120": 5.9, "101121": 5,
    "101200": 7.2, "101201": 5.7, "101210": 5.7, "101211": 5.2, "101220": 5.2, "101221": 2.5,
    "102001": 8.3, "102011": 7, "102021": 5.4, "102101": 6.5, "102111": 5.8, "102121": 2.6,
    "102201": 5.3, "102211": 2.1, "102221": 1.3, "110000": 9.5, "110001": 9, "110010": 8.8,
    "110011": 7.6, "110020": 7.6, "110021": 7, "110100": 9, "110101": 7.7, "110110": 7.5,
    "110111": 6.2, "110120": 6.1, "110121": 5.3, "110200": 7.7, "110201": 6.6, "110210": 6.8,
    "110211": 5.9, "110220": 5.2, "110221": 3, "111000": 8.9, "111001": 7.8, "111010": 7.6,
    "111011": 6.7, "111020": 6.2, "111021": 5.8, "111100": 7.4, "111101": 5.9, "111110": 5.7,
    "111111": 5.7, "111120": 4.7, "111121": 2.3, "111200": 6.1, "111201": 5.2, "111210": 5.7,
    "111211": 2.9, "111220": 2.4, "111221": 1.6, "112001": 7.1, "112011": 5.9, "112021": 3,
    "112101": 5.8, "112111": 2.6, "112121": 1.5, "112201": 2.3, "112211": 1.3, "112221": 0.6,
    "200000": 9.3, "200001": 8.7, "200010": 8.6, "200011": 7.2, "200020": 7.5, "200021": 5.8,
    "200100": 8.6, "200101": 7.4, "200110": 7.4, "200111": 6.1, "200120": 5.6, "200121": 3.4,
    "200200": 7, "200201": 5.4, "200210": 5.2, "200211": 4, "200220": 4, "200221": 2.2,
    "201000": 8.5, "201001": 7.5, "201010": 7.4, "201011": 5.5, "201020": 6.2, "201021": 5.1,
    "201100": 7.2, "201101": 5.7, "201110": 5.5, "201111": 4.1, "201120": 4.6, "201121": 1.9,
    "201200": 5.3, "201201": 3.6, "201210": 3.4, "201211": 1.9, "201220": 1.9, "201221": 0.8,
    "202001": 6.4, "202011": 5.1, "202021": 2, "202101": 4.7, "202111": 2.1, "202121": 1.1,
    "202201": 2.4, "202211": 0.9, "202221": 0.4, "210000": 8.8, "210001": 7.5, "210010": 7.3,
    "210011": 5.3, "210020": 6, "210021": 5, "210100": 7.3, "210101": 5.5, "210110": 5.9,
    "210111": 4, "210120": 4.1, "210121": 2, "210200": 5.4, "210201": 4.3, "210210": 4.5,
    "210211": 2.2, "210220": 2, "210221": 1.1, "211000": 7.5, "211001": 5.5, "211010": 5.8,
    "211011": 4.5, "211020": 4, "211021": 2.1, "211100": 6.1, "211101": 5.1, "211110": 4.8,
    "211111": 1.8, "211120": 2, "211121": 0.9, "211200": 4.6, "211201": 1.8, "211210": 1.7,
    "211211": 0.7, "211220": 0.8, "211221": 0.2, "212001": 5.3, "212011": 2.4, "212021": 1.4,
    "212101": 2.4, "212111": 1.2, "212121": 0.5, "212201": 1, "212211": 0.3, "212221": 0.1,
}

# ── Highest-severity composed vector per MacroVector value (per EQ) ────────
MAX_COMPOSED = {
    "eq1": {
        "0": ["AV:N/PR:N/UI:N/"],
        "1": ["AV:A/PR:N/UI:N/", "AV:N/PR:L/UI:N/", "AV:N/PR:N/UI:P/"],
        "2": ["AV:P/PR:N/UI:N/", "AV:A/PR:L/UI:P/"],
    },
    "eq2": {
        "0": ["AC:L/AT:N/"],
        "1": ["AC:H/AT:N/", "AC:L/AT:P/"],
    },
    "eq3": {
        "0": {
            "0": ["VC:H/VI:H/VA:H/CR:H/IR:H/AR:H/"],
            "1": ["VC:H/VI:H/VA:L/CR:M/IR:M/AR:H/", "VC:H/VI:H/VA:H/CR:M/IR:M/AR:M/"],
        },
        "1": {
            "0": ["VC:L/VI:H/VA:H/CR:H/IR:H/AR:H/", "VC:H/VI:L/VA:H/CR:H/IR:H/AR:H/"],
            "1": [
                "VC:L/VI:H/VA:L/CR:H/IR:M/AR:H/",
                "VC:L/VI:H/VA:H/CR:H/IR:M/AR:M/",
                "VC:H/VI:L/VA:H/CR:M/IR:H/AR:M/",
                "VC:H/VI:L/VA:L/CR:M/IR:H/AR:H/",
                "VC:L/VI:L/VA:H/CR:H/IR:H/AR:M/",
            ],
        },
        "2": {"1": ["VC:L/VI:L/VA:L/CR:H/IR:H/AR:H/"]},
    },
    "eq4": {
        "0": ["SC:H/SI:S/SA:S/"],
        "1": ["SC:H/SI:H/SA:H/"],
        "2": ["SC:L/SI:L/SA:L/"],
    },
    "eq5": {
        "0": ["E:A/"],
        "1": ["E:P/"],
        "2": ["E:U/"],
    },
}

# ── Max severity distance (in steps of 0.1) available within each EQ ────────
MAX_SEVERITY = {
    "eq1": {0: 1, 1: 4, 2: 5},
    "eq2": {0: 1, 1: 2},
    "eq3eq6": {0: {0: 7, 1: 6}, 1: {0: 8, 1: 8}, 2: {1: 10}},
    "eq4": {0: 6, 1: 5, 2: 4},
    "eq5": {0: 1, 1: 1, 2: 1},
}

_AV_LEVELS = {"N": 0.0, "A": 0.1, "L": 0.2, "P": 0.3}
_PR_LEVELS = {"N": 0.0, "L": 0.1, "H": 0.2}
_UI_LEVELS = {"N": 0.0, "P": 0.1, "A": 0.2}
_AC_LEVELS = {"L": 0.0, "H": 0.1}
_AT_LEVELS = {"N": 0.0, "P": 0.1}
_VC_LEVELS = {"H": 0.0, "L": 0.1, "N": 0.2}
_VI_LEVELS = {"H": 0.0, "L": 0.1, "N": 0.2}
_VA_LEVELS = {"H": 0.0, "L": 0.1, "N": 0.2}
_SC_LEVELS = {"H": 0.1, "L": 0.2, "N": 0.3}
_SI_LEVELS = {"S": 0.0, "H": 0.1, "L": 0.2, "N": 0.3}
_SA_LEVELS = {"S": 0.0, "H": 0.1, "L": 0.2, "N": 0.3}
_CR_LEVELS = {"H": 0.0, "M": 0.1, "L": 0.2}
_IR_LEVELS = {"H": 0.0, "M": 0.1, "L": 0.2}
_AR_LEVELS = {"H": 0.0, "M": 0.1, "L": 0.2}

_METRIC_LEVELS = {
    "AV": _AV_LEVELS, "PR": _PR_LEVELS, "UI": _UI_LEVELS,
    "AC": _AC_LEVELS, "AT": _AT_LEVELS,
    "VC": _VC_LEVELS, "VI": _VI_LEVELS, "VA": _VA_LEVELS,
    "SC": _SC_LEVELS, "SI": _SI_LEVELS, "SA": _SA_LEVELS,
    "CR": _CR_LEVELS, "IR": _IR_LEVELS, "AR": _AR_LEVELS,
}
_DISTANCE_METRICS = (
    "AV", "PR", "UI", "AC", "AT", "VC", "VI", "VA", "SC", "SI", "SA", "CR", "IR", "AR",
)


def m(selected: dict[str, str], metric: str) -> str:
    """Resolve the effective value of ``metric``, honoring Environmental overrides."""
    value = selected.get(metric, _OPTIONAL_DEFAULT)

    if metric == "E" and value == "X":
        return "A"
    if metric in ("CR", "IR", "AR") and value == "X":
        return "H"

    modified = selected.get("M" + metric)
    if modified is not None and modified != "X":
        return modified

    return value


def macro_vector(selected: dict[str, str]) -> str:
    """Compute the 6-digit MacroVector (EQ1..EQ6) for a CVSS v4.0 vector."""
    av, pr, ui = m(selected, "AV"), m(selected, "PR"), m(selected, "UI")
    if av == "N" and pr == "N" and ui == "N":
        eq1 = 0
    elif (av == "N" or pr == "N" or ui == "N") and av != "P":
        eq1 = 1
    else:
        eq1 = 2

    ac, at = m(selected, "AC"), m(selected, "AT")
    eq2 = 0 if (ac == "L" and at == "N") else 1

    vc, vi, va = m(selected, "VC"), m(selected, "VI"), m(selected, "VA")
    if vc == "H" and vi == "H":
        eq3 = 0
    elif vc == "H" or vi == "H" or va == "H":
        eq3 = 1
    else:
        eq3 = 2

    msi, msa = m(selected, "MSI"), m(selected, "MSA")
    sc, si, sa = m(selected, "SC"), m(selected, "SI"), m(selected, "SA")
    if msi == "S" or msa == "S":
        eq4 = 0
    elif si == "H" or sa == "H" or sc == "H":
        eq4 = 1
    else:
        eq4 = 2

    e = m(selected, "E")
    eq5 = {"A": 0, "P": 1, "U": 2}[e]

    cr, ir, ar = m(selected, "CR"), m(selected, "IR"), m(selected, "AR")
    if (cr == "H" and vc == "H") or (ir == "H" and vi == "H") or (ar == "H" and va == "H"):
        eq6 = 0
    else:
        eq6 = 1

    return f"{eq1}{eq2}{eq3}{eq4}{eq5}{eq6}"


def _extract_value(metric: str, vector: str) -> str:
    idx = vector.index(metric)
    rest = vector[idx + len(metric) + 1:]
    slash = rest.find("/")
    return rest[:slash] if slash > 0 else rest


def _get_eq_maxes(mv: str, eq: int) -> list[str]:
    return MAX_COMPOSED[f"eq{eq}"][mv[eq - 1]]


def base_score(selected: dict[str, str]) -> float:
    """Compute the CVSS v4.0 base score for a resolved metric-value mapping."""
    if all(m(selected, metric) == "N" for metric in ("VC", "VI", "VA", "SC", "SI", "SA")):
        return 0.0

    mv = macro_vector(selected)
    value = LOOKUP[mv]

    eq1, eq2, eq3, eq4, eq5, eq6 = (int(d) for d in mv)

    eq1_lower = f"{eq1 + 1}{eq2}{eq3}{eq4}{eq5}{eq6}"
    eq2_lower = f"{eq1}{eq2 + 1}{eq3}{eq4}{eq5}{eq6}"
    if eq3 == 1 and eq6 == 1:
        eq3eq6_lower_candidates = [f"{eq1}{eq2}{eq3 + 1}{eq4}{eq5}{eq6}"]
    elif eq3 == 0 and eq6 == 1:
        eq3eq6_lower_candidates = [f"{eq1}{eq2}{eq3 + 1}{eq4}{eq5}{eq6}"]
    elif eq3 == 1 and eq6 == 0:
        eq3eq6_lower_candidates = [f"{eq1}{eq2}{eq3}{eq4}{eq5}{eq6 + 1}"]
    elif eq3 == 0 and eq6 == 0:
        eq3eq6_lower_candidates = [
            f"{eq1}{eq2}{eq3}{eq4}{eq5}{eq6 + 1}",
            f"{eq1}{eq2}{eq3 + 1}{eq4}{eq5}{eq6}",
        ]
    else:
        eq3eq6_lower_candidates = [f"{eq1}{eq2}{eq3 + 1}{eq4}{eq5}{eq6 + 1}"]

    eq4_lower = f"{eq1}{eq2}{eq3}{eq4 + 1}{eq5}{eq6}"
    eq5_lower = f"{eq1}{eq2}{eq3}{eq4}{eq5 + 1}{eq6}"

    score_eq1_lower = LOOKUP.get(eq1_lower)
    score_eq2_lower = LOOKUP.get(eq2_lower)
    if len(eq3eq6_lower_candidates) == 2:
        left, right = (LOOKUP.get(c) for c in eq3eq6_lower_candidates)
        if left is None:
            score_eq3eq6_lower = right
        elif right is None:
            score_eq3eq6_lower = left
        else:
            score_eq3eq6_lower = left if left > right else right
    else:
        score_eq3eq6_lower = LOOKUP.get(eq3eq6_lower_candidates[0])
    score_eq4_lower = LOOKUP.get(eq4_lower)
    score_eq5_lower = LOOKUP.get(eq5_lower)

    eq1_maxes = _get_eq_maxes(mv, 1)
    eq2_maxes = _get_eq_maxes(mv, 2)
    eq3_eq6_maxes = MAX_COMPOSED["eq3"][mv[2]][mv[5]]
    eq4_maxes = _get_eq_maxes(mv, 4)
    eq5_maxes = _get_eq_maxes(mv, 5)

    distances: dict[str, float] = {}
    for eq1_max, eq2_max, eq3_eq6_max, eq4_max, eq5_max in itertools.product(
        eq1_maxes, eq2_maxes, eq3_eq6_maxes, eq4_maxes, eq5_maxes
    ):
        max_vector = eq1_max + eq2_max + eq3_eq6_max + eq4_max + eq5_max
        candidate = {
            metric: _METRIC_LEVELS[metric][m(selected, metric)]
            - _METRIC_LEVELS[metric][_extract_value(metric, max_vector)]
            for metric in _DISTANCE_METRICS
        }
        if all(v >= 0 for v in candidate.values()):
            distances = candidate
            break

    dist_eq1 = distances["AV"] + distances["PR"] + distances["UI"]
    dist_eq2 = distances["AC"] + distances["AT"]
    dist_eq3eq6 = (
        distances["VC"] + distances["VI"] + distances["VA"]
        + distances["CR"] + distances["IR"] + distances["AR"]
    )
    dist_eq4 = distances["SC"] + distances["SI"] + distances["SA"]

    step = 0.1
    max_sev_eq1 = MAX_SEVERITY["eq1"][eq1] * step
    max_sev_eq2 = MAX_SEVERITY["eq2"][eq2] * step
    max_sev_eq3eq6 = MAX_SEVERITY["eq3eq6"][eq3][eq6] * step
    max_sev_eq4 = MAX_SEVERITY["eq4"][eq4] * step

    n_existing_lower = 0
    normalized_total = 0.0

    if score_eq1_lower is not None:
        n_existing_lower += 1
        normalized_total += (value - score_eq1_lower) * (dist_eq1 / max_sev_eq1)
    if score_eq2_lower is not None:
        n_existing_lower += 1
        normalized_total += (value - score_eq2_lower) * (dist_eq2 / max_sev_eq2)
    if score_eq3eq6_lower is not None:
        n_existing_lower += 1
        normalized_total += (value - score_eq3eq6_lower) * (dist_eq3eq6 / max_sev_eq3eq6)
    if score_eq4_lower is not None:
        n_existing_lower += 1
        normalized_total += (value - score_eq4_lower) * (dist_eq4 / max_sev_eq4)
    if score_eq5_lower is not None:
        # eq5's proportion is always 0 per the reference implementation.
        n_existing_lower += 1

    mean_distance = 0.0 if n_existing_lower == 0 else normalized_total / n_existing_lower

    result = value - mean_distance
    result = max(0.0, min(10.0, result))
    return float(
        Decimal(result + _EPSILON).quantize(Decimal("0.1"), rounding=ROUND_HALF_UP)
    )
