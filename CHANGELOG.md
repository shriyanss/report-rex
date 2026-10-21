# Changelog

All notable changes to this project are documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

Significant changes should be added here alongside the change itself, not
batched up later.

## [Unreleased]

### Changed
- Report Rex is now open source under the MIT license and lives at
  <https://github.com/shriyanss/report-rex>.
- CI moved from GitLab CI to GitHub Actions (`ci.yml`, `docs.yml`,
  `release.yml`). Releases now publish to GitHub Releases (binaries and the
  `rex-presence` `.vsix`), `ghcr.io/shriyanss/report-rex`, and PyPI; docs
  deploy to GitHub Pages. `render-test` now renders inside the real runtime
  image, so `Dockerfile.ci` is gone.
- **Breaking (Go):** the module path is now `github.com/shriyanss/report-rex/cli`
  (was `gitlab.ritsec.cloud/cptc/report-rex/cli`), including the `-ldflags -X`
  version path.
- Docs `baseUrl` now comes from `DOCS_BASE_URL` (default `/`) instead of
  GitLab's `CI_PAGES_URL`.

## [0.8.3] - 2026-10-09

### Fixed
- `rit-cptc` draft builds no longer fail with "Dimension too large" at the
  cover page: the DRAFT watermark is now set at 2.5cm (scaled 6x, same
  look), below the size where the body font's word-space stretch overflows.
- `rit-cptc` `--docx` output now matches the PDF page for page: A4 geometry
  and fonts, the indigo brand-band cover in its own unnumbered section, the
  Confidentiality / Document Control page, a TOC with page numbers, numbered
  headings, `\clearpage` page breaks, the findings summary table, the PDF's
  finding panel, and the folded-corner + classification-banner page art —
  same page breaks, same line breaks, same TOC leader dots, code wrapping
  with `↪` continuations, and positions within a few points.

### Changed
- `rit-cptc` now typesets in Liberation Sans/Mono (SIL OFL, shipped in
  `templates/rit-cptc/assets/fonts/`) instead of TeX Gyre Heros/Cursor; its
  `--docx` clone embeds the same font files, so both outputs draw identical
  glyphs whether or not the fonts are installed.
- `rit-cptc` PDF typesetting now follows rules Word can reproduce, so the
  `--docx` clone breaks lines identically: font sizes on whole half-points in
  PostScript points (body 10bp, inline code 9.5bp, `\large` headings 11.5bp),
  no kerning or ff/fi ligatures, `\frenchspacing`, interword spaces that
  stretch but never shrink (so lines fill greedily, like Word's), no automatic
  hyphenation, free breaks after a typed `-`, rigid spacing around
  tables/charts and paragraphs, `\raggedbottom`, list indents of 18bp/36bp,
  `*` as the third-level bullet, `→` for continuation marks, and an em dash
  never starting a line. The visual change to the PDF is slight.
- DOCX charts are now vector SVG (via `pdftocairo`, with pandoc's PNG
  fallback for older Word) instead of 300dpi PNGs.
- DOCX charts use the template's own palette and colors, its font and 11pt
  size and table spacing, are sized in points (no longer shrunk), and no
  longer carry a stray "Chart" caption.
- Donut charts: slice counts are now centred in each ring segment instead of
  being partly or fully hidden under the hole, and the centre total is
  captioned "Total" so it no longer reads as another slice (#79).

## [0.8.2] - 2026-10-04

### Added
- `radar` (alias `spider`) chart type for `:::findings_graph:…:::`. With the
  `business` dataset every business-risk category gets a spoke; with fewer
  than three categories it falls back to `bar`.
- `material` chart dataset (aliases `material_impact`, `fair-mam`), tallying
  each finding's `material_impact` (FAIR-MAM) category.
- `rit-cptc`: findings summary table (ID, title, severity, CVSS) at the top
  of Vulnerability Findings, each row linking to its finding.
- Example report: FAIR-MAM material-impact appendix (`appendices/3.md`).
- Docs: "Bundled Lua Filters" page listing every template's `vuln_filter.lua`/
  `docx_vuln_filter.lua`, the vulnerability attributes each reads, and the
  LaTeX macros they expect.

### Changed
- `rit-cptc` reworked from editor review, ahead of merging it with
  `rex-default`:
  - Cover now uses rex-default's layout in rit-cptc colors. The brand content
    is centred in the band, there's a "Prepared by" block with your logo, the
    Report ID is removed, and the date is the build date (`\today`). The
    "Date Submitted" row in Document Control is also the build date. The
    `cover`/`mint` colors are removed.
  - Finding panel is now rex-default's tcolorbox, without Status, keeping the
    finding ID, Impact/Sophistication, Mitigations and Material Impact. It
    adds Business Risk. Affected assets are nested under Component.
  - Sections and findings are numbered (`3.1`, …).
  - Narrower margins, so footnotes sit lower. Extra space around charts and
    tables. Tables have full grid lines.
  - The page number now sits in the bottom-right banner (`CONFIDENTIAL | 3`), which is
    wider to fit it. The banner triangle is flush with the page edge.
- Example report:
  - The executive summary uses the OWASP matrix (linking to Appendix B) and a
    business-risk radar.
  - The Overall Risk Rating reads "The overall risk rating is …".
  - Rules of Engagement moved from Appendix A into Methodology › Scope.
- VS Code extension bumped to rex-presence 0.3.1, a repackage of 0.3.0 with
  no functional changes.

### Fixed
- Docs: Lua filter and template-structure pages now cover
  `docx_vuln_filter.lua` and `reference.docx` (previously claimed only
  `vuln_filter.lua` was discovered).
- The Timeline appendix leaves the date blank for a 1970-01-01 placeholder
  entry instead of printing the Unix epoch (#73). `validate` still flags it.
- A single-category pie or donut chart now draws a full disc with a centred
  label, instead of a seam from the centre to the rim with the label on it (#76).
- Long tokens in code blocks wrap inside the margin (#69): highlighted
  blocks now break inside pandoc's token macros, and plain/indented code
  blocks (LaTeX `verbatim`) wrap too, in every template.
- `rex report build` (the Go assembler) now gets the same fixes as the Python
  one: CVSS-score ordering within a severity (#67), the blank 1970 Timeline
  date (#73) and the single-category pie/donut (#76). They previously only
  reached `report-rex build`.
- An image's `{width=90%}` attribute block no longer leaks into the PDF as
  literal text when a space or line break separates it from the image, and an
  image line with text directly above/below now renders as a captioned figure
  instead of an inline image (#70). `report-rex build` also prints pandoc's
  warnings (e.g. a missing image) on success instead of discarding them.
- Pasted finding text keeps its structure (#71): a list or heading right
  after a paragraph, with no blank line between, now renders as a list or
  heading instead of running into the paragraph. `validate` also warns about
  indented text that renders as a code block (`indented_code_block`), section
  names written as plain lines (`section_heading_plain_text`), and leftover
  TODO/TBD/FIXME notes (`todo_marker`).
- Cover / Document Control (#74): the `rex-default` Document Control table now
  spans the page width, so values like the Report ID no longer wrap mid-word.
  `validate` flags the example's ACME client logo and `RR-2024-0142` /
  `RR-2024-0001` report IDs as leftover placeholders, and warns
  (`testing_window_incomplete`) when only one of `start_date`/`end_date` is set.
- Appendices nest properly (#75): headings inside an appendix are demoted one
  level so they sit under the appendix title instead of beside it, and
  appendices (Timeline included) are lettered A, B, C… by position, replacing
  any lettering in the title, so `rex ingest`'s "Appendix — …" titles no longer
  break the sequence. The example report's Methodology now points to Appendix A
  for scope instead of repeating a different scope table.
- Executive summaries can stay in step with the findings (#72): new inline
  `:::findings:total:::`, `:::findings:<severity>:::` and
  `:::findings:highest:::` tokens, which the example executive summary and
  overall risk rating now use instead of hard-coded counts. `validate` reports
  links to findings that aren't in the report (`dangling_reference`), and the
  lorem-ipsum placeholder check now matches any case and `loremipsum`.
- New findings use Description / Impact / Replication / Remediation (#68):
  Steps to Reproduce and Evidence are merged into Replication, and
  Recommendation is now Remediation, in the example report, `report-rex new`,
  `rex`'s finding skeleton, the MCP server's guidance and every vuln-template.
  `validate` checks `## Replication` and still accepts `## Steps to Reproduce`
  in existing reports.

## [0.8.1] - 2026-10-02

### Fixed
- Findings with the same severity are now ordered by CVSS score, highest
  first, instead of by title (#67).

## [0.8.0] - 2026-10-02

### Fixed
- `--docx` output now carries the template's styling (#66): each template
  ships a branded `reference.docx` (Arial/Courier New instead of the
  Office-only Aptos theme fonts that Pages/LibreOffice flag as missing,
  primary-colored headings, bordered tables with a shaded header row and
  banded rows, shaded inline code, accent-colored links), and the stray
  date line above the cover page is gone. Regenerate with
  `core/scripts/make_reference_docx.py` after changing a theme's colors.
- Long inline code (paths, URLs, script tags) no longer runs off the right
  margin in PDF output (#64): every template's filter now adds line-break
  points after `/ . : =` in inline code, and sets `\emergencystretch`.

### Changed
- `rit-cptc` is now the default template (#63): `rex report init` /
  `report-rex new` scaffold full reports with `rit-cptc` and quick-comms with
  `rit-cptc-comm`, and an `index.md` with no `template:` falls back to
  `rit-cptc`. **Breaking:** the old `default` template is renamed to
  `rex-default` — update `template: "default"` to `template: "rex-default"` in
  existing reports.
- VS Code extension bumped to rex-presence 0.3.0, shipping the Report view,
  admin features, template picker, and in-editor validation listed below.

### Added
- `rex recon` runs a full recon sweep against a target (`--host`/`--cidr`) and
  folds everything into the report (#60): a two-phase `nmap` scan (fast all-port
  discovery, then `-sV -sC` only on the open ports), then `nikto` and `nuclei`
  against the web apps it discovers. Adds a `nuclei` ingest path
  (`artifacts/nuclei.json` → `appendices/nuclei.md`). Requires `nmap`, `nikto`,
  and `nuclei` on `PATH` (fails fast otherwise); every sub-run is stamped into
  the Timeline.
- `rex ingest` now adds a Timeline entry for every run, e.g. `Port scan
  (nmap) against 10.0.1.5, 10.0.1.9 (+3 more)` (#59). Entries are kept in
  `artifacts/timeline.json` and written into the generated appendix's
  frontmatter. To hide one, set `"include": false` in the JSON.
- The VS Code extension catches broken finding frontmatter while you type
  (#58). It now lints unsaved markdown (debounced) through the new `rex report
  validate --stdin`, which validates editor buffers in place of the files on
  disk. A missing `title:` has a quick fix that inserts one.
- `rex report validate` now reports `frontmatter_invalid` (pointing at the
  first line that breaks the YAML, e.g. where an unclosed quote opens) and
  `title_missing`.
  Before, it silently dropped bad frontmatter, and only the Python validator
  had these checks. Both validators also add `frontmatter_not_at_top`, for a
  `---` block that has blank lines or a BOM before it and so is ignored
  (#58).
- **Enable Admin** setting in the VS Code extension (#57). The admin features
  (Revert Report to This Change, Delete for Everyone) are hidden unless
  `report-rex.admin.enabled` is on. They unlock only after rex-svr accepts
  the admin password, which is now asked once per session instead of on
  every action. New `rex admin verify` and `POST /api/v1/admin/verify` check
  an admin password without changing anything.
- Linting for the network-diagram and chart data files (#53). `rex report
  validate` (and so every build) now checks `artifacts/assets.json` and
  `artifacts/connections.json`, and checks how report files use them: a
  `:::network_diagram:::` with no `assets.json`, chart directives with no data,
  invalid JSON, bad IPs/CIDRs, misspelled keys, pivots to hosts that aren't in
  the inventory, and an unused `connections.json`. Issues now carry a line
  number (`line N:` in text output, `"line"` in `--json`). The VS Code
  extension watches those files by name, re-lints when they change (including
  `rex ingest` rewrites), and underlines the exact line.
- `rex ocr <image>` (#56): reads the text out of a screenshot (code, a Caido
  request/response) and prints it as a fenced Markdown code block. `--lang`
  sets the fence's language tag. It runs Tesseract inside the report-rex
  image (newly installed there), so screenshots never leave the machine. It
  inverts dark-theme screenshots, upscales small text, rebuilds indentation and
  blank lines from Tesseract's word positions, and turns curly quotes back into
  plain ones. `--no-docker` uses a local `tesseract`.
- Network diagram (#53): a `:::network_diagram:::` directive draws the
  `rex ingest` inventory (`artifacts/assets.json`) as a topology map. A core
  cloud links to one region per subnet, and each region has a switch bussed to
  OS-colored host icons, with a legend underneath. The build fails if
  `assets.json` is missing. An optional `artifacts/connections.json` labels
  subnets (`"subnets": {CIDR: label}`), draws pivots as dashed red arrows
  (`"pivots": {IP|CIDR: IP|CIDR|[...]}`), and outlines hosts that must always be
  shown (`"important": [IP, ...]`). A subnet with more than 12 hosts hides hosts
  that have no pivot first, and the diagram shrinks to fit the page. See
  *Charts → Network Diagram* in the docs.
- New validation error `metadata_missing` (#52): `index.md` must set
  `title`, `client` and `date`. It fails `rex report build` /
  `report-rex build` and shows as an error (red file) in VS Code.
- New validation error `screenshot_none` (#51): every finding in `reports/`
  must embed at least one screenshot. It fails `rex report build` /
  `report-rex build` and shows as an error (red file) in VS Code.
- Rex Blame (#49): see who last changed each line of a synced file, from
  `rex-svr`'s git history. In VS Code, right-click a file → *Rex Blame* opens
  the file's blame in a new tab. Rest the cursor on a line for 5s to see its
  author and age at the end of the line (`report-rex.blame.inline`). On the CLI
  this is `rex blame <path>`, and over HTTP it's `GET /api/v1/blame/{path}`.
- VS Code **Add Finding** can start from a `vuln-templates/` boilerplate
  (#48). Pick a template or a blank finding, then optionally enter a filename.
  On the CLI this is `rex report section vuln --list-templates` and
  `--file <name>`, which writes `reports/<name>.md` instead of the next `vuln-N.md`.
- Revert the whole synced report to an earlier change (#45): right-click an
  entry in the VS Code **Recent Changes** view, or run `rex revert`. It needs
  the new `rex-svr --admin-password` (or `--admin-password-file`), previews every file it would change
  (diffable), the change's git commit and the git log, and applies only the
  previewed plan. History is never rewritten: files get new revisions and git
  gets a `Revert to ...` commit.
- Delete files for everyone in one step (#46): multi-select in the VS Code
  explorer → *Report Rex: Delete for Everyone…*, or `rex delete <path>...`.
  Needs `rex-svr --admin-password`; handoffs and excluded paths are skipped,
  and the server records one git commit.
- VS Code **Report** view (#47): build the draft/final PDF and add sections
  in one click, and **Initialize Report** in an empty folder (runs
  `rex report init`). `rex report init` now ignores hidden entries such as
  `.git/` and `.vscode/` when checking that the target folder is empty.

### Security
- `rex sync` run in the directory `rex-svr` was started from no longer uploads
  the server's TLS private key and live database (with every session token)
  to the whole team, or the server's own git workspace back into itself:
  `rex-svr.db*`, `rex-svr.crt`, `rex-svr.key` and `rex-svr-git-workspace/`
  are now excluded from sync. `rex-svr` also refuses a `--cert`/`--key` inside
  `--workspace`, which committed the private key to git history (#62).

### Fixed
- `rex sync` no longer wipes the team's report when the synced folder is moved
  or emptied while it runs (#46). A missing sync folder is now an error, not
  an empty workspace. A pass that would delete at least 5 files and over 90% of
  the synced files is refused (edits and pulls keep syncing), with a warning
  pointing at the admin delete above. `rex-svr` enforces the same limit on
  per-file deletes (one user, 10 minutes), so older clients can't bypass it.
- Screenshots on consecutive lines with no blank line between them no longer
  render squashed together; each image-only line now becomes its own paragraph
  (#41).
- Sync data loss after a `rex-svr` restart (#62). `rex-svr --workspace .`
  put the default `./rex-svr.db` inside the workspace, so the first start's
  drift check deleted the live database. The next restart came up empty,
  and every client deleted its files. Related failures found alongside it:
  - `rex-svr` now refuses to start when `--db`/`--cert`/`--key` is inside
    `--workspace`, when `--workspace` is an existing non-empty folder or git
    repository it didn't create (whose files the first start used to delete
    permanently), or when most of the workspace is missing from the
    database. Before, that last case only logged a warning and then served
    the empty manifest anyway.
  - A client no longer deletes a file just because the server has no record
    of it (not even a deletion). It pushes the file back instead, so a
    server restarted on a fresh database (e.g. from another directory,
    since the defaults are relative) is repopulated instead of wiping
    everyone.
  - Long-running `rex sync`, `rex presence` and `rex mcp` log in again when
    the server no longer knows their session, instead of failing with 401
    until restarted.
- Renaming a folder no longer leaves an empty copy under the old name on other
  users' workspaces; `rex sync` now prunes directories emptied by a remote
  delete (#44).
- Findings with invalid YAML frontmatter or no `title` no longer render
  silently as "Untitled": `validate` (and so `build`) reports them as
  `frontmatter_invalid` / `title_missing` errors naming the file, a forced
  build fails with the file name instead of dropping the metadata, and an
  untitled finding falls back to its filename (#50).

## [0.7.0] - 2026-09-27

### Security
- All `rex-svr`/`rex` client-server communication is now encrypted (HTTPS/WSS
  only — no plaintext fallback). `rex-svr` auto-generates a self-signed
  certificate on first run (or serves one you provide via `--cert`/`--key`)
  and prints its fingerprint; every client command (`sync`, `presence`,
  `conflicts`, `resolve`, `handoffs`, `editor-conf`) gains `--ca-cert` (pin
  the server's certificate) and `--insecure` (skip verification on a trusted
  network) flags, and the VS Code/Neovim integrations pass these through.
  (Issue #35)

### Fixed
- Sync under multi-user load (issue #42): a long-running `rex sync` silently
  never pushed local deletes once it had pushed anything; re-creating a file
  a teammate deleted was rejected as a conflict (`Store.Put` treated a
  tombstone as a live file); every save of a conflicted file (and every
  restart) filed another open conflict; `rex resolve --merge` re-merged over
  hand-edited files and could submit files still containing markers; a
  merge resolution could overwrite a teammate's newer push; pulls could
  overwrite edits made mid-pass; the stored hash was the scan-time one, not
  what was pushed; converged state never refreshed its base; continuous
  writes starved the fsnotify debounce and git commits; WebSocket clients
  never noticed half-open connections. 10-client/100ms stress test: 2455
  conflicts with no convergence -> 49 (22 auto-merged), converged in 5s.
- `.vscode/` is excluded from sync (it holds per-user editor settings).
- `rex-svr`'s startup drift check could delete an entire synced workspace in
  one commit if its DB manifest ever came back emptier than what was
  genuinely on disk (e.g. an operational `--db`/`--workspace` mismatch
  between restarts). `GitMaterializer.DriftCheckAndRepair` now refuses to
  delete when the affected paths are both `>=5` files and `>50%` of the
  on-disk workspace, logging a warning instead, and its repair commits list
  every path actually touched instead of just a bare count. (Issue #37)

### Added
- `rex sync` auto-merges non-overlapping concurrent edits (three-way merge);
  only overlapping ones become conflicts, at most one open per (path, user).
- `rex sync --json` (NDJSON events, stdin `{"op":"reconcile"}`), `rex status
  [--json]` (connection test: health, login, recent changes, conflicts),
  `rex conflicts --json` / `--id N --side local|remote|base`, ID/BY columns
  in `rex conflicts`; `rex presence` forwards `remote_change` and
  `conflict_created`; `/healthz` reports the server version.
- VS Code extension (rex-presence 0.2.0) rebuilt: Report Rex activity-bar
  views (Sync & Server, Conflicts with compare/keep mine/take server/merge
  in editor/keep both, Recent Changes, Team, Handoffs), Start (tests the
  connection first) / Stop / Sync Now / Test Connection, conflict
  notifications and Explorer badges, status bar, TLS setup, Build Report
  and Add Finding/Appendix/Section commands, auto-restart with backoff, and
  a headless integration suite (`scripts/test-headless.sh`).
- `cli/scripts/sync_stress.py`: N-client extreme-load sync stress test.
- `rex mcp`'s `add_finding`/`update_finding` now reject any `body` that
  doesn't contain a one-liner `curl` PoC command — mirrors bug-bounty triage
  practice, where a reproducible one-liner is what lets the human triager
  confirm a finding immediately instead of re-deriving repro steps from an
  AI-written report. (Issue #40)
- `rex mcp`: a stdio MCP server so an AI harness (Claude Code, Codex, etc.)
  can add and update vulnerability findings without touching report files on
  disk directly. `add_finding`/`update_finding` always stamp `ai: true` on
  every AI-authored/edited finding, and `update_finding` refuses a finding
  that isn't already AI-owned unless `user_authorized: true` is set — the
  human user has to explicitly ask for a human-authored finding to be
  changed. If `--host` is given, writes are pushed through `rex-svr` under
  their own `--username` identity, and `update_finding` waits 5s before
  pushing if another session currently has the target file open. (Issue #39)
- VS Code extension: a **Report Rex** panel in the Explorer sidebar showing
  sync status, `rex-svr` connectivity, and the host/port/username/directory/
  password, each editable from the panel. `rex sync` is now off by default
  per workspace (`report-rex.sync.enabled`, toggle from the panel or
  Command Palette) instead of starting automatically, and the stored
  password is now scoped per workspace instead of shared across all of
  them. (Issue #38)
- `--password` flag on `report-rex build`/`rex report build`/`rex watch`/`build.sh`:
  encrypts the output PDF (AES-256, via `qpdf`) with the given password. No
  password means no encryption (default, unchanged). DOCX output is never
  encrypted. (Issue #36)
- Documented the `notes/` convention for sharing open-ended team notes over
  `rex-svr`: a plain directory under the workspace root, synced and
  committed to Git the same as any other file, with no dedicated command or
  scoping. (Issue #34)

## [0.6.2] - 2026-09-11

### Added
- `rex report build`/`rex watch` show an animated progress indicator while
  pandoc/XeLaTeX render the PDF — previously silent, so a build in progress
  was indistinguishable from a hung one. Suppress it with `--no-terminal-bs`
  (also skipped automatically when output isn't a terminal). (Issue #29)
- Generated PDF/DOCX reports now record the tool name and version (e.g.
  `Report Rex v0.5.0`) as the document creator/producer (PDF) or a
  `generator` custom property (DOCX), so a generated file's origin is
  identifiable from the file's own metadata. (Issue #31)

### Fixed
- `screenshot_text_too_small` fired on almost every screenshot, always reporting the
  glyph height as ~4px regardless of actual font size — the heuristic took the median of
  per-column vertical ink runs across the whole image, a statistic dominated by short
  partial intersections (curves, serifs, anti-aliasing) rather than true glyph height.
  Replaced with a horizontal ink-density row profile (a text line's true height is the
  union, across every column, of "some glyph has ink here"), which measures real text
  accurately. Now that it's reliable, promoted from a warning to an error — a too-small
  screenshot now fails `rex report build`/`report-rex build` instead of only printing a
  warning. (Issue #30)
- Long lines inside fenced code blocks ran off the page edge instead of
  wrapping in the `default` and `quick-comm` templates — `fvextra` (which
  backs pandoc's Skylighting `Shaded`/`Highlighting` environment) only wraps
  long lines when `\fvset{breaklines=true, breakanywhere=true}` is set; the
  `rit-cptc`/`rit-cptc-comm` templates already had it, `default`/`quick-comm`
  now do too. (Issue #28)
- Links inside appendix (and other) markdown tables silently lost their
  footnote text and printed a stray digit instead of a clickable link — a
  plain LaTeX `\footnote` doesn't survive inside pandoc's generated
  `longtable`. Table-row links now stay plain inline hyperlinks instead of
  being converted to footnotes. (Issue #27)
- Asset Inventory table: long unbroken values (dotted hostnames like
  `scanme.nmap.org`, IPs, `port/proto` labels) overflowed past the Hostname
  column's right border into OS instead of wrapping, because tabularray's
  `X`-column word-wrap has no break point without a space. `escLatex`/`_esc`
  now insert `\allowbreak{}` after `.`, `/`, and `:`, mirroring the fix the
  templates' `vuln_filter.lua` already applied to pandoc-authored tables.
  (Issue #21)

### Added
- `rex report section <vuln|appendix|narrative>` adds a new finding,
  appendix, or narrative section to an existing report and registers it
  under the right `structure:` key in `index.md`, instead of copying an
  existing file by hand and renumbering/re-registering it. `vuln` supports
  `--template <name>` to start from a `vuln-templates/*.md` stub (now
  embedded into the CLI binary), overlaying any flags passed and stripping
  only the `[!REX-STUB]` markers for the fields overridden. (Issue #23)
- `rex report init` now requires `--title`/`--client`/`--date`, or
  `--force` to scaffold with placeholder values anyway (with a warning) —
  previously it always shipped a placeholder title/client/date with no way
  to set them at scaffold time. (Issue #23)
- QA-gate validation (`Validate`/`validate_report`, both CLI ports) now
  scans `index.md`'s own frontmatter for leftover scaffold placeholder
  text, not just section/report/appendix files — previously an unedited
  `client`/`contact_name`/`contact_email`/date field in index.md was
  never checked at all.
- New placeholder sentinels: `Your Name`, `you@example.com`, the Unix
  epoch (`1970-01-01`, a date no real engagement can have), and a `Lorem
  Ipsum` marker embedded throughout the rewritten `reporting/example`/
  `reporting/communication/*` content.
- A hash-based `screenshot_placeholder_image` check flags the shipped
  Lorem Ipsum placeholder screenshot (`scripts/gen_placeholder_screenshot.py`)
  if it's still referenced unedited.
- `rex report build`'s validation guard moved into `RunReportBuild`
  itself (not just its `cmd/report_build.go` caller), so `rex watch`'s
  rebuild loop — previously unguarded — now enforces the QA gate too.
- `--docx` flag on `report-rex build`/`rex report build`/`rex watch`/`build.sh`:
  generates a `.docx` clone alongside the PDF, from the same assembled
  `combined.md`/`metadata.yaml`. Rather than converting the rendered PDF
  (an earlier attempt at that lost most of the report's formatting), pandoc
  renders the assembled Markdown a second time with `--to=docx`, using a new
  `filters/docx_vuln_filter.lua` per template (the DOCX-side counterpart to
  `filters/vuln_filter.lua`) that emits raw OpenXML for the branded cover
  page, table of contents field, and severity panels. Charts, which are
  PDF-only raw LaTeX/TikZ, are rasterized to PNG via `xelatex` + `pdftoppm`
  before the DOCX render so they appear as embedded images — for Docker-mode
  builds that rasterization runs inside the `report-rex` image itself, not
  on the host, so `--docx` still needs nothing beyond Docker. Supported by
  all four bundled templates (`default`, `rit-cptc`, `rit-cptc-comm`,
  `quick-comm`) in both the Python and Go CLIs. See
  [Output Formats](docs/docs/reference/output-formats.md).
- `--json` flag on `rex report validate`: encodes `{issues, errors, warnings}`
  to stdout instead of printing human-readable text, so tooling can consume
  it directly.
- Linter in both editor extensions (closes #26): the VS Code extension and
  the Neovim plugin now run `rex report validate --json` on save (and once on
  activation/`setup()`) and surface every issue as a native
  diagnostic/`vim.diagnostic` — no line/column, since `rex report validate`
  reports per-file, not per-line. Independent of presence/sync — works with
  no `rex-svr` host/username configured. Toggle at runtime via the "Report
  Rex: Toggle Linter" command (VS Code) or `:RexLintToggle` (Neovim); the
  auto-detected "is this a rex report project" check (`index.md` presence)
  can be manually overridden via the "Report Rex: Set Rex Project Override"
  command / `report-rex.projectOverride` setting (VS Code) or
  `:RexProjectOverride {auto|on|off}` / `project_override` setup option
  (Neovim).

### Changed
- `rex report init`'s example scaffold (`reporting/example`,
  `reporting/communication/*`) no longer ships realistic-looking ACME
  Corporation content, real dates, or real screenshots — it's now
  obviously-fake Lorem Ipsum text/epoch dates/a placeholder image, so an
  unedited copy fails `rex report validate` out of the box instead of
  silently building into a real-looking client report. (Issue #22)
- **Breaking**: `report-rex build`/`rex report build`/`rex watch`/`build.sh` now
  produce a DRAFT-watermarked report by default (renamed to `*-draft.pdf`),
  instead of a final report by default. The `--draft` flag is removed and
  replaced with `--final`, which opts out of the watermark and produces the
  plain output filename. This closes #25 — it was too easy to hand a client a
  final, unwatermarked report before it had been reviewed.

## [0.5.0] - 2026-08-31

### Added
- `rex ingest existing naabu -o <file>`: ingest naabu output the user already
  has on disk (its default, non-`-json` `host:port`-per-line format) instead
  of re-running naabu against the target.
- `rex ingest existing nmap -oX <file>`: ingest an nmap XML report the user
  already has on disk instead of re-running nmap against the target.
- `:::asset_graph:table:ports:::` (alias `matrix`) directive: renders the full
  host/port inventory as a native LaTeX table.

### Fixed
- The asset inventory appendix `rex ingest` generates
  (`appendices/assets.md`) didn't render correctly via `rex report build`:
  its `:::asset_graph:...:::` chart directives (and the parallel
  `:::web_graph:...:::` ones `rex ingest nikto` generates) were only ever
  implemented in the Python core, never ported to `rex`'s native Go report
  assembler — they passed through to the PDF as literal unrecognized text.
  Also replaced the hand-built markdown table in that appendix with the new
  `:::asset_graph:table:ports:::` directive: the old table's column widths
  came from a markdown separator row's dash count rather than actual content,
  so long IP addresses and version strings rendered truncated/overlapping.

## [0.4.23] - 2026-08-30

### Fixed
- `rex report build`/`rex image build`/`rex watch`/`build.sh` defaulted the
  Docker image tag to `report-rex:latest` and only auto-built it when
  missing — so an image built by an older CLI/Dockerfile version was silently
  reused forever, hiding image-level fixes (confirmed manually: v0.4.22's
  tcolorbox/logo-path fixes were invisible on a machine with an already-built
  `report-rex:latest` from before the fix, requiring a manual `docker rmi`).
  Default tag is now `report-rex:<version>` (the CLI's own version /
  `version.txt`), so a CLI upgrade naturally targets a new tag and rebuilds.

## [0.4.22] - 2026-08-30

### Fixed
- v0.4.21's arm64 Docker build fix got the image building, but `rex report
  build` still failed to render on real arm64 hardware, two more bugs deep:
  - tcolorbox 6.9.0 (whatever version this pinned TL2025 archive happens to
    ship) unconditionally calls PDF-tagging kernel macros that only exist
    once a document opts into `\DocumentMetadata{testphase=tagpdf}` — and
    that machinery is itself broken in this snapshot (missing
    `\IfPDFManagementActiveF`, then a missing `\socket_if_exist:nF`, then a
    broken tcolorbox-internal tagging counter, three layers deep). None of
    report-rex's templates use real PDF tagging, so the Dockerfile now pins
    tcolorbox to 6.8.0 (the last release before this tagging code was
    added) instead of chasing the rabbit hole.
  - `company_logo`/`company_logo_light`/`client_logo` paths were rewritten
    to absolute *host* paths (`absolutizeAssets`/`_absolutize_assets`) so
    XeLaTeX could resolve them regardless of pandoc's working directory —
    correct for the old Python/Docker architecture (where assembly ran
    *inside* the container against the mounted report dir), but broken by
    the Go CLI's redesign, where assembly happens on the host and only the
    assembled temp dir gets mounted into the render container. Removed
    `absolutizeAssets` from the Go port (the Python core is unaffected —
    different architecture, still correct there) and instead pointed
    pandoc's own working directory at the assemble dir for both `--no-docker`
    (`cmd.Dir`) and Docker (`docker run -w`) rendering, where `Assemble`
    already copies `assets/` — so the original relative paths just resolve
    correctly in both modes.
  - Verified end-to-end on real arm64 Kali Linux hardware: a full report
    with branded logos and severity finding panels renders correctly.

## [0.4.21] - 2026-08-30

### Fixed
- `rex report build`'s auto-built Docker image failed with `exec format
  error` on native arm64 Linux hosts — the image was `FROM
  pandoc/latex:3.6`, which only publishes a `linux/amd64` manifest, and
  `build.sh`/the Go CLI forced `--platform linux/amd64` to compensate
  (relying on QEMU emulation that Docker Desktop on Apple Silicon bundles
  but a bare-metal arm64 Linux Docker install doesn't). Switched the base
  image to `pandoc/latex:3.6-ubuntu`, which publishes real `amd64` and
  `arm64` manifests (native `aarch64-linux` TeX Live binaries, not
  emulated), and dropped the `--platform linux/amd64` pins so Docker
  builds/runs on the host's native architecture.

## [0.4.20] - 2026-08-27

### Fixed
- `infra-commit-state`'s MR-create REST call 405'd consistently from the
  CI runner across all 3 retries on v0.4.19, despite the identical call
  succeeding from an operator's machine with the same token — a
  runner-egress-specific quirk. Switched to `git push` options
  (`merge_request.create`/`merge_when_pipeline_succeeds`), the same push
  path that has never failed once this whole project.
- Committed v0.4.19's real deployed state directly (docs deployment
  included, confirming that fix too).

## [0.4.19] - 2026-08-27

### Added
- `infra-deploy` now publishes the docs site (`deploy-docs`) as part of
  every release — previously documented as a manual-only step, so the
  docs VM's nginx was live but only ever served cloud-init's placeholder
  empty webroot. Gets the same fresh-boot SSH retry treatment as
  `deploy-rex-image`, plus `nodejs`/`npm` for the Docusaurus build.

### Fixed
- `infra-commit-state`'s MR-creation call now succeeds —
  `INFRA_STATE_PUSH_TOKEN` has the needed "Merge Request: create/merge"
  scope, closing out the last manual step in the release automation.

## [0.4.18] - 2026-08-27

### Fixed
- `deploy-rex-image`'s retry window (90s) wasn't enough — `report-rex`'s
  cloud-init installs `docker.io`, which can take several minutes on a
  fresh VM, unlike the gateway's much lighter cloud-init. Bumped to ~6.7
  minutes (20 attempts × 20s).
- Cleaned up 5 orphaned `infra-state-update-*` branches left by
  `infra-commit-state`'s repeated 403s, and committed v0.4.17's real
  deployed state directly (single, correct, non-duplicated stack).

### Known issue
- `INFRA_STATE_PUSH_TOKEN` still needs "Merge Request: create/merge"
  scope granted.

## [0.4.17] - 2026-08-27

### Fixed
- `deploy-rex-image` hit the same fresh-VM boot race the gateway
  wait-loop already handled, on the `report-rex` instance instead — it's
  LAN-only, reachable only via SSH ProxyJump, so the direct TCP check
  didn't cover it. Its operations (`rsync --delete`, `docker build`,
  `systemctl restart`) are all safe to retry wholesale, so the whole
  script call is now wrapped in a retry loop instead of a second bespoke
  wait mechanism.
- Committed v0.4.16's real deployed state directly (single, correct,
  non-duplicated stack, WireGuard genuinely up on the gateway).

### Known issue
- `INFRA_STATE_PUSH_TOKEN` still needs "Merge Request: create/merge"
  scope granted — `infra-commit-state`'s MR-creation call still 403s.

## [0.4.16] - 2026-08-27

### Fixed
- Root cause of every `infra-deploy` SSH timeout across v0.4.8–v0.4.15:
  `enable_gateway_ssh` defaults to `false` and CI never set it `true` —
  the gateway's security group never had an SSH ingress rule at all,
  regardless of CIDR. `.infra-setup` now exports
  `TF_VAR_enable_gateway_ssh=true`.
- The v0.4.14 SSH-wait loop's `/dev/tcp` check had no connect timeout, so
  an unreachable host stalled each of 24 attempts for ~2 minutes (~48
  minutes total) instead of failing fast. Wrapped in `timeout 3`.
- `infra-commit-state`'s retry reused the same branch name
  (`infra-state-update-${CI_PIPELINE_ID}`), colliding with the first
  attempt's already-pushed commit. Switched to `CI_JOB_ID`, unique per
  attempt.
- v0.4.15's `infra-deploy` actually created real, correct, non-duplicated
  infra (confirming the retry: 0 fix from v0.4.15) — pulled that artifact
  state and committed it directly rather than destroying and recreating.

### Known issue
- `INFRA_STATE_PUSH_TOKEN` needs "Merge Request: create/merge" scope —
  the MR-creation API call currently 403s.

## [0.4.15] - 2026-08-26

### Fixed
- `infra-commit-state`'s direct `git push HEAD:main` could never have
  worked: `main`'s branch protection is `push_access_levels: [{No one}]`
  — nobody can push there directly, MR is mandatory for everyone. Now
  pushes to a per-pipeline branch and creates+merges an MR via the API.
- The v0.4.14 default `retry: 2` auto-ran `infra-deploy` three times in
  one pipeline, leaking a second full parallel VM stack — a partially
  successful `apply` isn't safe to blindly rerun from a fresh job
  attempt. `infra-destroy`/`infra-deploy` now opt out with `retry: 0`.
- Manually cleaned up v0.4.13/v0.4.14's leaked OpenStack resources again
  (data volume confirmed safe throughout both times — detached, `status:
  available`).

## [0.4.14] - 2026-08-26

### Fixed
- `infra-deploy`'s `terraform apply` succeeded on v0.4.13 (state correctly
  persisted, thanks to v0.4.13's fix) but `deploy-gateway-key`'s SSH still
  timed out 10s later — a fresh VM's boot/cloud-init and Neutron security
  group rule propagation aren't instant. Now polls the gateway's SSH port
  (bash `/dev/tcp`, no new dependency) for up to two minutes first.
- Added a pipeline-wide default `retry: 2` — today's releases needed a
  manual retry on nearly every job type (lint, test, publish, infra
  alike) for transient cluster resource pressure. Removed `e2e-ingest`'s
  now-redundant job-level retry.

## [0.4.13] - 2026-08-26

### Fixed
- v0.4.11 and v0.4.12 each had a successful `terraform apply` followed by
  a later step failing the job (SSH timeout, then floating-IP quota), so
  the updated state never got committed — `artifacts` defaults to
  `on_success` and `infra-commit-state` was skipped since its `needs`
  target failed. This leaked two full parallel VM stacks in OpenStack,
  with the protected data volume (multiattach) mounted on both
  `report-rex` VMs simultaneously. Manually inventoried and cleaned up
  via the local `openstack` CLI (data volume confirmed safe throughout —
  detached, `status: available`); reconciled the committed
  `terraform.tfstate` to match reality, verified with a clean local
  `terraform plan`: "32 to add, 0 to change, 0 to destroy".
  `infra-destroy`/`infra-deploy` now set `artifacts: when: always`, and
  `infra-commit-state`'s rule adds `when: always`, so this class of
  silent, untracked cloud drift can't recur.

## [0.4.12] - 2026-08-26

### Fixed
- `infra-deploy`'s `terraform apply` succeeded completely on v0.4.11 (33
  resources created), but `deploy-gateway-key`'s SSH to the gateway timed
  out: `TF_VAR_gateway_ssh_cidrs` was a static CI/CD variable captured
  from one past job's egress IP, and this run landed on a different
  cluster worker node with a different NAT egress IP. Every `.infra-setup`
  job now detects its own egress IP fresh at runtime; `infra-test`
  additionally re-applies the SSH security group rule with its own IP
  before attempting SSH, since it can land on yet another node than
  `infra-deploy`. Removed the now-unused static CI/CD variable.

## [0.4.11] - 2026-08-26

### Fixed
- `infra-destroy` failed on v0.4.10: `flag provided but not defined:
  -exclude`. That flag doesn't exist on `terraform destroy`/`apply`/`plan`
  at all — checked against 1.10.5 and the current latest, 1.15.9. v0.4.10
  was based on a wrong assumption. Fixed by generating a `-target=` list
  for every resource except the protected data volume, from `terraform
  state list`. Verified via a scratch debug job's `plan -destroy` against
  real infra state: "Plan: 0 to add, 0 to change, 35 to destroy", no
  `prevent_destroy` error. Also fixed the same wrong claim in
  `infra/Makefile`'s `destroy` target and the README example.

## [0.4.10] - 2026-08-26

### Fixed
- `infra-destroy` failed on v0.4.9: `flag provided but not defined:
  -exclude` — that flag needs Terraform 1.10+, and `TERRAFORM_VERSION` was
  still pinned to 1.9.8. Bumped to 1.10.5.



### Fixed
- `infra-destroy` computed a correct 36-resource destroy plan against real
  live infra on v0.4.8 (both the `$HOME` and `OS_CLIENT_CONFIG_FILE` fixes
  held), but errored on the `prevent_destroy`'d data volume — a bare
  `terraform destroy` can't selectively skip one protected resource, it
  just refuses the whole plan. Fixed with `-exclude=` in the CI job, the
  `infra/Makefile` `destroy` target, and the README example, all three of
  which had documented the same incorrect bare-destroy behavior.

## [0.4.8] - 2026-08-26

### Fixed
- `infra-destroy` failed again on v0.4.7 (past the `$HOME` fix): Terraform's
  OpenStack provider has a narrower `clouds.yaml` lookup than
  `python-openstackclient`'s — the file existed and was readable by the
  `openstack` CLI, but `terraform` still reported "no clouds.yml file
  found". Fixed by setting `OS_CLIENT_CONFIG_FILE` explicitly for both.
  Also fixed `keypair.tf` needing the SSH *public* key
  (`~/.ssh/cptc_infra.pub`), which was never derived from the decoded
  private key — now generated via `ssh-keygen -y`. Verified against real
  infra state via a scratch debug job: `terraform plan` now completes
  cleanly end to end.

## [0.4.7] - 2026-08-26

### Fixed
- `infra-destroy` failed on v0.4.6's first real run: `unable to load
  clouds.yaml: no clouds.yml file found`. This runner's arbitrary
  non-root UID has no `/etc/passwd` entry, so `$HOME` is unset and bare
  `~` silently fails to expand — `mkdir -p ~/.config/openstack` created a
  directory literally named `~` instead of writing to a real home.
  `.infra-setup` now sets `$HOME` explicitly up front.

## [0.4.6] - 2026-08-26

### Fixed
- `e2e-ingest` now retries (`retry: 2`) to absorb transient network flakes
  reaching external hosts (nmap.org/github.com) from the runner, which had
  reddened pipeline 2777 despite no actual code regression.
- `publish-docker` moved to the `privileged` runner pool, the same fix
  already applied to `e2e-cli` for the identical restricted-SCC `chown`
  gap. It was previously `allow_failure: true`, silently hiding a real,
  reproducible Kaniko failure (`chown /bin: operation not permitted`) in
  every release since v0.4.4. Confirmed green on the privileged pool
  (pipeline 2825) — `allow_failure` removed.

### Added
- `infra/` — the previously un-gitted `rex-infra` OpenStack deployment
  (Terraform, deploy scripts, `rex-svr` Dockerfile) now lives in this repo,
  with `terraform.tfstate` committed so CI can act on real infra state
  across runs.
- New `infra` CI stage: on every version tag, destroys and rebuilds the
  live CPTC OpenStack infra against the just-tagged report-rex source
  (`infra-destroy` → `infra-deploy` → `infra-commit-state`), then runs
  `infra/scripts/simulate-client` — a new smoke test that drives the real
  `rex-svr` HTTP API (`healthz` → `login` → `manifest`) over an SSH tunnel
  to the deployed server — as a release gate.

## [0.4.5] - 2026-08-23

### Changed
- `e2e-cli` and `build-ci-image` now run on the `privileged` runner
  (`tags: [privileged]`), giving `e2e-cli` a real Docker daemon instead
  of the unreachable dind fallback that the unprivileged pool's
  SecurityContextConstraint blocked. `e2e-cli`'s `before_script` now
  only falls back to `DOCKER_HOST=tcp://docker:2375` when the runner
  hasn't already provided one, and no longer forces
  `DOCKER_TLS_CERTDIR=""`, so it respects the privileged pool's own
  TLS-cert setup for its dind service. Confirmed green on the
  privileged runner (pipeline 2772) — `allow_failure` removed.

## [0.4.4] - 2026-08-23

### Changed
- `e2e-cli` (needs a real Docker daemon) and `publish-docker` (Kaniko,
  blocked on a `chown` capability this runner's SecurityContextConstraint
  strips even from its own build pod) are both `allow_failure: true`
  now, tracking the same category of gap — a runner/cluster capability
  neither `.gitlab-ci.yml` nor an image swap can grant. v0.4.3 already
  shipped the Python package, Go CLI binaries, and VS Code extension;
  this just stops the Docker image specifically from blocking future
  releases while that gap is open.

### Fixed
- `e2e-sync`/`e2e-handoffs`'s `apt-get install git` was redundant —
  `$GO_IMAGE` (`golang:1.25-bookworm`) already ships git, being built
  from `buildpack-deps:bookworm-scm`. One less `apt-get` call as this
  project's runner pool migrates to OKD.

## [0.4.3] - 2026-08-23

### Fixed
- v0.4.2's tag pipeline confirmed `publish-package`, `publish-cli`, and
  `publish-vscode-extension` all working, but `publish-docker` failed
  before its own script even started: `error: could not lock config
  file .../.docker-home/.gitconfig: No such file or directory`.
  Setting `HOME` at the job's `variables:` level affects GitLab
  Runner's own internal `git clone` too, which runs before
  `$CI_PROJECT_DIR` exists -- moved `HOME`/`DOCKER_CONFIG` to `export`
  lines inside `script:` instead (the same pattern `render-test`
  already used successfully), so they only take effect once the
  checkout is already in place.

## [0.4.2] - 2026-08-23

### Fixed
- v0.4.1's tag pipeline reached the publish stage for the first time
  but three of its four `publish-*` jobs still failed, each for a
  distinct reason discovered only by actually running them (the
  publish stage had never gotten this far before): `publish-docker`'s
  Kaniko image has a root-owned `/tmp`, unlike every other image used
  in this pipeline (moved its Docker config to a project-relative
  path instead); `publish-vscode-extension`'s `npx --prefix DIR vsce
  package` doesn't change vsce's own working directory the way `npm
  --prefix DIR` does, so it was looking for `package.json` at the
  repo root (a latent bug in the job's own script, not the runner —
  wrapped in a `cd` instead); and `publish-cli`'s `goreleaser` refused
  to run against a "dirty" git tree, because GitLab downloads every
  earlier stage's artifacts into a job by default and `e2e-ingest`'s
  `e2e-report/` showed up as an untracked directory (added
  `dependencies: []` to all four `publish-*` jobs, since none of them
  need any prior job's artifacts).

## [0.4.1] - 2026-08-22

### Fixed
- The `v0.4.0` tag pipeline's `publish` stage never ran: `build-ci-image`'s
  bare `changes: [Dockerfile.ci]` rule (no `if:` guard) evaluates as
  unconditionally true for pipeline types GitLab can't diff against a base,
  which includes tag pipelines — it fired despite `Dockerfile.ci` never
  changing, failed, and (with no `allow_failure`) blocked every later stage
  including `publish`. Restricted the rule to branch pipelines, where the
  diff is actually meaningful.
- `ruff`, `vale`, `build-ci-image`, and `publish-docker` were failing
  outright on this project's current runner pool, which assigns pods a
  non-root UID with no writable `$HOME`: `pip install`/`apt-get` silently
  fall back to locations that are either unreadable (`ruff` installed to a
  user site not on `PATH`) or unwritable (`apt-get`'s own package cache),
  and Kaniko's default `/kaniko/.docker/config.json` is root-owned.
  `ruff` now runs via `python -m ruff` (found through `sys.path`
  regardless of `PATH`); `vale` moved off `debian:bookworm-slim` to
  `$GO_IMAGE` (`buildpack-deps:bookworm-scm`), which already ships
  `curl`/`ca-certificates`/`tar` and needs no `apt-get` at all;
  `build-ci-image` and `publish-docker` point Kaniko's `DOCKER_CONFIG` at a
  `/tmp`-based path instead, which is writable regardless of UID.
- `publish-vscode-extension`'s `node:20-slim` image doesn't ship `curl`
  either (the same gap `publish-package` already works around with
  Python's `requests`); replaced both `curl` calls with Node's built-in
  global `fetch`, dropping the `apt-get install curl` step entirely.

## [0.4.0] - 2026-08-22

### Added
- `rex handoffs` (issue #16): a handoffs namespace for passing a thread of
  investigation to a teammate. A handoff is a Markdown file at
  `handoffs/<assignee>/<title>.md` whose canonical copy and Git history live on
  `rex-svr`, with `list`/`view`/`remove`/`sync` subcommands. Writing `to:` and a
  body is enough to create one: Rex stamps
  `rex_handoff_id`/`from`/`created_at`/`status` in place, deriving the creator
  from the authenticated client rather than trusting the file. The directory a
  handoff lives in is part of its identity — `handoffs/<username>/` must match
  the `to` metadata, and a mismatch is an explicit error on both client and
  server rather than a silently-picked winner, holding just that handoff back.
  Resolving (via `remove <id>` or by deleting the file) records
  `resolved_by`/`resolved_at` and retires the active path so it leaves every
  workspace, while the resolved copy is retained server-side and committed to
  Git; a resolved handoff is never resurrected by a later reconciliation, and
  the server refuses a raw `DELETE` on an open handoff so a client can't
  destroy the canonical copy. Scope defaults to the handoffs that involve you —
  given to you *or* given by you — with `--user <name>` for one queue and
  `--all` for everything; `rex handoffs sync --watch` reconciles continuously
  against a server-side subscription. New `cli/internal/handoff` package,
  `/api/v1/handoffs*` routes, and a `?prefix=` subscription filter on
  `/api/v1/ws`. Docs: [Handing Work to a
  Teammate](docs/docs/getting-started/handoffs.md) and the [Handoffs
  reference](docs/docs/reference/handoffs.md).

- `rex report validate` (issue #15): the QA-gate report validator — CVSS/timeline
  presence, placeholder/stub-marker detection, and screenshot heuristics
  (dark-mode luminance, annotation-color clustering, text-legibility
  estimate) — ported natively to Go (`cli/internal/report`, `cli/internal/cvss`),
  closing the previous gap where `validate` only existed via Docker/Python.
  Image analysis uses Go's stdlib `image` package instead of Pillow/numpy.
- `rex ingest nikto`/`rex ingest bloodhound` (issue #12): `nikto` follows the
  same live-scan pattern as `nmap`/`naabu` — merges parsed web findings into
  `artifacts/webfindings.json`, regenerates `appendices/web-findings.md`
  (a finding table plus a `:::web_graph:bar:host:::` chart) — while
  `bloodhound` imports an already-collected SharpHound/BloodHound-CE export
  (`.zip` or extracted directory) rather than running a live collector,
  since AD collection needs real domain credentials `rex` doesn't manage;
  it regenerates `appendices/ad-enum.md` (object counts plus high-value/
  Kerberoastable/AS-REP-roastable/unconstrained-delegation principal lists,
  read directly off the collector's raw AD attributes). Both register their
  appendix in `index.md` the same way the existing tools do.
- `core/src/report_rex_core/charts.py`: `render_web`, handling a new
  `:::web_graph:...:::` directive for `nikto`-sourced data, following the
  same pattern as `render_assets`.
- CI: `e2e-ingest` now also runs `rex ingest nikto` against `scanme.nmap.org`
  and `rex ingest bloodhound` against the official SpecterOps BloodHound-CE
  sample dataset (BloodHound collection needs real AD credentials no CI
  environment has, so there's no live-collector run to do there). Installs
  nikto from its upstream release tarball plus `libxml-writer-perl` rather
  than a Debian package, since Debian doesn't ship one.
- CI: `e2e-ingest` job (issue #12) runs `rex ingest nmap`/`rex ingest naabu`
  against `scanme.nmap.org` and hands the resulting report directory to
  `render-test`, which builds it into a real PDF alongside its existing
  per-template renders — a genuine end-to-end smoke test of the ingest
  feature rather than just unit tests. Split across two jobs because
  `e2e-ingest`'s image has no pandoc/XeLaTeX, and rendering there via Docker
  would hit the same dind bind-mount limitation `e2e-cli` already documents.
- `docs/`: documented `rex ingest` (Go CLI reference), the `asset_graph`
  chart directive it emits (Charts reference), and the generated
  `appendices/assets.md` file (Sections and Appendices reference).
- `rex ingest naabu [naabu-args...] --dir <report-dir>` (issue #12): same
  asset-inventory pipeline as `rex ingest nmap`, parsing `naabu -json`
  output instead. Ingesting with both tools against the same report merges
  into one shared `artifacts/assets.json` by IP (e.g. nmap's service/version
  detail fills in alongside naabu's faster port discovery for the same
  host). The tool-running/merge/appendix-regeneration logic that both share
  was factored into a single `toolSpec.run` in
  `cli/internal/ingest/runner.go`.
- `rex ingest nmap [nmap-args...] --dir <report-dir>` (issue #12): runs nmap
  natively, merges the parsed host/port inventory into
  `artifacts/assets.json` (accumulating across repeated scans, keyed by IP),
  archives the raw XML under `artifacts/nmap/`, regenerates a
  `appendices/assets.md` asset-inventory appendix (host/port table plus
  open-ports and OS breakdown charts), and registers that appendix in
  `index.md`'s `structure.appendices` if it isn't already listed. The new
  `:::asset_graph:pie:os:::`/`:::asset_graph:bar:ports:::` directive it emits
  is handled by `core/src/report_rex_core/charts.py`'s `render_assets`,
  reusing the same LaTeX chart renderers as `findings_graph` — no
  template/Lua changes were needed for it to work across every template.

### Added
- `e2e-handoffs` CI job (issue #16): a `-tags e2e` real-subprocess test suite
  (`cli/internal/synctest/handoff_e2e_test.go`) that builds real `rex`/`rex-svr`
  binaries and drives the full handoffs workflow against a real running
  server and real git history — create (via stamping, not hand-written
  metadata), list/view without syncing, edit, resolve via `rm`, verify no
  resurrection, verify the create/update/resolve trail in git log, a
  dir/`to` mismatch rejected by both the CLI and the raw HTTP endpoint,
  three-user scoping (a handoff between two other users never lands in a
  third user's workspace), and the server refusing a raw `DELETE` on an open
  handoff. Runs alongside the existing `e2e-sync` job.

### Changed
- `rex sync` now scopes the `handoffs/` directory instead of syncing it in
  full: it materializes the handoffs given to you and the ones you gave someone
  else, and leaves handoffs between two other people out of your workspace.
  Everything else in the workspace is unaffected. Creating, editing and
  resolving handoffs (including turning a local `rm` into a resolve) all work
  through plain `rex sync`; `rex handoffs sync` is for a different scope
  (`--user`, `--all`) or for handoffs without the rest of the workspace.
- `rex report build`/`rex watch` (issue #15): report assembly (`combined.md`/
  `metadata.yaml` — frontmatter parsing, CVSS scoring, cross-reference
  resolution, chart generation, timeline tables) now runs natively in Go
  instead of shelling out to the Python `report-rex` entrypoint inside
  Docker. Docker (or a local `pandoc`/XeLaTeX install with `--no-docker`) is
  now used only for the final `pandoc`/XeLaTeX render step — the one part of
  the pipeline that genuinely needs an external tool. The Python
  `report-rex-core` package is unchanged and still installable on its own.

### Fixed
- `rex ingest naabu`'s "Ingested N host(s)" count reported raw output lines
  instead of unique hosts: naabu emits one JSON line per open port (sometimes
  duplicated per host), so the freshly-parsed batch is now self-merged by IP
  before being folded into the existing inventory and before being counted.
- The Discovered Assets table's OS column showed blank instead of `Unknown`
  for hosts with no OS match, unlike the OS pie chart's fallback label.
- `templates/*/filters/vuln_filter.lua`'s generic table restyling: long
  unbroken table-cell tokens (IP/IPv6 addresses, hostnames, URLs — surfaced by
  `rex ingest`'s generated appendices, but a latent issue for any table with
  such content) had no space for pandoc's tabularray `X` columns to wrap at,
  so they overflowed into or overlapped neighboring cells. Fixed in all four
  templates by inserting a `\allowbreak{}` LaTeX break hint after `.`/`/`/`:`
  in every table cell (the empty `{}` matters — a bare trailing space gets
  silently swallowed by TeX's after-control-word space-skipping, which first
  surfaced as an unrelated-looking missing-space bug during testing).

## [0.3.0] - 2026-08-19

### Added
- `publish-vscode-extension` CI job: packages the `rex-presence` VS Code
  extension and uploads the `.vsix` to this project's Generic Package
  Registry on version tags, via the same `CI_JOB_TOKEN` auth
  `publish-package` already uses. Since the extension isn't on the VS Code
  Marketplace, this is now its only distribution channel — the
  getting-started docs and the extension's own README point at the
  registry download instead of asking users to build the `.vsix`
  themselves (building from source is still documented as a fallback for
  testing unreleased changes).
- A VS Code extension (`cli/editors/vscode/`, issue #13) mirroring the
  Neovim integration: presence-based file decorations in the Explorer, a
  status-bar indicator, one-time/concurrent-edit notifications, and a
  background `rex sync` job. Configured entirely through VS Code's Settings
  UI (`report-rex.*` settings plus a `Report Rex: Set Password` command)
  instead of generated config files — same `rex presence`/`rex sync`
  subprocess-bridge architecture as Neovim, no protocol logic duplicated in
  TypeScript. Conflict resolution stays a CLI-only workflow for now (`rex
  conflicts`/`rex resolve`).
- `rex editor-conf --ide nvim --remove` removes a previously generated config
  (the vendored plugin copy, plus the lazy.nvim spec or standalone entrypoint,
  matching whichever `--standalone`/`--nvim-config`/`--output` flags it was
  generated with) — doesn't require `--host`/`--username`.

### Fixed
- The VS Code extension's `package.json` pointed `main` at
  `./out/extension.js`, but `tsc` (per `tsconfig.json`'s `rootDir: "."`)
  actually emits to `./out/src/extension.js` — the extension would never
  have activated once installed. Also added a `repository` field, since
  packaging it (`vsce package`) was failing outright on the README's
  relative link to the Neovim plugin's README, undetected until now because
  neither `lint-vscode` nor `test-vscode` actually packages/loads the
  extension.

### Changed
- `rex editor-conf --ide nvim` now installs a lazy.nvim plugin spec into the
  user's existing Neovim config by default (LazyVim, or any plain lazy.nvim
  config following its own recommended `lua/plugins/` layout), instead of
  generating a standalone entrypoint that bypassed it — the old behavior is
  still available via `--standalone`, for a throwaway/test environment with
  no personal config to preserve. The new default only activates (presence,
  explorer highlighting, sync) when Neovim is opened inside the configured
  `--dir`, so it's safe to leave installed permanently even in a Neovim
  config that also handles unrelated projects.
- The Neovim integration now also starts a background `rex sync --dir` job
  alongside the presence bridge (both modes), so opening the editor in a
  report-rex workspace keeps the files themselves in sync too, not just
  presence — turning it into an actual collaborative environment rather than
  a presence indicator layered over possibly-stale files. This required
  giving `rex sync`/`rex conflicts`/`rex resolve` the same `$REX_PASSWORD`
  environment-variable fallback `rex presence` already had, since a
  backgrounded `rex sync` has no real terminal to prompt on.

### Fixed
- The Neovim plugin's file-explorer highlight (added below) never lit up at
  all on a stock LazyVim install: LazyVim's current default file explorer is
  `snacks.nvim`'s (`snacks_picker_list` filetype), which the highlight logic
  didn't recognize (only `netrw`/`neo-tree`/`NvimTree` were). Verified against
  a real LazyVim + `snacks.nvim` setup that highlighting now works there too.
- The Neovim plugin's file-explorer highlight (added below) could stay stuck
  red on a file well after a teammate stopped editing it: `VimLeavePre`
  queued a final `presence_release` write to the `rex presence` child
  process and then immediately killed it, which could win the race and drop
  that write, leaving the ~15-20s server-side TTL sweep as the only fallback
  instead of a near-instant explicit release. Fixed by giving the queued
  write a moment to actually flush before killing the process.

### Added
- `rex editor-conf --ide nvim` generates a ready-to-launch Neovim presence
  config (the plugin extracted alongside a small entrypoint Lua file) from
  the `rex` binary alone — no plugin manager or source checkout needed. The
  Neovim plugin also now highlights, in any loaded file-explorer buffer
  (`netrw`, `neo-tree`, `nvim-tree`), the files a teammate is currently
  editing, matched by filename against the same presence state already
  driving the statusline/notifications.
- Editor presence warnings and a first Neovim integration (issue #11),
  built on top of issue #9's advisory presence plumbing: presence is now
  tracked per editor session (`session_id`), with an explicit `viewing`/
  `editing` state and a dirty-buffer flag, on a tightened 15s lease with a
  5s heartbeat interval (was 60s/20s). New `presence_update`/
  `presence_release` WebSocket messages replace the old bare `heartbeat`
  message, and `GET /api/v1/presence` returns a REST snapshot of every
  active lease for clients that just connected. A new `rex presence`
  subcommand bridges an editor integration's stdin/stdout JSON-lines
  commands to this protocol (`syncclient.DialPresence`/`PresenceSession`
  handle the send-capable WebSocket connection, reconnecting with backoff
  like `rex sync` does). The first client is a Neovim plugin at
  `cli/editors/nvim`: it reports `viewing`/`editing` presence per buffer
  (with a 1s debounce on `BufEnter` so passing through a buffer doesn't
  report anything), shows a statusline indicator and a one-time
  notification when opening a file someone else has open, and a louder
  "CONCURRENT EDIT" notification when starting to edit a file someone else
  is already editing. As with all presence in this project, it's advisory
  only — connection failures never block editing, and nothing is ever
  persisted. See the new [Real-Time Sync & Editor Presence
  reference page](docs/docs/reference/sync.md) for setup.

### Changed
- Shortened the Go CLI's released binary names (issue #10): the client binary
  is now `rex` (was `report-rex`) and the sync server binary is now `rex-svr`
  (was `report-rex-server`). Updated everywhere the names appeared:
  goreleaser, the Makefile, Cobra command names/help text, CI, and docs. The
  Python `report-rex-core` package and its `report-rex` console script are
  unaffected.

## [0.2.1] - 2026-08-16

### Fixed
- `publish-package`'s "skip upload if this version is already published"
  check now uses Python's `requests` instead of shelling out to `curl`,
  which isn't installed in the `python:3.11-slim` image — it was failing
  the job outright (JSON decode error on empty output) rather than the
  intended fail-open behavior. Broke `publish-package` on every tagged
  release since it was introduced; first caught on the v0.2.0 release.

## [0.2.0] - 2026-08-16

### Added
- CI/build plumbing for the sync feature (issue #9): `.goreleaser.yml` now
  cross-compiles `report-rex-server` alongside `report-rex` (same
  5-platform matrix); both binaries are bundled into the existing single
  per-platform release archive (verified with a local
  `goreleaser release --snapshot`). `cli/Makefile` gains a `build-server`
  target. New `e2e-sync` CI job builds and runs both real binaries as
  subprocesses (`cli/internal/synctest`, `-tags e2e`) through initial
  push/pull, a concurrent-edit conflict + resolve, and a server restart to
  exercise drift repair -- catching integration issues in-process unit
  tests can't (this suite already caught and fixed a real bug: unresolved
  conflicts were being re-flagged as brand-new server-side conflict
  records on every reconcile pass in a long-running `sync` session).
- Advisory edit presence (issue #9, phase 9 of 9, final phase — optional
  per the issue, never required for sync correctness): clients send a
  best-effort WebSocket heartbeat while actively working, the server
  tracks a 60s lease per (username, path) with a 20s expected heartbeat
  interval, and broadcasts `alice is editing findings/XSS-001.md` /
  `alice stopped editing ...` to other connected `report-rex sync`
  sessions. Leases are in-memory only (never persisted) and are never
  checked before accepting a file write — presence is purely informational.
  Lease-expiry math is unit-tested with an injected clock; broadcast
  delivery is verified against a real two-client WebSocket exchange.
- Explicit offline/reconnection handling for `report-rex sync` (issue #9,
  phase 8 of 9): the WebSocket client now reports connect/disconnect/
  reconnecting transitions ("Connection lost... Working offline...
  Reconnecting... Reconnected. Reconciling workspace...") and fires an
  immediate reconcile pass on every successful (re)connect, so whatever
  changed locally while disconnected is pushed and whatever changed on the
  server is pulled in one pass — the same `Engine.Reconcile`/`Classify`
  logic phases 3 and 5 already built, since offline-then-reconnect is just
  "fresh local scan + fresh remote manifest + last-known-synced state",
  nothing bespoke. The filesystem watcher and periodic ticker keep running
  the entire time the server is unreachable, so local edits are never
  missed. Verified against a real server process kill/restart mid-session.
- `report-rex resolve <path> --merge` now performs a real automatic
  three-way text merge (issue #9, phase 7 of 9), instead of returning "not
  yet implemented". Uses a line-based diff3-style algorithm (exact Myers
  line diff via `sergi/go-diff`, not fuzzy context matching): BASE, LOCAL,
  and REMOTE are diffed independently against BASE, and any non-overlapping
  edits merge automatically. A base-content endpoint
  (`GET /conflicts/{id}?include=base`) was added so the client can fetch
  the originally-synced revision for the three-way comparison. If the
  merge can't be resolved automatically (overlapping or simultaneous
  same-position edits), both versions are preserved verbatim inside
  git-style `<<<<<<< LOCAL` / `=======` / `>>>>>>> REMOTE` markers written
  to the local file; re-running `resolve --merge` after hand-editing
  detects the markers are gone and submits the edited content as the
  resolution.
- New `report-rex conflicts` and `report-rex resolve <path>` client
  commands (issue #9, phases 5-6 of 9): `conflicts` lists every open
  divergence the server is holding; `resolve` accepts exactly one of
  `--remote`, `--local`, or `--keep-both` (`--merge` is recognized but not
  yet implemented — phase 7) and applies it both on the server (new
  canonical revision + git commit, or a derived `path.conflict-<user>.ext`
  sibling for `--keep-both`) and locally. `--remote`/`--local` on a
  delete/edit conflict map to keeping the deletion or restoring the edit,
  per the issue's explicit requirement that a delete racing an offline
  edit never auto-resolves either way. Divergence protection itself
  (optimistic concurrency via `base_revision`, hash/revision-based
  Local/Remote/Both-changed classification, no implicit last-write-wins)
  was already in place from phases 1 and 3 — this closes the loop by
  giving users a way to actually resolve what it flags.
- Real-time continuous synchronization for `report-rex sync` and
  `report-rex-server` (issue #9, phase 4 of 9): the server now broadcasts
  `file_updated`/`conflict_created` events over a WebSocket
  (`GET /api/v1/ws`) to every connected client immediately after each
  committed change, instead of clients only finding out on their next poll.
  The client watches its workspace with `fsnotify` for every file type
  (not just `.md`/`.yaml`/`.yml` — screenshots and other assets are
  first-class sync content) and triggers a reconcile pass on both a
  WebSocket notification and a filesystem change, debounced/coalesced so
  an editor's atomic temp-file-then-rename save doesn't cause duplicate
  work. A 30s ticker remains as a fallback in case a WS notification or
  filesystem event is ever missed. Also fixes a long-standing gap in
  `internal/watcher` (shared with `report-rex watch`): a subdirectory
  created after the watcher started was previously never picked up.
- New `report-rex sync` client command (issue #9, phase 3 of 9): logs into a
  `report-rex-server`, scans the local workspace, and reconciles it against
  the server's canonical manifest — pushing local-only changes, pulling
  remote-only changes, and (never silently) flagging divergent edits as
  explicit conflicts via the server's existing 409 conflict path. Sync
  decisions are made purely from content hashes and server-assigned
  revisions (`internal/synccommon.Classify`), never filesystem modification
  time. Local per-path sync state lives in a new `.report-rex/sync.db`
  (itself excluded from sync scope). This first cut re-reconciles on a
  timer; real-time filesystem watching lands in phase 4.
- `report-rex-server` now projects accepted canonical changes into a
  server-owned git working copy (issue #9, phase 2 of 9): debounced,
  coalesced commits (one commit per burst of writes rather than one per raw
  event), repo-local git identity (never depends on global `git config`),
  a `pending_materializations` durability queue so a crash between the DB
  accept and the git commit is recovered on the next flush/restart without
  losing the accepted change, and a startup drift check that re-materializes
  the git tree from the database whenever they disagree (the database is
  always authoritative; manual edits inside the server's git workspace are
  never treated as canonical).
- New standalone `report-rex-server` binary (issue #9, phase 1 of 9): owns a
  SQLite-backed canonical file store (`report-rex.db`) with per-path
  revisions, content hashes, and tombstones, plus username/shared-password
  authentication over bearer sessions. Exposes an HTTP API
  (`/api/v1/auth/login`, `/manifest`, `/files/{path}`, `/conflicts`,
  `/conflicts/{id}/resolve`, `/healthz`) as the foundation for the
  client-server collaboration layer. Verifies `git --version` on startup
  (fails clearly if git is missing) ahead of phase 2's git materialization
  work. No client-side `sync` command yet — that lands in a later phase.

## [0.1.2] - 2026-08-16

### Fixed
- `report-rex report build` now checks that `index.md` exists in the resolved
  report directory *before* touching Docker, failing fast and clearly instead
  of a slow round-trip ending in a container-side error.
- When the report directory demonstrably exists on the host but the container
  sees it as empty (commonly Docker Desktop not sharing a `/tmp`-rooted path
  with its VM), the CLI now surfaces an actionable hint instead of just the
  raw "index.md not found in /report" error.
- `version.txt` now actually matches the CLI's tagged releases — it was left
  at `0.1.0` through the `v0.1.1` release, so locally-built binaries
  (`make build`, which reads `version.txt`) and officially tagged release
  binaries (which read the git tag) embedded different version strings.

### Added
- `e2e-cli` CI job: builds the real `report-rex` binary and drives it through
  `image build` and `report init` against a live Docker daemon, catching
  regressions in the embed pipeline and image-build/scaffold logic that unit
  tests alone couldn't (they never touch a real docker daemon). `report
  build` isn't exercised here — this runner's Docker executor exposes the
  host's `/var/run/docker.sock` rather than supporting privileged dind, so a
  path from inside the job's own container isn't visible to that daemon at
  the same path, and `report build`'s bind mount comes up empty. Verified
  manually / via the release process instead.

## [0.1.1] - 2026-08-15

### Added
- Go CLI (`cli/`) wrapping the report-rex Docker/build workflow: `report-rex image
  build`, `report-rex report build`, `report-rex report init`, `report-rex watch`.
  The binary embeds the Dockerfile, Python `core/`, `templates/`, and example
  scaffolds at build time and can build the Docker image itself if one isn't
  already present locally.
- Standalone binary distribution via GoReleaser (macOS arm64, Linux amd64/arm64,
  Windows amd64/arm64), published as GitLab Releases and to the Package
  Registry on version tags.
- Root `version.txt` as the Go CLI's version source of truth (independent of
  `core/pyproject.toml`'s Python package version).
- `.goreleaser.yml`, `lint-go`, `test-go`, and `publish-cli` CI jobs.
- `dev`-branch development workflow: CLI work happens on `dev`; releases go via
  an MR from `dev` to `main`, then a `vX.Y.Z` tag pushed from `main`.

### Fixed
- `publish-package`'s internal-network workarounds (private-IP `/etc/hosts`
  override, self-signed CA trust) were hardcoded for `gitlab.lan` and applied
  unconditionally on every GitLab instance this repo is pushed to. They now
  gate behind `GITLAB_HOST_IP`/`GITLAB_TRUST_INTERNAL_CA` project CI/CD
  variables, set only where actually needed.

## [0.1.0] - 2026-08-15

### Added
- Retroactive baseline entry for the `v0.1.0` tag already present in the
  repository: the `report-rex-core` Python package, the Docker build image, the
  Docusaurus documentation site, and the GitLab CI pipeline (lint/test/build-docs/
  publish).
