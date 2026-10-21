---
sidebar_position: 2
---

# Real-Time Collaboration

A report engagement is rarely one person's work. `rex-svr` (a small sync
server) plus `rex sync` let a team keep one workspace in sync — no shared
drive, no "who has the latest copy" — with divergent edits flagged as
conflicts instead of silently overwritten. This page walks through going
from a single scaffolded report to a synced multi-person workspace; see the
[Sync Reference](../reference/sync.md) for the full command/API reference,
conflict resolution, and the Neovim/VS Code editor integrations' details.

Once a team is synced, [Handing Work to a Teammate](./handoffs.md) covers
passing a thread of investigation to whoever should pick it up.

Both `rex` and `rex-svr` ship in the same release archive — see
[Install & Build Your First Report](./cli.md) if you don't have them yet.
`rex-svr` also requires `git` on `PATH` — it refuses to start without it,
since every accepted change is materialized as real Git history.

## 1. Scaffold a report

```bash
rex report init --dir my-engagement
```

Same scaffold as [Install & Build Your First Report](./cli.md) — `index.md`
plus `sections/`, `reports/`, `appendices/`, `assets/`.

## 2. Start the sync server

Pick a shared password for the team and run:

```bash
rex-svr --port 9443 --password-file /path/to/password.txt \
  --db ./rex-svr.db --workspace ./rex-svr-git-workspace
```

`--workspace` is a directory `rex-svr` owns exclusively — it materializes
every accepted change there as real Git history. Keep it separate from
anyone's actual working copy, and keep `--db`/`--cert`/`--key` outside it:
`rex-svr` refuses to start with `--workspace .` or with a workspace that
already holds files it didn't put there (see [Running the
server](../reference/sync.md#running-the-server)). `--password-file` is preferred over
`--password` (mutually exclusive) since it avoids leaking the shared
password into shell history or the process list.

All client-server traffic is HTTPS/WSS — `rex-svr` serves no plaintext
protocol. On first run it generates a self-signed certificate (`./rex-svr.crt`/
`./rex-svr.key` by default, or wherever `--cert`/`--key` point) and prints its
path and fingerprint. Share `rex-svr.crt` with your team (Slack, a shared
drive, whatever's convenient) so their `rex` clients can trust it with
`--ca-cert`. See [TLS trust](../reference/sync.md#tls-trust) for the
`--ca-cert`/`--insecure` alternatives.

## 3. Push the initial report

From the machine that has `my-engagement/`:

```bash
rex sync --host rex-svr.example.com --username alice --dir my-engagement \
  --ca-cert /path/to/rex-svr.crt
```

(`--ca-cert` is omitted from the rest of this page for brevity — every
command that talks to `rex-svr` needs it, or `--insecure` on a trusted
network. See [TLS trust](../reference/sync.md#tls-trust).)

This reconciles the local directory against the (currently empty) server —
since only the local side has content, everything gets pushed. Leave it
running: it keeps watching the filesystem and reconciling as you edit.

## 4. Bring in a teammate

On a teammate's machine, pointed at an **empty** directory:

```bash
mkdir my-engagement && cd my-engagement
rex sync --host rex-svr.example.com --username bob --dir .
```

No manual copying needed — `rex sync` pulls everything the server already
has. From here on, both of you editing different files sync automatically;
editing the *same* file while offline from each other becomes a flagged
conflict rather than a silent overwrite:

```bash
rex conflicts --host rex-svr.example.com --username bob --dir .
rex resolve <path> --host rex-svr.example.com --username bob --dir . --local
```

See [Conflict resolution](../reference/sync.md#conflict-resolution) for the
full model, including the `--remote`/`--local`/`--merge`/`--keep-both`
options `rex resolve` accepts.

## 5. Shared team notes

For open-ended notes the whole team can see, just make a `notes/` directory
in the workspace:

```bash
mkdir -p notes
echo "# Scope notes" > notes/scope.md
```

It syncs and gets committed to Git exactly like everything else — no
scaffolding or setup required. See [Team notes](../reference/sync.md#team-notes)
for details.

## What's next

For advisory editor presence ("bob is editing this file") and background
sync started automatically when you open your editor, see the IDE guides:

- [Neovim](./ide/neovim.md)
- [VS Code](./ide/vscode.md)
