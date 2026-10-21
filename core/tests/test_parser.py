"""Tests for the index.md parser."""

import pytest
from pathlib import Path
from report_rex_core.parser import parse_index, split_frontmatter, parse_frontmatter


class TestSplitFrontmatter:
    def test_extracts_frontmatter(self):
        content = "---\ntitle: Test\n---\n\nBody here."
        fm, body = split_frontmatter(content)
        assert "title: Test" in fm
        assert "Body here." in body

    def test_no_frontmatter(self):
        content = "Just a plain markdown file."
        fm, body = split_frontmatter(content)
        assert fm == ""
        assert body == content

    def test_empty_frontmatter(self):
        content = "---\n---\n\nBody."
        fm, body = split_frontmatter(content)
        assert fm == ""
        assert "Body." in body


class TestParseIndex:
    def test_parses_valid_index(self, sample_report: Path):
        structure = parse_index(sample_report / "index.md")
        assert structure.metadata["title"] == "Test Report"
        assert structure.metadata["client"] == "ACME Corp"
        assert structure.template == "rex-default"
        assert len(structure.sections) == 1
        assert len(structure.reports) == 1
        assert len(structure.appendices) == 1

    def test_missing_file_raises(self, tmp_path: Path):
        index = tmp_path / "index.md"
        index.write_text(
            "---\ntemplate: rex-default\nstructure:\n  sections:\n    - missing.md\n---\n"
        )
        with pytest.raises(FileNotFoundError, match="missing.md"):
            parse_index(index)

    def test_no_frontmatter_raises(self, tmp_path: Path):
        index = tmp_path / "index.md"
        index.write_text("No frontmatter here.")
        with pytest.raises(ValueError, match="frontmatter"):
            parse_index(index)

    def test_default_template(self, tmp_path: Path):
        index = tmp_path / "index.md"
        index.write_text("---\ntitle: Test\nstructure: {}\n---\n")
        structure = parse_index(index)
        assert structure.template == "rit-cptc"


def test_parse_frontmatter_invalid_yaml_names_source():
    with pytest.raises(ValueError, match="reports/x.md: invalid YAML"):
        parse_frontmatter('---\na: "open\nb:\n  - c\n---\n', "reports/x.md")
