"""Tests for the QA-gate validator (validator.py)."""

import hashlib
from pathlib import Path

import numpy as np
from PIL import Image

from report_rex_core import builder, validator
from report_rex_core.parser import parse_index
from report_rex_core.validator import (
    ValidationResult,
    _check_cvss_present,
    _check_markdown_structure,
    _check_placeholder_screenshot,
    _check_placeholder_text,
    _check_screenshot_image,
    _check_required_metadata,
    _check_screenshot_present,
    _check_screenshots,
    _check_steps_to_reproduce,
    _check_stub_markers,
    _check_timeline_present,
    validate_report,
)

FIXTURE_DARK_IMAGE = Path(__file__).parent / "fixtures" / "screenshots" / "dark-terminal-example.png"

COMPLIANT_CAPTION = "A sufficiently long caption describing the evidence shown"
COMPLIANT_STEPS = (
    "1. Navigate to the login page and open the request in an intercepting proxy.\n"
    "2. Replace the username parameter with a payload and forward the request.\n"
    "3. Observe that authentication is bypassed entirely.\n"
)
COMPLIANT_TIMELINE = "timeline:\n  - date: \"2024-01-12T14:20:00+00:00\"\n    description: \"Found it\"\n    include: true\n"


def _make_png(path: Path, arr: np.ndarray) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    Image.fromarray(arr.astype(np.uint8), "RGB").save(path)
    return path


def _light_background(h=200, w=200) -> np.ndarray:
    return np.full((h, w, 3), 240, dtype=np.uint8)


def _dark_background(h=200, w=200) -> np.ndarray:
    return np.full((h, w, 3), 20, dtype=np.uint8)


def _with_annotation(arr: np.ndarray) -> np.ndarray:
    arr = arr.copy()
    arr[50:80, 50:130] = [255, 60, 0]  # 30*80 = 2400px red rectangle > MIN_SATURATED_CLUSTER_PX
    return arr


def _compliant_screenshot(path: Path) -> Path:
    return _make_png(path, _with_annotation(_light_background()))


def _staggered_text(h=100, w=200) -> np.ndarray:
    """Simulates real anti-aliased/serif text: each column has a short (~5px) ink run,
    staggered vertically by column so no single column spans the full ~50px line height —
    only the union across columns does. The old median-of-per-column-runs algorithm
    measured ~5px here (a false "too small" positive); row-banding should measure the
    full ~50px band instead."""
    arr = _light_background(h, w).copy()
    for x in range(w):
        start = 40 + (x % 50)
        arr[start:start + 5, x] = [20, 20, 20]
    return arr


def _uniform_small_text(h=100, w=200) -> np.ndarray:
    """A genuinely small ~6px ink band present in every column — a regression check that
    the fix still flags real small text, not just staggered text."""
    arr = _light_background(h, w).copy()
    arr[40:46, :] = [20, 20, 20]
    return arr


class TestStepsToReproduce:
    def test_missing_section_is_error(self):
        body = "## Description\n\nSomething happened.\n"
        issues = _check_steps_to_reproduce(Path("vuln.md"), body)
        assert len(issues) == 1
        assert issues[0].check == "steps_to_reproduce_missing"

    def test_under_100_chars_is_error(self):
        body = "## Steps to Reproduce\n\nToo short.\n"
        issues = _check_steps_to_reproduce(Path("vuln.md"), body)
        assert len(issues) == 1
        assert issues[0].check == "steps_to_reproduce_length"
        assert issues[0].severity == "error"

    def test_exactly_100_chars_passes(self):
        text = "x" * 100
        body = f"## Steps to Reproduce\n\n{text}\n\n## Evidence\n\nSomething else.\n"
        issues = _check_steps_to_reproduce(Path("vuln.md"), body)
        assert issues == []

    def test_over_100_chars_passes(self):
        body = f"## Steps to Reproduce\n\n{COMPLIANT_STEPS}\n"
        issues = _check_steps_to_reproduce(Path("vuln.md"), body)
        assert issues == []

    def test_replication_heading_is_checked(self):
        # #68: `## Replication` (steps + evidence) is the scaffolds' heading now.
        assert _check_steps_to_reproduce(Path("v.md"), f"## Replication\n\n{COMPLIANT_STEPS}\n") == []
        issues = _check_steps_to_reproduce(Path("v.md"), "## Replication\n\nToo short.\n")
        assert [i.check for i in issues] == ["steps_to_reproduce_length"]
        assert "'Replication'" in issues[0].message


class TestScreenshotCaption:
    def test_caption_under_35_chars_is_error(self, tmp_path: Path):
        _compliant_screenshot(tmp_path / "assets" / "screenshots" / "x.png")
        body = "![Short caption](../assets/screenshots/x.png)\n"
        issues = _check_screenshots(tmp_path / "reports" / "vuln.md", body, tmp_path)
        captions = [i for i in issues if i.check == "screenshot_caption_length"]
        assert len(captions) == 1

    def test_caption_35_chars_or_more_passes(self, tmp_path: Path):
        _compliant_screenshot(tmp_path / "assets" / "screenshots" / "x.png")
        body = f"![{COMPLIANT_CAPTION}](../assets/screenshots/x.png)\n"
        issues = _check_screenshots(tmp_path / "reports" / "vuln.md", body, tmp_path)
        assert not [i for i in issues if i.check == "screenshot_caption_length"]

    def test_caption_with_pandoc_width_attr_still_parsed(self, tmp_path: Path):
        _compliant_screenshot(tmp_path / "assets" / "screenshots" / "x.png")
        body = f"![{COMPLIANT_CAPTION}](../assets/screenshots/x.png){{width=90%}}\n"
        issues = _check_screenshots(tmp_path / "reports" / "vuln.md", body, tmp_path)
        assert not [i for i in issues if i.check == "screenshot_caption_length"]
        assert not [i for i in issues if i.check == "screenshot_missing"]


class TestScreenshotPresence:
    def test_no_screenshot_is_error(self):
        issues = _check_screenshot_present(Path("vuln.md"), "## Description\n\nNo image here.\n")
        assert [i.check for i in issues] == ["screenshot_none"]

    def test_screenshot_present_passes(self):
        body = f"![{COMPLIANT_CAPTION}](../assets/screenshots/x.png)\n"
        assert _check_screenshot_present(Path("vuln.md"), body) == []


class TestRequiredMetadata:
    def test_missing_fields_are_errors(self):
        issues = _check_required_metadata(Path("index.md"), {"client": "Acme", "date": ""})
        assert [i.check for i in issues] == ["metadata_missing", "metadata_missing"]

    def test_all_present_passes(self):
        meta = {"title": "T", "client": "Acme", "date": "1970-01-01"}
        assert _check_required_metadata(Path("index.md"), meta) == []


class TestCvssPresence:
    def test_missing_both_vector_and_severity_is_error(self):
        issues = _check_cvss_present(Path("vuln.md"), {})
        assert len(issues) == 1
        assert "cvss" in issues[0].check

    def test_cvss_vector_present_passes(self):
        issues = _check_cvss_present(
            Path("vuln.md"), {"cvss_vector": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"}
        )
        assert issues == []

    def test_manual_severity_present_passes(self):
        issues = _check_cvss_present(Path("vuln.md"), {"severity": "High"})
        assert issues == []

    def test_manual_cvss_score_present_passes(self):
        issues = _check_cvss_present(Path("vuln.md"), {"cvss_score": 7.5})
        assert issues == []


class TestTimelinePresence:
    def test_missing_timeline_is_error(self):
        issues = _check_timeline_present(Path("vuln.md"), {})
        assert len(issues) == 1
        assert issues[0].check == "timeline_missing"

    def test_empty_timeline_list_is_error(self):
        issues = _check_timeline_present(Path("vuln.md"), {"timeline": []})
        assert len(issues) == 1

    def test_timeline_present_passes(self):
        issues = _check_timeline_present(
            Path("vuln.md"),
            {"timeline": [{"date": "2024-01-01T00:00:00+00:00", "description": "x"}]},
        )
        assert issues == []


class TestPlaceholderText:
    def test_scaffolded_report_fails_validate(self, tmp_path: Path):
        target = tmp_path / "scaffolded"
        builder.scaffold(target, "rex-default")
        structure = parse_index(target / "index.md")
        result = validate_report(structure)
        placeholder_checks = {i.check for i in result.issues if i.check == "placeholder_text"}
        assert "placeholder_text" in placeholder_checks
        messages = " ".join(i.message for i in result.issues if i.check == "placeholder_text")
        assert builder.PLACEHOLDER_VULN_TITLE in messages
        assert builder.PLACEHOLDER_DESCRIPTION in messages
        assert builder.PLACEHOLDER_IMPACT in messages
        assert builder.PLACEHOLDER_RECOMMENDATION in messages

    def test_edited_report_without_placeholders_passes(self):
        meta = {"title": "Real Finding Title"}
        body = "## Description\n\nA real, specific description of the issue.\n"
        issues = _check_placeholder_text(Path("vuln.md"), meta, body)
        assert issues == []

    def test_partial_placeholder_leftover_is_caught(self):
        meta = {"title": "Real Finding Title"}
        body = (
            "## Description\n\nA real, specific description.\n\n"
            f"## Recommendation\n\n{builder.PLACEHOLDER_RECOMMENDATION}\n"
        )
        issues = _check_placeholder_text(Path("vuln.md"), meta, body)
        assert len(issues) == 1
        assert builder.PLACEHOLDER_RECOMMENDATION in issues[0].message

    def test_placeholder_in_non_title_metadata_field_is_caught(self):
        """A sentinel left in any metadata field (not just 'title') must be caught —
        this is what makes index.md's own frontmatter (client/contact_name/dates,
        none of which are 'title') checkable at all."""
        meta = {"title": "Real Finding Title", "contact_email": builder.PLACEHOLDER_CONTACT_EMAIL}
        issues = _check_placeholder_text(Path("index.md"), meta, "")
        assert len(issues) == 1
        assert builder.PLACEHOLDER_CONTACT_EMAIL in issues[0].message

    def test_epoch_date_and_lorem_ipsum_constants_are_registered(self):
        meta = {"title": "Real Finding Title", "date": f"{builder.PLACEHOLDER_EPOCH_DATE}T00:00:00+00:00"}
        body = f"Some real text. {builder.PLACEHOLDER_LOREM_IPSUM} dolor sit amet."
        issues = _check_placeholder_text(Path("index.md"), meta, body)
        checked = {i.message for i in issues}
        assert any(builder.PLACEHOLDER_EPOCH_DATE in m for m in checked)
        assert any(builder.PLACEHOLDER_LOREM_IPSUM in m for m in checked)

    def test_cover_template_defaults_are_registered(self):
        # #74: the example's report_id and ACME logo shipped on a real cover.
        meta = {"report_id": "RR-2024-0142", "client_logo": "assets/acme-logo.png"}
        checked = {i.message for i in _check_placeholder_text(Path("index.md"), meta, "")}
        assert any("RR-2024-0142" in m for m in checked)
        assert any(builder.PLACEHOLDER_CLIENT_LOGO in m for m in checked)


class TestTestingWindow:
    def test_start_without_end_warns(self):
        issues = validator._check_testing_window(Path("index.md"), {"start_date": "2026-09-27"})
        assert [(i.check, i.severity) for i in issues] == [("testing_window_incomplete", "warning")]
        assert "end_date" in issues[0].message

    def test_both_or_neither_is_fine(self):
        assert validator._check_testing_window(Path("i.md"), {}) == []
        assert validator._check_testing_window(
            Path("i.md"), {"start_date": "2026-09-27", "end_date": "2026-09-28"}) == []


class TestScreenshotImageHeuristics:
    def test_dark_mode_image_is_error(self, tmp_path: Path):
        img = _make_png(tmp_path / "dark.png", _dark_background())
        issues = _check_screenshot_image(Path("vuln.md"), img)
        assert any(i.check == "screenshot_dark_mode" for i in issues)

    def test_light_mode_image_passes_luminance_check(self, tmp_path: Path):
        img = _make_png(tmp_path / "light.png", _light_background())
        issues = _check_screenshot_image(Path("vuln.md"), img)
        assert not any(i.check == "screenshot_dark_mode" for i in issues)
        assert any(i.check == "screenshot_no_annotation" for i in issues)

    def test_no_annotation_is_error(self, tmp_path: Path):
        img = _make_png(tmp_path / "plain.png", _light_background())
        issues = _check_screenshot_image(Path("vuln.md"), img)
        assert any(i.check == "screenshot_no_annotation" for i in issues)

    def test_annotation_present_passes(self, tmp_path: Path):
        img = _compliant_screenshot(tmp_path / "annotated.png")
        issues = _check_screenshot_image(Path("vuln.md"), img)
        assert not any(i.check == "screenshot_no_annotation" for i in issues)

    def test_corrupt_image_is_error(self, tmp_path: Path):
        path = tmp_path / "corrupt.png"
        path.write_bytes(b"not a real png")
        issues = _check_screenshot_image(Path("vuln.md"), path)
        assert len(issues) == 1
        assert issues[0].check == "screenshot_unreadable"

    def test_text_legibility_issue_is_severity_error(self, tmp_path: Path):
        img = _make_png(tmp_path / "small.png", _uniform_small_text())
        issues = _check_screenshot_image(Path("vuln.md"), img)
        legibility = [i for i in issues if i.check == "screenshot_text_too_small"]
        assert legibility
        assert legibility[0].severity == "error"
        result = ValidationResult(issues=legibility)
        assert not result.ok

    def test_text_legibility_staggered_columns_does_not_falsely_flag(self, tmp_path: Path):
        img = _make_png(tmp_path / "staggered.png", _staggered_text())
        issues = _check_screenshot_image(Path("vuln.md"), img)
        assert not any(i.check == "screenshot_text_too_small" for i in issues)

    def test_text_legibility_uniform_small_text_still_flagged(self, tmp_path: Path):
        img = _make_png(tmp_path / "small.png", _uniform_small_text())
        issues = _check_screenshot_image(Path("vuln.md"), img)
        assert any(i.check == "screenshot_text_too_small" for i in issues)

    def test_real_dark_terminal_screenshot_is_error(self):
        issues = _check_screenshot_image(Path("vuln.md"), FIXTURE_DARK_IMAGE)
        assert any(i.check == "screenshot_dark_mode" for i in issues)

    def test_dark_override_marker_suppresses_dark_check(self, tmp_path: Path):
        fixture_copy = tmp_path / "assets" / "screenshots" / "dark-terminal-example.png"
        fixture_copy.parent.mkdir(parents=True, exist_ok=True)
        fixture_copy.write_bytes(FIXTURE_DARK_IMAGE.read_bytes())
        body = (
            "> [!OVERRIDE-DARK-IMG]\n"
            f"![{COMPLIANT_CAPTION}](../assets/screenshots/dark-terminal-example.png)\n"
        )
        issues = _check_screenshots(tmp_path / "reports" / "vuln.md", body, tmp_path)
        assert not any(i.check == "screenshot_dark_mode" for i in issues)

    def test_dark_override_marker_only_applies_to_its_own_image(self, tmp_path: Path):
        fixture_copy = tmp_path / "assets" / "screenshots" / "dark-terminal-example.png"
        fixture_copy.parent.mkdir(parents=True, exist_ok=True)
        fixture_copy.write_bytes(FIXTURE_DARK_IMAGE.read_bytes())
        _make_png(tmp_path / "assets" / "screenshots" / "dark2.png", _dark_background())
        body = (
            "> [!OVERRIDE-DARK-IMG]\n"
            f"![{COMPLIANT_CAPTION}](../assets/screenshots/dark-terminal-example.png)\n\n"
            f"![{COMPLIANT_CAPTION} two](../assets/screenshots/dark2.png)\n"
        )
        issues = _check_screenshots(tmp_path / "reports" / "vuln.md", body, tmp_path)
        dark_issues = [i for i in issues if i.check == "screenshot_dark_mode"]
        assert len(dark_issues) == 1
        assert "dark2.png" in dark_issues[0].message

    def test_override_marker_with_blank_line_before_image_still_applies(self, tmp_path: Path):
        fixture_copy = tmp_path / "assets" / "screenshots" / "dark-terminal-example.png"
        fixture_copy.parent.mkdir(parents=True, exist_ok=True)
        fixture_copy.write_bytes(FIXTURE_DARK_IMAGE.read_bytes())
        body = (
            "> [!OVERRIDE-DARK-IMG]\n\n"
            f"![{COMPLIANT_CAPTION}](../assets/screenshots/dark-terminal-example.png)\n"
        )
        issues = _check_screenshots(tmp_path / "reports" / "vuln.md", body, tmp_path)
        assert not any(i.check == "screenshot_dark_mode" for i in issues)

    def test_override_marker_not_immediately_above_does_not_apply(self, tmp_path: Path):
        fixture_copy = tmp_path / "assets" / "screenshots" / "dark-terminal-example.png"
        fixture_copy.parent.mkdir(parents=True, exist_ok=True)
        fixture_copy.write_bytes(FIXTURE_DARK_IMAGE.read_bytes())
        body = (
            "> [!OVERRIDE-DARK-IMG]\n\n"
            "Some other paragraph in between.\n\n"
            f"![{COMPLIANT_CAPTION}](../assets/screenshots/dark-terminal-example.png)\n"
        )
        issues = _check_screenshots(tmp_path / "reports" / "vuln.md", body, tmp_path)
        assert any(i.check == "screenshot_dark_mode" for i in issues)


class TestPlaceholderScreenshot:
    def test_known_hash_is_flagged(self, tmp_path: Path, monkeypatch):
        img = _make_png(tmp_path / "demo.png", _with_annotation(_light_background()))
        digest = hashlib.sha256(img.read_bytes()).hexdigest()
        monkeypatch.setattr(validator, "_PLACEHOLDER_SCREENSHOT_SHA256", digest)
        issues = _check_placeholder_screenshot(Path("vuln.md"), img)
        assert len(issues) == 1
        assert issues[0].check == "screenshot_placeholder_image"

    def test_different_image_passes(self, tmp_path: Path, monkeypatch):
        img = _make_png(tmp_path / "real.png", _with_annotation(_light_background()))
        monkeypatch.setattr(validator, "_PLACEHOLDER_SCREENSHOT_SHA256", "0" * 64)
        issues = _check_placeholder_screenshot(Path("vuln.md"), img)
        assert issues == []


def _write_compliant_report(tmp_path: Path, **vuln_overrides) -> Path:
    """Write a fully-compliant report tree; vuln_overrides values override the baseline
    finding frontmatter/body: 'frontmatter_extra' (str appended to vuln-1.md frontmatter),
    'body' (str, replaces the default body), 'screenshot_arr' (np.ndarray for the finding's
    screenshot), 'image_ref_extra' (str prepended directly before the image markdown line)."""
    (tmp_path / "sections").mkdir()
    (tmp_path / "reports").mkdir()
    (tmp_path / "appendices").mkdir()
    (tmp_path / "assets" / "screenshots").mkdir(parents=True)

    screenshot_arr = vuln_overrides.get("screenshot_arr", _with_annotation(_light_background()))
    _make_png(tmp_path / "assets" / "screenshots" / "evidence.png", screenshot_arr)

    (tmp_path / "sections" / "exec_summ.md").write_text(
        "---\ntitle: Executive Summary\n---\n\nA real summary of the engagement.\n"
    )

    default_body = (
        "## Description\n\nA real, specific description of the SQL injection issue found "
        "in the login endpoint.\n\n"
        f"## Steps to Reproduce\n\n{COMPLIANT_STEPS}\n\n"
        "## Evidence\n\n"
        f"{vuln_overrides.get('image_ref_extra', '')}"
        f"![{COMPLIANT_CAPTION}](../assets/screenshots/evidence.png)\n\n"
        "## Recommendation\n\nUse parameterized queries for all database access.\n"
    )
    body = vuln_overrides.get("body", default_body)

    frontmatter = (
        "---\n"
        "title: SQL Injection\n"
        'cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"\n'
        f"{COMPLIANT_TIMELINE}"
        f"{vuln_overrides.get('frontmatter_extra', '')}"
        "---\n\n"
    )
    (tmp_path / "reports" / "vuln-1.md").write_text(frontmatter + body)

    (tmp_path / "appendices" / "1.md").write_text(
        "---\ntitle: Appendix A\n---\n\nScope list.\n"
    )

    (tmp_path / "index.md").write_text(
        "---\n"
        "title: Test Report\n"
        "client: ACME Corp\n"
        "date: \"2024-01-01\"\n"
        "template: rex-default\n"
        "structure:\n"
        "  sections:\n"
        "    - sections/exec_summ.md\n"
        "  reports:\n"
        "    - reports/vuln-1.md\n"
        "  appendices:\n"
        "    - appendices/1.md\n"
        "---\n"
    )
    return tmp_path


class TestStubMarkers:
    def test_no_markers_passes(self):
        raw = "---\ntitle: Real Finding\n---\n\n## Description\n\nReal content.\n"
        assert _check_stub_markers(Path("vuln.md"), raw) == []

    def test_body_blockquote_marker_is_detected(self):
        raw = "## Description\n\n> [!REX-STUB]\n[Generic description.]\n"
        issues = _check_stub_markers(Path("vuln.md"), raw)
        assert len(issues) == 1
        assert issues[0].check == "stub_marker_present"
        assert issues[0].severity == "error"

    def test_yaml_comment_marker_in_frontmatter_is_detected(self):
        raw = '---\n# > [!REX-STUB]\ntitle: "[STUB] Generic Title"\n---\n\nBody.\n'
        issues = _check_stub_markers(Path("vuln.md"), raw)
        assert len(issues) == 1
        assert issues[0].check == "stub_marker_present"

    def test_multiple_markers_counted_in_one_issue(self):
        raw = (
            '---\n# > [!REX-STUB]\ntitle: "[STUB]"\n---\n\n'
            "> [!REX-STUB]\n[Generic body.]\n"
        )
        issues = _check_stub_markers(Path("vuln.md"), raw)
        assert len(issues) == 1
        assert "2 unresolved" in issues[0].message

    def test_validate_report_surfaces_stub_marker_for_copied_template(self, tmp_path: Path):
        report_dir = _write_compliant_report(
            tmp_path,
            frontmatter_extra='# > [!REX-STUB]\ncomponent: "[TARGET COMPONENT]"\n',
        )
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert "stub_marker_present" in checks


class TestValidateReport:
    def test_clean_report_has_no_issues(self, tmp_path: Path):
        report_dir = _write_compliant_report(tmp_path)
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        assert result.issues == []

    def test_multiple_findings_aggregate_across_files(self, tmp_path: Path):
        report_dir = _write_compliant_report(tmp_path)
        (report_dir / "reports" / "vuln-1.md").write_text(
            "---\ntitle: Finding One\n---\n\n## Description\n\nShort.\n"
        )
        (report_dir / "reports" / "vuln-2.md").write_text(
            "---\ntitle: Finding Two\ncvss_score: 5.0\n---\n\n"
            f"## Steps to Reproduce\n\n{COMPLIANT_STEPS}\n"
        )
        index_content = (report_dir / "index.md").read_text()
        index_content = index_content.replace(
            "    - reports/vuln-1.md\n", "    - reports/vuln-1.md\n    - reports/vuln-2.md\n"
        )
        (report_dir / "index.md").write_text(index_content)

        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        files = {i.file for i in result.issues}
        assert len(files) == 2

    def test_all_issues_reported_not_just_first(self, tmp_path: Path):
        report_dir = _write_compliant_report(tmp_path)
        (report_dir / "reports" / "vuln-1.md").write_text(
            "---\ntitle: Finding\n---\n\n## Description\n\nToo short.\n"
        )
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert "steps_to_reproduce_missing" in checks
        assert "cvss_missing" in checks
        assert "timeline_missing" in checks
        assert len(result.issues) >= 3

    def test_index_md_metadata_placeholder_is_caught(self, tmp_path: Path):
        """index.md's own frontmatter is otherwise never opened as a target file by
        validate_report — this is the regression test proving it now is."""
        report_dir = _write_compliant_report(tmp_path)
        index_content = (report_dir / "index.md").read_text()
        index_content = index_content.replace("client: ACME Corp", f"client: {builder.PLACEHOLDER_LOREM_IPSUM} Holdings")
        (report_dir / "index.md").write_text(index_content)
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        placeholder_issues = [i for i in result.issues if i.check == "placeholder_text"]
        assert any(str(i.file).endswith("index.md") for i in placeholder_issues)


class TestBadReportFixturesEndToEnd:
    """One full, realistic report directory per check, run through the real
    parse_index() -> validate_report() path, proving each check fires against an
    otherwise-plausible, fully-compliant report."""

    def test_bad_report_short_steps_to_reproduce(self, tmp_path: Path):
        report_dir = _write_compliant_report(
            tmp_path,
            body=(
                "## Description\n\nA real, specific description of the issue.\n\n"
                "## Steps to Reproduce\n\nClicked a button.\n\n"
                "## Evidence\n\n"
                f"![{COMPLIANT_CAPTION}](../assets/screenshots/evidence.png)\n\n"
                "## Recommendation\n\nFix it properly with parameterized queries.\n"
            ),
        )
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert checks == {"steps_to_reproduce_length"}

    def test_bad_report_short_caption(self, tmp_path: Path):
        report_dir = _write_compliant_report(
            tmp_path,
            body=(
                "## Description\n\nA real, specific description of the issue.\n\n"
                f"## Steps to Reproduce\n\n{COMPLIANT_STEPS}\n\n"
                "## Evidence\n\n"
                "![bug](../assets/screenshots/evidence.png)\n\n"
                "## Recommendation\n\nFix it properly with parameterized queries.\n"
            ),
        )
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert checks == {"screenshot_caption_length"}

    def test_bad_report_missing_cvss(self, tmp_path: Path):
        report_dir = _write_compliant_report(tmp_path)
        content = (report_dir / "reports" / "vuln-1.md").read_text()
        content = content.replace(
            'cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"\n', ""
        )
        (report_dir / "reports" / "vuln-1.md").write_text(content)
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert checks == {"cvss_missing"}

    def test_bad_report_missing_timeline(self, tmp_path: Path):
        report_dir = _write_compliant_report(tmp_path)
        content = (report_dir / "reports" / "vuln-1.md").read_text()
        content = content.replace(COMPLIANT_TIMELINE, "")
        (report_dir / "reports" / "vuln-1.md").write_text(content)
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert checks == {"timeline_missing"}

    def test_bad_report_leftover_placeholder_text(self, tmp_path: Path):
        report_dir = _write_compliant_report(
            tmp_path,
            body=(
                "## Description\n\nA real, specific description of the issue.\n\n"
                f"## Steps to Reproduce\n\n{COMPLIANT_STEPS}\n\n"
                "## Evidence\n\n"
                f"![{COMPLIANT_CAPTION}](../assets/screenshots/evidence.png)\n\n"
                f"## Recommendation\n\n{builder.PLACEHOLDER_RECOMMENDATION}\n"
            ),
        )
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert checks == {"placeholder_text"}

    def test_bad_report_dark_screenshot_no_override(self, tmp_path: Path):
        report_dir = _write_compliant_report(tmp_path)
        (report_dir / "assets" / "screenshots" / "evidence.png").write_bytes(
            FIXTURE_DARK_IMAGE.read_bytes()
        )
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert "screenshot_dark_mode" in checks

    def test_bad_report_dark_screenshot_with_override_passes_dark_check(self, tmp_path: Path):
        report_dir = _write_compliant_report(
            tmp_path, image_ref_extra="> [!OVERRIDE-DARK-IMG]\n",
        )
        (report_dir / "assets" / "screenshots" / "evidence.png").write_bytes(
            FIXTURE_DARK_IMAGE.read_bytes()
        )
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert "screenshot_dark_mode" not in checks

    def test_bad_report_no_annotation_on_screenshot(self, tmp_path: Path):
        report_dir = _write_compliant_report(tmp_path, screenshot_arr=_light_background())
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        assert "screenshot_no_annotation" in checks

    def test_bad_report_all_checks_simultaneously(self, tmp_path: Path):
        report_dir = _write_compliant_report(
            tmp_path,
            body=(
                "## Description\n\nA real description.\n\n"
                "## Steps to Reproduce\n\nToo short.\n\n"
                "## Evidence\n\n"
                "![bug](../assets/screenshots/evidence.png)\n\n"
                f"## Recommendation\n\n{builder.PLACEHOLDER_RECOMMENDATION}\n"
            ),
            screenshot_arr=_light_background(),
        )
        content = (report_dir / "reports" / "vuln-1.md").read_text()
        content = content.replace(
            'cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:N/A:N"\n', ""
        ).replace(COMPLIANT_TIMELINE, "")
        (report_dir / "reports" / "vuln-1.md").write_text(content)
        structure = parse_index(report_dir / "index.md")
        result = validate_report(structure)
        checks = {i.check for i in result.issues}
        expected = {
            "steps_to_reproduce_length",
            "screenshot_caption_length",
            "cvss_missing",
            "timeline_missing",
            "placeholder_text",
            "screenshot_no_annotation",
        }
        assert checks == expected


class TestFindingFrontmatter:
    """Issue #50: a finding with a missing title or broken YAML rendered as 'Untitled'."""

    def test_missing_title_is_error(self, sample_report: Path):
        (sample_report / "reports" / "vuln-1.md").write_text(
            "---\nseverity: High\n---\n\n## Description\n\nx\n"
        )
        result = validate_report(parse_index(sample_report / "index.md"))
        assert any(i.check == "title_missing" for i in result.errors)

    def test_invalid_yaml_is_reported_with_filename(self, sample_report: Path):
        (sample_report / "reports" / "vuln-1.md").write_text(
            '---\ntitle: X\nmaterial_impact: "Unclosed\nbusiness_risk:\n  - Operational\n'
            'affected_urls:\n  - "http://a"\n---\n\nBody\n'
        )
        result = validate_report(parse_index(sample_report / "index.md"))
        bad = [i for i in result.errors if i.check == "frontmatter_invalid"]
        assert len(bad) == 1
        assert "vuln-1.md" in bad[0].message

    def test_frontmatter_not_at_top_is_error(self, sample_report: Path):
        (sample_report / "reports" / "vuln-1.md").write_text("\n---\ntitle: X\n---\n\nBody\n")
        result = validate_report(parse_index(sample_report / "index.md"))
        bad = [i for i in result.errors if i.check == "frontmatter_not_at_top"]
        assert len(bad) == 1 and bad[0].line == 2


# ── Network-diagram artifacts ───────────────────────────────────────────────
# Same cases as validate_network_test.go, so both validators stay in parity.


def _network_fixture(tmp_path: Path, files: dict[str, str]) -> Path:
    (tmp_path / "artifacts").mkdir()
    for name, content in files.items():
        (tmp_path / "artifacts" / name).write_text(content)
    return tmp_path


def _by_check(issues):
    return {i.check: i for i in issues}


class TestCheckNetworkArtifacts:
    def test_directive_usage(self, tmp_path):
        base = _network_fixture(tmp_path, {})
        md = (Path("sections/m.md"), "---\ntitle: M\n---\n\nIntro.\n\n:::network_diagram:::\n\n:::asset_graph:pie:os:::\n")
        got = _by_check(validator._check_network_artifacts(base, [md]))
        assert (got["network_assets_missing"].severity, got["network_assets_missing"].line) == ("error", 7)
        assert got["network_assets_missing"].file == md[0]
        assert (got["chart_data_missing"].severity, got["chart_data_missing"].line) == ("warning", 9)

    def test_clean_report_has_no_issues(self, tmp_path):
        base = _network_fixture(tmp_path, {
            "assets.json": '[{"ip": "10.0.0.5"}, {"ip": "10.0.0.9"}]',
            "connections.json": '{"subnets": {"10.0.0.0/24": "LAN"}, '
                                '"pivots": {"10.0.0.5": ["10.0.0.9", "10.9.0.0/16"]}, "important": ["10.0.0.9"]}',
        })
        md = (Path("a.md"), ":::network_diagram:::\n:::asset_graph:bar:ports:::\n")
        assert validator._check_network_artifacts(base, [md]) == []

    def test_connections_problems(self, tmp_path):
        base = _network_fixture(tmp_path, {
            "assets.json": '[\n  {"ip": "10.0.0.5"},\n  {"ip": "not-an-ip"}\n]',
            "connections.json": '{\n'
                                '  "subnets": {"10.0.0.0/33": "Bad"},\n'
                                '  "pivots": {\n'
                                '    "10.0.0.5": "10.0.0.77",\n'
                                '    "10.0.0.5/24": "nope"\n'
                                '  },\n'
                                '  "pivot": {}\n'
                                '}',
        })
        issues = validator._check_network_artifacts(base, [])
        got = _by_check(issues)
        assert (got["assets_invalid"].severity, got["assets_invalid"].line) == ("error", 3)
        assert (got["connections_host_unknown"].severity, got["connections_host_unknown"].line) == ("warning", 4)
        assert (got["connections_key_unknown"].severity, got["connections_key_unknown"].line) == ("warning", 7)
        assert "connections_unused" in got
        invalid = [i for i in issues if i.check == "connections_invalid"]
        assert len(invalid) == 2  # the /33 CIDR and the "nope" target
        assert all(i.line for i in invalid)

    def test_invalid_json(self, tmp_path):
        base = _network_fixture(tmp_path, {
            "assets.json": '{"ip": "10.0.0.5"}',
            "connections.json": '{\n  "subnets": {\n    "10.0.0.0/24": "LAN",\n  }\n}',
        })
        issues = validator._check_network_artifacts(base, [(Path("a.md"), ":::network_diagram:::\n")])
        assert [i.check for i in issues] == ["artifact_json_invalid", "artifact_json_invalid"]
        assert all(i.severity == "error" for i in issues)
        # Python 3.13+ points at the trailing comma (line 3); older versions, like Go,
        # at the closing brace where parsing stopped (line 4).
        assert issues[1].line in (3, 4)


# ── Check 8 — Markdown structure (#71) ──────────────────────────────────────

_PASTED_FINDING = """---
title: "XSS"
---

## Recommendation

Untrusted data.
1. HTML-encode the note

TODO: confirm the cookie ID trick

Evidence
![shot](x.png)

Verify whether the note is rendered in:

    Shop/order views.

    Checkout pages.

1. Step one

    ```bash
    curl x
    ```

Mention `TODO` in code is fine.

```
TODO inside a fence is fine
    and so is indentation
```
"""


def test_markdown_structure_flags_pasted_text():
    issues = _check_markdown_structure(Path("v.md"), _PASTED_FINDING, is_report=True)
    assert [(i.check, i.line, i.severity) for i in issues] == [
        ("todo_marker", 10, "warning"),
        ("section_heading_plain_text", 12, "warning"),
        ("indented_code_block", 17, "warning"),
    ]


def test_markdown_structure_plain_heading_only_checked_in_findings():
    issues = _check_markdown_structure(Path("s.md"), "Evidence\n", is_report=False)
    assert issues == []



# ── Check 9 — Dangling finding links (#72) ──────────────────────────────────


def test_dangling_finding_links_are_errors():
    raw = "See [SQLi](./vuln-1.md) and [XSS](vuln-9.md).\n![](vuln-8.md)\n```\n[x](vuln-7.md)\n```\n"
    issues = validator._check_dangling_refs(Path("s.md"), raw, {"vuln-1", "vuln-1.md"})
    assert [(i.check, i.line, i.severity) for i in issues] == [
        ("dangling_reference", 1, "error"), ("dangling_reference", 2, "error"),
    ]
    assert "vuln-9.md" in issues[0].message


def test_lorem_ipsum_matched_in_any_case():
    issues = _check_placeholder_text(Path("s.md"), {}, "Lorem ipsum dolor at portal.loremipsum.example")
    assert [i.check for i in issues] == ["placeholder_text"]
    assert _check_placeholder_text(Path("s.md"), {}, "portal.loremipsum.example")
