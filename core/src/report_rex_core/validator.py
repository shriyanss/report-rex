"""QA-gate validation for report content: field completeness and screenshot heuristics.

This complements ``parser.parse_index`` (which only checks structural things — YAML
frontmatter presence, referenced files existing) with content-level checks: required
fields, minimum prose length, screenshot captions, leftover scaffold text, and a few
best-effort image heuristics (light-mode, annotation presence, text legibility).

``validate_report`` never raises for "bad content" — it collects every problem into a
``ValidationResult`` so callers (the ``validate`` CLI command) can report everything at
once instead of failing fast on the first issue.
"""

from __future__ import annotations

import hashlib
import ipaddress
import json
import re
from dataclasses import dataclass, field
from pathlib import Path

import numpy as np
from PIL import Image, UnidentifiedImageError

from .builder import (
    PLACEHOLDER_CLIENT_LOGO,
    PLACEHOLDER_CONTACT_EMAIL,
    PLACEHOLDER_CONTACT_NAME,
    PLACEHOLDER_DESCRIPTION,
    PLACEHOLDER_EPOCH_DATE,
    PLACEHOLDER_IMPACT,
    PLACEHOLDER_LOREM_IPSUM,
    PLACEHOLDER_METHODOLOGY,
    PLACEHOLDER_RECOMMENDATION,
    PLACEHOLDER_REPORT_IDS,
    PLACEHOLDER_VULN_TITLE,
)
from .charts import _endpoint
from .parser import ReportStructure, parse_frontmatter, split_frontmatter
from .preprocessor import (
    _ASSET_GRAPH_DIRECTIVE,
    _MD_IMG_REF,
    _MD_LINK_REF,
    _NETWORK_DIRECTIVE,
    _WEB_GRAPH_DIRECTIVE,
)

Severity = str  # "error" | "warning"


@dataclass
class ValidationIssue:
    file: Path
    check: str
    message: str
    severity: Severity = "error"
    line: int = 0  # 1-based line the issue points at; 0 = the whole file


@dataclass
class ValidationResult:
    issues: list[ValidationIssue] = field(default_factory=list)

    @property
    def errors(self) -> list[ValidationIssue]:
        return [i for i in self.issues if i.severity == "error"]

    @property
    def warnings(self) -> list[ValidationIssue]:
        return [i for i in self.issues if i.severity == "warning"]

    @property
    def ok(self) -> bool:
        return not self.errors


# ── Tunable thresholds ──────────────────────────────────────────────────────

MIN_STEPS_TO_REPRODUCE_CHARS = 100
MIN_CAPTION_CHARS = 35

# Light-mode check: average border luminance (0-255 scale) below this is treated as
# dark-mode. 128 is the midpoint of the range — a light screenshot's background is
# typically near-white (~230-255) and a dark one near-black (~20-45), both comfortably
# on either side of the midpoint.
MIN_LUMINANCE_LIGHT_MODE = 128

# Annotation check: minimum count of "saturated marker color" (bright red/orange/yellow)
# pixels required to count as "an annotation was found". Deliberately low to minimize
# false negatives (missing a real annotation) at the cost of occasionally accepting an
# incidental red/orange/yellow UI element as if it were an annotation.
MIN_SATURATED_CLUSTER_PX = 200

# Font-size / legibility check: minimum estimated glyph height in pixels, at the image's
# native resolution. This is an approximation (run-length "ink" density), not a real
# typography measurement — shipped as a warning, not an error.
MIN_TEXT_HEIGHT_PX = 10

# Font-size / legibility check: minimum ink pixel count for a row to count as "part of a
# text line" in the row-density profile.
MIN_ROW_INK_PX = 3

# Border/corner sampling geometry for the light-mode check: fraction of image width/height
# used as the sampling margin from each edge.
LUMINANCE_SAMPLE_MARGIN_FRACTION = 0.03

_PLACEHOLDER_STRINGS = (
    PLACEHOLDER_VULN_TITLE,
    PLACEHOLDER_DESCRIPTION,
    PLACEHOLDER_IMPACT,
    PLACEHOLDER_RECOMMENDATION,
    PLACEHOLDER_METHODOLOGY,
    PLACEHOLDER_CONTACT_NAME,
    PLACEHOLDER_CONTACT_EMAIL,
    PLACEHOLDER_EPOCH_DATE,
    PLACEHOLDER_LOREM_IPSUM,
    *PLACEHOLDER_REPORT_IDS,
    PLACEHOLDER_CLIENT_LOGO,
)

# SHA-256 of the generated placeholder screenshot shipped at
# reporting/example/assets/screenshots/*.png (see scripts/gen_placeholder_screenshot.py).
# A module-level variable, not a constant, so tests can monkeypatch it.
_PLACEHOLDER_SCREENSHOT_SHA256 = "f38bed7f262ca07135c0ed47f7eb128f2d6e813a686983c5c8ce15034bd0ee9f"

_HEADING = re.compile(r"^(#{1,5})(\s)", re.MULTILINE)

_MD_IMAGE_REF = re.compile(
    r"!\[([^\]]*)\]\(\s*([^)\s]+\.(?:png|jpe?g|gif))(?:\s+[^)]*)?\)(?:\{[^}]*\})?",
    re.IGNORECASE,
)

_DARK_OVERRIDE_MARKER = re.compile(r"^>\s*\[!OVERRIDE-DARK-IMG\]\s*$", re.MULTILINE)

# `> [!REX-STUB]` (optionally as a YAML comment, `# > [!REX-STUB]`) marks a stub
# value/section left over from a vuln-templates/ template that hasn't been filled in yet.
_STUB_MARKER = re.compile(r"^[ \t]*#?[ \t]*>\s*\[!REX-STUB\]\s*$", re.MULTILINE)


# ── Public entry point ──────────────────────────────────────────────────────


def validate_report(structure: ReportStructure) -> ValidationResult:
    """Run every QA-gate check against an already-parsed report structure.

    Assumes structure.reports/sections/appendices paths already exist on disk
    (parse_index() would have raised otherwise) — this function only opens and
    re-reads each file's content for field-level checks.
    """
    result = ValidationResult()
    finding_names = {p.stem for p in structure.reports} | {p.name for p in structure.reports}
    md_files: list[tuple[Path, str]] = []  # every content file, for _check_network_artifacts
    for path in structure.reports:
        raw = path.read_text("utf-8")
        md_files.append((path, raw))
        result.issues += _check_frontmatter_at_top(path, raw)
        try:
            meta = parse_frontmatter(raw, path)
        except ValueError as exc:
            result.issues.append(ValidationIssue(path, "frontmatter_invalid", str(exc)))
            continue
        _, body = split_frontmatter(raw)
        result.issues += _check_title_present(path, meta)
        result.issues += _check_steps_to_reproduce(path, body)
        result.issues += _check_cvss_present(path, meta)
        result.issues += _check_timeline_present(path, meta)
        result.issues += _check_screenshot_present(path, body)
        result.issues += _check_placeholder_text(path, meta, body)
        result.issues += _check_screenshots(path, body, structure.base_dir)
        result.issues += _check_stub_markers(path, raw)
        result.issues += _check_markdown_structure(path, raw, is_report=True)
        result.issues += _check_dangling_refs(path, raw, finding_names)
    for path in structure.sections + structure.appendices:
        raw = path.read_text("utf-8")
        md_files.append((path, raw))
        result.issues += _check_frontmatter_at_top(path, raw)
        try:
            meta = parse_frontmatter(raw, path)
        except ValueError as exc:
            result.issues.append(ValidationIssue(path, "frontmatter_invalid", str(exc)))
            continue
        _, body = split_frontmatter(raw)
        result.issues += _check_placeholder_text(path, meta, body)
        result.issues += _check_screenshots(path, body, structure.base_dir)
        result.issues += _check_stub_markers(path, raw)
        result.issues += _check_markdown_structure(path, raw, is_report=False)
        result.issues += _check_dangling_refs(path, raw, finding_names)
    result.issues += _check_placeholder_text(structure.base_dir / "index.md", structure.metadata, "")
    result.issues += _check_required_metadata(structure.base_dir / "index.md", structure.metadata)
    result.issues += _check_testing_window(structure.base_dir / "index.md", structure.metadata)
    result.issues += _check_network_artifacts(structure.base_dir, md_files)
    return result


# index.md frontmatter fields every template renders on the cover/header —
# without them the report comes out "Untitled".
REQUIRED_METADATA = ("title", "client", "date")


def _check_required_metadata(path: Path, meta: dict) -> list[ValidationIssue]:
    return [
        ValidationIssue(path, "metadata_missing", f"No '{key}' in index.md frontmatter — it's required.", "error")
        for key in REQUIRED_METADATA
        if not str(meta.get(key) or "").strip()
    ]


def _check_testing_window(path: Path, meta: dict) -> list[ValidationIssue]:
    # The cover/Document Control "Testing Window" is start_date – end_date; with only
    # one of them it shows a single date (or none) instead of a range (#74).
    has_start, has_end = (bool(str(meta.get(k) or "").strip()) for k in ("start_date", "end_date"))
    if has_start == has_end:
        return []
    missing = "end_date" if has_start else "start_date"
    return [ValidationIssue(
        path, "testing_window_incomplete",
        f"No '{missing}' in index.md frontmatter — the Testing Window shows a single date "
        "instead of a start–end range.", "warning",
    )]


def _check_frontmatter_at_top(path: Path, raw: str) -> list[ValidationIssue]:
    # Blank lines or a BOM before the '---' make split_frontmatter() ignore the whole block.
    if raw.startswith("---"):
        return []
    stripped = raw.removeprefix("\ufeff").lstrip(" \t\r\n")
    if stripped.split("\n", 1)[0].strip() != "---":
        return []
    line = raw[: len(raw) - len(stripped)].count("\n") + 1
    return [ValidationIssue(
        path, "frontmatter_not_at_top",
        "Frontmatter must start on the very first line — anything before the '---' "
        "(even a blank line) makes Rex ignore it.",
        line=line,
    )]


def _check_title_present(path: Path, meta: dict) -> list[ValidationIssue]:
    # No title (or frontmatter not detected at all) renders as a nameless finding.
    if meta.get("title"):
        return []
    return [ValidationIssue(
        path, "title_missing",
        "No 'title' in frontmatter (is the '---' block at the very top of the file?).",
    )]


# ── Check 1 — Replication (Steps to Reproduce) length ───────────────────────


def _section_body(body: str, heading: str) -> str | None:
    """Return the raw text under a `## <heading>` markdown heading, or None if absent."""
    matches = list(_HEADING.finditer(body))
    for idx, m in enumerate(matches):
        level = len(m.group(1))
        line_end = body.find("\n", m.end())
        line_end = len(body) if line_end == -1 else line_end
        heading_text = body[m.end():line_end].strip()
        # Ignore any trailing pandoc {#anchor} attribute block on the heading line.
        heading_text = re.sub(r"\{[^}]*\}\s*$", "", heading_text).strip()
        if heading_text.lower() != heading.lower():
            continue
        start = line_end + 1 if line_end < len(body) else len(body)
        end = len(body)
        for next_m in matches[idx + 1:]:
            if len(next_m.group(1)) <= level:
                end = next_m.start()
                break
        return body[start:end]
    return None


# `## Replication` (steps + evidence) replaced `## Steps to Reproduce` + `## Evidence`
# in the scaffolds (#68); reports written with the older heading still validate.
_REPLICATION_HEADINGS = ("Replication", "Steps to Reproduce")


def _check_steps_to_reproduce(path: Path, body: str) -> list[ValidationIssue]:
    heading, section = next(
        ((h, sec) for h in _REPLICATION_HEADINGS if (sec := _section_body(body, h)) is not None),
        (None, None),
    )
    if section is None:
        return [ValidationIssue(
            path, "steps_to_reproduce_missing",
            "No '## Replication' section found (or the older '## Steps to Reproduce').", "error",
        )]
    text_len = len(section.strip())
    if text_len < MIN_STEPS_TO_REPRODUCE_CHARS:
        return [ValidationIssue(
            path, "steps_to_reproduce_length",
            f"'{heading}' section is {text_len} characters "
            f"(minimum {MIN_STEPS_TO_REPRODUCE_CHARS}).", "error",
        )]
    return []


# ── Check 2 & 3 — Screenshot caption length + image heuristics ─────────────


def _resolve_image_path(file_path: Path, base_dir: Path, rel_target: str) -> Path:
    return (file_path.parent / rel_target).resolve()


def _has_dark_override(body: str, image_match_start: int) -> bool:
    """True if a `> [!OVERRIDE-DARK-IMG]` line appears directly above the image markdown,
    ignoring blank lines in between (but not other content lines)."""
    preceding_lines = body[:image_match_start].splitlines()
    while preceding_lines and not preceding_lines[-1].strip():
        preceding_lines.pop()
    if not preceding_lines:
        return False
    return bool(_DARK_OVERRIDE_MARKER.match(preceding_lines[-1]))


def _check_screenshots(path: Path, body: str, base_dir: Path) -> list[ValidationIssue]:
    issues = []
    for m in _MD_IMAGE_REF.finditer(body):
        caption, rel_target = m.group(1), m.group(2)
        if len(caption.strip()) < MIN_CAPTION_CHARS:
            issues.append(ValidationIssue(
                path, "screenshot_caption_length",
                f"Screenshot caption {caption.strip()!r} is {len(caption.strip())} characters "
                f"(minimum {MIN_CAPTION_CHARS}): {rel_target}", "error",
            ))
        image_path = _resolve_image_path(path, base_dir, rel_target)
        if not image_path.exists():
            issues.append(ValidationIssue(
                path, "screenshot_missing",
                f"Referenced screenshot does not exist: {image_path}", "error",
            ))
            continue
        issues += _check_screenshot_image(
            path, image_path,
            skip_dark_check=_has_dark_override(body, m.start()),
        )
        issues += _check_placeholder_screenshot(path, image_path)
    return issues


def _check_screenshot_present(path: Path, body: str) -> list[ValidationIssue]:
    # Every finding must embed at least one screenshot.
    if _MD_IMAGE_REF.search(body):
        return []
    return [ValidationIssue(
        path, "screenshot_none",
        "No screenshot found — every finding needs at least one ![caption](path.png) image.",
        "error",
    )]


def _check_placeholder_screenshot(finding_path: Path, image_path: Path) -> list[ValidationIssue]:
    """Flags the shipped Lorem Ipsum placeholder screenshot (see
    scripts/gen_placeholder_screenshot.py) via a flat SHA-256 equality check —
    not an image-similarity system, just "is this literally the demo image"."""
    digest = hashlib.sha256(image_path.read_bytes()).hexdigest()
    if digest == _PLACEHOLDER_SCREENSHOT_SHA256:
        return [ValidationIssue(
            finding_path, "screenshot_placeholder_image",
            f"Screenshot is the shipped Lorem Ipsum placeholder image — replace it with a "
            f"real annotated screenshot: {image_path}", "error",
        )]
    return []


def _load_image(image_path: Path) -> Image.Image | None:
    """Return an opened, RGB-converted Pillow Image, or None if unreadable/corrupt."""
    try:
        with Image.open(image_path) as img:
            img.load()
            return img.convert("RGB")
    except (UnidentifiedImageError, OSError, ValueError):
        return None


def _check_screenshot_image(
    finding_path: Path, image_path: Path, *, skip_dark_check: bool = False,
) -> list[ValidationIssue]:
    """Run the image heuristics against one screenshot. skip_dark_check is set when the
    markdown body has a preceding `> [!OVERRIDE-DARK-IMG]` marker for this image — the
    annotation and legibility checks still run even when the dark-mode check is skipped."""
    img = _load_image(image_path)
    if img is None:
        return [ValidationIssue(
            finding_path, "screenshot_unreadable",
            f"Could not open image as a valid PNG/JPEG/GIF: {image_path}", "error",
        )]
    arr = np.asarray(img)  # (H, W, 3) uint8

    issues = []
    if not skip_dark_check:
        issues += _check_light_mode(finding_path, image_path, arr)
    issues += _check_annotation_present(finding_path, image_path, arr)
    issues += _check_text_legibility(finding_path, image_path, arr)
    return issues


def _check_light_mode(finding_path: Path, image_path: Path, arr: np.ndarray) -> list[ValidationIssue]:
    h, w, _ = arr.shape
    my = max(1, int(h * LUMINANCE_SAMPLE_MARGIN_FRACTION))
    mx = max(1, int(w * LUMINANCE_SAMPLE_MARGIN_FRACTION))
    border_pixels = np.concatenate([
        arr[:my, :, :].reshape(-1, 3),
        arr[-my:, :, :].reshape(-1, 3),
        arr[:, :mx, :].reshape(-1, 3),
        arr[:, -mx:, :].reshape(-1, 3),
    ])
    luminance = border_pixels.astype(float) @ np.array([0.299, 0.587, 0.114])
    avg_luminance = float(luminance.mean())
    if avg_luminance < MIN_LUMINANCE_LIGHT_MODE:
        return [ValidationIssue(
            finding_path, "screenshot_dark_mode",
            f"Screenshot appears to be dark-mode (avg border luminance {avg_luminance:.1f}, "
            f"minimum {MIN_LUMINANCE_LIGHT_MODE}): {image_path}. Add a "
            f"'> [!OVERRIDE-DARK-IMG]' line directly above the image reference if this is "
            f"intentional.", "error",
        )]
    return []


def _check_annotation_present(finding_path: Path, image_path: Path, arr: np.ndarray) -> list[ValidationIssue]:
    r, g, b = arr[..., 0].astype(int), arr[..., 1].astype(int), arr[..., 2].astype(int)
    maxc = np.maximum(np.maximum(r, g), b)
    minc = np.minimum(np.minimum(r, g), b)
    saturation = np.where(maxc > 0, (maxc - minc) / np.maximum(maxc, 1), 0)
    red_orange_yellow = (maxc == r) & (r - b > 40)
    mask = red_orange_yellow & (saturation > 0.6) & (maxc > 128)
    count = int(mask.sum())
    if count < MIN_SATURATED_CLUSTER_PX:
        return [ValidationIssue(
            finding_path, "screenshot_no_annotation",
            f"No annotation marks (arrows/highlight boxes) detected — only {count} saturated "
            f"red/orange/yellow pixels found (minimum {MIN_SATURATED_CLUSTER_PX}): {image_path}",
            "error",
        )]
    return []


def _check_text_legibility(finding_path: Path, image_path: Path, arr: np.ndarray) -> list[ValidationIssue]:
    """Approximate typical text-line height via a horizontal ink-density row profile.

    A row counts as "textish" once enough of its pixels are dark relative to the image
    mean, and contiguous textish rows are run-length encoded into bands. Any single glyph
    touching a row is enough to mark it textish, so a band's height reflects a full line's
    vertical extent (union across every column in the line) rather than one column's
    incidental — usually partial — intersection with a curve, serif, or anti-aliasing edge.

    Not OCR. This is an approximation of visual text size, not a measurement of actual
    point size or DPI-correct legibility — treat any single estimate with skepticism,
    hence warning-level.
    """
    gray = arr.astype(float).mean(axis=2)
    threshold = gray.mean() * 0.6
    ink = gray < threshold
    if ink.sum() < 50:
        return []

    textish = ink.sum(axis=1) >= MIN_ROW_INK_PX
    diffs = np.diff(textish.astype(int))
    starts = np.where(diffs == 1)[0] + 1
    ends = np.where(diffs == -1)[0] + 1
    if textish[0]:
        starts = np.insert(starts, 0, 0)
    if textish[-1]:
        ends = np.append(ends, len(textish))
    band_heights = (ends - starts).tolist()

    plausible = [h for h in band_heights if 2 <= h <= 60]
    if not plausible:
        return []
    median_height = float(np.median(plausible))
    if median_height < MIN_TEXT_HEIGHT_PX:
        return [ValidationIssue(
            finding_path, "screenshot_text_too_small",
            f"Estimated text height ~{median_height:.1f}px is below the legibility threshold "
            f"({MIN_TEXT_HEIGHT_PX}px) — text may be too small to read at typical rendered "
            f"width. This is an approximate heuristic, not a precise measurement: {image_path}",
            "error",
        )]
    return []


# ── Check 4 — Missing CVSS field ────────────────────────────────────────────


def _check_cvss_present(path: Path, meta: dict) -> list[ValidationIssue]:
    # Mirrors preprocessor._enrich()'s fallback condition — keep in sync if that changes.
    has_vector = bool(str(meta.get("cvss_vector", "")).strip())
    has_manual = meta.get("severity") is not None or meta.get("cvss_score") is not None
    if not has_vector and not has_manual:
        return [ValidationIssue(
            path, "cvss_missing",
            "No CVSS data: missing both 'cvss_vector' and 'severity'/'cvss_score'.", "error",
        )]
    return []


# ── Check 5 — Missing per-finding timeline entry ────────────────────────────


def _check_timeline_present(path: Path, meta: dict) -> list[ValidationIssue]:
    entries = meta.get("timeline")
    if not entries or not isinstance(entries, list):
        return [ValidationIssue(
            path, "timeline_missing",
            "No 'timeline' entry in this finding's frontmatter.", "error",
        )]
    return []


# ── Check 6 — Unresolved [!REX-STUB] template markers ───────────────────────


def _check_stub_markers(path: Path, raw: str) -> list[ValidationIssue]:
    count = len(_STUB_MARKER.findall(raw))
    if count:
        return [ValidationIssue(
            path, "stub_marker_present",
            f"{count} unresolved '[!REX-STUB]' marker(s) — replace the stub "
            "value/section and remove the marker line before building.", "error",
        )]
    return []


# ── Check 7 — Leftover scaffold placeholder text ────────────────────────────


_LOREM_IPSUM = re.compile(r"lorem\s*ipsum", re.IGNORECASE)


def _check_placeholder_text(path: Path, meta: dict, body: str) -> list[ValidationIssue]:
    haystacks = [str(meta), body]
    issues = []
    for needle in _PLACEHOLDER_STRINGS:
        if needle == PLACEHOLDER_LOREM_IPSUM:
            # Any case, with or without the space: "Lorem ipsum dolor", "loremipsum.example" (#72).
            found = any(_LOREM_IPSUM.search(h) for h in haystacks)
        else:
            found = any(needle in h for h in haystacks)
        if found:
            issues.append(ValidationIssue(
                path, "placeholder_text",
                f"Scaffold placeholder text left in report: {needle!r}", "error",
            ))
    return issues


# ── Check 8 — Markdown that won't render as the author meant (#71) ─────────
#
# Text pasted from elsewhere often loses its markdown: indentation that turns a
# list into a code block, section headings that are plain lines, and leftover
# TODO notes. All warnings: each is sometimes intended. Mirrors validate.go's
# checkMarkdownStructure.

_FENCE_LINE = re.compile(r"^[ \t]*(```|~~~)")
_LIST_ITEM = re.compile(r"^[ \t]*(?:[-*+]|\d+[.)])[ \t]")
_INLINE_CODE = re.compile(r"`[^`]*`")
_TODO_MARKER = re.compile(r"\b(TODO|TBD|FIXME)\b")
_SECTION_NAMES = (
    "description", "impact", "steps to reproduce", "evidence",
    "recommendation", "remediation", "replication",
)


def _check_markdown_structure(path: Path, raw: str, *, is_report: bool) -> list[ValidationIssue]:
    issues: list[ValidationIssue] = []
    lines = raw.split("\n")
    start = 0
    if raw.startswith("---"):
        start = next((i + 1 for i in range(1, len(lines)) if lines[i].strip() == "---"), 0)
    in_fence = False
    prev = last_text = ""  # the previous line / the last non-blank line
    for n, line in enumerate(lines[start:], start + 1):
        if _FENCE_LINE.match(line):
            in_fence = not in_fence
        elif not in_fence and line.strip():
            indented = line.startswith(("    ", "\t"))
            if (indented and not prev.strip() and last_text
                    and not last_text.startswith((" ", "\t")) and not _LIST_ITEM.match(last_text)):
                issues.append(ValidationIssue(
                    path, "indented_code_block",
                    "Indented text renders as a monospace code block. Use a fenced ``` block "
                    "for code or diagrams, or remove the indentation for prose and lists.",
                    "warning", line=n,
                ))
            if is_report and not indented and line.strip().lower() in _SECTION_NAMES:
                issues.append(ValidationIssue(
                    path, "section_heading_plain_text",
                    f"'{line.strip()}' is a plain line, not a heading — write '## {line.strip()}'.",
                    "warning", line=n,
                ))
            if m := _TODO_MARKER.search(_INLINE_CODE.sub("", line)):
                issues.append(ValidationIssue(
                    path, "todo_marker",
                    f"'{m.group(1)}' note left in the text.", "warning", line=n,
                ))
        if not in_fence or _FENCE_LINE.match(line):
            prev = line
            if line.strip():
                last_text = line
    return issues


# ── Check 9 — Links to findings that aren't in the report (#72) ────────────


def _check_dangling_refs(path: Path, raw: str, finding_names: set[str]) -> list[ValidationIssue]:
    # `[text](vuln-2.md)` / `![](vuln-2.md)` become "Finding #N" links only when
    # vuln-2.md is one of structure.reports; otherwise they render as broken links.
    issues: list[ValidationIssue] = []
    in_fence = False
    for n, line in enumerate(raw.split("\n"), 1):
        if _FENCE_LINE.match(line):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        targets = [m.group(1) for m in _MD_IMG_REF.finditer(line)]
        targets += [m.group(2) for m in _MD_LINK_REF.finditer(line)]
        for target in targets:
            if Path(target).stem not in finding_names and Path(target).name not in finding_names:
                issues.append(ValidationIssue(
                    path, "dangling_reference",
                    f"Link to '{target}' doesn't match any finding in structure.reports — "
                    "it would render as a broken link.", "error", line=n,
                ))
    return issues


# ── Network-diagram artifacts (artifacts/assets.json, connections.json) ─────
#
# Lints the artifact JSON files the chart directives read, found by filename
# under artifacts/, against how the report's markdown files use them. Every
# issue carries a line so editors can point at the offending directive or JSON
# entry. Mirrors validate_network.go's checkNetworkArtifacts.


def _line_at(text: str, offset: int) -> int:
    return text.count("\n", 0, max(offset, 0)) + 1


def _json_line(raw: str, value: str) -> int:
    """The 1-based line a string value/key first appears on in raw JSON, or 0."""
    i = raw.find(json.dumps(value, ensure_ascii=False))
    return _line_at(raw, i) if i >= 0 else 0


def _decode_artifact(path: Path, raw: str, want_type: type, want: str):
    """(decoded value, None) — or (None, artifact_json_invalid issue)."""
    name = f"artifacts/{path.name}"
    try:
        value = json.loads(raw)
    except json.JSONDecodeError as exc:
        return None, ValidationIssue(
            path, "artifact_json_invalid", f"{name} is not valid JSON: {exc.msg}.", "error", exc.lineno
        )
    if not isinstance(value, want_type) or (
        want_type is list and not all(isinstance(a, dict) for a in value)
    ):
        return None, ValidationIssue(path, "artifact_json_invalid", f"{name} must be {want}.", "error")
    return value, None


def _check_network_artifacts(base_dir: Path, files: list[tuple[Path, str]]) -> list[ValidationIssue]:
    issues: list[ValidationIssue] = []
    artifacts = base_dir / "artifacts"

    # How the report files use the artifacts.
    chart_directives = (
        (_ASSET_GRAPH_DIRECTIVE, "asset_graph", "assets.json"),
        (_WEB_GRAPH_DIRECTIVE, "web_graph", "webfindings.json"),
    )
    uses_network = False
    for path, raw in files:
        for m in _NETWORK_DIRECTIVE.finditer(raw):
            uses_network = True
            if not (artifacts / "assets.json").exists():
                issues.append(ValidationIssue(
                    path, "network_assets_missing",
                    ":::network_diagram::: needs artifacts/assets.json, which doesn't exist — "
                    "run `rex ingest` first, or the build fails.",
                    "error", _line_at(raw, m.start()),
                ))
        for regex, name, artifact in chart_directives:
            if (artifacts / artifact).exists():
                continue
            for m in regex.finditer(raw):
                issues.append(ValidationIssue(
                    path, "chart_data_missing",
                    f':::{name}::: has no data — artifacts/{artifact} doesn\'t exist, so it renders "No data available".',
                    "warning", _line_at(raw, m.start()),
                ))

    # assets.json: every host needs a valid IP for the diagram to place it.
    inventory = None  # None when assets.json is absent or unreadable
    assets_path = artifacts / "assets.json"
    if assets_path.exists():
        raw = assets_path.read_text("utf-8")
        assets, issue = _decode_artifact(assets_path, raw, list, "a JSON array of host objects")
        if issue:
            issues.append(issue)
        else:
            inventory = set()
            for i, a in enumerate(assets, 1):
                ip_str = "" if a.get("ip") is None else str(a.get("ip"))
                try:
                    inventory.add(ipaddress.ip_address(ip_str.strip()))
                except ValueError:
                    issues.append(ValidationIssue(
                        assets_path, "assets_invalid",
                        f"Host #{i} has an invalid 'ip' {json.dumps(ip_str)} — the network diagram can't place it.",
                        "error", _json_line(raw, ip_str),
                    ))

    # connections.json: hand-written, so check every key and value.
    conn_path = artifacts / "connections.json"
    if not conn_path.exists():
        return issues
    raw = conn_path.read_text("utf-8")
    conn, issue = _decode_artifact(conn_path, raw, dict, "a JSON object")
    if issue:
        return issues + [issue]

    def invalid(at: str, msg: str) -> None:
        issues.append(ValidationIssue(conn_path, "connections_invalid", msg, "error", _json_line(raw, at)))

    def unknown_host(ip, s: str, key: str) -> None:
        if inventory is not None and ip not in inventory:
            issues.append(ValidationIssue(
                conn_path, "connections_host_unknown",
                f"{key}: {s} isn't in artifacts/assets.json — it'll be drawn as an unknown host. Typo?",
                "warning", _json_line(raw, s),
            ))

    def endpoint(s: str) -> None:
        try:
            e = _endpoint(s)
        except ValueError:
            invalid(s, f"pivots: {json.dumps(s)} is not a valid IP or CIDR.")
            return
        if not isinstance(e, (ipaddress.IPv4Network, ipaddress.IPv6Network)):
            unknown_host(e, s, "pivots")

    for key in sorted(conn):
        value = conn[key]
        if key == "subnets":
            if not isinstance(value, dict):
                invalid(key, "'subnets' must be an object mapping CIDR → label.")
                continue
            for cidr in sorted(value):
                try:
                    ipaddress.ip_network(cidr.strip(), strict=False)
                except ValueError:
                    invalid(cidr, f"subnets: {json.dumps(cidr)} is not a valid CIDR (e.g. 10.0.0.0/24).")
        elif key == "pivots":
            if not isinstance(value, dict):
                invalid(key, "'pivots' must be an object mapping source → target(s).")
                continue
            for src in sorted(value):
                endpoint(src)
                dst = value[src]
                if isinstance(dst, str):
                    endpoint(dst)
                elif isinstance(dst, list):
                    for d in dst:
                        if isinstance(d, str):
                            endpoint(d)
                        else:
                            invalid(src, f"pivots: every target of {json.dumps(src)} must be an IP/CIDR string.")
                else:
                    invalid(src, f"pivots: the target of {json.dumps(src)} must be an IP/CIDR string or a list of them.")
        elif key == "important":
            if not isinstance(value, list):
                invalid(key, "'important' must be a list of IPs.")
                continue
            for v in value:
                s = str(v)
                try:
                    ip = ipaddress.ip_address(s.strip())
                except ValueError:
                    invalid(s, f"important: {json.dumps(s)} is not a valid IP.")
                    continue
                unknown_host(ip, s, "important")
        else:
            issues.append(ValidationIssue(
                conn_path, "connections_key_unknown",
                f"Unknown key {json.dumps(key)} — expected 'subnets', 'pivots' or 'important'; it's ignored.",
                "warning", _json_line(raw, key),
            ))
    if not uses_network:
        issues.append(ValidationIssue(
            conn_path, "connections_unused",
            "No report file uses :::network_diagram:::, so this file has no effect.", "warning",
        ))
    return issues
