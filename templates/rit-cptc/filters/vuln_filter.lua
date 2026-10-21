-- vuln_filter.lua (rit-cptc template)
--   * Converts ::: {.vulnerability ...} divs into a severity-colored tcolorbox
--     panel (rex-default style): severity chip, finding ID and CVSS on top,
--     then a label/value grid. Affected assets sit under Component.
--   * Inserts a findings summary table before the first finding.
--   * Restyles all pandoc tables as full-width tabularray tables with an
--     indigo header row, zebra striping and visible grid lines.

local severity_color = {
  Critical = "colorCritical",
  High     = "colorHigh",
  Medium   = "colorMedium",
  Low      = "colorLow",
  Info     = "colorInfo",
  None     = "colorInfo",
}

local function esc(s)
  if s == nil then return "" end
  return (s:gsub("([&%%$#_{}~^\\])", "\\%1"))
end

local function has(v) return v ~= nil and v ~= "" end

-- ── Vulnerability panel ─────────────────────────────────────────────────────
local function Div(el)
  if not el.classes:includes("vulnerability") then return nil end
  local a = el.attributes
  local sev = a.severity or "Info"
  local color = severity_color[sev] or "colorInfo"

  -- Top line: severity chip + finding ID + CVSS score
  local top = string.format("\\sevchip{%s}{%s}", color, esc(sev))
  if has(a.finding) then
    top = top .. string.format("\\hspace{8pt}{\\large\\bfseries\\color{%s}%s}", color, esc(a.finding))
  end
  if has(a.cvss) then
    top = top .. string.format("\\hspace{10pt}{\\large\\bfseries CVSS %s}", esc(a.cvss))
    if has(a.cvssversion) then
      top = top .. string.format("{\\small\\color{colorMute}~v%s}", esc(a.cvssversion))
    end
  end

  -- Detail rows (only those present)
  local rows = {}
  local function add(k, v)
    if has(v) then rows[#rows + 1] = string.format("%s & %s \\\\", k, v) end
  end
  -- URLs/vectors have no spaces to wrap at; allow breaks after / . : , (#64).
  local function mono(v)
    return has(v) and ("\\texttt{\\fontsize{9.8pt}{12pt}\\selectfont " .. esc(v):gsub("([/.:,])", "%1\\allowbreak{}") .. "}") or nil
  end
  local function plain(v) return has(v) and esc(v) or nil end

  add("CVSS Vector", mono(a.vector))
  local eval = {}
  if has(a.impact) then eval[#eval + 1] = "\\textbf{Impact:} " .. esc(a.impact) end
  if has(a.sophistication) then eval[#eval + 1] = "\\textbf{Sophistication:} " .. esc(a.sophistication) end
  add("Evaluation", #eval > 0 and table.concat(eval, "\\quad ") or nil)
  add("CWE", plain(a.cwe))
  add("OWASP", plain(a.owasp))
  add("MITRE ATT\\&CK", plain(a.mitre))
  add("Mitigations", plain(a.mitigations))
  add("Business Risk", plain(a.business))
  add("Material Impact", plain(a.material))
  add("Component", plain(a.component))
  -- Affected assets read as a subset of the component above them.
  add(has(a.component) and "\\hspace{0.8em}\\textrightarrow~Affected" or "Affected", mono(a.affected))

  local detail = ""
  if #rows > 0 then
    detail = "\\par\\vspace{5pt}\n\\noindent" ..
      "\\begin{tblr}{colspec={Q[l,wd=3.1cm,font=\\bfseries\\footnotesize\\color{colorMute}] X[l]}, rowsep=1.8pt, width=\\linewidth}\n" ..
      table.concat(rows, "\n") .. "\n\\end{tblr}"
  end

  return pandoc.RawBlock("latex", string.format(
    "\\begin{vulnbox}{%s}\n%s%s\n\\end{vulnbox}", color, top, detail))
end

-- ── Findings summary table ──────────────────────────────────────────────────
-- Runs before Div (see the filter list at the bottom) so the .vulnerability
-- attributes are still readable. Each finding is a `## Title {#finding-N}`
-- header followed by its vulnerability div.
local function Pandoc(doc)
  local blocks = doc.blocks
  local first, rows = nil, {}
  for i, b in ipairs(blocks) do
    local nxt = blocks[i + 1]
    if b.t == "Header" and b.identifier:match("^finding%-%d+$")
        and nxt and nxt.t == "Div" and nxt.classes:includes("vulnerability") then
      first = first or i
      local a = nxt.attributes
      local sev = a.severity or "Info"
      local title = pandoc.write(pandoc.Pandoc({ pandoc.Plain(b.content) }), "latex"):gsub("%s+$", "")
      rows[#rows + 1] = string.format("%s & \\hyperref[%s]{%s} & \\sevchip{%s}{%s} & %s \\\\",
        has(a.finding) and esc(a.finding) or tostring(#rows + 1), b.identifier, title,
        severity_color[sev] or "colorInfo", esc(sev), has(a.cvss) and esc(a.cvss) or "—")
    end
  end
  if not first then return nil end
  local latex = "\\begin{center}\n\\begin{tblr}{colspec={Q[c,wd=1.5cm] X[l] Q[c,wd=2.4cm] Q[c,wd=1.6cm]}, " ..
    "width=\\linewidth, rowsep=3pt, row{1}={bg=colorPrimary, fg=white, font=\\bfseries}, " ..
    "cells={valign=m}, row{even}={bg=colorPrimary!6}, hlines={colorPrimary!30}, vlines={colorPrimary!30}}\n" ..
    "ID & Finding & Severity & CVSS \\\\\n" .. table.concat(rows, "\n") .. "\n\\end{tblr}\n\\end{center}\n\\clearpage"
  table.insert(blocks, first, pandoc.RawBlock("latex", latex))
  return doc
end

-- ── Table restyling ─────────────────────────────────────────────────────────
local align_map = {
  AlignLeft = "l", AlignRight = "r", AlignCenter = "c", AlignDefault = "l",
}

local function cell_to_latex(cell)
  local s = pandoc.write(pandoc.Pandoc(cell.contents), "latex")
  s = s:gsub("%s+$", ""):gsub("\r?\n", " ")
  -- Long unbroken tokens (hostnames, IPs, URLs) have no spaces for a
  -- tabularray X-column to wrap at, so without a break hint they overflow
  -- into (or overlap) neighboring cells. \allowbreak after '.', '/', ':'
  -- gives LaTeX a place to break without printing anything when it doesn't.
  s = s:gsub("([%./:])", "%1\\allowbreak{}")
  return s
end

local function render_row(row)
  local out = {}
  for _, cell in ipairs(row.cells) do
    out[#out + 1] = cell_to_latex(cell)
  end
  return table.concat(out, " & ") .. " \\\\"
end

local function Table(el)
  local colspecs = el.colspecs

  local has_width = false
  for _, cs in ipairs(colspecs) do
    if cs[2] and cs[2] > 0 then has_width = true end
  end
  local cols = {}
  for _, cs in ipairs(colspecs) do
    local al = align_map[cs[1]] or "l"
    local coef = 1
    if has_width and cs[2] and cs[2] > 0 then
      coef = math.max(1, math.floor(cs[2] * 12 + 0.5))
    end
    cols[#cols + 1] = string.format("X[%d,%s]", coef, al)
  end
  local colspec = table.concat(cols, " ")

  local lines = {}
  for _, row in ipairs(el.head.rows) do
    lines[#lines + 1] = render_row(row)
  end
  for _, body in ipairs(el.bodies) do
    for _, row in ipairs(body.body) do
      lines[#lines + 1] = render_row(row)
    end
  end

  local opts = string.format(
    "colspec={%s}, width=\\linewidth, rowsep=3pt, " ..
    "row{1}={bg=colorPrimary, fg=white, font=\\bfseries}, " ..
    "row{even}={bg=colorPrimary!6}, hlines={colorPrimary!30}, vlines={colorPrimary!30}", colspec)

  local latex = "\\begin{center}\n\\begin{tblr}{" .. opts .. "}\n" ..
    table.concat(lines, "\n") .. "\n\\end{tblr}\n\\end{center}"
  return pandoc.RawBlock("latex", latex)
end

-- ── Inline code ─────────────────────────────────────────────────────────────
-- Long paths/URLs in inline code have no spaces to wrap at, so \texttt runs
-- off the page (#64). Let pandoc escape it, then allow breaks after / . : =.
local function Code(el)
  if #el.classes > 0 then return nil end -- keep highlighted inline code as-is
  local s = pandoc.write(pandoc.Pandoc({ pandoc.Plain({ el }) }), "latex"):gsub("%s+$", "")
  return pandoc.RawInline("latex", (s:gsub("([/.:=])", "%1\\allowbreak{}")))
end

-- An em dash never starts a line: the space before it doesn't break. (The
-- PDF and --docx filters both do this, so TeX and Word break lines alike.)
local function Inlines(els)
  for i = 1, #els - 1 do
    if els[i].t == "Space" and els[i + 1].t == "Str" and els[i + 1].text:match("^\u{2014}") then
      els[i] = pandoc.Str("\u{00A0}")
    end
  end
  return els
end

-- Summary first (needs the raw .vulnerability divs), then the conversions.
return {
  { Pandoc = Pandoc },
  { Inlines = Inlines },
  { Div = Div, Table = Table, Code = Code },
}
