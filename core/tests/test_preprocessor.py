"""Tests for the markdown assembler."""

from pathlib import Path

import pytest
import yaml

from report_rex_core.parser import parse_index
from report_rex_core.preprocessor import (
    _appendix_title,
    _apply_stats,
    _enrich,
    _separate_images,
    _sort_key,
    _timeline_table,
    _transform,
    _vuln_div,
    assemble,
)


class TestVulnDiv:
    def test_emits_fenced_div(self):
        meta = {"title": "SQL Injection", "severity": "Critical", "cvss_score": 9.8,
                "status": "Open", "affected_urls": ["https://example.com"]}
        result = _vuln_div(meta)
        assert "::: {.vulnerability" in result
        assert 'severity="Critical"' in result
        assert 'cvss="9.8"' in result
        assert 'affected="https://example.com"' in result
        # The title is now carried by the section heading, not the div.
        assert "title=" not in result

    def test_defaults_for_missing_fields(self):
        result = _vuln_div({})
        assert 'severity="Info"' in result
        assert 'status="Open"' in result

    def test_emits_reference_fields(self):
        meta = {"severity": "High", "cwe": "CWE-79", "owasp": "A03:2021",
                "mitre": "T1059", "component": "Profile"}
        result = _vuln_div(meta)
        assert 'cwe="CWE-79"' in result
        assert "A03:2021" in result and "Injection" in result  # label resolved
        assert "T1059" in result

    def test_sanitizes_braces_in_affected_urls(self):
        meta = {"severity": "High", "affected_urls": ["https://example.com/api/{id}"]}
        result = _vuln_div(meta)
        assert 'affected="https://example.com/api/(id)"' in result

    def test_emits_evaluation_and_material_fields(self):
        meta = {
            "severity": "Critical",
            "impact": "Critical",
            "sophistication": "Low",
            "material_impact": "Proprietary Data Loss",
            "mitigations": "M1032, M1038",
        }
        result = _vuln_div(meta)
        assert 'impact="Critical"' in result
        assert 'sophistication="Low"' in result
        assert 'material="Proprietary Data Loss"' in result
        assert 'mitigations="M1032, M1038"' in result

    def test_emits_finding_id_from_number(self):
        result = _vuln_div({"severity": "High", "finding_number": 3})
        assert 'finding="F-03"' in result

    def test_omits_optional_fields_when_absent(self):
        result = _vuln_div({"severity": "Low"})
        assert "impact=" not in result
        assert "sophistication=" not in result
        assert "material=" not in result
        assert "mitigations=" not in result
        assert "finding=" not in result


class TestTimelineTable:
    def test_included_and_omitted_entries_appear(self):
        entries = [
            {"date": "2024-01-01T00:00:00+00:00", "description": "A"},
            {"date": "2024-01-02T00:00:00+00:00", "description": "B", "include": True},
        ]
        result = _timeline_table(entries)
        assert "A" in result
        assert "B" in result

    def test_excluded_entries_are_dropped(self):
        entries = [
            {"date": "2024-01-01T00:00:00+00:00", "description": "Keep", "include": True},
            {"date": "2024-01-02T00:00:00+00:00", "description": "Drop", "include": False},
        ]
        result = _timeline_table(entries)
        assert "Keep" in result
        assert "Drop" not in result

    def test_sorted_chronologically_regardless_of_input_order(self):
        entries = [
            {"date": "2024-01-03T00:00:00+00:00", "description": "Third"},
            {"date": "2024-01-01T00:00:00+00:00", "description": "First"},
            {"date": "2024-01-02T00:00:00+00:00", "description": "Second"},
        ]
        result = _timeline_table(entries)
        assert result.index("First") < result.index("Second") < result.index("Third")

    def test_epoch_placeholder_date_renders_blank(self):
        result = _timeline_table([{"date": "1970-01-01T10:15:00+00:00", "description": "Recon"}])
        assert "1970" not in result
        assert "|  | Recon |" in result

    def test_empty_input_returns_empty_string(self):
        assert _timeline_table([]) == ""

    def test_all_excluded_returns_empty_string(self):
        entries = [{"date": "2024-01-01T00:00:00+00:00", "description": "x", "include": False}]
        assert _timeline_table(entries) == ""

    def test_malformed_date_raises(self):
        with pytest.raises(ValueError):
            _timeline_table([{"date": "not-a-date", "description": "x"}])


class TestEnrich:
    def test_computes_severity_and_score_from_vector(self):
        meta = _enrich({"cvss_vector": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"})
        assert meta["severity"] == "Critical"
        assert meta["cvss_score"] == "9.8"
        assert meta["cvss_version"] == "3.1"

    def test_falls_back_to_manual_severity_without_vector(self):
        meta = _enrich({"severity": "Medium", "cvss_score": 5.0})
        assert meta["severity"] == "Medium"
        assert meta["cvss_score"] == "5.0"


class TestSortKey:
    @staticmethod
    def _order(*metas):
        return [m["title"] for _, m in sorted(((Path("x.md"), m) for m in metas), key=_sort_key)]

    def test_higher_score_first_within_same_severity(self):
        assert self._order(
            {"title": "A", "severity": "High", "cvss_score": "7.7"},
            {"title": "B", "severity": "High", "cvss_score": "8.0"},
        ) == ["B", "A"]

    def test_severity_outranks_score(self):
        assert self._order(
            {"title": "A", "severity": "High", "cvss_score": "8.9"},
            {"title": "B", "severity": "Critical", "cvss_score": "9.0"},
        ) == ["B", "A"]

    def test_equal_scores_fall_back_to_title(self):
        assert self._order(
            {"title": "b", "severity": "High", "cvss_score": "8.0"},
            {"title": "A", "severity": "High", "cvss_score": "8.0"},
        ) == ["A", "b"]

    def test_missing_or_invalid_score_sorts_last_in_bucket(self):
        assert self._order(
            {"title": "A", "severity": "Medium"},
            {"title": "B", "severity": "Medium", "cvss_score": "n/a"},
            {"title": "C", "severity": "Medium", "cvss_score": 5.0},
        ) == ["C", "A", "B"]


class TestTransform:
    def test_external_link_becomes_footnote(self):
        out = _transform("See [the docs](https://example.com/x).", {})
        assert "the docs^[<https://example.com/x>]" in out

    def test_double_bang_link_stays_inline(self):
        out = _transform("See !![the docs](https://example.com/x).", {})
        assert "[the docs](https://example.com/x)" in out
        assert "^[" not in out

    def test_link_in_table_row_stays_inline(self):
        # A footnote (`^[...]`) inside a pandoc-generated `longtable` silently
        # loses its text in the rendered PDF, so table-row links must stay
        # plain inline links instead (issue #27).
        out = _transform("| [Burp Suite](https://portswigger.net/burp) | scanner |", {})
        assert "[Burp Suite](https://portswigger.net/burp)" in out
        assert "^[" not in out

    def test_cross_reference_resolved(self):
        registry = {"vuln-2": (2, "Stored XSS", "finding-2")}
        out = _transform("Amplifies [the XSS](./vuln-2.md).", registry)
        assert "Finding \\#2 — Stored XSS" in out
        assert "(#finding-2)" in out

    def test_image_cross_reference_resolved(self):
        registry = {"vuln-1": (1, "SQLi", "finding-1")}
        out = _transform("![](./vuln-1.md)", registry)
        assert "Finding \\#1 — SQLi" in out


class TestAssemble:
    def test_creates_combined_md(self, sample_report: Path, tmp_path: Path):
        structure = parse_index(sample_report / "index.md")
        out_dir = tmp_path / "out"
        combined, metadata = assemble(structure, out_dir)
        assert combined.exists()
        assert metadata.exists()

    def test_combined_contains_section_content(self, sample_report: Path, tmp_path: Path):
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert "Summary text." in content
        assert "Executive Summary" in content

    def test_combined_contains_vuln_div_and_heading(self, sample_report: Path, tmp_path: Path):
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert ".vulnerability" in content
        assert 'severity="Critical"' in content
        # Finding title is a numbered subsection with an anchor.
        assert "## SQL Injection {#finding-1}" in content

    def test_combined_contains_appendix(self, sample_report: Path, tmp_path: Path):
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert "Appendix A" in content
        assert "Scope list." in content

    def test_metadata_yaml_has_report_meta_and_stats(self, sample_report: Path, tmp_path: Path):
        structure = parse_index(sample_report / "index.md")
        _, metadata = assemble(structure, tmp_path / "out")
        meta = yaml.safe_load(metadata.read_text())
        assert meta["title"] == "Test Report"
        assert meta["client"] == "ACME Corp"
        assert meta["stats"]["total"] == 1
        assert meta["stats"]["critical"] == 1
        assert meta["generator"].startswith("Report Rex v")

    def test_dark_override_marker_is_stripped_not_rendered(self, sample_report: Path, tmp_path: Path):
        vuln_path = sample_report / "reports" / "vuln-1.md"
        content = vuln_path.read_text()
        content = content.replace(
            "## Description\n\nSQL injection found.\n",
            "## Description\n\nSQL injection found.\n\n"
            "> [!OVERRIDE-DARK-IMG]\n"
            "![A screenshot](../assets/screenshots/x.png)\n",
        )
        vuln_path.write_text(content)
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        rendered = combined.read_text()
        assert "OVERRIDE-DARK-IMG" not in rendered

    def test_asset_graph_directive_renders_without_assets_json(self, sample_report: Path, tmp_path: Path):
        (sample_report / "appendices" / "1.md").write_text(
            "---\ntitle: Appendix A\n---\n\n:::asset_graph:pie:os:::\n"
        )
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert "asset_graph" not in content
        assert "No data available for the os breakdown." in content

    def test_asset_graph_directive_renders_from_artifacts_json(self, sample_report: Path, tmp_path: Path):
        artifacts_dir = sample_report / "artifacts"
        artifacts_dir.mkdir()
        (artifacts_dir / "assets.json").write_text(
            '[{"ip": "10.0.0.1", "os": "Linux", '
            '"ports": [{"number": "22", "proto": "tcp", "service": "ssh"}]}]'
        )
        (sample_report / "appendices" / "1.md").write_text(
            "---\ntitle: Appendix A\n---\n\n:::asset_graph:bar:ports:::\n"
        )
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert "asset_graph" not in content
        assert "tikzpicture" in content

    def test_web_graph_directive_renders_from_artifacts_json(self, sample_report: Path, tmp_path: Path):
        artifacts_dir = sample_report / "artifacts"
        artifacts_dir.mkdir()
        (artifacts_dir / "webfindings.json").write_text(
            '[{"host": "a.com", "port": "80", "id": "1", "description": "d"}]'
        )
        (sample_report / "appendices" / "1.md").write_text(
            "---\ntitle: Appendix A\n---\n\n:::web_graph:bar:host:::\n"
        )
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert "web_graph" not in content
        assert "tikzpicture" in content

    def test_network_diagram_requires_assets_json(self, sample_report: Path, tmp_path: Path):
        (sample_report / "appendices" / "1.md").write_text(
            "---\ntitle: Appendix A\n---\n\n:::network_diagram:::\n"
        )
        structure = parse_index(sample_report / "index.md")
        with pytest.raises(FileNotFoundError, match="assets.json"):
            assemble(structure, tmp_path / "out")

    def test_network_diagram_renders_from_artifacts_json(self, sample_report: Path, tmp_path: Path):
        artifacts_dir = sample_report / "artifacts"
        artifacts_dir.mkdir()
        (artifacts_dir / "assets.json").write_text('[{"ip": "10.0.0.1", "os": "Linux"}]')
        (artifacts_dir / "connections.json").write_text(
            '{"subnets": {"10.0.0.0/24": "Corp"}, "pivots": {"10.0.0.1": "10.9.9.0/24"}}'
        )
        (sample_report / "appendices" / "1.md").write_text(
            "---\ntitle: Appendix A\n---\n\n:::network_diagram:::\n"
        )
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert "network_diagram" not in content
        assert "Corp" in content and "colorCritical" in content

    def test_network_diagram_bad_cidr_raises(self, sample_report: Path, tmp_path: Path):
        artifacts_dir = sample_report / "artifacts"
        artifacts_dir.mkdir()
        (artifacts_dir / "assets.json").write_text('[{"ip": "10.0.0.1"}]')
        (artifacts_dir / "connections.json").write_text('{"pivots": {"10.0.0.1": "nope/24"}}')
        (sample_report / "appendices" / "1.md").write_text(":::network_diagram:::\n")
        structure = parse_index(sample_report / "index.md")
        with pytest.raises(ValueError, match="network diagram"):
            assemble(structure, tmp_path / "out")

    def test_assets_copied(self, sample_report: Path, tmp_path: Path):
        structure = parse_index(sample_report / "index.md")
        out_dir = tmp_path / "out"
        assemble(structure, out_dir)
        assert (out_dir / "assets").is_dir()

    def test_timeline_absent_when_not_defined(self, sample_report: Path, tmp_path: Path):
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert "— Timeline" not in content

    def test_timeline_merged_from_index_and_content_files(self, sample_report: Path, tmp_path: Path):
        (sample_report / "index.md").write_text(
            "---\n"
            "title: Test Report\n"
            "client: ACME Corp\n"
            "template: rex-default\n"
            "timeline:\n"
            '  - date: "2024-01-01T00:00:00+00:00"\n'
            '    description: "From index"\n'
            "structure:\n"
            "  sections:\n"
            "    - sections/exec_summ.md\n"
            "  reports:\n"
            "    - reports/vuln-1.md\n"
            "  appendices:\n"
            "    - appendices/1.md\n"
            "---\n"
        )
        (sample_report / "reports" / "vuln-1.md").write_text(
            "---\ntitle: SQL Injection\nseverity: Critical\ncvss_score: 9.8\n"
            "status: Open\naffected_urls:\n  - https://example.com/login\n"
            "timeline:\n"
            '  - date: "2024-01-02T00:00:00+00:00"\n'
            '    description: "From vuln file"\n'
            "    include: true\n"
            '  - date: "2024-01-03T00:00:00+00:00"\n'
            '    description: "Excluded note"\n'
            "    include: false\n"
            "---\n\n## Description\n\nSQL injection found.\n"
        )
        structure = parse_index(sample_report / "index.md")
        combined, _ = assemble(structure, tmp_path / "out")
        content = combined.read_text()
        assert "— Timeline" in content
        assert "From index" in content
        assert "From vuln file" in content
        assert "Excluded note" not in content


def test_separate_images():
    assert _separate_images("![a](a.png)\n![b](b.png){width=90%}") == "![a](a.png)\n\n![b](b.png){width=90%}"
    assert _separate_images("![a](a.png)\n\n![b](b.png)") == "![a](a.png)\n\n![b](b.png)"
    fenced = "```\n![a](a.png)\n![b](b.png)\n```"
    assert _separate_images(fenced) == fenced


def test_separate_images_reattaches_detached_attributes():
    # #70: `{width=90%}` split off the image rendered as literal text.
    assert _separate_images("Evidence\n![a](a.png)\n{width=90%}\nNext") == (
        "Evidence\n\n![a](a.png){width=90%}\n\nNext"
    )
    assert _separate_images("![a](a.png) {width=90%}") == "![a](a.png){width=90%}"
    assert _separate_images("![a](a.png)\n{width=90%} Next") == "![a](a.png){width=90%}\n\nNext"
    assert _separate_images("![a](a.png) {.wide #fig}") == "![a](a.png){.wide #fig}"


def test_separate_images_leaves_non_attribute_braces_and_code_alone():
    assert _separate_images("![a](a.png)\n{not attributes}") == "![a](a.png)\n\n{not attributes}"
    fenced = "```\n![a](a.png)\n{width=90%}\n```"
    assert _separate_images(fenced) == fenced


def test_untitled_finding_falls_back_to_filename(sample_report: Path, tmp_path: Path):
    (sample_report / "reports" / "vuln-1.md").write_text("---\nseverity: High\n---\n\nBody\n")
    combined, _ = assemble(parse_index(sample_report / "index.md"), tmp_path / "out")
    assert "## vuln-1 {#finding-1}" in combined.read_text()


@pytest.mark.parametrize("index, title, expected", [
    (0, 'Appendix A — Scope and Rules of Engagement', 'Appendix A — Scope and Rules of Engagement'),
    (2, 'Appendix — Discovered Assets', 'Appendix C — Discovered Assets'),
    (1, 'Appendix 2: Tools', 'Appendix B — Tools'),
    (1, 'appendix b - Coverage', 'Appendix B — Coverage'),
    (3, 'Glossary', 'Appendix D — Glossary'),
    (0, '', 'Appendix A'),
    (0, 'Appendix', 'Appendix A'),
    (0, 'Appendixes of note', 'Appendix A — Appendixes of note'),
])
def test_appendix_title_letters_by_position(index, title, expected):
    # #75: "Appendix A", "Appendix B", then an unlettered ingest appendix.
    assert _appendix_title(index, title) == expected


def test_appendix_body_headings_nest_under_appendix_title(sample_report: Path, tmp_path: Path):
    (sample_report / "appendices" / "1.md").write_text(
        "---\ntitle: Appendix A — Scope\n---\n\n## Scope\n\nList.\n\n## Rules of Engagement\n\nRules.\n"
    )
    combined, _ = assemble(parse_index(sample_report / "index.md"), tmp_path / "out")
    content = combined.read_text()
    start = content.index("# Appendices")
    appendix = content[start:]
    assert "\n## Appendix A — Scope\n" in appendix
    assert "\n### Scope\n" in appendix and "\n### Rules of Engagement\n" in appendix
    # Every heading below an appendix title is at least one level deeper.
    assert not [line for line in appendix.splitlines()[1:]
                if line.startswith("## ") and not line.startswith("## Appendix ")]


def test_apply_stats_tokens():
    # #72: summary prose derived from the findings, not hard-coded.
    stats = {"total": 3, "critical": 0, "high": 3, "medium": 0, "low": 0, "info": 0}
    body = "Found :::findings:total::: issues (:::findings:high::: High); top: :::findings:highest:::."
    assert _apply_stats(body, stats) == "Found 3 issues (3 High); top: High."
    assert _apply_stats(":::findings:highest:::", dict.fromkeys(stats, 0)) == "None"
    assert _apply_stats(":::findings:bogus:::", stats) == ":::findings:bogus:::"


def test_stat_tokens_rendered_in_sections(sample_report: Path, tmp_path: Path):
    (sample_report / "sections" / "exec_summ.md").write_text(
        "---\ntitle: Executive Summary\n---\n\n:::findings:total::: finding(s), worst :::findings:highest:::.\n"
    )
    combined, _ = assemble(parse_index(sample_report / "index.md"), tmp_path / "out")
    assert ":::findings:" not in combined.read_text()
