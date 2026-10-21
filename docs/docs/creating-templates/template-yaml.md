---
sidebar_position: 2
---

# report-rex.yaml

Every template must include a `report-rex.yaml` manifest:

```yaml
name: "my-template"           # Machine-readable name (matches directory name)
display_name: "My Template"   # Human-readable name
version: "1.0.0"              # Semantic version
type: "full-report"           # "full-report" or "communication"
description: "..."            # One-line description
engine: "xelatex"             # LaTeX engine: xelatex (recommended) or pdflatex
pandoc_version: ">=3.0"       # Minimum pandoc version required
```

## Theme Variables

`theme.yaml` provides default values for pandoc variables. Users can override any value in their `index.md` frontmatter:

```yaml
# theme.yaml — shipped with the template
colors:
  primary: "1A1A2E"
  accent:  "0F3460"
  critical: "B71C1C"
  high:     "E65100"
  medium:   "F57F17"
  low:      "1565C0"
  info:     "37474F"

# logo:
#   path: "assets/logo-placeholder.png"
#   width: "4cm"

# mainfont:    "TeX Gyre Heros"
# headingfont: "TeX Gyre Adventor"   # only used by templates that reference it, e.g. rit-cptc
# monofont:    "Source Code Pro"
```

And in `index.md` (overrides theme.yaml defaults):

```yaml
colors:
  primary: "2C3E50"   # override just the primary color
logo:
  path: "assets/client-logo.png"
  width: "3.5cm"
```

Not every template defines every key — `quick-comm`'s `theme.yaml` only sets
`colors.primary`/`colors.accent`, and only `rit-cptc`/`rit-cptc-comm` read
`headingfont`. Unused keys are harmless; a template's `template.tex` and
`theme.yaml` are the source of truth for which ones it honors. See
[Template Overview](./overview.md) for real examples from each bundled
template.

See also: [Overrides reference](../reference/overrides.md) for every override mechanism the tool has, in one place.
