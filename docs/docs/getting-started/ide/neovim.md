---
sidebar_position: 1
---

# Neovim

Instead of running `rex sync` by hand, `rex editor-conf` generates a Neovim
integration that starts it — plus advisory edit-presence, seeing "bob is
editing this file" before you step on each other — automatically whenever
you open the workspace. It never prevents you from editing; presence is a
hint, not a lock.

This assumes you already have a `rex sync`-managed workspace — see
[Real-Time Collaboration](../server.md) if you haven't set one up yet.

## Installing

```bash
cd my-engagement
rex editor-conf --ide nvim --host rex-svr.example.com --username bob \
  --ca-cert /path/to/rex-svr.crt
```

By default this installs into your existing Neovim config — a lazy.nvim
plugin spec at `<nvim-config>/lua/plugins/rex-presence.lua`, plus the plugin
source at `<nvim-config>/rex-presence-nvim/` — without touching anything
else in it (works with LazyVim or any plain lazy.nvim config). It only
activates when Neovim is opened inside the workspace directory (`--dir`,
default `.`). Restart Neovim (or run `:Lazy sync`) to load it.

Both the required flags:

| Flag | Notes |
|---|---|
| `--ide` | Editor to generate a config for — only `nvim` is supported today |
| `--host` | The `rex-svr` host |
| `--username` | Must match the account you `rex sync` as |

...and the common optional ones:

| Flag | Default | Notes |
|---|---|---|
| `--port` | `9443` | Sync server port |
| `--password` | — | Omit and set `$REX_PASSWORD` instead, to avoid a plaintext password in the generated file |
| `--ca-cert` | — | Path to `rex-svr`'s self-signed certificate, baked into the generated config to trust it |
| `--insecure` | off | Bake `--insecure` (skip TLS verification) into the generated config instead — alternative to `--ca-cert` |
| `--bin` | `"rex"` on `$PATH` | Override if `rex` isn't on `$PATH` |
| `--dir` | `.` | Workspace to keep synced (and, unless `--standalone`, to scope activation to) |
| `--nvim-config` | `$XDG_CONFIG_HOME/nvim` or `~/.config/nvim` | Neovim config directory to install into; ignored with `--standalone` |

Exactly one of `--ca-cert`/`--insecure` is required — `rex-svr` only serves
HTTPS/WSS. If you omit `--password`, export `REX_PASSWORD` before launching
Neovim.

## `--standalone`

For a throwaway/test environment rather than your daily-driver config,
`--standalone` generates a self-contained entrypoint instead of touching
your real Neovim config:

```bash
rex editor-conf --ide nvim --host rex-svr.example.com --username bob --standalone --output ./nvim-test
```

This writes `./nvim-test/rex-presence-nvim/` (plugin source) and
`./nvim-test/rex-presence-init.lua` (entrypoint). Launch it with:

```bash
nvim -u ./nvim-test/rex-presence-init.lua <file>
```

## What it does

Once loaded, the plugin:

- Runs a background `rex sync --dir <workspace>` job, so opening Neovim in
  the workspace is enough to keep files synced — no separately, manually
  launched `rex sync` needed.
- Warns you when opening a file a teammate already has open (`⚠ <user> is
  currently editing this file`), and warns them back — a louder
  `CONCURRENT EDIT` notification — if you start editing it too
  (`InsertEnter`).
- Highlights files a teammate is currently editing (`RexPresenceEditing`,
  linked to `DiagnosticError` by default) inside any loaded file explorer —
  `netrw`, `neo-tree`, `nvim-tree`, or `snacks.nvim` (including LazyVim's
  default `snacks_picker_list`) — matched by filename, no setup required.
- Exposes a statusline component. It's not wired into your statusline
  automatically (LazyVim's `lualine`, for example, already owns
  `statusline`) — add it to your own config:

  ```lua
  require("rex-presence").statusline()
  ```

  Shows nothing when you're alone on a file, `⚠ alice viewing` when someone
  has it open, `⚠ alice editing` once they start typing.

If `rex presence` fails to start, the server is unreachable, or the process
crashes, the plugin simply shows no presence information — it never blocks
editing. The same applies to the background `rex sync` job: if it fails or
dies, editing is unaffected, just unsynced.

## Removing it

```bash
rex editor-conf --ide nvim --remove
```

(add `--standalone --output <dir>` if you generated a standalone config)
deletes the generated plugin files — `--host`/`--username` aren't required
for removal. Restart Neovim for the removal to take effect.

See [Editor presence](../../reference/sync.md#editor-presence) for the full
reference, including the `rex-presence` plugin's `setup()` options for a
fully manual install.
