---
sidebar_position: 3
---

# LaTeX Template

`template.tex` is a [pandoc LaTeX template](https://pandoc.org/MANUAL.html#templates). It uses `$variable$` syntax for substitution and `$if(variable)$...$endif$` for conditionals.

## Required Variables

These are populated from your `index.md` frontmatter and theme.yaml:

| Variable | Source | Description |
|----------|--------|-------------|
| `$title$` | index.md | Report title |
| `$client$` | index.md | Client name |
| `$date$` | index.md | Report date |
| `$version$` | index.md | Version string |
| `$confidentiality$` | index.md | e.g. "Confidential" |
| `$for(authors)$` | index.md | Authors list loop |
| `$body$` | pandoc | Rendered markdown content |
| `$colors.primary$` | theme.yaml / index.md | Hex color (no `#`) |
| `$if(draft)$` | build flag | True by default; false when `--final` is set |
| `$if(logo.path)$` | index.md | True when a logo is configured |

## Minimal Template

```latex
\documentclass[11pt, a4paper]{article}
\usepackage{xcolor}
\usepackage{geometry}
\geometry{margin=2.5cm}

$if(colors.primary)$
\definecolor{colorPrimary}{HTML}{$colors.primary$}
$else$
\definecolor{colorPrimary}{HTML}{1A1A2E}
$endif$

$if(draft)$
\usepackage{draftwatermark}
\SetWatermarkText{DRAFT}
$endif$

\providecommand{\tightlist}{\setlength{\itemsep}{2pt}\setlength{\parskip}{0pt}}

\begin{document}

\begin{center}
  {\Huge\bfseries\color{colorPrimary} $title$}\\[0.5cm]
  {\large $client$} \quad $date$
\end{center}

$body$

\end{document}
```

## Vulnerability Severity Boxes

The default template's Lua filter (`filters/vuln_filter.lua`) emits
`\begin{vulnbox}{<color>} ... \end{vulnbox}`, calling two macros for the chip
row inside it. Your `template.tex` must define the `vulnbox` environment and
both macros, or the filter's output will fail to compile:

```latex
\usepackage[most]{tcolorbox}
\usepackage{tabularray}

\definecolor{colorCritical}{HTML}{B71C1C}
\definecolor{colorHigh}{HTML}{E65100}
\definecolor{colorMedium}{HTML}{F9A825}
\definecolor{colorLow}{HTML}{1565C0}
\definecolor{colorInfo}{HTML}{546E7A}
\definecolor{colorMute}{HTML}{6B7280}

% The panel the filter wraps every finding in — \begin{vulnbox}{colorCritical}...\end{vulnbox}
\newtcolorbox{vulnbox}[1]{
  enhanced, breakable, colframe=#1, colback=#1!5!white,
  arc=2pt, boxrule=1pt, leftrule=4pt,
  left=10pt, right=10pt, top=8pt, bottom=8pt,
  before skip=4pt, after skip=12pt,
}

% Filled colored badge with white text — the filter calls \sevchip{colorCritical}{Critical}
\newcommand{\sevchip}[2]{{\colorbox{#1}{\textcolor{white}{\bfseries\strut\,#2\,}}}}

% Small uppercase metadata label — the filter calls \metalabel{Status}
\newcommand{\metalabel}[1]{{\footnotesize\bfseries\color{colorMute}\MakeUppercase{#1}}}
```

Detail rows (CVSS vector, CWE, OWASP, MITRE, affected URLs, …) are rendered by
the filter as a `tabularray` `tblr` table inside the box, so `tabularray` must
be loaded too. See [Lua Filters](./lua-filters.md) for the exact LaTeX the
filter produces from a `.vulnerability` div.
