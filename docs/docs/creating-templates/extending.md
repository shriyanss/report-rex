---
sidebar_position: 7
title: Extending & Branding
---

# Extending & Branding

Report Rex is designed to be reusable across firms and adaptable per engagement without forking the shared template. Two mechanisms make this possible: per-report `header-includes`, and the branding/color fields.

See also: [Overrides reference](../reference/overrides.md) for every override mechanism the tool has, in one place.

## Per-Report LaTeX (`header-includes`)

A single report can inject extra LaTeX — packages or macros — into the preamble without editing the shared `template.tex`. Add a `header-includes:` list to your `index.md` frontmatter; pandoc passes each entry into the document preamble.

```yaml
---
title: "Web Application Penetration Test"
client: "ACME Corporation"
# ...
header-includes:
  - \usepackage{pdfpages}
  - \newcommand{\highlight}[1]{\colorbox{yellow!40}{#1}}
  - |
    \usepackage{soul}
    \sethlcolor{yellow}
---
```

Each list entry is emitted verbatim into the preamble (the default template loops over `header-includes` near the end of its preamble). This lets one engagement pull in a package or define a macro it needs, while every other report using the same template is unaffected — effectively customizing that one report's template inline.

Use it for one-off needs (an extra package, a client-specific macro, an included PDF appendix). For changes you want across all reports, edit the template instead — see [LaTeX Template](./latex-template.md).

## Multi-Company Branding

The tool ships firm-neutral. Any company can rebrand a report or an entire template:

1. **Firm & client names** — set `company` (your firm) and `client` in `index.md`. The firm appears on the footer-left, header, and cover; the client on the footer-right and cover.
2. **Logos** — set the three logo fields:
   - `company_logo` — dark firm logo for the light page header,
   - `company_logo_light` — light firm logo for the dark cover band,
   - `client_logo` — client logo on the cover.
3. **Colors** — override the palette via `colors` in `index.md` for one report, or set defaults in the template's `theme.yaml` for every report built with that template. Keys are hex values without `#`:

```yaml
colors:
  primary:  "1A1A2E"
  accent:   "0F3460"
  critical: "B71C1C"
  high:     "E65100"
  medium:   "F9A825"
  low:      "1565C0"
  info:     "546E7A"
```

`primary` and `accent` drive the cover bands, headings, and links; the five severity colors drive the finding panels and severity [charts](../reporting-structure/charts.md).

See [index.md Reference](../reporting-structure/index-md.md#branding) for where each field renders, and [Template YAML](./template-yaml.md) for `theme.yaml` defaults.
