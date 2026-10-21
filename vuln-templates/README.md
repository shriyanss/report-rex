# vuln-templates/

Default, copy-pasteable vulnerability finding write-ups for common finding
classes. These are **not** wired into Report Rex's build logic — they're
plain Markdown content you copy into an engagement's `reports/` directory,
same as any other finding file (see `reporting/CLAUDE.md`).

## Using a template

The quickest way is **Add Finding** in the VS Code extension's Report view,
or `rex report section vuln --template ssrf [--file ssrf-webhook]` (run
`--list-templates` to see the names). Either one writes the file into
`reports/` and adds it to `index.md` for you. Then do steps 3–5 below.

To do it by hand:

1. Copy the relevant file into your engagement's `reports/` directory and
   rename it to fit your numbering, e.g.:
   ```bash
   cp vuln-templates/ssrf.md reporting/client-acme-2024/reports/vuln-5.md
   ```
2. Add it to `index.md`'s `structure.reports` list.
3. Fill in every stubbed value and section, **and delete its `> [!REX-STUB]`
   marker line** as you go — CVSS vector, affected component/URLs, CWE/OWASP/
   MITRE references, timeline date, and the four body sections (Description,
   Impact, Replication, Remediation).
4. Add an annotated, light-mode screenshot to `## Replication` as evidence.
5. Run `report-rex validate --dir <your-report-dir>`.

## The `[!REX-STUB]` marker

Every stub value/section in these templates is preceded by a marker line:

- In frontmatter (YAML), as a comment: `# > [!REX-STUB]`
- In the body, as a blockquote: `> [!REX-STUB]`

`report-rex build` runs the QA-gate validator before rendering, and **any
remaining `[!REX-STUB]` marker is a hard build error** (`stub_marker_present`)
— this is intentional, so a template that got copied but never edited can't
silently end up in a client deliverable. The build won't succeed until every
marker (and the stub content beneath it) has been removed.

## Available templates

| File | Severity | Finding |
|------|----------|---------|
| `csrf.md` | Medium | Cross-Site Request Forgery |
| `ssrf.md` | High | Server-Side Request Forgery |
| `broken-access-control.md` | High | Broken Access Control (Vertical Privilege Escalation) |
| `open-redirect.md` | Low | Open Redirect |
| `default-credentials.md` | Critical | Default / Weak Credentials |
| `outdated-software.md` | High | Outdated Software / Known-Vulnerable Component |
| `insecure-file-upload.md` | Critical | Insecure / Unrestricted File Upload |
| `weak-tls-config.md` | Medium | Weak TLS/SSL Configuration |
| `information-disclosure.md` | Low | Verbose Error Messages / Information Disclosure |
| `missing-rate-limiting.md` | Medium | Missing Rate Limiting / Brute Force |
| `xxe.md` | High | XML External Entity (XXE) Injection |
| `ssti.md` | Critical | Server-Side Template Injection |

For fully worked, filled-in examples (not templates), see
`reporting/example/reports/vuln-{1..4}.md` — SQL Injection, Stored XSS, IDOR,
and Missing Security Headers.
