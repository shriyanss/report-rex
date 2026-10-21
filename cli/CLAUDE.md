# cli/ — Claude Guidance

## What This Is

The Go module for the standalone `rex` binary (Cobra CLI), plus the `rex-svr` sync
server. Wraps the Docker
build pipeline: embeds the Dockerfile, Python `core/`, `templates/`, and reporting
scaffolds at build time, and drives `docker build`/`docker run` on the user's behalf.

## Structure

```
cli/
├── main.go                  calls cmd.Execute()
├── Makefile                 embed / build / test / lint / clean targets
├── scripts/prepare-embed.sh syncs repo source into internal/embedded/data/
├── cmd/                     Cobra command tree (root, image, report build/init, watch, ingest, recon, mcp, version)
├── internal/
│   ├── version/             CLI version (ldflags-injected at build/release time)
│   ├── embedded/            go:embed accessors over internal/embedded/data/
│   ├── report/              native Go port of report_rex_core's parser/preprocessor/
│   │                        validator (issue #15): index.md parsing, combined.md/
│   │                        metadata.yaml assembly, chart generation, QA-gate checks
│   │                        incl. screenshot heuristics (stdlib image, no Pillow/numpy)
│   ├── cvss/                 native Go port of report_rex_core's CVSS v3.x/v4.0 scoring
│   ├── docker/               docker CLI wrapper: preflight, image build/exists, and the
│   │                        final pandoc/XeLaTeX render step (docker + native pandoc) —
│   │                        report assembly itself is native Go via internal/report,
│   │                        not Docker/Python
│   ├── buildscript/         shared BuildFlags struct (mirrors build.sh's flags)
│   ├── scaffold/            `report init` — extracts embedded example/quick-comm trees
│   └── watcher/             fsnotify-based rebuild loop for `watch`
└── .gitignore                internal/embedded/data/, bin/
```

## The embed-at-build step

`go:embed` cannot reference paths outside its own module directory (no `../`). So
`scripts/prepare-embed.sh` copies everything the binary needs to bundle — the root
`Dockerfile`, `.vale.ini`, `core/`, `templates/`, `styles/`, and the
`reporting/example`/`reporting/communication/*` scaffold trees — into a gitignored
`internal/embedded/data/` before every build or test. Run it via:

```bash
make embed   # or: bash scripts/prepare-embed.sh
```

`make build` and `make test` run it automatically. **Never edit files under
`internal/embedded/data/` directly** — they're regenerated from the real source on
every run and are never committed.

## The docker-shell-out pattern

`internal/docker` has no Docker SDK dependency; it shells out to the `docker` CLI
binary via `os/exec`, the same way `build.sh` does. This keeps the binary small,
CGO-free, and trivially cross-compilable.

`RunReportBuild` (`internal/docker/run.go`) parses and assembles the report natively
via `internal/report` — no Docker or Python involved for that part — then renders the
assembled `combined.md`/`metadata.yaml` to a PDF via `pandoc` directly
(`internal/docker/render.go`): inside the `report-rex` image (with `--entrypoint
pandoc`, bypassing its Python entrypoint) by default, or via a local `pandoc`/XeLaTeX
install when `--no-docker` is set. Docker is only in the loop for that final render
step now; if you change `pandocArgs`, check it still matches
`core/src/report_rex_core/renderer.py`'s flags.

## Testing approach

`internal/docker`'s image-build/run logic needs a live Docker daemon to test
meaningfully. Keep daemon-dependent logic behind small, mockable functions
(`ImageExists`, `BuildImage`, `run`) so `go test ./...` stays fast and daemon-free in
CI (`test-go` job); a real end-to-end check (`rex report build` against
`reporting/example`, producing an actual PDF) should be run manually or in a
docker-in-docker job, not as part of the default unit-test lane.

## Adding a New Command

```go
var myCmd = &cobra.Command{
    Use:   "my-command",
    Short: "One-line description shown in --help",
    RunE: func(cmd *cobra.Command, args []string) error {
        // ...
        return nil
    },
}

func init() {
    parentCmd.AddCommand(myCmd)
}
```

## Common Pitfalls

- Forgetting to run `make embed` before `go build`/`go test` outside the Makefile —
  `internal/embedded/data/` won't exist and the build will fail with a missing-embed
  error.
- `rex report build` defaults `--dir` to the current directory (not
  `reporting/example` like `build.sh`) — this is intentional, see root `CLAUDE.md`.
- `rex report init` extracts the full example scaffolds (not
  `core/src/report_rex_core/builder.py`'s minimal starter-text scaffold) — a
  deliberate semantic divergence from the Python `report-rex new` command.
- `cli/editors/vscode/`'s `package.json` `main` field must point at
  `./out/src/extension.js`, not `./out/extension.js` — `tsconfig.json` sets
  `rootDir: "."` (it also compiles `test/**`), so `tsc` mirrors the `src/`
  subdirectory into `out/src/` rather than flattening it. A wrong `main`
  doesn't fail `npm run build` or `npm run lint`/`test` (they never load the
  extension host), so this only surfaces once something actually activates
  the extension — e.g. `vsce package` + `code --install-extension` — which
  is why it went unnoticed until the `.vsix` release job was
  added.
- `npx vsce package` for `cli/editors/vscode/` fails outright
  (`ERROR Couldn't detect the repository...`) unless the extension's
  `package.json` has a `repository` field — `vsce` needs it to resolve the
  README's relative link to `../nvim/README.md`. It auto-detects github.com
  repositories, so no `--baseContentUrl`/`--baseImagesUrl` flags are needed.
