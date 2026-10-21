---
sidebar_position: 4
---

# Lua Filters

Lua filters intercept pandoc's abstract syntax tree (AST) and transform nodes before rendering. Report Rex uses a filter to convert vulnerability fenced divs into styled LaTeX boxes.

## How Vulnerability Divs Work

The preprocessor turns each `reports/vuln-N.md` file's YAML frontmatter into a
fenced div like:

```markdown
::: {.vulnerability severity="Critical" cvss="9.8" status="Open" cwe="CWE-89" affected="https://example.com/login"}
:::
```

The default template's filter (`filters/vuln_filter.lua`) converts this into raw LaTeX:

```latex
\begin{vulnbox}{colorCritical}
\sevchip{colorCritical}{Critical}\hspace{10pt}{\large\bfseries CVSS 9.8}\hfill\metalabel{Status}~\textbf{Open}
\vspace{5pt}
\begin{tblr}{colspec={Q[l,wd=2.9cm,font=\bfseries\footnotesize\color{colorMute}] X[l]}, rowsep=1.8pt, width=\linewidth}
CWE & CWE-89 \\
Affected & \texttt{\small https://example.com/login} \\
\end{tblr}
\end{vulnbox}
```

Only attributes present on the div produce a row — the filter skips anything
empty. The `vulnbox` environment and the `\sevchip`/`\metalabel` macros must
be defined in your `template.tex`; see [LaTeX Template](./latex-template.md#vulnerability-severity-boxes).

## Writing a Custom Filter

```lua
-- filters/my_filter.lua

function Div(el)
  if not el.classes:includes("my-class") then return nil end

  local title = el.attributes.title or "No title"
  local latex = string.format("\\mybox{%s}", title)
  return pandoc.RawBlock("latex", latex)
end
```

## Filter Execution

The renderer auto-discovers two filters by file name, each passed to pandoc
as `--lua-filter` only if it exists in the template directory:

- `filters/vuln_filter.lua` — the PDF build (emits raw LaTeX)
- `filters/docx_vuln_filter.lua` — the `--docx` build (emits raw OpenXML; see [Output Formats](../reference/output-formats.md))

There's no mechanism today for a template to register additional filters —
if you need more, add the extra logic to the existing file (pandoc runs all
top-level functions like `Div`, `Table`, `Code`, etc. in a single filter
file), or fork the renderer's pandoc invocation in
`core/src/report_rex_core/renderer.py`. See [Bundled Lua Filters](./bundled-filters.md)
for what each shipped template's filters do.

## Useful pandoc Lua APIs

- `pandoc.RawBlock("latex", str)` — emit raw LaTeX
- `el.classes:includes("name")` — check if div has a class
- `el.attributes.key` — read a fenced div attribute
- `pandoc.utils.stringify(inline)` — extract plain text from inline elements

Full reference: [pandoc Lua filter documentation](https://pandoc.org/lua-filters.html)
