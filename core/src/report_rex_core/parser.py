"""Parse index.md and resolve the full report structure."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

import yaml


@dataclass
class ReportStructure:
    metadata: dict
    sections: list[Path]
    reports: list[Path]
    appendices: list[Path]
    base_dir: Path
    template: str = "rit-cptc"


def parse_index(index_path: Path) -> ReportStructure:
    """Parse index.md, validate all referenced files exist, return structure."""
    content = index_path.read_text(encoding="utf-8")
    frontmatter, _ = split_frontmatter(content)
    if not frontmatter:
        raise ValueError(f"{index_path} has no YAML frontmatter")

    meta = yaml.safe_load(frontmatter) or {}
    base_dir = index_path.parent
    structure = meta.get("structure", {})

    def resolve(paths: list[str] | None) -> list[Path]:
        resolved = []
        for p in paths or []:
            full = base_dir / p
            if not full.exists():
                raise FileNotFoundError(f"Referenced file not found: {full}")
            resolved.append(full)
        return resolved

    return ReportStructure(
        metadata=meta,
        sections=resolve(structure.get("sections")),
        reports=resolve(structure.get("reports")),
        appendices=resolve(structure.get("appendices")),
        base_dir=base_dir,
        template=meta.get("template", "rit-cptc"),
    )


def split_frontmatter(content: str) -> tuple[str, str]:
    """Split YAML frontmatter from markdown body. Returns (frontmatter, body)."""
    if not content.startswith("---"):
        return "", content
    end = content.find("\n---", 3)
    if end == -1:
        return "", content
    return content[3:end].strip(), content[end + 4 :].strip()


def parse_frontmatter(content: str, source: Path | str | None = None) -> dict:
    """Parse YAML frontmatter. Raises ValueError naming `source` on invalid YAML."""
    fm, _ = split_frontmatter(content)
    try:
        return yaml.safe_load(fm) or {} if fm else {}
    except yaml.YAMLError as exc:
        raise ValueError(f"{source or 'frontmatter'}: invalid YAML frontmatter: {exc}") from exc
