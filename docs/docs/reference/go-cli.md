---
sidebar_position: 4
---

# Go CLI Reference

Report Rex is also distributed as a **standalone Go binary called `rex`** — no
repository checkout, no manual `docker build`, no `./build.sh`. It embeds the
Dockerfile, the bundled templates, and example scaffolds at build time. Parsing
`index.md`, assembling the report, and running the QA-gate checks are all native
Go — no Docker or Python involved for any of that. Only the final Markdown→PDF
render step needs `pandoc`/XeLaTeX, either inside the Docker image (built
automatically the first time it's needed) or via a local install with
`--no-docker`.

:::info Go CLI vs Python CLI
These are two different tools with two different names:

- **Go CLI** (this page) — the `rex` binary, a self-contained executable you
  download once. Its commands are namespaced (`rex report build`, `rex report
  init`, `rex report validate`, `rex image build`, `rex watch`). Report parsing,
  assembly, and validation are native Go; it only reaches for Docker (or a local
  `pandoc`) for the final render.
- **Python CLI** (see [CLI Reference](./cli)) — the `report-rex` command,
  installed via `pip install report-rex-core`, runs *inside* the Docker container
  (or natively if you have pandoc/XeLaTeX installed). Its commands are flat
  (`report-rex build`, `report-rex new`, `report-rex validate`).

If you just want to build a report without cloning the repo, use the Go CLI below.
:::

## Installing

Download the binary for your platform from the [Releases page](https://github.com/shriyanss/report-rex/releases),
then put it on your `PATH`:

```bash
tar -xzf report-rex_<version>_<os>_<arch>.tar.gz
sudo mv rex /usr/local/bin/
rex version
```

Prebuilt binaries are published for macOS (arm64), Linux (amd64, arm64), and Windows
(amd64, arm64).

The same archive also contains `rex-svr`, the sync server binary used by the
[real-time sync feature](./sync) — no separate download needed.

## Commands

### `rex report init`

Scaffold a new report directory from a full example, so you have real sample content
to work from rather than empty starter files. Refuses to run against a non-empty
target directory.

`--title`/`--client`/`--date` are required unless `--force` is passed — this
keeps a real client name and date from being silently left as placeholder
scaffold text. Passing `--force` instead skips them and prints a warning; any
of the three can still be supplied alongside `--force` and will be used.

```
rex report init [OPTIONS]

Options:
  --dir string      Target directory
  --type string     Report type: full-report or quick-comm
  --kind string     quick-comm kind: critical-finding or progress-update
  --title string    Engagement title
  --client string   Client name
  --date string     Engagement date, YYYY-MM-DD
  --force           Scaffold with placeholder title/client/date instead of requiring them
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dir string` | `.` | Target directory. Must be empty (or not yet exist). |
| `--type string` | `full-report` | Report type: `full-report` or `quick-comm`. |
| `--kind string` | `progress-update` | For `--type quick-comm` only: `critical-finding` or `progress-update`. Ignored for `full-report`. |
| `--title string` | — | Engagement title. Required unless `--force`. |
| `--client string` | — | Client name. Required unless `--force`. |
| `--date string` | — | Engagement date (`YYYY-MM-DD`). Required unless `--force`. |
| `--force` | off | Scaffolds with placeholder title/client/date instead of requiring them; prints a warning. |

```bash
rex report init --dir ./acme-engagement --title "Acme Web App Pentest" --client "Acme Corp" --date 2026-09-01

rex report init --dir ./alert --type quick-comm --kind critical-finding --title "Critical SQLi Alert" --client "Acme Corp" --date 2026-09-01

rex report init --dir ./draft --force   # placeholder title/client/date, with a warning
```

### `rex report section`

Add a new finding, appendix, or narrative section to an *existing* report
directory and register it under the right `structure:` key in `index.md` —
without copying an existing file by hand, renumbering it, and hand-editing
`index.md`. Every subcommand takes `--dir` (default `.`) and requires an
existing `index.md` there.

#### `rex report section vuln`

Creates `reports/vuln-N.md` (auto-numbered past the highest existing
`vuln-N.md`) and registers it under `structure.reports`.

```
rex report section vuln [OPTIONS]

Options:
  --dir string              Report directory
  --title string            Finding title (required unless --template)
  --template string         Start from a vuln-templates/<name>.md stub
  --list-templates          List the --template names (name<TAB>title) and exit
  --file string             Write reports/<file>.md instead of the next vuln-N.md
  --cvss-vector string      CVSS v3.1 vector string
  --component string        Affected component
  --cwe string              CWE identifier, e.g. CWE-89
  --owasp string            OWASP Top 10 category, e.g. A03:2021
  --mitre string            MITRE ATT&CK technique, e.g. T1190
  --mitigations string      MITRE mitigation IDs, e.g. "M1032, M1038"
  --impact string           Impact rating
  --sophistication string   Attacker sophistication required
  --material-impact string  Material impact category
  --business-risk strings   Business risk category (repeatable)
  --affected-url strings    Affected URL (repeatable)
  --timeline-date string    Timeline entry date, RFC3339 (needs --timeline-desc)
  --timeline-desc string    Timeline entry description (needs --timeline-date)
```

With `--template <name>` (one of the classes in `vuln-templates/`, e.g.
`ssrf`, `xxe`, `csrf`), the finding starts from that template's
`[!REX-STUB]`-marked stub; any flag you pass overwrites the corresponding
field and removes its stub marker, while fields you don't pass keep their
markers so `rex report validate` still flags them. Without `--template`, a
minimal finding is generated from only the flags supplied, with blank
`## Description`/`## Impact`/`## Replication`/`## Remediation` headings to
fill in.

`--file ssrf-webhook` names the new file `reports/ssrf-webhook.md` instead of
the next `reports/vuln-N.md`. Use a plain name with no path separators, and
pick one that doesn't exist yet. `--list-templates` prints the available
template names with their stub titles.

```bash
rex report section vuln --template ssrf --title "SSRF in Webhook Handler" \
  --affected-url "https://acme.example/api/webhook"

rex report section vuln --title "Missing Rate Limiting on Login"
```

#### `rex report section appendix`

Creates `appendices/N.md` (auto-numbered) and registers it under
`structure.appendices`.

```
rex report section appendix --title "Appendix — Extra Notes"
```

#### `rex report section narrative`

Creates `sections/<name>.md` (a filename slug, since narrative sections like
`exec_summ.md`/`methodology.md` aren't numbered) and registers it under
`structure.sections`.

```
rex report section narrative --name risk-summary --title "Risk Summary"
```

### `rex report build`

Build a PDF from the report in the current directory. Parses and assembles the
report natively — no Docker or Python involved for that part — then runs the
same QA-gate checks as `rex report validate` (below) and aborts the build if
any check fails, unless `--force` is passed. Once validation passes, it renders
the assembled report to a PDF via `pandoc`: inside the Docker image (auto-built
from embedded source the first time it's needed) by default, or via a local
`pandoc`/XeLaTeX install with `--no-docker`.

```
rex report build [OPTIONS]

Options:
  --dir string       Report directory
  --template string  Template override
  --output string    Output PDF path
  --final            Produce the final report, no DRAFT watermark (default: draft, renamed to *-draft.pdf)
  --docx             Also generate a .docx clone alongside the PDF
  --force            Skip QA-gate validation
  --no-docker        Render via a local pandoc/XeLaTeX install instead of Docker
  --image string     Docker image tag to build/run
  --no-terminal-bs   Suppress the build progress indicator
  --password string  Encrypt the output PDF with this password (requires qpdf)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dir string` | `.` | Report directory. |
| `--template string` | from `index.md` | Template override. |
| `--output string` | `report.pdf` | Output PDF path. |
| `--final` | off | Produces the final report with no DRAFT watermark. Default is draft, which renames the output to `*-draft.pdf`. |
| `--docx` | off | Also generates a `.docx` clone next to the PDF (same basename, `.docx` extension) — see [Output Formats](./output-formats.md). |
| `--force` | off | Skips the pre-build QA-gate validation, so the build proceeds even if it would otherwise fail. |
| `--no-docker` | off | Renders via a locally installed `pandoc`/XeLaTeX instead of Docker. No local Python install needed either way — report assembly is always native Go. `--docx` additionally needs `pdftocairo` on `PATH` in this mode (chart conversion). |
| `--image string` | `report-rex:latest` | Docker image tag to build/run. |
| `--no-terminal-bs` | off | Suppresses the animated progress indicator shown while pandoc/XeLaTeX render (silently, so a build can otherwise look stuck). Automatically skipped anyway when stderr isn't a terminal (e.g. piped or redirected). |
| `--password string` | — | Encrypts the output PDF (AES-256, via `qpdf`) with this password. Omit for an unencrypted PDF (default, unchanged). Runs inside the Docker image in Docker mode; `--no-docker` needs a local `qpdf` on `PATH`. The `.docx` clone, if any, is never encrypted. |

```bash
rex report build --dir ./acme-engagement --output acme-report.pdf

rex report build --dir ./acme-engagement --template branded --final --force

rex report build --dir ./acme-engagement --docx

rex report build --dir ./acme-engagement --no-docker

rex report build --dir ./acme-engagement --password 'hunter2'
```

### `rex report validate`

Validate a report directory without rendering: `index.md` parses, every file
referenced under `structure:` exists, and the QA-gate content checks pass —
CVSS presence, a timeline entry, a `## Replication` section (or the older `## Steps to
Reproduce`) of at least 100 characters, a `title` and parseable frontmatter starting on the very
first line, no leftover scaffold placeholder text, per-screenshot
caption length/file existence/light-mode/annotation/text-legibility checks, and
the chart-data files under `artifacts/` (valid `assets.json`/`connections.json`,
and the data each chart directive needs).
Entirely native Go — no Docker, no Python. This is the exact same check `rex
report build` runs before rendering, so use it standalone to iterate on
content without paying for a full render each time.

```
rex report validate [OPTIONS]

Options:
  --dir string   Report directory to validate
  --json         Output issues as JSON
  --stdin        Validate content from stdin in place of files on disk
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dir string` | `.` | Report directory to validate. |
| `--json` | off | Print `{issues, errors, warnings}` as JSON. Editor extensions use this. |
| `--stdin` | off | Read a JSON object of `{"<absolute path>": "<content>"}` from stdin and validate that content in place of those files on disk. The VS Code extension uses it to lint unsaved buffers as you type. |

```bash
rex report validate --dir ./acme-engagement
```

Output is grouped by file, in the same format as the Python CLI's `report-rex
validate` — see [CLI Reference](./cli#report-rex-validate) for the full list of
checks and the `> [!OVERRIDE-DARK-IMG]` marker. Errors exit non-zero; warnings
print but don't fail the command. Issues that point at a specific line (a chart
directive, a `connections.json` entry) are printed as `line N:`, and
`--json` output carries it as `"line": N`, which the VS Code extension uses to
underline that line.

### `rex image build`

Explicitly (re)build the Docker image from the binary's embedded source, without
running a report build.

```
rex image build [OPTIONS]

Options:
  --tag string  Tag to apply to the built image
```

| Flag | Default | Description |
|------|---------|-------------|
| `--tag string` | `report-rex:latest` | Tag to apply to the built image. |

```bash
rex image build

rex image build --tag report-rex:v2
```

### `rex watch`

Watch a report directory and rebuild automatically on every `.md`/`.yaml`/`.yml`
change, similar to `tsc --watch`. Accepts the same flags as `rex report build`
(`--dir`, `--template`, `--output`, `--final`, `--docx`, `--force`, `--no-docker`, `--image`, `--no-terminal-bs`, `--password`).

```
rex watch [same flags as report build]
```

```bash
rex watch --dir ./acme-engagement --output acme-report.pdf
```

Press Ctrl-C to stop.

### `rex ingest`

Run a scan tool and fold its results into the report's appendices — no Docker
involved, since it only touches the local filesystem and the tool's own binary.

```
rex ingest <tool> [tool-args...] [OPTIONS]

Options:
  --dir string   Report directory
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dir string` | `.` | Report directory. Must already contain `index.md`. |

`<tool>` is the first positional argument; everything after it is passed straight
through to that tool. Because tool flags like nmap's `-sV` look like `rex`'s own
flags, put `--dir` *before* the tool name — `rex` stops parsing its own flags once
it reaches the tool name, so anything after that point (including further `-`
flags) is forwarded untouched:

```bash
rex ingest --dir ./acme-engagement nmap -sV 10.0.1.0/24

rex ingest --dir ./acme-engagement naabu -host 10.0.1.5 -top-ports 1000

rex ingest --dir ./acme-engagement nikto -h https://portal.acme.com

rex ingest --dir ./acme-engagement bloodhound ./collected-20240115.zip
```

Currently supported tools:

| Tool | What it adds |
|------|--------------|
| `nmap` | Host/port/service/version detail, parsed from `nmap`'s `-oX` XML output (appended automatically — don't pass `-oX` yourself). |
| `naabu` | Fast host/port discovery, parsed from `naabu`'s `-json` output (appended automatically). No service/version detail — naabu is a port scanner, not a service scanner. |
| `nikto` | Web server findings (missing headers, outdated software, exposed files, etc.), parsed from `nikto`'s `-Format xml -output <file>` output (appended automatically — don't pass `-Format`/`-output` yourself). |
| `bloodhound` | Active Directory enumeration data (domains, users, computers, groups, and their privileged/roastable/delegation attributes), imported from an existing SharpHound/BloodHound-CE collector export. |

`bloodhound` behaves differently from the other three tools: its first
positional argument is a **path** to a `.zip` collector export (or a directory
you've already extracted one into) rather than tool flags to forward. `rex`
never runs a live BloodHound collector itself, because collecting AD data
needs real domain credentials and network access to the target Active
Directory environment — a separate, out-of-band step you run yourself with
SharpHound (Windows) or `bloodhound-python`, before handing the resulting
export to `rex ingest bloodhound <path>`.

Every ingest run (including `rex ingest existing`) also adds an entry to the
report's **Timeline** table, e.g. `Port scan (nmap) against 10.0.1.5, 10.0.1.9
(+3 more)`. The entry names the hosts (or AD domains) the run actually found, up
to five of them. Entries are recorded in `artifacts/timeline.json` and written
into the `timeline:` frontmatter of the generated appendix (`assets.md`,
`web-findings.md` or `ad-enum.md`). That appendix is regenerated on every run,
so edit or hide an entry in `artifacts/timeline.json` (set `"include": false`)
rather than in the appendix itself.

#### `rex ingest existing`

```bash
rex ingest --dir ./acme-engagement existing naabu -o naabu.txt

rex ingest --dir ./acme-engagement existing nmap -oX nmap.xml
```

If you already ran a tool yourself and have its output on disk — instead of
letting `rex` run it — use `rex ingest existing <tool> [tool-args...]`. It
reads the named file straight off disk and parses it; the tool itself is
never invoked, so no binary/PATH/network access is needed. Supported tools:

| Tool | Expected file | Found via |
|------|----------------|-----------|
| `naabu` | naabu's own **default** (non-`-json`) text output — one `host:port` line per open port, exactly as naabu writes it via its own `-o <file>` flag. | `-o` in the args you pass |
| `nmap` | an `nmap -oX` XML report — the same schema `rex ingest nmap` already parses from live `-oX -` output, so any `-oX` file nmap wrote (with or without `rex`) works unchanged. | `-oX` in the args you pass |

Each `nmap`/`naabu` run (`existing` or not):

1. Merges the parsed hosts into `artifacts/assets.json`, keyed by IP. Running
   `ingest` more than once — with different targets, or with a different tool
   against the same target — accumulates into this one file rather than
   overwriting it: an existing host's ports are unioned with the new scan's, and
   `nmap`'s service/version detail fills in alongside a host `naabu` already
   discovered.
2. Archives the raw tool output under `artifacts/<tool>/<unix-timestamp>.<ext>`
   as an evidence trail.
3. Regenerates `appendices/assets.md` from scratch from the merged inventory — a
   host/port table plus two charts (see [Charts](../reporting-structure/charts.md#asset-charts)).
   This file is fully generated; hand edits to it are overwritten on the next
   `ingest` run.
4. Adds `appendices/assets.md` to `index.md`'s `structure.appendices` list, if
   it isn't already there. This is a targeted text edit, not a full YAML
   rewrite, so it doesn't disturb any comments elsewhere in `index.md`. If
   `structure.appendices` can't be found at all (no appendices key yet), `rex`
   leaves `index.md` untouched and prints the path to add manually instead of
   guessing at the YAML structure to create.

`artifacts/assets.json` also feeds the `:::network_diagram:::` directive. Put it
on its own line in any section or appendix to draw the inventory as a topology
map. Subnet labels, pivots, and must-show hosts go in a hand-written
`artifacts/connections.json`, which `ingest` never touches. See
[Charts → Network Diagram](../reporting-structure/charts.md#network-diagram).

`nikto` and `bloodhound` follow the same shape, into their own artifact/appendix
files instead of `assets.json`/`assets.md`:

- **`nikto`** merges parsed findings into `artifacts/webfindings.json`, keyed
  on host/port/finding-ID/URI/description (description is part of the key
  because nikto reuses one finding ID across several distinct checks against
  the same URI — e.g. separate missing-security-header findings), archives the
  raw XML under `artifacts/nikto/<unix-timestamp>.xml`, regenerates
  `appendices/web-findings.md`, and registers it in `structure.appendices`.
- **`bloodhound`** merges parsed AD principals into `artifacts/bloodhound.json`,
  keyed on domain/type/name (a re-import of the same principal overwrites its
  attributes rather than duplicating the row), archives the raw collector
  files under `artifacts/bloodhound/`, regenerates `appendices/ad-enum.md`, and
  registers it in `structure.appendices`.

```bash
cd acme-engagement
rex ingest nmap -sV scanme.nmap.org
rex ingest naabu -host scanme.nmap.org -top-ports 100
rex ingest nikto -h https://portal.acme.com
rex ingest bloodhound ./collected-20240115.zip
rex report build --output acme-report.pdf
```

### `rex recon`

Run a whole recon sweep against a target in one command and fold everything into
the report's appendices and Timeline — a convenience wrapper that chains the same
machinery `rex ingest` uses.

```
rex recon --host <host> [--host ...] [--cidr <range> ...] [--dir <dir>]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--dir string` | `.` | Report directory. Must already contain `index.md`. |
| `--host strings` | — | Target host (repeatable, or comma-separated). |
| `--cidr strings` | — | Target CIDR range (repeatable, or comma-separated). |

Pass at least one `--host` or `--cidr`; both are forwarded to `nmap` as targets.
`recon` then runs, in order:

1. **A two-phase `nmap` scan.** First a fast all-port discovery pass (`-p-`) to
   find which ports are open, then a second pass with service/version detection
   and default scripts (`-sV -sC`) against *only* those open ports — so the
   slow, script-heavy pass never scans all 65535 ports. Results land in
   `artifacts/assets.json` / `appendices/assets.md`, exactly as
   `rex ingest nmap` would.
2. **`nikto`** against each discovered web app (one invocation per target).
3. **`nuclei`** against the discovered web apps (all in one run), merged into
   `artifacts/nuclei.json` and `appendices/nuclei.md` — a severity-sorted table
   of template/finding/host.

Web apps are detected from the `nmap` results: any open port whose service name
mentions `http`/`ssl`, or whose number is a well-known web port (80, 443, 8080,
8443, 8000, 8888), becomes an `http(s)://host[:port]` target (a host's first
hostname is preferred over its IP).

```bash
rex recon --dir ./acme-engagement --host 10.0.1.5
rex recon --dir ./acme-engagement --cidr 10.0.1.0/24
```

`recon` **fails fast**: `nmap`, `nikto`, and `nuclei` must all be installed and
on your `PATH`, or it aborts before scanning anything. As with `rex ingest`,
every sub-run (the `nmap` detail pass, each `nikto` run, the `nuclei` run) adds a
timestamped entry to the report's **Timeline** table.

### `rex ocr`

```
rex ocr <image> [--lang text] [--image TAG] [--no-docker]
```

Reads the text out of a screenshot (code, a terminal, a Caido or Burp
request/response) and prints it as a fenced Markdown code block, ready to
paste into a finding. It runs [Tesseract](https://github.com/tesseract-ocr/tesseract)
inside the report-rex image, so nothing leaves your machine.

On top of plain Tesseract, `rex ocr`:

- converts the image to grayscale, inverts dark-theme screenshots, and upscales
  small UI text before reading it
- places each word at its monospace column, so indentation, alignment and
  blank lines survive (Tesseract's text output drops leading whitespace)
- turns curly quotes (`“ ” ‘ ’`) back into the plain `"` and `'`

| Flag | Default | Description |
|------|---------|-------------|
| `--lang` | `text` | Language tag on the code fence (`http`, `python`, ...) |
| `--image` | `report-rex:<version>` | Docker image to run (auto-built if missing) |
| `--no-docker` | `false` | Use a local `tesseract` install instead of Docker |

```bash
rex ocr screenshots/login-request.png --lang http >> reports/vuln-3.md
```

OCR isn't perfect. Proofread the output, especially `0`/`O` and `1`/`l`/`I`.
Sharp, high-resolution screenshots give the best results.

### `rex version`

```
rex version
```

Prints the CLI version. Equivalent to the built-in `rex --version`/`rex -v`.

## Sync commands

These commands drive the [real-time sync feature](./sync) against a
running `rex-svr`. `--host` and `--username` are required on all of them, but
enforced in code rather than via Cobra's flag-required machinery — `rex sync
--help` won't mark them `(required)`, you'll only see the error at runtime if
they're missing.

### `rex sync`

```
rex sync [OPTIONS]

Options:
  --host string       Sync server host (required)
  --port int          Sync server port
  --username string   Username to authenticate as (required)
  --password string   Shared server password (prompted interactively, or $REX_PASSWORD, if omitted)
  --dir string         Workspace directory to synchronize
  --ca-cert string     Path to the server's self-signed certificate, to trust it
  --insecure           Skip TLS certificate verification (alternative to --ca-cert)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--host string` | — | Sync server host. Required. |
| `--port int` | `9443` | Sync server port. |
| `--username string` | — | Username to authenticate as. Required. |
| `--password string` | — | Shared server password. Prompted interactively (or read from `$REX_PASSWORD`) if omitted. |
| `--dir string` | `.` | Workspace directory to synchronize. |
| `--ca-cert string` | — | Path to the server's self-signed certificate (printed by `rex-svr` on startup), to trust it. |
| `--insecure` | off | Skip TLS certificate verification. Alternative to `--ca-cert`; exactly one is required. |

```bash
rex sync --host sync.example.com --username alice --dir ./acme-engagement
```

Logs in, does an initial reconcile, then runs until Ctrl-C: watches the local
filesystem and a WebSocket connection to the server, reconciling on either
trigger plus a 30-second safety-net ticker. See [Sync Reference](./sync) for
the reconciliation model.

The whole workspace syncs in full with one exception: the `handoffs/`
directory is scoped to the handoffs that involve you — given to you or given
by you — since a handoff is addressed to two specific people rather than the
team. See [Handoffs](./handoffs).

### `rex conflicts`

```
rex conflicts [OPTIONS]

Options:
  --host string       Sync server host (required)
  --port int          Sync server port
  --username string   Username to authenticate as (required)
  --password string   Shared server password (prompted interactively, or $REX_PASSWORD, if omitted)
  --dir string         Workspace directory
  --ca-cert string     Path to the server's self-signed certificate, to trust it
  --insecure           Skip TLS certificate verification (alternative to --ca-cert)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--host string` | — | Sync server host. Required. |
| `--port int` | `9443` | Sync server port. |
| `--username string` | — | Username to authenticate as. Required. |
| `--password string` | — | Shared server password. Prompted interactively (or read from `$REX_PASSWORD`) if omitted. |
| `--dir string` | `.` | Workspace directory. Accepted for flag-shape parity with `sync`/`resolve` but unused — this command only talks to the server API. |
| `--ca-cert string` | — | Path to the server's self-signed certificate (printed by `rex-svr` on startup), to trust it. |
| `--insecure` | off | Skip TLS certificate verification. Alternative to `--ca-cert`; exactly one is required. |

```bash
rex conflicts --host sync.example.com --username alice
```

Lists open conflicts on the server as a `PATH / KIND / REV / CREATED` table.

### `rex resolve`

```
rex resolve <path> [OPTIONS]

Options:
  --host string       Sync server host (required)
  --port int          Sync server port
  --username string   Username to authenticate as (required)
  --password string   Shared server password (prompted interactively, or $REX_PASSWORD, if omitted)
  --dir string         Workspace directory
  --remote              Keep the server's version; overwrite the local copy
  --local               Make the local version canonical on the server
  --merge               Attempt an automatic three-way merge (text files only)
  --keep-both           Keep both versions as separate files (binary conflicts)
  --suffix string       Suffix for --keep-both's derived filename
  --id int64            Disambiguate which conflict to resolve if a path has more than one open
  --ca-cert string      Path to the server's self-signed certificate, to trust it
  --insecure            Skip TLS certificate verification (alternative to --ca-cert)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--host string` | — | Sync server host. Required. |
| `--port int` | `9443` | Sync server port. |
| `--username string` | — | Username to authenticate as. Required. |
| `--password string` | — | Shared server password. Prompted interactively (or read from `$REX_PASSWORD`) if omitted. |
| `--dir string` | `.` | Workspace directory. |
| `--ca-cert string` | — | Path to the server's self-signed certificate (printed by `rex-svr` on startup), to trust it. |
| `--insecure` | off | Skip TLS certificate verification. Alternative to `--ca-cert`; exactly one is required. |
| `--remote` | off | Keep the server's version; overwrite the local copy. |
| `--local` | off | Make the local version canonical on the server. |
| `--merge` | off | Attempt an automatic three-way merge (text files only). |
| `--keep-both` | off | Keep both versions as separate files (binary conflicts). |
| `--suffix string` | your username | Suffix for `--keep-both`'s derived filename. |
| `--id int64` | `0` (unambiguous) | Disambiguate which conflict to resolve if a path has more than one open. |

```bash
rex resolve reports/vuln-3.md --host sync.example.com --username alice --remote

rex resolve reports/vuln-3.md --host sync.example.com --username alice --keep-both --suffix alice
```

Exactly one of `--remote`/`--local`/`--merge`/`--keep-both` is required.
`--merge` only applies to text-content conflicts (divergent edits) — it's
rejected for a delete-vs-edit conflict. Speaking of which: for a delete/edit
conflict, `--remote`/`--local` don't mean "server copy"/"local copy" as they
do for a normal divergent edit — `--remote` keeps the deletion and `--local`
restores the locally-edited file, since one side of the conflict has no
content at all. See [Conflict resolution](./sync#conflict-resolution) for the
full model.

### `rex presence`

```
rex presence [OPTIONS]

Options:
  --host string       Sync server host (required)
  --port int          Sync server port
  --username string   Username to authenticate as (required)
  --password string   Shared server password (prompted interactively, or $REX_PASSWORD, if omitted)
  --ca-cert string    Path to the server's self-signed certificate, to trust it
  --insecure          Skip TLS certificate verification (alternative to --ca-cert)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--host string` | — | Sync server host. Required. |
| `--port int` | `9443` | Sync server port. |
| `--username string` | — | Username to authenticate as. Required. |
| `--password string` | — | Shared server password. Prompted interactively (or read from `$REX_PASSWORD`) if omitted. |
| `--ca-cert string` | — | Path to the server's self-signed certificate (printed by `rex-svr` on startup), to trust it. |
| `--insecure` | off | Skip TLS certificate verification. Alternative to `--ca-cert`; exactly one is required. |

```bash
rex presence --host sync.example.com --username alice --ca-cert /path/to/rex-svr.crt
```

Not meant to be run by hand — a headless bridge that reads presence updates as
NDJSON on stdin and writes presence/connection events as NDJSON on stdout, for
editor integrations to drive. See [Editor presence](./sync#editor-presence).

### `rex editor-conf`

```
rex editor-conf --ide <editor> [OPTIONS]

Options:
  --ide string          Editor to generate a config for: nvim (required)
  --host string         Sync server host (required)
  --port int            Sync server port
  --username string     Username to authenticate as (required)
  --password string     Shared server password (omit and set $REX_PASSWORD instead of baking it into the generated file)
  --bin string          rex binary to invoke from the editor
  --dir string          Report-rex workspace to keep synced, and (unless --standalone) to scope activation to
  --standalone           Generate a self-contained nvim -u entrypoint instead of installing into your existing Neovim config
  --nvim-config string  Neovim config directory to install into; ignored with --standalone
  --output string       Directory to write the generated config into; only used with --standalone
  --remove              Remove a previously generated config instead of generating one (--host/--username not required)
  --ca-cert string      Path to the server's self-signed certificate, baked into the generated config to trust it
  --insecure            Bake --insecure into the generated config (alternative to --ca-cert)
```

| Flag | Default | Description |
|------|---------|-------------|
| `--ide string` | — | Editor to generate a config for: `nvim`. Required. |
| `--host string` | — | Sync server host. Required (unless `--remove`). |
| `--port int` | `9443` | Sync server port. |
| `--username string` | — | Username to authenticate as. Required (unless `--remove`). |
| `--password string` | — | Shared server password. Omit and set `$REX_PASSWORD` instead to avoid baking it into the generated file. |
| `--ca-cert string` | — | Path to the server's self-signed certificate, baked into the generated config to trust it. |
| `--insecure` | off | Bake `--insecure` (skip TLS certificate verification) into the generated config. Alternative to `--ca-cert`; exactly one is required. |
| `--bin string` | `rex` on `$PATH` | `rex` binary to invoke from the editor. |
| `--dir string` | `.` | Report-rex workspace to keep synced, and (unless `--standalone`) to scope activation to. |
| `--standalone` | off | Generate a self-contained `nvim -u` entrypoint instead of installing into your existing Neovim config. |
| `--nvim-config string` | `$XDG_CONFIG_HOME/nvim` or `~/.config/nvim` | Neovim config directory to install into. Ignored with `--standalone`. |
| `--output string` | `.` | Directory to write the generated config into. Only used with `--standalone`. |
| `--remove` | off | Remove a previously generated config instead of generating one (`--host`/`--username` not required). |

Generates a ready-to-use editor integration — no plugin manager or source
checkout required, since the editor integration itself is embedded in the
`rex` binary. For `--ide nvim`:

- **Default**: extracts the plugin into `<nvim-config>/rex-presence-nvim/`
  and writes a lazy.nvim plugin spec to
  `<nvim-config>/lua/plugins/rex-presence.lua` — installs into your existing
  Neovim config (LazyVim or plain lazy.nvim) without touching anything else
  in it. Only activates when Neovim is opened inside `--dir`.
- **`--standalone`**: extracts the plugin into `<output>/rex-presence-nvim/`
  and writes `<output>/rex-presence-init.lua`, a self-contained entrypoint
  you launch with `nvim -u <output>/rex-presence-init.lua`, bypassing
  whatever Neovim config normally applies — for a throwaway/test
  environment, not a daily driver.

Both modes also start a background `rex sync --dir <--dir>` alongside the
presence bridge, so the workspace stays synced automatically. See
[Editor presence](./sync#editor-presence).

```bash
cd acme-engagement
rex editor-conf --ide nvim --host sync.example.com --username alice

rex editor-conf --ide nvim --host sync.example.com --username alice \
  --standalone --output ~/.config/rex-presence
```

`--remove` (with the same `--standalone`/`--nvim-config`/`--output` flags you
generated with) deletes the generated files instead — no `--host`/`--username`
needed:

```bash
rex editor-conf --ide nvim --remove                    # default (lazy.nvim plugin) mode
rex editor-conf --ide nvim --remove --standalone --output ~/.config/rex-presence
```

### `rex handoffs`

```
rex handoffs <list|view|remove|sync> [OPTIONS]

Options (all subcommands):
  --host string       Sync server host (required)
  --port int          Sync server port
  --username string   Username to authenticate as (required)
  --password string   Shared server password (prompted interactively, or $REX_PASSWORD, if omitted)
  --dir string        Workspace directory
  --user string       Operate on another user's handoff queue
  --all               Operate on every user's handoffs
  --ca-cert string    Path to the server's self-signed certificate, to trust it
  --insecure          Skip TLS certificate verification (alternative to --ca-cert)

rex handoffs list [--resolved]
rex handoffs view <id>
rex handoffs remove <id>
rex handoffs sync [--watch]
```

| Flag | Default | Description |
|------|---------|-------------|
| `--host string` | — | Sync server host. Required. |
| `--port int` | `9443` | Sync server port. |
| `--username string` | — | Username to authenticate as. Required. |
| `--password string` | — | Shared server password. Prompted interactively (or read from `$REX_PASSWORD`) if omitted. |
| `--dir string` | `.` | Workspace directory. |
| `--ca-cert string` | — | Path to the server's self-signed certificate (printed by `rex-svr` on startup), to trust it. |
| `--insecure` | off | Skip TLS certificate verification. Alternative to `--ca-cert`; exactly one is required. |
| `--user string` | — | Operate on one operator's queue, by assignee. |
| `--all` | off | Operate on every handoff. Overrides `--user`. |
| `--resolved` | off | `list` only: include resolved handoffs. |
| `--watch` | off | `sync` only: keep running and reconcile continuously. |

Manages work handoffs — `handoffs/<assignee>/<title>.md` files whose canonical
copy and Git history live on `rex-svr`.

```bash
# hand something to a teammate
mkdir -p handoffs/shriyanss
printf -- '---\nto: shriyanss\n---\n\n# Investigate Next.js auth\n' \
  > handoffs/shriyanss/investigate-nextjs-auth.md
rex handoffs sync --host sync.example.com --username alice

# on the other side
rex handoffs list  --host sync.example.com --username shriyanss
rex handoffs sync  --host sync.example.com --username shriyanss --watch
```

With no scope flag, every subcommand covers the handoffs that involve you —
given to you *or* given by you — and nothing else. `remove <id>` (or deleting
the local file) resolves a handoff while the server keeps the canonical copy.

`rex sync` already applies that same default scope to the `handoffs/`
directory, so creating, editing and resolving handoffs all work without ever
running `rex handoffs sync`; use it when you want a *different* scope
(`--user`, `--all`), or handoffs on their own without the rest of the
workspace.

See [Handing Work to a Teammate](../getting-started/handoffs) for a
walkthrough, and [Handoffs](./handoffs) for the metadata schema, the
path/metadata consistency rule, and the resolution semantics.
