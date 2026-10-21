"""CLI entry point for report-rex."""

from __future__ import annotations

import sys
from pathlib import Path

import click

from .banner import render_banner
from .builder import build, scaffold


@click.group(invoke_without_command=True)
@click.version_option(package_name="report-rex-core")
@click.pass_context
def main(ctx: click.Context) -> None:
    """Report Rex — generate professional pentest reports from Markdown."""
    if ctx.invoked_subcommand is None:
        banner = render_banner()
        if banner:
            click.echo(banner)
        click.echo(ctx.get_help())


@main.command()
@click.option("--dir", "report_dir", default=".", show_default=True,
              type=click.Path(exists=True, file_okay=False, path_type=Path),
              help="Report directory containing index.md")
@click.option("--output", "-o", default="report.pdf", show_default=True,
              type=click.Path(path_type=Path),
              help="Output PDF filename")
@click.option("--template", "template_override", default=None,
              help="Override template (default: from index.md)")
@click.option("--templates-root", default=None,
              type=click.Path(exists=True, file_okay=False, path_type=Path),
              help="Root directory containing templates (default: <repo>/templates)")
@click.option("--final", is_flag=True, default=False,
              help="Produce the final report, no DRAFT watermark")
@click.option("--docx", "docx", is_flag=True, default=False,
              help="Also generate a .docx clone alongside the PDF")
@click.option("--skip-validation", is_flag=True, default=False,
              help="Skip the QA-gate checks that normally run before building")
@click.option("--password", default=None,
              help="Encrypt the output PDF with this password (requires qpdf on PATH)")
def build_cmd(
    report_dir: Path,
    output: Path,
    template_override: str | None,
    templates_root: Path | None,
    final: bool,
    docx: bool,
    skip_validation: bool,
    password: str | None,
) -> None:
    """Build a PDF report from a report directory."""
    draft = not final
    if templates_root is None:
        templates_root = _find_templates_root()

    if not skip_validation:
        _, result = _run_validation(report_dir)
        if not result.ok:
            click.echo(
                f"\nBuild aborted: {len(result.errors)} validation error(s). "
                "Fix them, or pass --skip-validation to build anyway.",
                err=True,
            )
            sys.exit(1)

    output_docx = output.with_suffix(".docx") if docx else None

    click.echo(f"Building: {report_dir} → {output}")
    if draft:
        click.echo("  Mode: DRAFT")
    if output_docx:
        click.echo(f"  Also: {output_docx}")

    try:
        build(
            report_dir=report_dir.resolve(),
            templates_root=templates_root.resolve(),
            output_pdf=output.resolve(),
            template_override=template_override,
            draft=draft,
            output_docx=output_docx.resolve() if output_docx else None,
            password=password,
        )
        click.echo(f"Done: {output}")
    except Exception as exc:
        click.echo(f"Error: {exc}", err=True)
        sys.exit(1)


@main.command("new")
@click.argument("target_dir", default="reporting", type=click.Path(path_type=Path))
@click.option("--template", "template_name", default="rit-cptc", show_default=True,
              help="Template to scaffold for")
def new_cmd(target_dir: Path, template_name: str) -> None:
    """Scaffold a new report directory with starter files."""
    if target_dir.exists() and any(target_dir.iterdir()):
        click.echo(f"Directory '{target_dir}' already exists and is not empty.", err=True)
        sys.exit(1)

    scaffold(target_dir, template_name)
    click.echo(f"Created report scaffold at: {target_dir}")
    click.echo("  Edit index.md to configure your report.")


@main.command()
@click.option("--dir", "report_dir", default=".", show_default=True,
              type=click.Path(exists=True, file_okay=False, path_type=Path),
              help="Report directory to validate")
def validate(report_dir: Path) -> None:
    """Validate a report directory: structure, required fields, and screenshot QA."""
    structure, result = _run_validation(report_dir)
    if not result.issues:
        click.echo(f"Valid — {len(structure.sections)} sections, "
                   f"{len(structure.reports)} findings, "
                   f"{len(structure.appendices)} appendices")
    if not result.ok:
        sys.exit(1)


def _run_validation(report_dir: Path):
    """Parse + run the QA-gate checks against report_dir, printing every issue found.

    Exits 1 immediately on structural failures (missing index.md, unparsable
    frontmatter, missing referenced files) since content checks can't run without a
    valid structure. Otherwise returns (structure, ValidationResult) for the caller
    to act on — callers decide for themselves whether errors should abort.
    """
    from .parser import parse_index
    from .validator import validate_report

    index_path = report_dir / "index.md"
    if not index_path.exists():
        click.echo(f"Error: index.md not found in {report_dir}", err=True)
        sys.exit(1)

    try:
        structure = parse_index(index_path)
    except Exception as exc:
        click.echo(f"Invalid: {exc}", err=True)
        sys.exit(1)

    result = validate_report(structure)

    if not result.issues:
        return structure, result

    resolved_dir = report_dir.resolve()
    by_file: dict[Path, list] = {}
    for issue in result.issues:
        by_file.setdefault(issue.file, []).append(issue)

    for file_path, file_issues in sorted(by_file.items()):
        try:
            rel = file_path.relative_to(resolved_dir)
        except ValueError:
            rel = file_path
        click.echo(f"{rel}:")
        for issue in file_issues:
            label = "error" if issue.severity == "error" else "warning"
            where = f"line {issue.line}: " if issue.line else ""
            click.echo(f"  [{label}] {where}{issue.check}: {issue.message}", err=issue.severity == "error")

    n_errors, n_warnings = len(result.errors), len(result.warnings)
    click.echo(
        f"\n{n_errors} error(s), {n_warnings} warning(s) across {len(by_file)} file(s)",
        err=n_errors > 0,
    )
    return structure, result


# Register the build command under the "build" name
main.add_command(build_cmd, "build")


def _find_templates_root() -> Path:
    """Walk up from cwd to find the templates/ directory."""
    here = Path.cwd()
    for parent in [here, *here.parents]:
        candidate = parent / "templates"
        if candidate.is_dir():
            return candidate
    # Fall back to a sibling of the package install location
    return Path(__file__).parent.parent.parent.parent / "templates"
