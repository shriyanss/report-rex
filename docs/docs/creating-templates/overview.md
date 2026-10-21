---
sidebar_position: 1
---

# Template Overview

A Report Rex template is a self-contained directory that controls everything about how the PDF looks: fonts, colors, cover page, vulnerability box styles, headers, footers. Most engagements don't need a new template — they need to **pick** one of the bundled templates and **tweak** a few colors or a logo. This page covers that end-user workflow first; template *authoring* (writing `template.tex`/Lua filters from scratch) is covered in the rest of this section.

## Template Structure

```
my-template/
├── report-rex.yaml     # Template manifest (required)
├── template.tex        # pandoc LaTeX template (required)
├── theme.yaml          # Default variable values (optional but recommended)
├── reference.docx      # Word styles for --docx output (optional)
├── filters/
│   ├── vuln_filter.lua      # Lua filter: fenced div → LaTeX (PDF)
│   └── docx_vuln_filter.lua # Lua filter: fenced div → OpenXML (DOCX, optional)
└── assets/
    ├── logo-placeholder.png
    └── fonts/           # Custom fonts (copied into Docker image)
```

## Selecting a Template

A template is selected one of three ways, in order of precedence:

1. **`--template` at build time** — overrides whatever `index.md` says:
   ```bash
   # Go CLI
   rex report build --dir my-engagement --template rit-cptc --output report.pdf

   # Docker/native build script
   ./build.sh --dir my-engagement --template rit-cptc --output report.pdf
   ```
   The value can be a bare name looked up under `templates/` (`rit-cptc`), or an absolute path to a template directory elsewhere on disk (`/opt/shared-templates/acme`). Bare relative paths and `.zip` files are **not** resolved automatically today — unzip a shared template into `templates/` first (see [Sharing Templates](./sharing.md)).
2. **`template:` in `index.md` frontmatter** — the normal case, checked in per-engagement:
   ```yaml
   template: "rit-cptc"
   ```
3. **Fallback** — if neither is set, `rit-cptc` is used.

## Bundled Templates

Report Rex ships with four ready-to-use templates in `templates/`:

| Template | Type | Best for |
|----------|------|----------|
| `rex-default` | `full-report` | Generic pentest reports — navy cover, TOC, severity boxes, branded header/footer |
| `rit-cptc` | `full-report` | **Default for new reports.** Bold client-facing reports — indigo brand-band cover, numbered sections, magenta confidentiality banners, a findings summary table, and tcolorbox finding panels (finding ID, CVSS, Impact / Sophistication, MITRE ATT&CK, Material Impact) |
| `rit-cptc-comm` | `communication` | **Default for new quick-comms.** `rit-cptc`-branded one-pagers — same palette as `rit-cptc` in a lightweight masthead layout, no cover/TOC |
| `quick-comm` | `communication` | Mid-engagement one-pagers — lightweight neutral layout, no cover page or TOC |

The `rit-cptc`/`rit-cptc-comm` templates surface a few extra optional finding
fields — `impact`, `sophistication`, `material_impact`, and `mitigations`. They
are optional everywhere and ignored by templates that don't use them; see
[Vulnerability Files](../reporting-structure/vulnerability-files.md).

Each template's `report-rex.yaml` and `theme.yaml` are the source of truth for its metadata and default colors — read the sections below for how to override them, or open the files directly under `templates/<name>/`.

### `rex-default`

> Professional penetration test report with cover page, TOC, and colored severity boxes.

Ships with a navy/blue palette (`templates/rex-default/theme.yaml`):

```yaml
colors:
  primary:  "1A1A2E"   # dark navy
  accent:   "0F3460"   # medium blue
  critical: "B71C1C"
  high:     "E65100"
  medium:   "F57F17"
  low:      "1565C0"
  info:     "37474F"
```

To rebrand it for a client without touching the shared template, override just what you need in that report's `index.md`:

```yaml
template: "rex-default"
colors:
  primary: "2C3E50"
company_logo: "assets/company-logo-dark.png"
client_logo: "assets/client-logo.png"
```

### `rit-cptc`

> Bold pentest report: indigo brand-band cover (Prepared by your logo, build date), numbered sections, magenta confidentiality banners, corner accents, a findings summary table, and tcolorbox finding panels (finding ID, CVSS, Impact/Sophistication, MITRE ATT&CK, Material Impact, Component with its affected assets).

Its palette (`templates/rit-cptc/theme.yaml`) drives the cover, headings, and confidentiality banners:

```yaml
colors:
  primary:  "26307C"   # indigo
  accent:   "C71C64"   # magenta
```

The cover always shows the build date (`\today`), not the `date` field. Example override — a different brand color, and your firm's logos on the cover:

```yaml
template: "rit-cptc"
colors:
  primary: "0E4C92"
company_logo: "assets/company-logo-dark.png"        # "Prepared by" block (light background)
company_logo_light: "assets/company-logo.png"       # cover band (dark background)
```

### `rit-cptc-comm`

> Lightweight RIT CPTC-branded one/two-pager for mid-engagement progress updates and incident notifications: royal-blue/mint masthead, magenta confidentiality banner, and compact severity panels.

Shares `rit-cptc`'s palette so a communication and the final report look like one family. Use it for anything that needs the same branding as `rit-cptc` but doesn't need a cover page or TOC:

```yaml
template: "rit-cptc-comm"
title: "Day 3 Progress Update"
colors:
  accent: "A6185C"   # slightly darker magenta for this client
```

### `quick-comm`

> Lightweight one/two-pager for mid-engagement progress updates and incident notifications.

Neutral palette (`templates/quick-comm/theme.yaml`), only two colors by default:

```yaml
colors:
  primary: "1A1A2E"
  accent:  "0F3460"
```

Example override for a firm with its own brand colors:

```yaml
template: "quick-comm"
colors:
  primary: "0D3B24"
  accent:  "146C43"
company_logo: "assets/company-logo-dark.png"
```

## Customizing Without Forking

Every color, font, and logo in `theme.yaml` is just a pandoc metadata default — anything set there can be overridden per-report from `index.md` frontmatter, with no copy of the template directory required. `theme.yaml` is passed to pandoc first; `index.md`'s frontmatter is passed second, so its values win.

For example, starting from `rit-cptc`'s defaults:

```diff
 # templates/rit-cptc/theme.yaml (unmodified, shared by every report)
 colors:
   primary:  "26307C"
   accent:   "C71C64"
```

```diff
 # my-engagement/index.md (one report's frontmatter)
 template: "rit-cptc"
+colors:
+  primary: "0F2A5C"   # this client's brand navy instead of the default indigo
+logo:
+  path: "assets/client-logo.png"
+  width: "3.5cm"
```

This covers most client-specific branding needs — colors, fonts (`mainfont`/`monofont`), and logos. See [report-rex.yaml](./template-yaml.md) for the full list of overridable theme keys and [Extending & Branding](./extending.md) for the firm/client logo fields and per-report `header-includes`.

See also: [Overrides reference](../reference/overrides.md) for every override mechanism the tool has, in one place.

## Sharing a Template

Built a template your whole team should use? Package it as a zip and hand it out — see [Sharing Templates](./sharing.md) for the full packaging and installation walkthrough.
