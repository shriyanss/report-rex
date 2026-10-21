"""Assemble multiple markdown files into a single pandoc-ready document.

Beyond concatenation, the preprocessor performs the transformations that make
Report Rex feel like a report generator rather than a markdown stitcher:

* **CVSS** — a finding's ``cvss_vector`` is parsed and its base score and
  qualitative severity are computed automatically (see :mod:`.cvss`).
* **Ordering** — findings are sorted by severity (Critical → Info), then by
  CVSS score (highest first), with the title as a tie-breaker, and numbered
  sequentially.
* **Cross-references** — ``![](./vuln-2.md)`` / ``[x](vuln-2.md)`` become
  ``Finding #N — Title`` hyperlinks to the referenced finding.
* **Links** — ``[text](https://…)`` renders as a footnote; ``!![text](https://…)``
  stays an inline clickable link.
* **Charts** — ``:::findings_graph:pie:severity:::`` directives are replaced with
  generated LaTeX charts (see :mod:`.charts`).
* **Asset charts** — ``:::asset_graph:pie:os:::`` directives in an appendix are
  replaced with charts tallied from ``artifacts/assets.json`` (written by
  ``rex ingest nmap``/``naabu``), if that file exists.
* **Web finding charts** — ``:::web_graph:bar:host:::`` directives are replaced
  with charts tallied from ``artifacts/webfindings.json`` (written by
  ``rex ingest nikto``), if that file exists.
* **Network diagram** — ``:::network_diagram:::`` is replaced with a subnet/host
  diagram from ``artifacts/assets.json`` (required — the build fails without it),
  with subnet labels and pivots from the optional ``artifacts/connections.json``.
"""

from __future__ import annotations

import re
from datetime import date, datetime
from pathlib import Path

import yaml

from . import __version__, charts
from .cvss import SEVERITY_RANK, CvssError, compute
from .parser import ReportStructure, parse_frontmatter, split_frontmatter
from .references import label as ref_label
from .references import split_codes

# ── Path rebasing ───────────────────────────────────────────────────────────


def _rebase_paths(body: str, file_path: Path, base_dir: Path) -> str:
    """Rewrite relative image/link paths so they work from the assembled root."""
    try:
        depth = len(file_path.parent.relative_to(base_dir).parts)
    except ValueError:
        return body
    if depth == 0:
        return body
    prefix = "../" * depth
    return re.sub(r"(\]\()(" + re.escape(prefix) + r")", r"\1", body)


# ── Finding metadata / CVSS ─────────────────────────────────────────────────


def _enrich(meta: dict) -> dict:
    """Return finding metadata with computed severity / score / version.

    An explicit ``cvss_vector`` drives severity and score. A manually supplied
    ``severity``/``cvss_score`` is used only when no vector is present.
    """
    out = dict(meta)
    vector = str(meta.get("cvss_vector", "")).strip()
    if vector:
        try:
            result = compute(vector)
            out["severity"] = result.severity if result.severity != "None" else "Info"
            out["cvss_score"] = f"{result.score:.1f}"
            out["cvss_version"] = result.version
        except CvssError:
            out.setdefault("severity", meta.get("severity", "Info"))
    else:
        out.setdefault("severity", meta.get("severity", "Info"))
        if meta.get("cvss_score") is not None:
            out["cvss_score"] = str(meta.get("cvss_score"))
    return out


def _sort_key(item: tuple[Path, dict]):
    _, meta = item
    sev = meta.get("severity", "Info")
    try:
        score = float(meta.get("cvss_score"))
    except (TypeError, ValueError):
        score = 0.0
    return (-SEVERITY_RANK.get(sev, 0), -score, str(meta.get("title", "")).lower())


# ── Cross-references & links ────────────────────────────────────────────────

_MD_IMG_REF = re.compile(r"!\[[^\]]*\]\(\s*([^)\s]+\.md)\s*\)")
_MD_LINK_REF = re.compile(r"(?<!!)\[([^\]]*)\]\(\s*([^)\s]+\.md)\s*\)")
_HTTP_FOOTNOTE = re.compile(r"(?<!!)\[([^\]]+)\]\(\s*(https?://[^)\s]+)\s*\)")
_HTTP_DIRECT = re.compile(r"!!\[([^\]]+)\]\(\s*(https?://[^)\s]+)\s*\)")
_TABLE_ROW = re.compile(r"^[ \t]*\|.*\|[ \t]*$")


def _resolve_ref(target: str, registry: dict) -> str | None:
    stem = Path(target).stem
    entry = registry.get(stem) or registry.get(Path(target).name)
    if not entry:
        return None
    n, title, anchor = entry
    return f"[Finding \\#{n} — {title}](#{anchor})"


def _transform(body: str, registry: dict) -> str:
    # 1. Cross-file references to other findings (image and link forms).
    def _img_sub(m):
        return _resolve_ref(m.group(1), registry) or m.group(0)

    def _link_sub(m):
        return _resolve_ref(m.group(2), registry) or m.group(0)

    body = _MD_IMG_REF.sub(_img_sub, body)
    body = _MD_LINK_REF.sub(_link_sub, body)

    # 2. Plain external links → footnote (skipped for images, !! links, and
    #    table rows — pandoc emits table rows as a LaTeX `longtable`, where a
    #    `\footnote` silently loses its text instead of rendering, so those
    #    links stay plain inline links instead).
    def _footnote_line(line: str) -> str:
        if _TABLE_ROW.match(line):
            return line
        return _HTTP_FOOTNOTE.sub(lambda m: f"{m.group(1)}^[<{m.group(2)}>]", line)

    body = "\n".join(_footnote_line(line) for line in body.split("\n"))

    # 3. !![text](url) → inline clickable link.
    body = _HTTP_DIRECT.sub(lambda m: f"[{m.group(1)}]({m.group(2)})", body)

    return body


# ── Chart directives ────────────────────────────────────────────────────────

_GRAPH_DIRECTIVE = re.compile(
    r"^[ \t]*:::[ \t]*(?:findings_graph|graph)"
    r":([a-zA-Z]+)(?::([a-zA-Z_-]+))?[ \t]*:::[ \t]*$",
    re.MULTILINE,
)

_ASSET_GRAPH_DIRECTIVE = re.compile(
    r"^[ \t]*:::[ \t]*asset_graph"
    r":([a-zA-Z]+)(?::([a-zA-Z_-]+))?[ \t]*:::[ \t]*$",
    re.MULTILINE,
)

_WEB_GRAPH_DIRECTIVE = re.compile(
    r"^[ \t]*:::[ \t]*web_graph"
    r":([a-zA-Z]+)(?::([a-zA-Z_-]+))?[ \t]*:::[ \t]*$",
    re.MULTILINE,
)

_NETWORK_DIRECTIVE = re.compile(
    r"^[ \t]*:::[ \t]*network_diagram[ \t]*:::[ \t]*$", re.MULTILINE
)


def _apply_charts(body: str, findings_data: list[dict]) -> str:
    def _sub(m):
        gtype, dataset = m.group(1), m.group(2) or "severity"
        return charts.render(findings_data, gtype, dataset)

    return _GRAPH_DIRECTIVE.sub(_sub, body)


# Inline `:::findings:total:::` / `:::findings:high:::` / `:::findings:highest:::`
# tokens, so summary prose stays in step with the findings (#72).
_STAT_TOKEN = re.compile(r":::findings:(total|critical|high|medium|low|info|highest):::")
_SEVERITIES = ("Critical", "High", "Medium", "Low", "Info")


def _apply_stats(body: str, stats: dict) -> str:
    highest = next((sev for sev in _SEVERITIES if stats[sev.lower()]), "None")
    return _STAT_TOKEN.sub(lambda m: highest if m.group(1) == "highest" else str(stats[m.group(1)]), body)


def _apply_asset_charts(body: str, assets_data: list[dict]) -> str:
    def _sub(m):
        gtype, dataset = m.group(1), m.group(2) or "os"
        return charts.render_assets(assets_data, gtype, dataset)

    return _ASSET_GRAPH_DIRECTIVE.sub(_sub, body)


def _apply_web_charts(body: str, webfindings_data: list[dict]) -> str:
    def _sub(m):
        gtype, dataset = m.group(1), m.group(2) or "host"
        return charts.render_web(webfindings_data, gtype, dataset)

    return _WEB_GRAPH_DIRECTIVE.sub(_sub, body)


def _apply_network(body: str, base_dir: Path) -> str:
    if not _NETWORK_DIRECTIVE.search(body):
        return body
    if not (base_dir / "artifacts" / "assets.json").exists():
        raise FileNotFoundError(
            ":::network_diagram::: needs artifacts/assets.json — run `rex ingest` first"
        )
    try:
        diagram = charts.render_network(
            _load_json_artifact(base_dir, "assets.json"),
            _load_json_artifact(base_dir, "connections.json") or {},
        )
    except ValueError as e:
        raise ValueError(f"network diagram: {e}") from e
    return _NETWORK_DIRECTIVE.sub(lambda _: diagram, body)


def _load_json_artifact(base_dir: Path, name: str) -> list[dict]:
    """Load an artifacts/<name>.json file written by `rex ingest`; empty if absent."""
    path = base_dir / "artifacts" / name
    if not path.exists():
        return []
    return yaml.safe_load(path.read_text("utf-8")) or []


# ── Heading demotion (finding bodies) ───────────────────────────────────────

_HEADING = re.compile(r"^(#{1,5})(\s)", re.MULTILINE)


def _demote(body: str, levels: int = 1) -> str:
    return _HEADING.sub(lambda m: "#" * min(len(m.group(1)) + levels, 6) + m.group(2), body)


# ── QA-gate directives (validator.py) ───────────────────────────────────────

# `> [!OVERRIDE-DARK-IMG]` tells `report-rex validate` to skip its dark-mode screenshot
# check for the image immediately below it (see validator.py). It's a directive for the
# validator, not report content, so strip the line before the body reaches pandoc —
# otherwise it renders as a visible blockquote in the final PDF.
_DARK_OVERRIDE_MARKER_LINE = re.compile(r"^>\s*\[!OVERRIDE-DARK-IMG\]\s*\n?", re.MULTILINE)


def _strip_qa_directives(body: str) -> str:
    return _DARK_OVERRIDE_MARKER_LINE.sub("", body)


# ── Image separation ────────────────────────────────────────────────────────

# Image lines with no blank line between them are one pandoc paragraph, so the images
# render inline and squash together. Give each image-only line its own paragraph.
#
# An image-only line run together with text (`Evidence` / `![..](..)` / `Next`) also
# renders inline, as a captionless image inside that paragraph, so give image-only
# lines a blank line on both sides.
#
# pandoc only attaches an attribute block written directly after the image. One split
# off by a space or a line break (`![..](..) {width=90%}` / `{width=90%}` on the next
# line) renders as literal text instead (#70), so re-attach it.
_IMG_LINE = re.compile(r"^[ \t]*!\[[^\]]*\]\([^)]*\)(\{[^}]*\})?[ \t]*$")
_FENCE = re.compile(r"^[ \t]*(```|~~~)")
_ATTRS = r"\{[ \t]*(?:[.#][^}\n]*|[^}\n]*=[^}\n]*)\}"  # {width=90%} / {.class} / {#id}
_IMG_SPACED_ATTRS = re.compile(r"(!\[[^\]]*\]\([^)]*\))[ \t]+(" + _ATTRS + ")")
_ATTRS_LINE = re.compile(r"^[ \t]*(" + _ATTRS + r")[ \t]*")


def _separate_images(body: str) -> str:
    out: list[str] = []
    in_fence = prev_img = False
    for line in body.split("\n"):
        if _FENCE.match(line):
            in_fence = not in_fence
        elif not in_fence:
            line = _IMG_SPACED_ATTRS.sub(r"\1\2", line)
            if prev_img and out[-1].rstrip().endswith(")") and (m := _ATTRS_LINE.match(line)):
                out[-1] = out[-1].rstrip() + m.group(1)
                line = line[m.end():]
                if not line:
                    continue
        is_img = not in_fence and bool(_IMG_LINE.match(line))
        if line.strip() and (is_img or prev_img) and out and out[-1].strip():
            out.append("")
        out.append(line)
        prev_img = is_img
    return "\n".join(out)


# ── Assembly ────────────────────────────────────────────────────────────────


def assemble(structure: ReportStructure, output_dir: Path) -> tuple[Path, Path]:
    """Assemble the full report markdown and metadata into output_dir.

    Returns (combined_md_path, metadata_yaml_path).
    """
    output_dir.mkdir(parents=True, exist_ok=True)

    import shutil

    assets_src = structure.base_dir / "assets"
    if assets_src.exists():
        shutil.copytree(assets_src, output_dir / "assets", dirs_exist_ok=True)

    # --- Timeline entries collected from index.md and any content file ---
    timeline_entries: list[dict] = list(structure.metadata.get("timeline") or [])

    # --- Pre-compute finding metadata (CVSS, ordering, numbering) ---
    findings: list[tuple[Path, dict]] = []
    for path in structure.reports:
        meta = _enrich(parse_frontmatter(path.read_text("utf-8"), path))
        timeline_entries.extend(meta.get("timeline") or [])
        findings.append((path, meta))
    findings.sort(key=_sort_key)

    registry: dict = {}
    findings_data: list[dict] = []
    for n, (path, meta) in enumerate(findings, 1):
        anchor = f"finding-{n}"
        title = str(meta.get("title") or path.stem)
        registry[path.stem] = (n, title, anchor)
        registry[path.name] = (n, title, anchor)
        findings_data.append(meta)

    stats = _stats(findings_data)
    assets_data = _load_json_artifact(structure.base_dir, "assets.json")
    webfindings_data = _load_json_artifact(structure.base_dir, "webfindings.json")

    def render_body(body: str, path: Path, *, demote: bool) -> str:
        body = _separate_images(_strip_qa_directives(body))
        body = _rebase_paths(body, path, structure.base_dir)
        if demote:
            body = _demote(body, 1)
        body = _apply_charts(body, findings_data)
        body = _apply_stats(body, stats)
        body = _apply_asset_charts(body, assets_data)
        body = _apply_web_charts(body, webfindings_data)
        body = _apply_network(body, structure.base_dir)
        return _transform(body, registry)

    chunks: list[str] = []

    # --- Sections (exec summary, methodology, etc.) ---
    for path in structure.sections:
        raw = path.read_text("utf-8")
        meta = parse_frontmatter(raw, path)
        timeline_entries.extend(meta.get("timeline") or [])
        _, body = split_frontmatter(raw)
        if title := meta.get("title"):
            chunks.append(f"# {title}\n\n")
        chunks.append(render_body(body, path, demote=False).strip())
        chunks.append("\n\n\\clearpage\n\n")

    # --- Vulnerability Findings ---
    if findings:
        chunks.append("# Vulnerability Findings\n\n")
        for n, (path, meta) in enumerate(findings, 1):
            _, body = split_frontmatter(path.read_text("utf-8"))
            title = str(meta.get("title") or path.stem)
            anchor = f"finding-{n}"
            meta["finding_number"] = n
            chunks.append(f"## {title} {{#{anchor}}}\n\n")
            chunks.append(_vuln_div(meta))
            chunks.append("\n\n")
            chunks.append(render_body(body, path, demote=True).strip())
            chunks.append("\n\n\\clearpage\n\n")

    # --- Appendices ---
    appendix_paths = structure.appendices
    for path in appendix_paths:
        timeline_entries.extend(parse_frontmatter(path.read_text("utf-8"), path).get("timeline") or [])
    timeline_table = _timeline_table(timeline_entries)

    if appendix_paths or timeline_table:
        chunks.append("# Appendices\n\n")
        for i, path in enumerate(appendix_paths):
            raw = path.read_text("utf-8")
            meta = parse_frontmatter(raw, path)
            _, body = split_frontmatter(raw)
            chunks.append(f"## {_appendix_title(i, str(meta.get('title') or ''))}\n\n")
            # Demoted, like finding bodies, so an appendix's own `## Scope` nests
            # under its title instead of sitting beside it (#75).
            chunks.append(render_body(body, path, demote=True).strip())
            chunks.append("\n\n")
        if timeline_table:
            chunks.append(f"## {_appendix_title(len(appendix_paths), 'Timeline')}\n\n")
            chunks.append(timeline_table)
            chunks.append("\n\n")

    combined_path = output_dir / "combined.md"
    combined_path.write_text("\n".join(chunks), encoding="utf-8")

    # Report metadata + computed stats for the template.
    out_meta = dict(structure.metadata)
    out_meta.setdefault("generator", f"Report Rex v{__version__}")
    out_meta["stats"] = stats
    _absolutize_assets(out_meta, structure.base_dir)
    metadata_path = output_dir / "metadata.yaml"
    metadata_path.write_text(yaml.dump(out_meta, allow_unicode=True, sort_keys=False), encoding="utf-8")

    return combined_path, metadata_path


# "Appendix", "Appendix B —", "Appendix 2:" … — any lettering the author (or
# `rex ingest`, which writes "Appendix — Discovered Assets") put in the title.
_APPENDIX_PREFIX = re.compile(r"^Appendix(?:\s+[A-Za-z0-9]{1,3})?\s*(?:[—–:-]\s*|$)", re.IGNORECASE)


def _appendix_title(index: int, title: str) -> str:
    """Letter appendices by position (A, B, …) so lettering is always consistent (#75)."""
    name = _APPENDIX_PREFIX.sub("", title.strip(), count=1)
    letter = chr(ord("A") + index) if index < 26 else str(index + 1)
    return f"Appendix {letter} — {name}" if name else f"Appendix {letter}"


def _absolutize_assets(meta: dict, base_dir: Path) -> None:
    """Rewrite logo/branding paths to absolute so XeLaTeX finds them.

    Images referenced only from the LaTeX template (not from markdown) are not
    copied into pandoc's working directory, so relative paths would break.
    """

    def fix(value):
        if not isinstance(value, str) or not value.strip():
            return value
        p = Path(value)
        if p.is_absolute():
            return value
        candidate = (base_dir / value).resolve()
        return str(candidate) if candidate.exists() else value

    for key in ("company_logo", "company_logo_light", "client_logo"):
        if key in meta:
            meta[key] = fix(meta[key])
    logo = meta.get("logo")
    if isinstance(logo, dict) and "path" in logo:
        logo["path"] = fix(logo["path"])


def _stats(findings_data: list[dict]) -> dict:
    counts = {"Critical": 0, "High": 0, "Medium": 0, "Low": 0, "Info": 0}
    for f in findings_data:
        sev = f.get("severity", "Info")
        if sev in ("None", ""):
            sev = "Info"
        counts[sev] = counts.get(sev, 0) + 1
    return {
        "total": len(findings_data),
        "critical": counts["Critical"],
        "high": counts["High"],
        "medium": counts["Medium"],
        "low": counts["Low"],
        "info": counts["Info"],
    }


_EPOCH = date(1970, 1, 1)


def _timeline_table(entries: list[dict]) -> str:
    """Render included timeline entries, sorted chronologically, as a pandoc table.

    Entries may come from index.md or any section/report/appendix file's own
    frontmatter; ``include`` defaults to True and ``False`` entries are dropped.
    """
    rows = []
    for entry in entries:
        if entry.get("include") is False:
            continue
        raw_date = str(entry.get("date", "")).strip()
        try:
            parsed = datetime.fromisoformat(raw_date.replace("Z", "+00:00"))
        except ValueError as exc:
            raise ValueError(f"Invalid timeline date {raw_date!r}: {exc}") from exc
        description = str(entry.get("description", "")).replace("|", "\\|")
        rows.append((parsed, description))

    if not rows:
        return ""

    rows.sort(key=lambda r: r[0])
    lines = ["| Date | Description |", "|------|-------------|"]
    # A 1970-01-01 date is the scaffold's unset placeholder (validate flags it as
    # placeholder_text); leave the cell blank rather than print the Unix epoch.
    lines += [
        f"| {'' if dt.date() == _EPOCH else dt.strftime('%Y-%m-%d %H:%M')} | {desc} |"
        for dt, desc in rows
    ]
    return "\n".join(lines)


def _vuln_div(meta: dict) -> str:
    """Emit a pandoc fenced div that the Lua filter converts to a styled box."""

    def _safe(v: str) -> str:
        return v.replace('"', "'").replace("{", "(").replace("}", ")")

    affected = meta.get("affected_urls")
    affected_raw = ", ".join(affected) if isinstance(affected, list) else str(affected or "")

    owasp = " ; ".join(ref_label(c) for c in split_codes(meta.get("owasp")))
    mitre = " ; ".join(ref_label(c) for c in split_codes(meta.get("mitre")))
    mitigations = ", ".join(split_codes(meta.get("mitigations")))
    business = ", ".join(split_codes(meta.get("business_risk")))

    finding_number = meta.get("finding_number")
    finding_id = f"F-{int(finding_number):02d}" if finding_number else ""

    fields = {
        "severity": meta.get("severity", "Info"),
        "finding": finding_id,
        "impact": str(meta.get("impact", "")),
        "sophistication": str(meta.get("sophistication", "")),
        "cvss": str(meta.get("cvss_score", "")),
        "vector": str(meta.get("cvss_vector", "")),
        "cvssversion": str(meta.get("cvss_version", "")),
        "status": meta.get("status", "Open"),
        "affected": affected_raw,
        "cwe": str(meta.get("cwe", "")),
        "owasp": owasp,
        "mitre": mitre,
        "mitigations": mitigations,
        "material": str(meta.get("material_impact", "")),
        "business": business,
        "component": str(meta.get("component", "")),
    }
    attrs = " ".join(f'{k}="{_safe(str(v))}"' for k, v in fields.items() if str(v).strip())
    return f"::: {{.vulnerability {attrs}}}\n:::"
