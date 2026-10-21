---
sidebar_position: 8
---

# Overrides

Report Rex has several override mechanisms scattered across templates, report
content, and validation. This page indexes all of them in one place — each
section links to the full writeup.

## Template selection (`--template`)

The `--template` build flag overrides whatever `index.md`'s `template:` field
says. If neither is set, the build fails — there's no implicit default.

```bash
rex report build --dir my-engagement --template rit-cptc --output report.pdf
```

See [Selecting a Template](../creating-templates/overview.md#selecting-a-template),
[CLI Reference](./cli.md), [Go CLI Reference](./go-cli.md), and
[build.sh Reference](./build-sh.md).

## Theme variables (`index.md` overrides `theme.yaml`)

Any pandoc variable a template's `theme.yaml` defines as a default —
`colors`, `logo.path`/`width`, `mainfont`/`headingfont`/`monofont` — can be
overridden per-report in `index.md` frontmatter, no template fork required.
`theme.yaml` is passed to pandoc first; `index.md` second, so its values win.

```yaml
# my-engagement/index.md
template: "rit-cptc"
colors:
  primary: "0F2A5C"   # this client's brand navy
logo:
  path: "assets/client-logo.png"
  width: "3.5cm"
```

See [Template YAML](../creating-templates/template-yaml.md#theme-variables),
[Customizing Without Forking](../creating-templates/overview.md#customizing-without-forking),
and [Color overrides](../reporting-structure/index-md.md#color-overrides).

## Logo fallback

`theme.yaml`'s `logo.path` is a template-level default for the page header.
`index.md`'s own `company_logo` always takes priority when both are set.

See [`logo.path` fallback](../reporting-structure/index-md.md#logopath-fallback).

## Per-report LaTeX (`header-includes`)

A single report can inject extra LaTeX — packages or macros — into its
preamble via `index.md`'s `header-includes:` list, customizing that one
report inline without editing the shared template.

```yaml
header-includes:
  - \usepackage{pdfpages}
```

See [Per-Report LaTeX](../creating-templates/extending.md#per-report-latex-header-includes).

## `> [!OVERRIDE-DARK-IMG]` — skip the dark-mode screenshot check

Placed directly above a screenshot reference, this skips only the
`screenshot_dark_mode` validation check for that one image (annotation,
caption, and legibility checks still run). Stripped before pandoc renders.

```markdown
> [!OVERRIDE-DARK-IMG]
![Reverse shell obtained on the target host](../assets/screenshots/rev-shell.png)
```

See [`> [!OVERRIDE-DARK-IMG]`](../reporting-structure/markdown-features.md#-override-dark-img--skip-the-dark-mode-screenshot-check)
and [Overriding the dark-mode check](./cli.md#overriding-the-dark-mode-check).

## Editor lint detection override

Separate from report content: the VS Code extension's `report-rex.projectOverride`
setting and the Neovim plugin's `project_override` config force lint
on/off regardless of the auto-detected "is this a rex report project" check.

See [VS Code Extension](../getting-started/ide/vscode.md) and
[Neovim Plugin](../getting-started/ide/neovim.md).
