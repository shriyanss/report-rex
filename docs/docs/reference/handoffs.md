---
sidebar_position: 6
---

# Handoffs

During an engagement one operator often needs to hand a thread of
investigation to someone better suited to it. A **handoff** is how you say
that in Rex: a plain Markdown file in the workspace, whose canonical copy
lives on `rex-svr` and whose every change is committed to the server's Git
history.

This page is the reference. For a walkthrough, start with
[Handing Work to a Teammate](../getting-started/handoffs.md).

```text
handoffs/<assignee>/<title>.md
```

For example, Alice starts looking at an application, realises it's a Next.js
app, and wants `shriyanss` to take the JavaScript-specific work:

```bash
mkdir -p handoffs/shriyanss
cat > handoffs/shriyanss/investigate-nextjs-auth.md <<'EOF'
---
to: shriyanss
---

# Investigate Next.js authentication

The application appears to use Next.js. Please check whether the
authentication flow exposes any framework-specific attack surface.
EOF

rex handoffs sync --host rex-svr --username alice
```

That's the whole creation flow. `to:` is the only metadata you have to
write — `rex handoffs sync` fills in the rest.

## Metadata

Rex stamps the identity fields in place on the first sync, so the file above
becomes:

```yaml
---
created_at: "2026-08-22T17:15:00-04:00"
from: alice
rex_handoff_id: 8f83c1
status: open
to: shriyanss
---
```

| Key | Meaning |
|---|---|
| `rex_handoff_id` | Stable 6-hex-character id. Every command that takes a handoff takes this |
| `from` | Creator. Derived from the authenticated Rex client, not from what the file claims |
| `to` | Assignee. Must match the directory the file lives in |
| `status` | `open` or `resolved` |
| `created_at` | When the handoff was stamped |
| `resolved_by` / `resolved_at` | Written when the handoff is resolved |

`rex-svr` re-checks all of this on every upload, so a hand-edited or
out-of-date file can't reassign a handoff behind your back. Concretely, the
server refuses to store a handoff whose `from` isn't the authenticated user
on create, whose `from` or `rex_handoff_id` changes after create, or whose
`status` is edited to `resolved` by hand (resolution has to go through the
resolve operation, so the file gets retired at the same time).

## The directory is part of the identity

`handoffs/<username>/` and the `to` metadata must agree. Rex never picks a
winner between them — an ambiguous assignee could deliver work to the wrong
person — so a mismatch is an error and the handoff is not synchronized until
it's corrected:

```text
$ rex handoffs sync --host rex-svr --username alice

error: invalid handoff metadata

handoffs/alice/nextjs.md

Directory assignee: alice
Metadata assignee:  shriyanss

The handoff directory and `to` metadata must match.
```

The same check runs on create, sync, list, view, and on the server. Other
handoffs in the same pass still synchronize normally; only the inconsistent
one is held back.

## Scopes

Every subcommand operates on a scope:

| Flag | Scope |
|---|---|
| *(none)* | handoffs that involve you — given to you **or** given by you |
| `--user <name>` | one operator's queue, by assignee |
| `--all` | every handoff |

The default deliberately isn't "the `handoffs/<you>/` directory". A handoff
concerns two people, and you need the ones you *sent* as much as the ones you
were given — otherwise you couldn't follow up on your own request. Handoffs
between two other people are never in your default scope.

Scope decides which handoffs get **materialized** into your workspace. It
isn't an access-control boundary: `--user` and `--all` are available to
everyone, and `rex handoffs list --all` shows the whole team's queues.

## Commands

### `rex handoffs list`

```text
$ rex handoffs list --host rex-svr --username shriyanss

ID       FROM    TO          STATUS   TITLE
8f83c1   alice   shriyanss   open     investigate-nextjs-auth
f921ae   bob     shriyanss   open     inspect-webpack-bundle
```

Resolved handoffs are hidden unless you pass `--resolved`. Any handoff with
inconsistent metadata is reported on stderr and makes the command exit
non-zero, so a misassigned file can't sit unnoticed.

### `rex handoffs view <id>`

Reads a handoff straight from the server, without changing what's
synchronized locally — useful for checking on a handoff in someone else's
queue. Works for resolved handoffs too.

```text
$ rex handoffs view 8f83c1 --host rex-svr --username alice

ID:      8f83c1
From:    alice
To:      shriyanss
Status:  open
Created: 2026-08-22T17:15:00-04:00
Path:    handoffs/shriyanss/investigate-nextjs-auth.md

# Investigate Next.js authentication
...
```

### `rex handoffs sync`

One reconciliation pass, then exit:

```text
$ rex handoffs sync --host rex-svr --username shriyanss

Synchronizing your handoffs...

+ handoffs/shriyanss/investigate-nextjs-auth.md

1 handoff(s) synchronized
```

A synchronized handoff is just a Markdown file — edit it in your editor and
the next pass sends the change back. Add `--watch` to stay running and
reconcile continuously; it composes with the scope flags:

```bash
rex handoffs sync --watch                    # your queue
rex handoffs sync --user alice --watch       # alice's queue
rex handoffs sync --all --watch              # everyone's
```

In watch mode the server only pushes events for the scope you subscribed to,
so watching one queue doesn't wake you for the rest of the workspace.

### `rex handoffs remove <id>`

Resolves a handoff. The canonical copy stays on the server:

```text
$ rex handoffs remove 8f83c1 --host rex-svr --username shriyanss

Resolved handoffs/shriyanss/investigate-nextjs-auth.md [8f83c1]
The server keeps the canonical copy; `rex handoffs list --resolved` still shows it.
```

Deleting the file does the same thing:

```bash
rm handoffs/shriyanss/investigate-nextjs-auth.md
rex handoffs sync --host rex-svr --username shriyanss
```

Either way the handoff's active path is retired, so it disappears from every
subscriber's workspace, while `rex handoffs list --resolved` and
`rex handoffs view <id>` keep working — the resolved copy is retained in the
server's revision history and in its Git history.

Three details worth knowing:

- **Deleting only resolves within your scope.** Dropping your own copy of a
  handoff you *sent* someone else doesn't finish their work for them; Rex just
  stops tracking your copy.
- **The server refuses a raw delete.** `DELETE /files/handoffs/...` on an open
  handoff is rejected — retiring one has to go through the resolve operation,
  which is what records who finished it and when. Clients never delete a
  canonical handoff file.
- **Editor saves aren't deletions.** Many editors save by writing a temp file,
  unlinking the original, and renaming — Rex lets watcher events settle and
  re-checks that the file is genuinely gone before resolving anything.

A resolved handoff is never resurrected. The canonical copy still exists on
the server, but reconciliation treats "exists" and "should be materialized"
as different questions.

## Git history

Git stays server-authoritative: clients send handoff operations, `rex-svr`
commits them. Rapid edits are coalesced the same way ordinary sync writes
are, so an editor's autosaves don't become one commit each.

```text
$ git log --format='%s%n%b'

handoff: 1 file(s) by shriyanss
- handoff resolve handoffs/shriyanss/investigate-nextjs-auth.md [8f83c1] (shriyanss)

handoff: 1 file(s) by shriyanss
- handoff update handoffs/shriyanss/investigate-nextjs-auth.md [8f83c1] (shriyanss)

handoff: 1 file(s) by alice
- handoff create handoffs/shriyanss/investigate-nextjs-auth.md [8f83c1] (alice)
```

## Lifecycle

The initial lifecycle is deliberately small:

```text
              assignee removes
┌──────────┐  ────────────────►  ┌──────────┐
│   OPEN   │                     │ RESOLVED │
└──────────┘                     └──────────┘
```

`CANCELLED`/`REOPENED` can be added later without a migration, since state
lives in the file itself.

## `rex sync` and handoffs

`rex sync` synchronizes the whole workspace, `handoffs/` included — but the
handoff namespace is the one part of the tree it does **not** sync in full.
Everything else in a report workspace belongs to the whole team; a handoff is
addressed to two specific people. So `rex sync` applies the default scope:

- handoffs given to you — `handoffs/<you>/`
- handoffs you gave someone else — wherever they live
- **not** handoffs between two other people

Creating, editing and resolving all work through plain `rex sync`, including
stamping a hand-written `to:`-only file and turning a local `rm` into a
resolve. `rex handoffs sync` exists for when you want a *different* scope
(`--user`, `--all`) or handoffs on their own without the rest of the
workspace.

One edge case: if a handoff was materialized under an older Rex and is now
out of scope, `rex sync` stops syncing it but does not delete your copy —
removing a file you already have isn't Rex's call. Delete it yourself if you
don't want it.

## HTTP API

Three routes, alongside the ones in
[Real-Time Sync → HTTP & WebSocket API](./sync#http--websocket-api):

| Route | Notes |
|---|---|
| `GET /handoffs?user=&all=1&resolved=1` | `{handoffs: [{id, path, title, from, to, status, revision, created_at, resolved_by, resolved_at}], invalid: [{path, dir_assignee, meta_assignee, error}]}`. Defaults to the authenticated user's queue. `invalid` is always returned in full — a handoff whose path and metadata disagree can't be attributed to a queue, which is exactly what's wrong with it |
| `GET /handoffs/{id}` | Same fields plus `content`, so a handoff can be read without being synchronized |
| `POST /handoffs/{id}/resolve` | `{id, path, revision}`. Writes the final metadata as one more canonical revision, then retires the active path. `409` if it's already resolved |

`POST /files/handoffs/<assignee>/<title>.md` runs the same validation as the
CLI: `422` if the directory and `to` metadata disagree, if `from` isn't the
authenticated user on create, if `from`/`rex_handoff_id` change afterwards, or
if `status` is hand-edited to `resolved`. `DELETE /files/handoffs/...` on an
open handoff is `422` too — resolve it instead.

`GET /ws` also accepts `?prefix=handoffs/<user>/` to narrow a connection's
subscription to one queue. `rex handoffs sync --user <name>` uses it; the
default scope spans two directions, so it subscribes to `handoffs/` and
filters locally.
