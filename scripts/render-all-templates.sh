#!/usr/bin/env bash
# Render every template against the content it targets, plus the e2e-ingest
# report if present. Data-driven from each template's report-rex.yaml `type`,
# so new templates are covered automatically:
#   * full-report   → render reporting/example (draft + --final)
#   * communication → render each reporting/communication/* one-pager
# Needs pandoc/xelatex/report-rex on PATH — CI runs it inside the runtime image:
#   docker run --rm --entrypoint bash -v "$PWD:/w" -w /w report-rex:ci scripts/render-all-templates.sh
#
# --skip-validation everywhere: this content is the deliberately-fake Lorem
# Ipsum scaffold, meant to fail `rex report validate` unedited — this only
# smoke-tests that templates render.
set -u
mkdir -p ci-output
fail=0

render() { # <dir> <out> [extra args...]
  local dir="$1" out="$2"; shift 2
  if report-rex build --dir "$dir" --templates-root templates --output "$out" --skip-validation "$@" && test -f "$out"; then
    echo "  ✓ $dir → $out"
  else
    echo "  ✗ $dir → $out FAILED"; fail=1
  fi
}

for tdir in templates/*/; do
  tname=$(basename "$tdir")
  ttype=$(grep -E '^type:' "$tdir/report-rex.yaml" | head -1 | sed 's/.*"\(.*\)".*/\1/')
  echo "── Template: $tname (type: ${ttype:-full-report}) ──"
  if [ "$ttype" = "communication" ]; then
    for cdir in reporting/communication/*/; do
      render "$cdir" "ci-output/${tname}-$(basename "$cdir").pdf" --template "$tname"
    done
  else
    render reporting/example "ci-output/example-${tname}-draft.pdf" --template "$tname"
    render reporting/example "ci-output/example-${tname}.pdf" --template "$tname" --final
  fi
done

if [ -f e2e-report/index.md ]; then
  echo "── Ingested asset appendix (e2e-ingest) ──"
  render e2e-report ci-output/e2e-ingest-report.pdf
fi

[ "$fail" -eq 0 ] && echo "✓ All templates rendered successfully" || { echo "✗ One or more templates failed"; exit 1; }
