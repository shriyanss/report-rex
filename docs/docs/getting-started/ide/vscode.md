---
sidebar_position: 2
---

# VS Code

There's a VS Code extension with the same advisory presence + background-sync
features as the [Neovim integration](./neovim.md), configured through the
Settings UI instead of a generated file. It warns you when a teammate is
already viewing or editing the file you just opened, warns them back if you
start editing it too, and highlights those files in the Explorer. It never
prevents you from editing — presence is a hint, not a lock.

This assumes you already have a `rex sync`-managed workspace — see
[Real-Time Collaboration](../server.md) if you haven't set one up yet.

## Installing

The extension isn't published to the VS Code Marketplace, but every version
release attaches a `.vsix` to its [GitHub Release](https://github.com/shriyanss/report-rex/releases).
Download and install it:

```bash
gh release download --repo shriyanss/report-rex --pattern 'rex-presence-*.vsix'
code --install-extension rex-presence-*.vsix
```

### Building from source

To build the `.vsix` yourself instead (e.g. to test an unreleased change):

```bash
cd cli/editors/vscode
npm install
npm run build        # tsc
npx vsce package    # produces rex-presence-0.1.0.vsix
code --install-extension rex-presence-0.1.0.vsix
```

There's no VS Code extension host in CI, so after installing, verify it
manually:

1. Open a report-rex workspace, set `report-rex.host`/`.username` in
   Settings, then run **Report Rex: Set Password**.
2. From a second client (e.g. the Neovim plugin, or another VS Code window
   against the same `rex-svr`), open/edit a shared file and confirm: a red
   badge appears on that file in the Explorer, the status bar reflects it
   when you open the same file, and opening/concurrently editing it shows
   the expected notifications.
3. Edit a file locally and confirm it shows up via `GET /api/v1/manifest`
   on the server (see [Sync Reference](../../reference/sync.md)), confirming
   `rex sync` is running.

Requirements:
- `rex` on your `$PATH` (or set `report-rex.bin` to its full path).
- Open VS Code at the workspace **root** — the extension only activates when
  the root contains `report-rex.yaml` or `index.md`, since presence and sync
  both work in workspace-relative paths.

## Setup

Unlike the Neovim plugin (generated per-project via `rex editor-conf`), this
extension is configured entirely through VS Code's own Settings UI:

| Setting | Default | Notes |
|---|---|---|
| `report-rex.host` | *(required)* | `rex-svr` host to connect to |
| `report-rex.username` | *(required)* | Must match the account you also `rex sync` as |
| `report-rex.port` | `9443` | |
| `report-rex.dir` | workspace root | Workspace directory to sync/scope activation to |
| `report-rex.caCert` | — | Path to `rex-svr`'s self-signed certificate, to trust it. `rex-svr` only serves HTTPS/WSS, so this or `report-rex.insecure` is required |
| `report-rex.insecure` | `false` | Skip TLS certificate verification — alternative to `report-rex.caCert` |
| `report-rex.bin` | `"rex"` | Path to the `rex` binary, if it isn't on `$PATH` |
| `report-rex.verbose` | `false` | Notify on presence-bridge/sync-job connection errors, in the "Report Rex" output channel |

The password isn't a setting (plain settings are visible/exportable — the
wrong place for a secret). Instead, run **Report Rex: Set Password** from the
Command Palette; it's stored in VS Code's `SecretStorage`. Changing any
`report-rex.*` setting, or setting the password, restarts the background
processes automatically — no reload needed.

## What it does

Once configured, the extension:

- Runs `rex sync` in the background for the lifetime of the extension — no
  separately, manually launched `rex sync` needed.
- Shows a red badge in the Explorer (and tabs, quick-open — anywhere VS Code
  shows a file) on files a teammate is currently editing, matched by exact
  workspace-relative path.
- Shows `⚠ alice editing` / `⚠ alice viewing` in the status bar for the active
  editor when someone else has it open.
- Shows a one-time warning notification when you open a file someone else
  already has open, and a louder "CONCURRENT EDIT" error notification if you
  start editing a file someone else is *already* editing.

It also lints the report on save, running `rex report validate` and showing
each issue in the Problems panel and as a squiggle. This works without any
`rex-svr` too. Besides the markdown files, it lints the chart-data files by
name: `artifacts/assets.json`, `connections.json`, and `webfindings.json`. It
re-checks whenever one of them changes (including rewrites by `rex ingest`),
and checks them against how the report's files use them. For example, a
`:::network_diagram:::` with no `assets.json` is flagged on the directive's
line, and a typo'd pivot IP is flagged on its line in `connections.json`. Toggle
it with **Report Rex: Toggle Linter**.

Markdown is also linted as you type (half a second after you stop), using the
unsaved text. Broken finding frontmatter shows up right away instead of as an
"Untitled" finding in the PDF: invalid YAML such as an unclosed quote (flagged
on its line), a missing `title:`, a finding with no frontmatter, and a `---`
block that doesn't start on the very first line. A missing title has a quick
fix (the lightbulb, or `Ctrl+.`/`Cmd+.`) that inserts a `title:` line.

It also works without any `rex-svr`: the **Report** view at the top of the
Report Rex sidebar builds the draft or final PDF and adds findings,
appendices and narrative sections in one click. **Add Finding** lets you
start from a blank finding or one of the bundled `vuln-templates/` stubs
(SSRF, XXE, CSRF, ...), then asks for an optional filename. Leave it empty to
get the next `reports/vuln-N.md`. Open an empty folder and the
same view offers **Initialize Report**, which runs `rex report init` with the
type, title, client and date you pick (hidden `.git/`/`.vscode/` entries are
fine).

**Not included** (stays a CLI-only workflow): conflict resolution. Use
`rex conflicts`/`rex resolve` from a terminal — there's no in-editor merge UI.
See [Conflict resolution](../../reference/sync.md#conflict-resolution).

If `rex presence` fails to start, the server is unreachable, or the process
crashes, the extension simply shows no presence information — it never
blocks editing. The same applies to the background `rex sync` job: if it
fails or dies, editing is unaffected, just unsynced. Enable
`report-rex.verbose` to see connection problems in the "Report Rex" output
channel.

See [Editor presence → VS Code](../../reference/sync.md#vs-code) for the full
reference.
