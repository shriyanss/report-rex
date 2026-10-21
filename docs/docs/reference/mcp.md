---
sidebar_position: 8
---

# MCP Server

`rex mcp` runs a [Model Context Protocol](https://modelcontextprotocol.io)
server so an AI harness (Claude Code, Codex, or anything else that speaks
MCP) can add and update vulnerability findings without touching report files
on disk directly. Every finding it creates is stamped `ai: true` in its
frontmatter, and it refuses to modify a finding that isn't already
AI-owned unless you explicitly authorize it.

## Connecting a harness

```bash
rex mcp --dir ./my-engagement
```

Point your harness's MCP client config at this command over stdio — for
example, Claude Code's `.mcp.json`:

```json
{
  "mcpServers": {
    "report-rex": {
      "command": "rex",
      "args": ["mcp", "--dir", "/absolute/path/to/my-engagement"]
    }
  }
}
```

## Tools

| Tool | Purpose |
|---|---|
| `list_findings` | List every finding: path, title, severity, status, and whether it's `ai: true` |
| `get_finding` | Read one finding's full frontmatter and body |
| `add_finding` | Create a new `reports/vuln-N.md` and register it in `index.md`. Always stamped `ai: true` |
| `update_finding` | Update an existing finding's fields/body |

`update_finding` refuses a finding whose frontmatter doesn't already carry
`ai: true`, unless the call sets `user_authorized: true`. A harness should
only ever set that when the human user explicitly asked it to change a
finding it didn't create — this is what stops an AI-driven caller from
silently rewriting a human-authored finding. AI-owned findings accept
further AI edits without it.

If `add_finding`/`update_finding` is given a `body`, it must include a
one-liner `curl` PoC command (e.g. under `## Replication`) — the same
reason bug bounty triagers require a one-liner repro: the human reviewing
AI-submitted findings needs to be able to reproduce them immediately,
without re-deriving repro steps from prose. A body with no `curl` line is
rejected.

## Syncing through rex-svr

If the workspace is also synced with [`rex-svr`](./sync), pass the same
connection flags `rex sync` takes so MCP-driven writes push through under
their own identity:

```bash
rex mcp --dir ./my-engagement \
  --host sync.example.com --username mcp-claude --ca-cert /path/to/rex-svr.crt
```

Give each MCP-connected harness its own `--username` (default `mcp`) —
`rex-svr` accepts any username against the one shared password and creates
it on first login, so every AI-driven change is attributed distinctly from
human edits via `modified_by` in the manifest.

`--password` falls back to `$REX_PASSWORD`; unlike `rex sync`, `rex mcp`
never prompts interactively, since stdin is the MCP protocol channel, not a
terminal.

Before pushing an update to a file, `rex mcp` checks the [editor presence
snapshot](./sync#editor-presence): if another session currently has that
finding open, it waits 5 seconds before pushing rather than racing that
edit. A brand-new finding from `add_finding` has no existing session to
race, so this only applies to `update_finding`.
