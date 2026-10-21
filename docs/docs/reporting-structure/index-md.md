---
sidebar_position: 1
---

# index.md Reference

`index.md` is the entry point for a report directory. Its YAML frontmatter defines all report metadata and the ordered document structure. The markdown body (after the frontmatter) is ignored — all report content comes from the files listed under `structure`.

`title`, `client` and `date` are required — if any is missing, `report-rex validate` reports a `metadata_missing` error, which fails the build and shows `index.md` in red in VS Code.

## Full Schema

```yaml
---
# ── Identity ─────────────────────────────────────────────────────────────
title: "Web Application Penetration Test"   # required
subtitle: "ACME Customer Portal — Final Report"  # optional
client: "ACME Corporation"                  # required
company: "Report Rex"                       # optional — your pentest firm's name
engagement_type: "Web Application Penetration Test"  # optional
report_id: "RR-2024-0142"                   # optional — engagement/report identifier
date: "2024-01-15"                          # required (ISO 8601)
start_date: "2024-01-10"                    # optional — testing window start
end_date: "2024-01-14"                      # optional — testing window end
version: "1.0"                              # optional (default: "1.0")
confidentiality: "Confidential"             # optional (default: "Confidential")
template: "rit-cptc"                        # optional (default: "rit-cptc")

# ── Contact & distribution ────────────────────────────────────────────────
contact_name: "Jane Doe"                    # optional — point of contact
contact_email: "jane.doe@reportrex.example" # optional
retest_date: "2024-02-15"                   # optional — planned retest date
distribution:                               # optional — list of recipients
  - "ACME Security Team"
  - "ACME CISO Office"
  - "Report Rex Engagement Lead"

# ── Authors ───────────────────────────────────────────────────────────────
authors:
  - name: "Jane Doe"
    role: "Lead Penetration Tester"
  - name: "John Smith"
    role: "Penetration Tester"

# ── Branding ──────────────────────────────────────────────────────────────
company_logo: "assets/company-logo-dark.png"   # dark logo, for the light page header
company_logo_light: "assets/company-logo.png"  # light logo, for the dark cover band
client_logo: "assets/acme-logo.png"            # client logo, shown on the cover

# ── Color overrides (override theme.yaml) ─────────────────────────────────
colors:
  primary:  "1A1A2E"    # hex without #
  accent:   "0F3460"
  critical: "B71C1C"
  high:     "E65100"
  medium:   "F9A825"
  low:      "1565C0"
  info:     "546E7A"

# ── Timeline (optional) ────────────────────────────────────────────────────
timeline:
  - date: "2024-01-10T09:00:00+00:00"     # ISO 8601
    description: "Kicked off testing"
    include: true                          # optional, defaults to true

# ── Structure (defines document order) ────────────────────────────────────
structure:
  sections:               # Front-matter sections, rendered in order
    - sections/exec_summ.md
    - sections/methodology.md
  reports:                # Vulnerability findings (auto-sorted by severity)
    - reports/vuln-1.md
    - reports/vuln-2.md
  appendices:             # Appendices, rendered in order
    - appendices/1.md
---
```

All paths in `structure` are relative to the directory containing `index.md`. `report-rex validate`/`build` resolve every path immediately and fail with a clear `FileNotFoundError` if any referenced file is missing — there's no silent skipping of a mistyped filename.

## Where the fields appear

| Field | Cover | Document Control | Header / Footer |
|-------|:-----:|:----------------:|:---------------:|
| `title`, `subtitle` | ✓ | | header (right) |
| `client` | ✓ | ✓ | footer (right) |
| `company` | ✓ | ✓ | footer (left) |
| `engagement_type` | ✓ | ✓ | |
| `report_id` | ✓ | ✓ | |
| `date`, `version`, `confidentiality` | ✓ | ✓ | header (right) |
| `start_date`, `end_date` | | ✓ (testing window) | |
| `contact_name`, `contact_email` | | ✓ (point of contact) | |
| `retest_date`, `distribution` | | ✓ | |
| `authors` | | ✓ (assessment team) | |

## Branding

Three logo fields drive the visual branding:

- **`company_logo`** — your firm's logo in a *dark* variant, placed in the light page header (top-left of every page).
- **`company_logo_light`** — your firm's logo in a *light* variant, placed on the dark cover band.
- **`client_logo`** — the client's logo, shown under "Prepared for" on the cover. Leave it out to omit the logo. `validate` flags the example's `acme-logo.png` and `RR-2024-0142` report ID as leftover placeholders, and warns when only one of `start_date`/`end_date` is set, since the Testing Window then shows a single date.

Logo paths are relative to the report directory; the preprocessor resolves them to absolute paths so XeLaTeX can find them regardless of pandoc's working directory. If a logo field is omitted, the template falls back to the firm name (`company`) rendered as text.

### `logo.path` fallback

`theme.yaml` for a template can define a default logo used for the page header when `company_logo` isn't set:

```yaml
# theme.yaml
logo:
  path: "assets/logo.png"
```

This is a template-level default (handy for a firm that always uses the same header logo across every engagement); an `index.md`'s own `company_logo` always takes priority when both are set.

Report Rex is firm-agnostic. To rebrand for a different company, set `company`, the logo fields, and override `colors` (see below). See [Extending & Branding](../creating-templates/extending.md) for how a single report can also inject its own LaTeX.

## Timeline

`timeline` is a list of `{date, description, include}` entries. It's optional and can be set in `index.md`'s own frontmatter, **and/or** in the frontmatter of any file listed under `structure.sections`, `structure.reports`, or `structure.appendices` — entries from every file are merged into a single list. This lets a tester log an event right in the finding or section file they're working on, instead of editing `index.md` centrally.

All merged entries are sorted chronologically by `date` (an ISO 8601 timestamp) and rendered as a "Timeline" table in the Appendices section. `include` defaults to `true`; set `include: false` to keep an entry out of the rendered report while still keeping it in the source file for the tester's own record.

## Color overrides

Any of the seven palette colors can be overridden in `index.md` (or set as defaults in the template's `theme.yaml`). Values are hex strings **without** a leading `#`. Keys: `primary`, `accent`, `critical`, `high`, `medium`, `low`, `info`. The severity colors are also used for the auto-generated [charts](./charts.md) and for the color-graded matrix chart cells.

See also: [Overrides reference](../reference/overrides.md) for every override mechanism the tool has, in one place.
