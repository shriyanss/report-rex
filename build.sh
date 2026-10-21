#!/usr/bin/env bash
# build.sh — Generate a PDF report from a reporting directory.
#
# Usage:
#   ./build.sh [options]
#
# Options:
#   --dir PATH          Report directory containing index.md  (default: reporting/example)
#   --template NAME     Template name or path                 (default: from index.md)
#   --output FILE       Output PDF path                       (default: report.pdf)
#   --final             Produce the final report, no DRAFT watermark (default: draft, appends -draft to filename)
#   --docx              Also generate a .docx clone alongside the PDF
#   --force             Skip QA-gate checks (validation errors) and build anyway
#   --no-docker         Run natively (requires pandoc, xelatex, python in PATH)
#   --image TAG         Docker image tag                      (default: report-rex:<version.txt>)
#   --password PASS     Encrypt the output PDF with this password (requires qpdf)
#   -h, --help          Show this help

set -euo pipefail

# ── Defaults ────────────────────────────────────────────────────────────────
REPORT_DIR="reporting/example"
TEMPLATE=""
OUTPUT="report.pdf"
DRAFT=true
DOCX=false
FORCE=false
NO_DOCKER=false
PASSWORD=""
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Tag by version, not :latest — an already-built :latest from a prior
# checkout would otherwise be reused forever, silently hiding image fixes.
IMAGE="report-rex:$(cat "$SCRIPT_DIR/version.txt")"

# ── Portable absolute-path helper ─────────────────────────────────────────────
# `realpath -m` (resolve a path that need not exist) is GNU-only; BSD/macOS
# realpath lacks it. Resolve the parent dir (which exists) and re-attach the
# basename so this works for not-yet-created output files on any platform.
abspath() {
  local path="$1" dir base
  dir="$(dirname "$path")"
  base="$(basename "$path")"
  printf '%s/%s\n' "$(cd "$dir" && pwd)" "$base"
}

# ── Argument parsing ─────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)       REPORT_DIR="$2";  shift 2 ;;
    --template)  TEMPLATE="$2";    shift 2 ;;
    --output)    OUTPUT="$2";      shift 2 ;;
    --final)     DRAFT=false;      shift   ;;
    --docx)      DOCX=true;        shift   ;;
    --force)     FORCE=true;       shift   ;;
    --no-docker) NO_DOCKER=true;   shift   ;;
    --image)     IMAGE="$2";       shift 2 ;;
    --password)  PASSWORD="$2";    shift 2 ;;
    -h|--help)
      sed -n '/^# build.sh/,/^$/p' "$0" | grep '^#' | sed 's/^# \?//'
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# ── Draft filename suffix ─────────────────────────────────────────────────────
if [[ "$DRAFT" == "true" ]]; then
  OUTPUT="${OUTPUT%.pdf}-draft.pdf"
fi

# ── Build args ────────────────────────────────────────────────────────────────
BUILD_ARGS=(build)
BUILD_ARGS+=(--output "$OUTPUT")
[[ -n "$TEMPLATE" ]]     && BUILD_ARGS+=(--template "$TEMPLATE")
[[ "$DRAFT" == "false" ]] && BUILD_ARGS+=(--final)
[[ "$DOCX" == "true" ]]  && BUILD_ARGS+=(--docx)
[[ "$FORCE" == "true" ]] && BUILD_ARGS+=(--skip-validation)
[[ -n "$PASSWORD" ]]     && BUILD_ARGS+=(--password "$PASSWORD")

# ── Native mode ──────────────────────────────────────────────────────────────
if [[ "$NO_DOCKER" == "true" ]]; then
  REPORT_DIR_ABS="$(realpath "$REPORT_DIR")"
  OUTPUT_ABS="$(abspath "$OUTPUT")"
  TEMPLATES_ROOT="$SCRIPT_DIR/templates"

  echo "▶ Building (native): $REPORT_DIR_ABS → $OUTPUT_ABS"
  report-rex "${BUILD_ARGS[@]}" \
    --dir "$REPORT_DIR_ABS" \
    --templates-root "$TEMPLATES_ROOT" \
    --output "$OUTPUT_ABS"
  echo "✓ Done: $OUTPUT_ABS"
  exit 0
fi

# ── Docker mode ───────────────────────────────────────────────────────────────
# Resolve absolute paths — Docker requires them for -v mounts.
REPORT_DIR_ABS="$(realpath "$REPORT_DIR")"
OUTPUT_ABS="$(abspath "$OUTPUT")"
OUTPUT_DIR="$(dirname "$OUTPUT_ABS")"
OUTPUT_FILE="$(basename "$OUTPUT_ABS")"

# Create output directory if it doesn't exist
mkdir -p "$OUTPUT_DIR"

echo "▶ Building (Docker: $IMAGE): $REPORT_DIR_ABS → $OUTPUT_ABS"

docker run --rm \
  -v "${REPORT_DIR_ABS}:/report:ro" \
  -v "${SCRIPT_DIR}/templates:/opt/report-rex/templates:ro" \
  -v "${OUTPUT_DIR}:/output" \
  "$IMAGE" \
  "${BUILD_ARGS[@]}" \
  --dir /report \
  --templates-root /opt/report-rex/templates \
  --output "/output/${OUTPUT_FILE}"

echo "✓ Done: $OUTPUT_ABS"
