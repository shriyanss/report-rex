---
sidebar_position: 5
title: Markdown Features
---

# Markdown Features

Beyond standard Markdown, Report Rex applies a few transformations while assembling your report. This page covers cross-references between findings, link handling, image sizing, syntax highlighting, and the two special directives that gate `report-rex validate`/`build`.

## Cross-References Between Findings

To link from one file to a finding, reference the finding's `.md` file. Both the image and link forms work:

```markdown
See ![](./vuln-2.md) for details.
See [the XSS finding](vuln-2.md) for details.
```

Each is replaced with a hyperlink reading **"Finding #N — &lt;Title&gt;"**, where `N` is that finding's number in the [sorted](./vulnerability-files.md#automatic-sorting--numbering) findings list. Resolution is by filename, so the exact link text you write (or leave empty) does not matter — the generated text always reflects the finding's current sorted number and title.

This means you can reorder or reprioritize findings without breaking or hand-updating your cross-references. A reference to a filename that isn't in `structure.reports` is left untouched and would render as a broken link, so `validate` reports it as a `dangling_reference` error, with its line number.

## Finding Statistics in Text

Summary prose such as the executive summary can quote numbers computed from the findings, so it never contradicts them:

| Token | Replaced with |
|-------|---------------|
| `:::findings:total:::` | Number of findings |
| `:::findings:critical:::`, `:::findings:high:::`, `:::findings:medium:::`, `:::findings:low:::`, `:::findings:info:::` | Number of findings at that severity |
| `:::findings:highest:::` | The highest severity present (`Critical` … `Info`), or `None` with no findings. Use it as the overall risk rating. |

```markdown
The assessment identified **:::findings:total::: vulnerabilities**; overall risk: **:::findings:highest:::**.
```

## Link Handling

By default, a normal external link becomes a **footnote**: the link text stays inline and the URL is moved to a footnote at the bottom of the page.

```markdown
See the [OWASP SQL Injection Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/...).
```

renders the text "OWASP SQL Injection Prevention Cheat Sheet" inline, with the URL as a footnote — clean for print.

To keep an external link as an **inline clickable link** instead, prefix it with `!!`:

```markdown
See the !![OWASP XSS Prevention Cheat Sheet](https://cheatsheetseries.owasp.org/...).
```

Internal links and finding cross-references always stay clickable.

| Syntax | Result |
|--------|--------|
| `[text](https://…)` | Text inline, URL as a footnote |
| `!![text](https://…)` | Inline clickable link |
| `[text](vuln-N.md)` / `![](vuln-N.md)` | "Finding #N — Title" hyperlink |

## Image Sizing

A standard pandoc image attribute block after the image sets its rendered width:

```markdown
![Authentication bypass via SQL injection](../assets/screenshots/sqli-auth-bypass.png){width=90%}
```

`width` accepts a percentage (relative to the text width) or an absolute LaTeX length (e.g. `{width=10cm}`). Omit the attribute block to let the template pick a default size. An attribute block split off from its image by a space or a line break is re-attached, and an image on its own line always renders as a captioned figure, even with text directly above or below it. A missing image file is reported as a `[WARNING] Could not fetch resource` line in the build output.

Consecutive image lines don't need a blank line between them — the preprocessor puts each image in its own paragraph so they never render squashed together.

## Syntax Highlighting

Fenced code blocks with a language are syntax-highlighted in the PDF:

````markdown
```python
query = "SELECT * FROM users WHERE username = ?"
cursor.execute(query, (username,))
```

```http
POST /api/login HTTP/1.1
Host: portal.acme.com
```
````

Languages such as `python`, `http`, `bash`, `nginx`, `json`, and many others are supported. Inline `` `code` `` is rendered in a monospaced font.

Only have a screenshot of the request or code? `rex ocr` turns it into a code
block you can paste:

```bash
rex ocr ../assets/screenshots/login-request.png --lang http
```

Proofread the result, since OCR can confuse `l`, `1` and `I`. See
[Go CLI → `rex ocr`](../reference/go-cli.md#rex-ocr).

## `> [!OVERRIDE-DARK-IMG]` — Skip the Dark-Mode Screenshot Check

`report-rex validate` (and therefore `build`, which validates first) flags any screenshot whose border pixels look like a dark-themed UI, since dark screenshots tend to print poorly. If a dark screenshot is intentional — say, a terminal window or a dark-themed admin panel that's genuinely part of the evidence — place this directive on its own line directly above the image reference (blank lines in between are fine):

```markdown
> [!OVERRIDE-DARK-IMG]
![Reverse shell obtained on the target host](../assets/screenshots/rev-shell.png){width=90%}
```

This skips only the `screenshot_dark_mode` check for that one image — the caption-length, missing-file, annotation, and legibility checks still run. The directive line is stripped by the preprocessor before pandoc sees it, so it never appears as a stray blockquote in the rendered PDF.

See also: [Overrides reference](../reference/overrides.md) for every override mechanism the tool has, in one place.

## `> [!REX-STUB]` — Mark an Unfilled Scaffold Value

`report-rex new` scaffolds starter files with placeholder text (e.g. `"Describe the vulnerability here."`). Some template authors additionally ship `vuln-templates/` snippets — pre-built finding bodies for common vulnerability classes — with a stub value or section left for the tester to fill in, marked with:

```markdown
> [!REX-STUB]
```

placed on its own line right after the value/section that still needs real content (or as a YAML comment, `# > [!REX-STUB]`, inside frontmatter). Any occurrence of this marker anywhere in a section, finding, or appendix file is an **unconditional validation error** (`stub_marker_present`) — since `build` runs validation first, a copied-but-unedited template snippet blocks `report-rex build` until you replace the stub content and delete the marker line. Use `--skip-validation` only if you deliberately want to build with the stub still present (e.g. to preview layout).

```yaml
---
title: "SQL Injection"
component: "TODO: fill in the affected component"
# > [!REX-STUB]
---
```

## See Also

- [Vulnerability Files](./vulnerability-files.md) for the required `## Replication` length check and CVSS requirements the validator also enforces.
- [Charts](./charts.md) for the `:::findings_graph:...:::` directive, which is applied in the same assembly pass as the transformations on this page.
