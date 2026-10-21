# templates/ — Claude Guidance

## What This Is

Self-contained LaTeX template directories. Each directory is independently shareable as a zip file. Users drop them into their `templates/` directory and reference by name.

## Template Structure

```
<name>/
├── report-rex.yaml        Template manifest (required)
├── template.tex           pandoc LaTeX template — uses $variable$ syntax (required)
├── theme.yaml             Default pandoc variable values (recommended)
├── reference.docx         --docx style sheet (generated: core/scripts/make_reference_docx.py)
├── filters/
│   ├── vuln_filter.lua    Lua filter: .vulnerability divs → tcolorbox LaTeX
│   └── docx_vuln_filter.lua  --docx counterpart: raw OpenXML cover + finding panels
└── assets/
    ├── logo-placeholder.png
    └── fonts/             .ttf/.otf files — copied into Docker at build time
```

## Templates Provided

- **`default/`** — Full pentest report: cover page, TOC, section headings, severity boxes, branded header/footer
- **`rit-cptc/`** — Bold client-facing report (default): indigo brand-band cover (rex-default layout), numbered sections, magenta confidentiality banners + corner accents, findings summary table, tcolorbox finding panels (finding ID, CVSS, Impact/Sophistication, MITRE ATT&CK, Material Impact, Component → Affected). Being merged with `rex-default`.
- **`rit-cptc-comm/`** — `rit-cptc`-branded communication one-pager: royal-blue/mint masthead, magenta banner, compact severity panel; `type: communication`
- **`quick-comm/`** — Lightweight neutral one-pager: inline header block, no TOC, no separate cover page

## pandoc Template Syntax

`template.tex` uses pandoc's template language, NOT Jinja2:

- Variables: `$title$`, `$client$`, `$date$`
- Conditionals: `$if(logo.path)$...$endif$`
- Loops: `$for(authors)$...$endfor$`
- Nested: `$authors.name$`
- Body: `$body$` — where the processed markdown content is inserted

**Never mix pandoc `$...$` syntax with Jinja2 `{{ }}` in the same file.**

## Lua Filter

`filters/vuln_filter.lua` intercepts pandoc Div nodes with class `.vulnerability` and returns a `RawBlock("latex", ...)` that creates a tcolorbox severity box.

Attributes available: `severity`, `cvss`, `cvssversion`, `vector`, `status`, `affected`, `cwe`, `owasp`, `mitre`, `mitigations`, `component`, `business`, `finding`, `impact`, `sophistication`, `material`

The last four (`finding`, `impact`, `sophistication`, `material`) are optional and emitted from the finding's `finding_number` (auto-assigned), `impact`, `sophistication`, and `material_impact` frontmatter. The `rex-default` filter ignores them; the `rit-cptc` filter renders them in its finding panel. `title` is **not** passed to the div — it is carried by the finding's section heading.

The filter references color aliases (`vulncritical`, `vulnhigh`, etc.) that must be defined in `template.tex` via `\colorlet{vulncritical}{colorCritical}`.

## Adding a New Template

1. Copy `templates/rex-default/` to `templates/<name>/`
2. Edit `report-rex.yaml` (name, display_name, description)
3. Edit `template.tex` for layout changes
4. Edit `theme.yaml` for default colors/fonts
5. Test: `./build.sh --dir reporting/example --template <name> --output test.pdf`

## LaTeX Packages in Use (rex-default template)

| Package | Purpose |
|---------|---------|
| `tcolorbox` | Severity finding boxes |
| `fontawesome5` | Icons |
| `titlesec` | Section heading styles |
| `fancyhdr` | Page headers/footers |
| `geometry` | Page margins |
| `xcolor` | Color definitions |
| `listings` | Code block formatting |
| `draftwatermark` | DRAFT watermark (default; suppressed by --final) |
| `unicode-math` | Unicode math support |
| `fontspec` | Custom fonts via XeLaTeX |

## Fonts

Place `.ttf`/`.otf` files in `assets/fonts/`. The Dockerfile copies them to `/usr/local/share/fonts/` and runs `fc-cache`. Reference by family name in `theme.yaml`:

```yaml
mainfont: "My Font Family"
monofont: "My Mono Font"
```

Without custom fonts, the template falls back to `TeX Gyre Heros`.
