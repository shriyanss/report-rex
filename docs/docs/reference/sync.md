---
sidebar_position: 5
---

# Real-Time Sync & Editor Presence

`rex` can keep a report workspace synchronized across a team in real time,
backed by a small server (`rex-svr`) that owns the canonical copy and its Git
history. Each teammate's local files are just a normal Git-free directory —
`rex sync` reconciles it against the server in the background.

`rex-svr` ships in the same release archive as `rex` — see [Go CLI Reference →
Installing](./go-cli#installing) if you don't have it yet.

`rex-svr` requires `git` on `PATH` — it refuses to start without it, since
every committed change is materialized as real Git history.

For assigning work between teammates on top of this, see
[Handoffs](./handoffs) — a scoped namespace with its own `rex handoffs`
command, layered on the same server. For open-ended notes visible to the
whole team, see [Team notes](#team-notes) below.

## Running the server

```bash
rex-svr --port 9443 --password-file /path/to/password.txt \
  --db ./rex-svr.db --workspace ./rex-svr-git-workspace
```

| Flag | Default | Notes |
|---|---|---|
| `--port` | `9443` | TCP port to listen on |
| `--password` | — | Shared password clients authenticate with. Mutually exclusive with `--password-file` |
| `--password-file` | — | File containing the shared password. Preferred over `--password` — avoids leaking it into shell history or the process list |
| `--admin-password` | — | Admin password required to [revert the report to an earlier change](#reverting-to-an-earlier-change) or [delete many files at once](#deleting-files-for-everyone). Both are disabled if unset. Mutually exclusive with `--admin-password-file` |
| `--admin-password-file` | — | File containing the admin password. Preferred over `--admin-password` for the same reason as `--password-file` |
| `--db` | `./rex-svr.db` | Path to the server's SQLite database (the canonical store) |
| `--workspace` | `./rex-svr-git-workspace` | Directory `rex-svr` owns exclusively — it materializes every committed change there as real Git history. Never point a client's `rex sync --dir` at it |
| `--log-level` | `info` | `info` or `debug` |
| `--cert` | `./rex-svr.crt` | TLS certificate file. Auto-generated as a self-signed cert alongside `--key` if it doesn't already exist |
| `--key` | `./rex-svr.key` | TLS private key file, auto-generated alongside `--cert` |

Exactly one of `--password`/`--password-file` is required. On startup the
server also runs a drift check that re-materializes the Git working tree from
the database if they've diverged (e.g. after an unclean shutdown) — the
database is always the source of truth, never the working tree. `GET
/api/v1/healthz` (no auth) reports `{status, git_ok, db_ok,
pending_materializations}` for monitoring.

`rex-svr` refuses to start, without changing anything on disk, when:

- `--db`, `--cert`, or `--key` is inside `--workspace` (e.g. `--workspace .`
  with the defaults). The drift check would delete them and every commit
  would add them to Git history.
- `--workspace` is a non-empty directory, or a Git repository, that
  `rex-svr` didn't create. Point it at a new or empty directory instead.
- More than half the files in the workspace aren't in the database. That
  almost always means the wrong or a fresh `--db` (the defaults are relative
  to the directory `rex-svr` is started from). Point `--db` at the database
  that belongs to this workspace, or move the workspace aside to
  deliberately start over.

### TLS trust

All client-server communication is HTTPS/WSS — there is no plaintext mode.
`rex-svr` generates a self-signed certificate on first run (or serves
whichever one `--cert`/`--key` point at) and prints its path and SHA-256
fingerprint on startup. Since it's self-signed, every client command needs
exactly one of:

| Flag | Meaning |
|---|---|
| `--ca-cert <path>` | Pins the exact certificate at that path (the `.crt` file the server printed — distribute it to your team out-of-band, e.g. Slack) |
| `--insecure` | Skips TLS verification entirely — convenient on a trusted network, but doesn't authenticate the server |

Both flags are available on `rex sync`, `rex presence`, `rex conflicts`,
`rex resolve`, `rex handoffs`, and baked into generated configs by
`rex editor-conf` (see below).

## Syncing a workspace

```bash
rex sync --host sync.example.com --username alice --dir ./my-engagement \
  --ca-cert /path/to/rex-svr.crt
```

Prompts for the shared password if `--password` is omitted. Runs until
Ctrl-C, watching the local filesystem and a WebSocket connection to the
server, reconciling on both triggers plus a 30s safety-net ticker. If the
connection drops, `rex sync` keeps working offline (filesystem watching and
the ticker keep running) and reconciles everything once it reconnects,
reconnecting with exponential backoff (1s, doubling, capped at 30s). If the
server no longer recognizes the client's session (it was restarted on a
fresh database), the client logs in again automatically. A file that the
server has no record of at all, not even as deleted, is pushed back
rather than deleted locally, so a server that lost its database is
repopulated by its clients instead of emptying them.

Local sync state lives in `.report-rex/sync.db` inside the workspace
directory (a per-path record of the last-synced hash/revision, plus your
cached login session) — that directory itself is never synced. Reconciliation
always compares content hashes and server revisions, never file modification
times, so touching a file without changing its content is never mistaken for
an edit.

Everything under a `.git`, `.report-rex`, `.vscode`, or
`rex-svr-git-workspace` directory, `.DS_Store`, `*.swp`, `*.swo`, and `*.tmp`
files, `rex-svr`'s default state files (`rex-svr.db*`, `rex-svr.crt`,
`rex-svr.key`), and symlinks are excluded from sync — they're never scanned,
uploaded, or watched. Running `rex sync` in the directory `rex-svr` was
started from therefore never uploads the server's private key or database.

### Conflict resolution

Divergent edits become conflicts rather than silent overwrites:

```bash
rex conflicts --host sync.example.com --username alice --dir ./my-engagement --ca-cert /path/to/rex-svr.crt
rex resolve <path> --host sync.example.com --username alice --dir ./my-engagement --local --ca-cert /path/to/rex-svr.crt
```

`resolve` accepts exactly one of `--local`, `--remote`, `--merge` (three-way
merge, text files only), or `--keep-both`; `--id` disambiguates if a path has
more than one open conflict. See the [`rex resolve` flag reference](./go-cli#rex-resolve)
for the full flag list — including how `--remote`/`--local` mean something
different for a delete-vs-edit conflict than for a normal divergent edit.

`--merge` runs a line-based three-way merge (Myers diff): non-overlapping
changes merge automatically, and overlapping ones are written into the file
as

```
<<<<<<< LOCAL
...
=======
...
>>>>>>> REMOTE
```

Edit the file to remove the markers and re-run `rex resolve <path> --merge`
— it detects the markers are gone and submits your hand-resolved content.

## Admin features

Reverting and deleting for everyone need the server's `--admin-password`.
In VS Code they're hidden unless **Enable Admin** (`report-rex.admin.enabled`)
is on, and they unlock only once the server accepts the admin password.
From the CLI, check a password without changing anything:

```bash
REX_ADMIN_PASSWORD=... rex admin verify --host ... --username alice
```

It fails if the password is wrong or the server has no `--admin-password`.

## Reverting to an earlier change

If a bad change lands, anyone holding the server's `--admin-password` can
roll the **whole report** back to how it stood at one change — from the VS Code
**Recent Changes** view (right-click an entry → *Revert Report to This
Change…*) or with `rex revert`:

```bash
# Preview: every file that would change, that change's git commit, and the
# recent server git log. Prints a plan token.
REX_ADMIN_PASSWORD=... rex revert --host ... --username alice \
  --path reports/vuln-1.md --revision 2 --before

# Apply exactly that plan.
REX_ADMIN_PASSWORD=... rex revert --host ... --username alice \
  --path reports/vuln-1.md --revision 2 --before --plan <token>
```

Without `--before`, the report goes back to right *after* that change; with
it, to right *before* it — undoing that change and everything since. If
anything changes on the server between the preview and the apply, the apply
is refused (`409 stale_plan`) and you preview again. Handoffs are never
touched.

A revert never rewrites history. Each reverted file gets a new revision
holding its old content, so every client just pulls it like any other
update, and the server's Git history gains one `Revert to ...` commit on top
of the bad ones, which stay there.

## Deleting files for everyone

`rex sync` refuses to push a local delete that would remove **at least 5
files and more than 90%** of the synced files: that's far likelier a moved
or emptied folder than intent. Deleting the sync folder out from under a
running `rex sync` (e.g. `mv`-ing it) is an error too, never "every file was
deleted". The refused files stay deleted locally, but nothing happens on the
server. Edits and pulls keep syncing, and each pass logs a warning until you
put the files back or delete them deliberately.

The server enforces the same limit: once one user's per-file deletes in the
last 10 minutes would pass it, `rex-svr` refuses the rest with
`mass_delete_refused` (HTTP 422). Older `rex` clients get stopped too.

To delete them deliberately, anyone holding `--admin-password` can select
the files or folders in the VS Code explorer → right-click → *Report Rex:
Delete for Everyone…*, or run:

```bash
REX_ADMIN_PASSWORD=... rex delete --host ... --username alice reports/vuln-1.md assets/old.png
```

Every client removes them on its next sync. Handoffs and sync-excluded paths
are skipped, the server keeps each file's history, and git gets one
`Delete N file(s) by ... (admin)` commit, so a [revert](#reverting-to-an-earlier-change)
brings them back.

## Blame

See who last changed each line of a synced file, from the server's git
history. In VS Code, right-click a file in the explorer → *Rex Blame* to open
the whole file's blame in a new tab. Rest the cursor on a line for 5 seconds
and its author, age, and commit appear at the end of the line, GitLens-style.
Turn that off with `report-rex.blame.inline`. From the CLI:

```bash
rex blame --host ... --username alice reports/vuln-1.md
```

Server commits batch every edit made within a few seconds, so blame is
per-commit. A line saved moments ago shows as *Not committed yet* until the
server commits it. Blame reads the server's copy, so VS Code skips the inline
annotation while your buffer has unsaved edits.

## Team notes

There's no dedicated command or namespace for shared notes — a plain
`notes/` directory under the workspace root works like any other synced
folder:

```bash
mkdir -p notes
echo "# Scope notes" > notes/scope.md
```

`rex sync` picks it up the same way it picks up everything else, and
`rex-svr` commits it to Git the same way it commits everything else — there's
nothing notes-specific to configure. Unlike [Handoffs](./handoffs), which
scopes each file to one assignee via `to:` frontmatter, notes have no
scoping: every file under `notes/` is visible and writable by the whole
team.

## Editor presence

Sync conflicts are still resolvable after the fact, but most of them are
avoidable if you know a teammate already has a file open. `rex-svr` tracks
**advisory edit presence**: which workspace-relative files are currently
being viewed or edited, per editor session, on a 15-second lease (renewed by
each `presence_update`; a client is expected to send one roughly every 5
seconds, though the server doesn't enforce that cadence — it just expires a
lease that's gone quiet for 15s). It's purely a hint — nothing about it is
ever persisted or enforced, and it never blocks a write.

### Neovim

The first editor integration is a Neovim plugin that spawns two background
bridge processes: `rex presence` (advisory presence, over WebSocket) and
`rex sync` (the actual file sync from the sections above) — opening Neovim
in a report-rex workspace turns it into a live collaborative environment,
not just a presence indicator layered over possibly-stale files.

`rex editor-conf` generates the whole thing — no manual Lua required:

```bash
cd my-engagement
rex editor-conf --ide nvim --host sync.example.com --username alice --ca-cert /path/to/rex-svr.crt
```

By default this installs a [lazy.nvim](https://github.com/folke/lazy.nvim)
plugin spec at `<nvim config>/lua/plugins/rex-presence.lua` (LazyVim, and any
plain lazy.nvim config following its own recommended layout, auto-imports
every file under `lua/plugins/`) plus the plugin itself, vendored alongside
it — it does **not** touch your existing plugins, keymaps, or colorscheme.
Restart Neovim (or `:Lazy sync`) and it's active. It only does anything when
Neovim is opened inside the workspace directory (`--dir`, default the
current directory) — opening Neovim anywhere else is a no-op, so it's safe to
leave installed permanently even though the same Neovim config also handles
your other projects.

Omit `--password` and export `$REX_PASSWORD` instead to avoid baking the
password into the generated files. Re-running the command safely regenerates
both — switching to a different engagement means re-running it, not
accumulating stale configs.

For a disposable environment with no personal Neovim config to preserve
(e.g. a shared test box), add `--standalone` instead: it generates a
self-contained `rex-presence-init.lua` you launch directly with
`nvim -u <that file>`, bypassing whatever's normally installed there.

```bash
rex editor-conf --ide nvim --host sync.example.com --username alice --standalone --output ~/.config/rex-presence
nvim -u ~/.config/rex-presence/rex-presence-init.lua
```

Add `require("rex-presence").statusline()` to your own `lualine`/statusline
config to show `⚠ alice editing` when someone else has the current file
open — not wired in automatically in the default (lazy.nvim plugin) mode,
since `lualine` already owns `statusline` and auto-wiring it could clobber
your own config; the `--standalone` mode does set it for you, since there's
nothing else to clobber there. Opening a file someone's already editing
shows a one-time notification; starting to edit a file someone else is
*already* editing shows a louder one. Files someone else is currently
editing are also highlighted in your file explorer (netrw, `neo-tree`,
`nvim-tree`, or `snacks.nvim`'s explorer — LazyVim's current default) — no
configuration needed. See [Neovim](../getting-started/ide/neovim.md) for the
full option list and setup walkthrough.

### VS Code

A VS Code extension (`cli/editors/vscode/`) drives the same protocol as the
Neovim plugin — spawning `rex presence` and `rex sync` as background
processes — but surfaces presence through native VS Code APIs
(`FileDecorationProvider`, a status bar item, `SecretStorage`) instead of
Lua autocommands.

Unlike the Neovim plugin, which `rex editor-conf` generates per project,
the VS Code extension is configured entirely through its own Settings UI —
no generated files:

```jsonc
// .vscode/settings.json, or User Settings
{
  "report-rex.host": "sync.example.com",
  "report-rex.username": "alice"
}
```

Then run **Report Rex: Set Password** from the Command Palette — the
password is stored in VS Code's `SecretStorage`, not a plain setting, since
settings are visible/exportable. The extension only activates in a
workspace containing `report-rex.yaml` or `index.md`.

It highlights files someone else is currently editing directly in the
Explorer (a native `FileDecorationProvider` badge, keyed by exact
workspace-relative path — no basename-collision ambiguity, unlike the
Neovim plugin's rendered-buffer-text scan), shows the same one-time/louder
notifications as Neovim on open/concurrent-edit, and reflects presence in
the status bar. It also runs `rex sync` in the background, same as Neovim.

Conflict resolution stays a CLI-only workflow (`rex conflicts`/`rex
resolve`, above) — there's no in-editor merge UI. See
[VS Code](../getting-started/ide/vscode.md) for the full settings list,
install steps, and manual test steps.

### Other editors

JetBrains and anything else can drive the same protocol via `rex
presence`'s stdin/stdout JSON-lines interface, or talk to `rex-svr`'s
`/api/v1/ws` presence messages and `GET /api/v1/presence` directly — the
protocol is editor-independent.

## HTTP & WebSocket API

For editor integrations that want to talk to `rex-svr` directly instead of
shelling out to `rex presence`. All routes are under `/api/v1` on `https://`
(the WebSocket route upgrades from `https://`, i.e. `wss://`) — `rex-svr`
serves no plaintext protocol — JSON in/out, and (except login/`healthz`)
require `Authorization: Bearer <token>` — or a `?token=` query param, since
browsers can't set headers on a WebSocket upgrade request. See
[TLS trust](#tls-trust) above for trusting the server's self-signed
certificate.

| Route | Notes |
|---|---|
| `POST /auth/login` | `{username, password}` → `{token, username}`. Any username is accepted as long as the password matches the server's one shared password — there's no per-user credential. Tokens don't expire; there's no logout endpoint |
| `GET /manifest` | `{files: [{path, revision, sha256, size, deleted, modified_by, updated_at}]}` for the whole workspace |
| `GET /files/{path}` | Raw file bytes, with `X-Revision`/`X-Sha256`/`X-Modified-By` headers. `410 Gone` (with `X-Deleted: true`) if the path is tombstoned |
| `POST /files/{path}` | Upload. `X-Base-Revision` header states which revision you're building on (omit for a brand-new path); a mismatch means someone else changed it first — `409` with a `conflict_id` rather than a silent overwrite |
| `DELETE /files/{path}` | Requires `X-Base-Revision`; same `409`-on-mismatch behavior |
| `GET /conflicts` | Open conflicts: `{id, path, kind, remote_revision, created_at}` |
| `GET /conflicts/{id}?include=local,remote,base` | Conflict detail; `include` opts into fetching each side's actual content (omitted by default to keep the response small) |
| `POST /conflicts/{id}/resolve` | `{resolution, merged_content?, keep_both_suffix?}` — mirrors `rex resolve`'s resolutions (`use_remote`, `use_local`, `merge`, `keep_both`, `keep_deletion`, `restore_local`) |
| `POST /revert/preview` | `{path, revision, before?, admin_password}` → `{ops: [{path, action, from_revision, target_revision?, content?}], plan, commit?, git_log}`. `403` if `--admin-password` is unset or wrong |
| `POST /revert` | Same body plus `plan` from the preview → `{ops, commit?}`. `409 stale_plan` if the workspace moved since the preview |
| `POST /admin/verify` | `{admin_password}` → `{ok: true}`. Changes nothing. `403` if `--admin-password` is unset or wrong |
| `POST /admin/delete` | `{paths, admin_password}` → `{deleted: [{path, revision, deleted}], skipped, commit?}`. Missing, already-deleted, excluded and handoff paths are skipped. `403` if `--admin-password` is unset or wrong |
| `GET /blame/{path}` | `{path, lines: [{line, commit?, user, time, summary, text}]}` from `git blame` on the server's workspace. `404` if the path doesn't exist or has no commit yet, `503` without git |
| `POST /sync/restore` | Same response shape as `GET /manifest` — the actual workspace overwrite happens client-side; this endpoint exists for symmetry with the other `/sync/*`-style clients |
| `GET /presence` | Snapshot of all current presence leases — `{presence: [{path, username, session_id, state, dirty, expires_at}]}` |
| `GET /ws` | WebSocket upgrade. Server → client: `file_updated`, `conflict_created`, `presence` messages. Client → server: `presence_update {path, session_id, state, dirty}` and `presence_release {path, session_id}` — every `presence_update` renews that session's 15s lease, there's no separate heartbeat message type |
| `GET /healthz` | No auth. `{status, git_ok, db_ok, pending_materializations}` |

File content never travels over the WebSocket — `file_updated` just tells a
client "this path changed, go re-fetch it," which is why `rex sync` treats
every WS message as "re-run a full reconcile pass" rather than trying to
apply a diff.
