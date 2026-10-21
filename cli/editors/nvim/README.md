# rex-presence.nvim

Neovim integration for Report Rex's advisory editor presence: it warns you
when a teammate is already viewing or editing the file you just opened, and
warns them back if you start editing it too. It never prevents you from
editing — presence is a hint, not a lock (see the parent issue's design
notes for why).

If `dir` is set, it also runs `rex sync` in the background for that
workspace, so opening Neovim there is enough to keep files synced too — no
separately, manually launched `rex sync` needed.

It also lints: on save, it runs `rex report validate` against `dir` and
surfaces every issue as a `vim.diagnostic` (issue #26). Linting is
independent of presence/sync — it works with no `host`/`username` passed to
`setup()` at all.

## How it works

There's no WebSocket client in Neovim/Lua, so this plugin spawns a small
bridge process, `rex presence`, and talks to it over stdin/stdout as
newline-delimited JSON. `rex presence` holds the actual connection to
`rex-svr` and does the reconnect/backoff handling; the plugin just tells it
what buffer you're looking at and reads back who else is around. A second
background process, plain `rex sync`, handles the actual file sync.

```
Neovim (Lua)  <--stdio-->  rex presence  <--WebSocket-->  rex-svr
Neovim (Lua)  <--jobstart-->  rex sync   <--HTTP/WS-->    rex-svr
```

## Requirements

- `rex` on your `$PATH` (or pass `bin = "/path/to/rex"` to `setup`).
- Neovim run from your workspace root (e.g. `cd my-engagement && nvim
  reports/findings/sql-injection.md`) — presence is reported using paths
  relative to Neovim's `cwd`, since `rex-svr`/`rex sync` both work in
  workspace-relative paths too.

## Quick start

`rex editor-conf --ide nvim --host ... --username ...` generates the whole
thing from the `rex` binary alone — no manual Lua needed. By default it
installs a lazy.nvim plugin spec into your existing Neovim config (LazyVim,
or any plain lazy.nvim config using its own recommended `lua/plugins/`
layout) — see the [sync reference
docs](https://github.com/shriyanss/report-rex/blob/main/docs/docs/reference/sync.md#neovim)
for the full flow, including `--standalone` for a throwaway/test environment.
The rest of this file documents the plugin itself, for a manual install.

## Setup

With `lazy.nvim`, pointing at this path in the repo (adjust to how you've
vendored/cloned `report-rex`):

```lua
{
  dir = "~/report-rex/cli/editors/nvim",
  name = "rex-presence",
  config = function()
    require("rex-presence").setup({
      host = "rex-svr.lan",
      username = "alice",
      -- password = "...",       -- or set $REX_PASSWORD before starting nvim
      dir = "/abs/path/to/my-engagement", -- keeps this workspace synced via rex sync
      scoped = true,              -- only activate when Neovim is opened inside `dir`
    })
  end,
}
```

`setup()` options:

| Option              | Default          | Notes                                          |
|---------------------|------------------|-------------------------------------------------|
| `host`               | *(required for presence/sync)* | `rex-svr` host — linting works without it |
| `username`           | *(required for presence/sync)* | must match the account you `rex sync` as |
| `port`               | `9443`           |                                                  |
| `password`           | `$REX_PASSWORD`  | avoid hard-coding it in your config              |
| `bin`                | `"rex"`          | override if `rex` isn't on `$PATH`               |
| `dir`                | *(none)*         | absolute workspace path; also the target of a background `rex sync` job and the `--dir` passed to `rex report validate` |
| `scoped`             | `false`          | when true (with `dir` set), `setup()` does nothing unless Neovim's cwd is inside `dir` — for a config installed permanently alongside your other projects |
| `verbose`            | `false`          | notify on presence-bridge/sync-job connection errors |
| `lint_enabled`       | `true`           | run the linter; toggle at runtime with `:RexLintToggle` |
| `project_override`   | `"auto"`         | `"auto"` detects a rex report project via `index.md`; `"on"`/`"off"` force linting on/off regardless — set at runtime with `:RexProjectOverride {auto\|on\|off}` |

## Statusline

```lua
vim.o.statusline = "%f %{v:lua.require('rex-presence').statusline()}"
```

Shows nothing when you're alone on a file, `⚠ alice viewing` when someone
has it open, `⚠ alice editing` once they start typing. Not auto-wired by
`rex editor-conf`'s default (lazy.nvim plugin) mode, since LazyVim's
`lualine` already owns `statusline` — add
`require("rex-presence").statusline()` to your own `lualine` sections
instead if you want it there.

## Notifications

- Opening a file someone else already has open: a one-time `⚠ <user> is
  currently editing this file` notification.
- Starting to edit (`InsertEnter`) a file someone else is *already* editing:
  a louder `CONCURRENT EDIT` notification.

Neither of these repeats on every keystroke — see `init.lua` for the exact
triggers (`BufEnter`, `InsertEnter`, `BufLeave`/`VimLeavePre`).

## File explorer highlighting

Files someone else is currently editing are highlighted (`RexPresenceEditing`,
linked to `DiagnosticError` by default) inside any loaded `netrw`, `neo-tree`,
`nvim-tree`, or `snacks.nvim` explorer (`snacks_picker_list`, LazyVim's
current default) buffer — matched by filename, so a same-named file in a
different directory highlights too. No setup required.

## Linting

`rex report validate --dir <dir> --json` runs once on `setup()` and again on
every `BufWritePost` of a `*.md` file, and every issue it finds is applied as
a `vim.diagnostic` on the buffer it names (sign column, virtual text,
`:Telescope diagnostics`/`vim.diagnostic.setqflist()`, whatever you already
use for diagnostics). An `Issue` has no line/column info, so every diagnostic
spans line 1 of the file it names — a known limitation of
`rex report validate` itself, not something faked here.

Commands:

| Command | Effect |
|---|---|
| `:RexLintRun` | Run `rex report validate` now and refresh diagnostics |
| `:RexLintToggle` | Turn the linter on/off at runtime |
| `:RexProjectOverride {auto\|on\|off}` | Override whether `dir` is treated as a rex report project |

## Manual check

There's no Neovim runtime in CI, so the pure logic (stdout line
parsing/buffering, statusline text, explorer highlight matching, lint
issue-to-diagnostic mapping) has a standalone self-check instead of a
wired-in test suite. Run it after touching `bridge.lua`, `statusline.lua`,
`explorer.lua`, or `lint.lua`:

```bash
cd cli/editors/nvim
nvim --headless -l lua/rex-presence/bridge_spec.lua
nvim --headless -l lua/rex-presence/explorer_spec.lua
nvim --headless -l lua/rex-presence/lint_spec.lua
```

Prints `ok` and exits 0 on success.

## Failure behavior

If `rex presence` fails to start, the server is unreachable, or the process
crashes, the plugin simply shows no presence information — it never blocks
editing. The same applies to the background `rex sync` job: if it fails to
start or dies, editing is unaffected, just unsynced. Set `verbose = true` in
`setup()` to get a notification when either reports a connection problem.

Likewise, if `rex report validate` fails to spawn or exits with unparseable
output, diagnostics are simply cleared rather than shown stale — linting
never blocks or delays saving.
