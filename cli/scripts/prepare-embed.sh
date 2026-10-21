#!/usr/bin/env bash
# Syncs the repo source that the Go binary needs to embed (Dockerfile, core/,
# templates/, vale config, reporting scaffolds, and vuln-templates) into
# cli/internal/embedded/data/.
#
# go:embed cannot reference paths outside its module directory (no "../"), so this
# script materializes a local copy before every `go build`/`go test`. The destination
# is gitignored and regenerated on demand — never edit it directly.
set -euo pipefail

CLI_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_ROOT="$(cd "$CLI_DIR/.." && pwd)"
DATA_DIR="$CLI_DIR/internal/embedded/data"

rm -rf "$DATA_DIR"
mkdir -p "$DATA_DIR/build-context" "$DATA_DIR/scaffolds/full-report" "$DATA_DIR/scaffolds/quick-comm" "$DATA_DIR/editors/nvim" "$DATA_DIR/vuln-templates"

# --- Docker build context: everything the root Dockerfile COPYs ---
cp "$REPO_ROOT/Dockerfile" "$DATA_DIR/build-context/Dockerfile"
cp "$REPO_ROOT/.vale.ini" "$DATA_DIR/build-context/.vale.ini"

cp -R "$REPO_ROOT/core/." "$DATA_DIR/build-context/core/"
find "$DATA_DIR/build-context/core" \
  \( -name '__pycache__' -o -name '.pytest_cache' -o -name '.ruff_cache' -o -name '*.egg-info' \) \
  -exec rm -rf {} + 2>/dev/null || true
find "$DATA_DIR/build-context/core" -name '*.pyc' -delete

cp -R "$REPO_ROOT/templates/." "$DATA_DIR/build-context/templates/"
cp -R "$REPO_ROOT/styles/." "$DATA_DIR/build-context/styles/"

# --- report init scaffolds ---
cp -R "$REPO_ROOT/reporting/example/." "$DATA_DIR/scaffolds/full-report/"
cp -R "$REPO_ROOT/reporting/communication/." "$DATA_DIR/scaffolds/quick-comm/"

# --- editor integrations (for `rex editor-conf`) ---
cp -R "$REPO_ROOT/cli/editors/nvim/." "$DATA_DIR/editors/nvim/"

# --- vuln finding templates (for `rex report section vuln --template`) ---
cp -R "$REPO_ROOT/vuln-templates/." "$DATA_DIR/vuln-templates/"

echo "Embedded source synced into $DATA_DIR"
