---
sidebar_position: 3
---

# Handing Work to a Teammate

Halfway through an engagement you find something that isn't yours to finish.
You've been poking at a web app, you realise it's a Next.js build, and the
person who actually knows Next.js internals is on the other side of the room.

A **handoff** is how you pass that thread over: a Markdown file in the shared
workspace, addressed to one teammate, that they pick up, work on, and close
out. `rex-svr` keeps the canonical copy and commits every change to Git, so
there's a record of who asked for what and what came of it.

This page walks through using them. For the metadata schema, the full flag
reference, and the HTTP API, see the [Handoffs Reference](../reference/handoffs.md).

You need a running `rex-svr` and a synced workspace first — see
[Real-Time Collaboration](./server.md).

## 1. Write the handoff

A handoff is a file at `handoffs/<teammate>/<short-title>.md`. The directory
is who it's for; the filename is what it's about.

```bash
mkdir -p handoffs/shriyanss

cat > handoffs/shriyanss/investigate-nextjs-auth.md <<'EOF'
---
to: shriyanss
---

# Investigate Next.js authentication

The app at https://target.example.com is a Next.js build — I found
`/_next/static/` chunks and an `x-powered-by: Next.js` header.

Could you look at whether the auth flow leaks anything framework-specific?
I've got as far as noticing the session cookie isn't `HttpOnly`.
EOF
```

That's all you have to write. One `to:` line and whatever context the next
person needs.

Write it like a message to a colleague, because that's what it is — what you
tried, what you found, what you want them to look at. The whole point is that
they don't have to redo your first hour.

## 2. Send it

If you're already running `rex sync`, it goes out on the next pass — nothing
else to do. Otherwise:

```bash
rex handoffs sync --host rex-svr --username alice
```

```text
Synchronizing your handoffs...

+ handoffs/shriyanss/investigate-nextjs-auth.md

1 handoff(s) synchronized
```

Rex fills in the bookkeeping for you, so the file now starts:

```yaml
---
created_at: "2026-08-22T17:15:00-04:00"
from: alice
rex_handoff_id: 8f83c1
status: open
to: shriyanss
---
```

`from` comes from who you're logged in as, not from anything the file claims,
and `rex_handoff_id` is the short id every command uses to refer to this
handoff.

## 3. Pick one up

On the other side, `shriyanss` sees it in his queue:

```bash
rex handoffs list --host rex-svr --username shriyanss
```

```text
ID       FROM    TO          STATUS   TITLE
8f83c1   alice   shriyanss   open     investigate-nextjs-auth
```

He can read it without pulling it into his workspace:

```bash
rex handoffs view 8f83c1 --host rex-svr --username shriyanss
```

Or sync it down and open it like any other file:

```bash
rex handoffs sync --host rex-svr --username shriyanss
nvim handoffs/shriyanss/investigate-nextjs-auth.md
```

Again — if he's running `rex sync`, it's already there.

## 4. Work in it

A synced handoff is a normal Markdown file. Append findings as you go; each
save syncs back, and Alice sees them on her next pass:

```md
## Findings

Confirmed Next.js 14 with the App Router. `/api/admin/users` is reachable
without a session cookie — returns 200 with a user list. Writing this up as
a separate finding.
```

Rapid saves are batched into one commit, so an autosaving editor doesn't turn
the Git history into confetti.

## 5. Close it out

When the work is done, either delete the file:

```bash
rm handoffs/shriyanss/investigate-nextjs-auth.md
```

or resolve it by id:

```bash
rex handoffs remove 8f83c1 --host rex-svr --username shriyanss
```

Both do the same thing. The handoff leaves everyone's workspace, and the
server keeps the finished copy:

```bash
rex handoffs list --resolved --host rex-svr --username shriyanss
rex handoffs view 8f83c1   --host rex-svr --username shriyanss
```

It won't reappear on the next sync, and it stays in the server's Git history
alongside every edit it accumulated.

## What shows up in your workspace

`rex sync` doesn't hand you the whole team's to-do list. In the `handoffs/`
directory you get:

- handoffs **given to you**
- handoffs **you gave** someone else
- nothing else — two teammates trading work between themselves stays between
  them

So a four-person engagement where everyone runs `rex sync` looks like this:

```text
alice/                       shriyanss/
└── handoffs/                └── handoffs/
    └── shriyanss/               ├── shriyanss/
        └── nextjs.md            │   └── nextjs.md      ← alice → shriyanss
                                 └── carol/
                                     └── graphql.md     ← shriyanss → carol

carol/                       bob/
└── handoffs/                └── handoffs/
    └── carol/                   └── (nothing — bob isn't in either handoff)
        └── graphql.md
```

To look wider — to help with someone's queue, or to see everything in flight:

```bash
rex handoffs list --all                    # the whole team's handoffs
rex handoffs sync --user carol             # materialize carol's queue too
rex handoffs sync --all --watch            # everything, continuously
```

Deleting a file only resolves a handoff in the queue you've subscribed to. If
you sent one to a teammate and delete your copy, you're just tidying your own
workspace — you haven't closed their task.

## Staying in sync continuously

`rex handoffs sync --watch` stays running and reconciles as changes happen —
useful if you want handoff traffic without syncing a whole report workspace:

```bash
rex handoffs sync --watch --host rex-svr --username shriyanss
```

It composes with the scope flags (`--user alice --watch`, `--all --watch`),
and the server only pushes it events for the scope you asked for.

## When the directory and the metadata disagree

The directory a handoff lives in and its `to:` field are the same fact stated
twice, so they have to match. If they don't, Rex says so and holds that
handoff back rather than guessing which one you meant:

```text
$ rex handoffs sync --host rex-svr --username alice

error: invalid handoff metadata

handoffs/alice/nextjs.md

Directory assignee: alice
Metadata assignee:  shriyanss

The handoff directory and `to` metadata must match.
```

This is what stops a copy-pasted or half-edited file from landing in the wrong
person's queue. Fix either side — move the file or change `to:` — and it syncs
on the next pass. Your other handoffs are unaffected.

## Tips

- **One handoff, one thread of work.** Two unrelated questions are two files.
  It keeps the queue readable and lets each be closed when it's done.
- **Titles are filenames.** `investigate-nextjs-auth` beats `todo2`; it's what
  shows up in `list` and in the Git log.
- **Don't put findings only in handoffs.** A handoff is working
  correspondence; it disappears from workspaces once resolved. Anything that
  belongs in the deliverable goes in `reports/`.
- **`view` before you `sync`.** Reading a handoff out of someone else's queue
  doesn't change what's on your disk.

## Next steps

- [Handoffs Reference](../reference/handoffs.md) — metadata schema, all flags,
  Git behavior, HTTP API
- [Real-Time Collaboration](./server.md) — the sync server the whole thing
  runs on
