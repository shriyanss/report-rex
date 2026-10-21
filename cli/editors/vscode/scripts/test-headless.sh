#!/usr/bin/env bash
# Runs the extension's integration suite (test/integration) in a real,
# headless VS Code against a real rex-svr -- inside Docker with Xvfb, so the
# host needs neither a display, node, nor sudo. Works on any Linux box with
# Docker (e.g. a shared test VM) and in CI.
#
#   scripts/test-headless.sh <dir containing linux rex + rex-svr>
#
# Env: VSCODE_VERSION (default: stable; e.g. 1.85.2 to check the minimum
# supported engine), NODE_IMAGE (default: node:20-bookworm).
set -euo pipefail

EXT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$(cd "${1:?usage: $0 <dir containing rex and rex-svr>}" && pwd)"
for b in rex rex-svr; do
  [[ -x "$BIN/$b" ]] || { echo "missing executable $BIN/$b" >&2; exit 2; }
done

# node_modules is an anonymous volume so the container's Linux install never
# clobbers the host's; .vscode-test is a named volume so the VS Code
# download is cached across runs.
docker run --rm \
  -v "$EXT":/ext \
  -v /ext/node_modules \
  -v rex-vscode-test-cache:/ext/.vscode-test \
  -v "$BIN":/rexbin:ro \
  -e REX_BIN_DIR=/rexbin \
  -e VSCODE_VERSION="${VSCODE_VERSION:-stable}" \
  -w /ext \
  "${NODE_IMAGE:-node:20-bookworm}" \
  bash -euo pipefail -c '
    # The container runs as root; hand build output back to the host user.
    trap "chown -R \$(stat -c %u:%g /ext) /ext/out 2>/dev/null || true" EXIT
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
      xvfb xauth git ca-certificates \
      libnss3 libatk1.0-0 libatk-bridge2.0-0 libgtk-3-0 libgbm1 libasound2 \
      libxkbfile1 libsecret-1-0 libxss1 libdrm2 >/dev/null
    npm ci --no-audit --no-fund
    npm run build
    xvfb-run -a --server-args="-screen 0 1280x1024x24" npm run test:integration
  '
