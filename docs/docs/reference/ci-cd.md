---
sidebar_position: 3
---

# CI/CD

Report Rex uses GitHub Actions. Three workflows live in `.github/workflows/`:

| Workflow | Runs on | What it does |
|----------|---------|--------------|
| `ci.yml` | Every push to `main`/`dev`, every pull request | Lint and test everything |
| `docs.yml` | Push to `main` (build-only on pull requests touching `docs/`) | Build and deploy the docs to GitHub Pages |
| `release.yml` | `vX.Y.Z` tags | Publish binaries, the VS Code extension, the Docker image, and the Python package |

## CI jobs

| Job | What it exercises |
|-----|--------------------|
| `lint` | `ruff check core/src/`, `scripts/check_vuln_templates.py`, Vale prose style over `reporting/`, `docs/docs/`, `vuln-templates/`, and `gofmt`/`go vet` for `cli/` |
| `test-python` | `pytest core/tests/` with coverage |
| `test-go` | `go build`/`go test` for `cli/`, plus the `-tags e2e` suite: real `rex`/`rex-svr` subprocesses covering sync, conflicts, restarts, handoffs, and data safety |
| `vscode` | Lint, build, and `node:test` suite for the VS Code extension |
| `e2e-cli` | Builds the real `rex` binary and drives `rex image build` / `rex report init` against the runner's Docker daemon |
| `e2e-ingest` | Runs `rex ingest nmap`/`naabu`/`nikto` against `scanme.nmap.org` (nmap's own public test target) and imports a BloodHound sample dataset, checking the generated appendices and `index.md` registration |
| `render-test` | Builds the runtime image from `Dockerfile` and runs `scripts/render-all-templates.sh` inside it: every template against the content it targets, plus `e2e-ingest`'s scanned report. PDFs are uploaded as the `ci-output` artifact |

Jobs that build `cli/` run `bash cli/scripts/prepare-embed.sh` first. It syncs the
Dockerfile, `core/`, `templates/`, and example scaffolds into the gitignored
`cli/internal/embedded/data/` directory that `go:embed` needs.

To run the render smoke test locally:

```bash
docker build -t report-rex:ci .
docker run --rm --entrypoint bash -v "$PWD:/w" -w /w report-rex:ci scripts/render-all-templates.sh
```

## Docs

`docs.yml` builds the Docusaurus site with `DOCS_BASE_URL=/report-rex/` and deploys
it with GitHub Pages. Without `DOCS_BASE_URL` the site builds for the root path
(`/`), which is what local builds and self-hosted deployments use.

## Publishing a Release

Bump `version.txt`, then tag a commit on `main`:

```bash
git tag v1.0.0
git push origin v1.0.0
```

`release.yml` runs four jobs:

- **`goreleaser`** — cross-compiles `rex`/`rex-svr` for all 5 target platforms
  and creates the GitHub Release with the archives and `checksums.txt`.
- **`vsix`** — packages the `rex-presence` VS Code extension and attaches the
  `.vsix` to that release. This is the extension's only distribution channel (see
  [VS Code](../getting-started/ide/vscode.md#installing)).
- **`docker`** — builds the runtime image for `linux/amd64` and `linux/arm64` and
  pushes `ghcr.io/shriyanss/report-rex:<tag>` and `:latest`.
- **`pypi`** — builds `report-rex-core` and uploads it to PyPI with trusted
  publishing. Already-published versions are skipped, since `report-rex-core`'s
  version doesn't move in lockstep with `version.txt`.

### One-time repository setup (forks)

- **Pages**: Settings → Pages → Source: *GitHub Actions*.
- **PyPI**: add a trusted publisher for the repository, workflow `release.yml`,
  environment `pypi`.
- **GHCR**: after the first release, set the `report-rex` package's visibility to
  public.

## Building Reports in CI

To generate PDFs inside a pipeline, run the Python CLI inside the published image:

```yaml
jobs:
  report:
    runs-on: ubuntu-latest
    container: ghcr.io/shriyanss/report-rex:latest
    steps:
      - uses: actions/checkout@v4
      - run: report-rex build --dir reporting/my-engagement --output report.pdf --skip-validation
      - uses: actions/upload-artifact@v4
        with:
          name: report
          path: report.pdf
```

Call `report-rex` directly rather than `rex report build` or `./build.sh`. The job
already runs inside the image, so it needs no Docker-in-Docker.
