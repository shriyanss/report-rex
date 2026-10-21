# rex-presence (VS Code)

VS Code integration for Report Rex's advisory editor presence (issue #13,
the VS Code counterpart to the [Neovim
plugin](../nvim/README.md)/issue #11): it warns you when a teammate is
already viewing or editing the file you just opened, warns them back if you
start editing it too, and highlights those files directly in the Explorer.
It never prevents you from editing — presence is a hint, not a lock.

If `report-rex.dir`/the workspace root looks like a report-rex workspace, it
can also run `rex sync` in the background, so opening the workspace is
enough to keep files synced too — no separately, manually launched `rex
sync` needed. Sync is **off by default** per workspace; turn it on from the
**Report Rex** panel in the Explorer sidebar, or the **Report Rex: Toggle
Sync** command (issue #38).

It also lints: on save, it runs `rex report validate` against the workspace
and surfaces every issue in the Problems panel/as squiggles (issue #26). The
chart-data files `artifacts/{assets,connections,webfindings}.json` are linted
too (issue #53). They're watched by filename, so a change re-lints even when
`rex ingest` makes it outside the editor.
Markdown is also linted as you type, debounced, using the unsaved buffers
(`rex report validate --stdin`), so broken finding frontmatter (invalid
YAML, a missing `title:`, a `---` block not on the first line) shows up
right away (issue #58). `title_missing` has a quick fix that inserts a
`title:` line.
Linting is independent of presence/sync — it works with no `report-rex.host`/
`.username` configured at all.

## How it works

Unlike Neovim/Lua, Node has native WebSocket support — but this extension
still spawns the same small bridge processes the Neovim plugin does,
`rex presence` and `rex sync`, rather than reimplementing the WS/REST
protocol, reconnect/backoff, and file-watch reconciliation in TypeScript.
`rex presence` holds the actual connection to `rex-svr`; the extension just
tells it what document you're looking at (over NDJSON on stdin/stdout) and
reads back who else is around. A second background process, plain
`rex sync`, handles the actual file sync.

```
VS Code (TS)  <--stdio(NDJSON)-->  rex presence  <--WebSocket-->  rex-svr
VS Code (TS)  <--child_process--->  rex sync     <--HTTP/WS----->  rex-svr
```

## Requirements

- `rex` on your `$PATH` (or set `report-rex.bin` to its full path).
- A workspace whose root contains `report-rex.yaml` or `index.md` (the
  extension only activates then) — the same workspace-relative-path
  assumption `rex-svr`/`rex sync` both make, so open VS Code at the
  workspace root, not a subdirectory.

## Setup

Unlike the Neovim plugin (which is generated per-project via
`rex editor-conf`), this extension is configured entirely through VS
Code's own Settings UI, or the **Report Rex** panel in the Explorer sidebar
(issue #38) — no generated files. Every `report-rex.*` value the panel
writes lands in this workspace's settings, never User/global settings, so
different workspaces/engagements never share a host, dir, or password.

| Setting | Default | Notes |
|---|---|---|
| `report-rex.host` | *(required)* | `rex-svr` host |
| `report-rex.username` | *(required)* | must match the account you `rex sync` as |
| `report-rex.port` | `9443` | |
| `report-rex.dir` | workspace root | workspace directory to sync/scope activation to |
| `report-rex.sync.enabled` | `false` | run `rex sync` in the background; off until turned on per workspace |
| `report-rex.admin.enabled` | `false` | **Enable Admin**: show the admin features (revert, delete for everyone); see [Admin features](#admin-features) |
| `report-rex.bin` | `"rex"` | override if `rex` isn't on `$PATH` |
| `report-rex.verbose` | `false` | log presence-bridge/sync-job connection errors to the "Report Rex" output channel |
| `report-rex.lint.enabled` | `true` | run `rex report validate` on save and as you type; toggle at runtime via **Report Rex: Toggle Linter** |
| `report-rex.projectOverride` | `"auto"` | `"auto"` detects a rex report project via `index.md`; `"on"`/`"off"` force linting on/off regardless — set via **Report Rex: Set Rex Project Override** |

The password isn't a setting (plain settings are visible/exportable, the
wrong place for a secret) — click the Password row in the panel, or run
**Report Rex: Set Password** from the Command Palette; it's stored in VS
Code's `SecretStorage`, keyed per workspace folder so it's never shared
across workspaces. Changing any `report-rex.*` setting, or setting the
password, restarts the bridges automatically — no reload needed.

### Report Rex panel

The **Report Rex** view in the Explorer sidebar shows, and lets you edit,
this workspace's state at a glance: whether Sync is on/off (click to
toggle), whether `rex-svr` is currently reachable (Online/Offline/Unknown,
read-only), and the Host/Port/Username/Directory/Password rows (click any of
them to change it). It's a plain `TreeView`, not a webview — every row just
runs the same input-box/command flow the Command Palette commands above do.

### Building and starting a report

The **Report** view (top of the Report Rex sidebar) has one-click Build
Draft / Build Final (PDF lands in `.report-rex/build/`, never synced) and Add
Finding / Appendix / Narrative Section. In a folder with no report yet it
shows **Initialize Report** instead, which asks for the type (full report or
quick-comm), title, client and date and runs `rex report init` for you.
Hidden entries like `.git/` or `.vscode/` don't stop init.

### Admin features

Reverting the report and deleting files for everyone are admin-only and
hidden by default. Turn on **Enable Admin** (`report-rex.admin.enabled`) to
show them. Turning it on asks for `rex-svr`'s `--admin-password` and checks
it against the server (`rex admin verify`); if you skip that, the first admin
action asks instead. The password stays in memory for the session only. It's
forgotten when you reconnect, turn the setting off, or the server rejects it.

### Reverting the report

Right-click an entry in **Recent Changes** → *Revert Report to This
Change…* to roll the whole report back, either to right after that change
or to right before it (undoing it and everything since). Needs
[admin features](#admin-features). It lists every file that would change
(pick one to diff it), the change's git commit, and the server's git log
before you confirm. See
[the sync docs](../../../docs/docs/reference/sync.md#reverting-to-an-earlier-change).

### Deleting files for everyone

`rex sync` won't push a delete of nearly the whole report (a moved or emptied
folder); it warns instead. To delete many files on purpose, select them (or
folders) in the explorer → right-click → *Report Rex: Delete for Everyone…*,
and confirm (needs [admin features](#admin-features)). See
[the sync docs](../../../docs/docs/reference/sync.md#deleting-files-for-everyone).

### Rex Blame

Right-click a file in the explorer → *Rex Blame* to open who last changed
each line in a new tab. Rest the cursor on a line for 5 seconds to see its
author and age at the end of the line (`report-rex.blame.inline`). See
[the sync docs](../../../docs/docs/reference/sync.md#blame).

## Feature parity with the Neovim plugin

- **File decorations**: files someone else is currently editing get a red
  badge in the Explorer (and everywhere else VS Code shows a file: tabs,
  quick-open) — via `vscode.window.registerFileDecorationProvider`, VS
  Code's native equivalent of the Neovim plugin's rendered-explorer-text
  basename scan. Matched by exact workspace-relative path, not basename, so
  two same-named files in different directories don't cross-highlight (a
  strict improvement over the Neovim plugin).
- **Status bar**: shows `⚠ alice editing`/`⚠ alice viewing` for the active
  editor when someone else has it open.
- **Notifications**: opening a file someone else already has open shows a
  one-time warning; starting to edit a file someone else is *already*
  editing shows a louder "CONCURRENT EDIT" error notification.
- **Background sync**: `rex sync` runs for the lifetime of the extension,
  once enabled per workspace (`report-rex.sync.enabled`, off by default).
- **Linter**: `rex report validate --dir <dir> --json` runs on save of a
  markdown file, on any create/change/delete of an
  `artifacts/{assets,connections,webfindings}.json` file (debounced 300ms), and
  on activation. Every issue it finds is registered against the relevant file
  as a diagnostic (Problems panel + squiggle), grouped by file with the failing
  check's name in the message. An issue with a `line` (chart directives,
  `assets.json`/`connections.json` entries) underlines that line; the rest have
  no position and sit on line 1 of the file they name.

**Not included** (stays a CLI-only workflow, same as today): conflict
resolution. Use `rex conflicts`/`rex resolve` from a terminal — there's no
in-editor merge UI here.

## Installing

Every release attaches a `.vsix` to its GitHub Release — see [the VS Code getting-started
page](../../../docs/docs/getting-started/ide/vscode.md#installing) for the
download command. Prefer that for day-to-day use; build from source (below)
only when testing an unreleased change.

## Building & testing

```bash
cd cli/editors/vscode
npm install
npm run build   # tsc
npm run test    # node --test — pure NDJSON-parsing/presence-reducer logic only
```

There's no VS Code extension host in CI, so `FileDecorationProvider`
registration, the status bar, the Report Rex panel, and actually spawning
`rex` are manual-check only:

1. `npm run build && npx vsce package` to produce a `.vsix`.
2. `code --install-extension rex-presence-0.1.0.vsix`.
3. Open a report-rex workspace, open the **Report Rex** view in the
   Explorer sidebar. Confirm it shows Sync: Off and no `rex sync` process is
   running (`ps`). Click Host/Username/Port/Directory, fill them in, and
   confirm the values land in this workspace's `.vscode/settings.json`, not
   User settings. Click Password and set one; run **Report Rex: Set
   Password** again for a second workspace and confirm the two don't share a
   password. Click Sync to toggle it on and confirm a `rex sync` process
   starts and the row flips to On; confirm the Server row shows
   Online/Offline against a reachable/unreachable `rex-svr`.
4. From a second client (e.g. the Neovim plugin, or another VS Code window
   against the same `rex-svr`), open/edit a shared file and confirm: a red
   badge appears on that file in the Explorer, the status bar reflects it
   when you open the same file, and opening/concurrently editing it shows
   the expected notifications.
5. With Sync on, edit a file locally and confirm it shows up via
   `GET /api/v1/manifest` on the server, confirming `rex sync` is running.
6. Open a `reports/*.md` finding, remove its `## Replication` section
   or trim its screenshot caption below 35 characters, and save — confirm a
   Problems-panel entry/squiggle appears. Run **Report Rex: Toggle Linter**
   and confirm it clears; toggle back on and save again to confirm it
   reappears.

## Failure behavior

If `rex presence` fails to start, the server is unreachable, or the process
crashes, the extension simply shows no presence information — it never
blocks editing. The same applies to the background `rex sync` job: if it
fails to start or dies, editing is unaffected, just unsynced. Enable
`report-rex.verbose` to see connection problems in the "Report Rex" output
channel.

Likewise, if `rex report validate` fails to spawn or exits with unparseable
output, diagnostics are simply cleared rather than shown stale — linting
never blocks or delays saving.
