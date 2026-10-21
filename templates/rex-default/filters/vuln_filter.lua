-- vuln_filter.lua (rex-default template)
--   * Converts ::: {.vulnerability ...} divs into a styled severity panel.
--   * Restyles all pandoc tables as full-width tabularray tables with a
--     colored header row and zebra striping.

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

-- ── Vulnerability panel ─────────────────────────────────────────────────────
function Div(el)
  if not el.classes:includes("vulnerability") then return nil end
  local a = el.attributes
  local sev = a.severity or "Info"
  local color = severity_color[sev] or "colorInfo"

  -- Top line: severity chip + CVSS score + status
  local top = string.format("\\sevchip{%s}{%s}", color, esc(sev))
  if a.cvss and a.cvss ~= "" then
    top = top .. string.format("\\hspace{10pt}{\\large\\bfseries CVSS %s}", esc(a.cvss))
    if a.cvssversion and a.cvssversion ~= "" then
      top = top .. string.format("{\\small\\color{colorMute}~v%s}", esc(a.cvssversion))
    end
  end
  top = top .. string.format(
    "\\hfill\\metalabel{Status}~\\textbf{%s}", esc(a.status or "Open"))

  -- Detail rows (only those present)
  local rows = {}
  local function add(k, v, mono)
    if v and v ~= "" then
      local val = esc(v)
      if mono then val = "\\texttt{\\small " .. val .. "}" end
      rows[#rows + 1] = string.format("%s & %s \\\\", k, val)
    end
  end
  add("CVSS Vector", a.vector, true)
  add("CWE", a.cwe, false)
  add("OWASP", a.owasp, false)
  add("MITRE ATT\\&CK", a.mitre, false)
  add("Business Risk", a.business, false)
  add("Affected", a.affected, true)
  add("Component", a.component, false)

  local detail = ""
  if #rows > 0 then
    detail = "\n\\vspace{5pt}\n" ..
      "\\begin{tblr}{colspec={Q[l,wd=2.9cm,font=\\bfseries\\footnotesize\\color{colorMute}] X[l]}, rowsep=1.8pt, width=\\linewidth}\n" ..
      table.concat(rows, "\n") .. "\n\\end{tblr}"
  end

  local latex = string.format(
    "\\begin{vulnbox}{%s}\n%s%s\n\\end{vulnbox}", color, top, detail)
  return pandoc.RawBlock("latex", latex)
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

function Table(el)
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
    "row{even}={bg=colorPrimary!5}, hlines={colorPrimary!12}", colspec)

  local latex = "\\begin{center}\n\\begin{tblr}{" .. opts .. "}\n" ..
    table.concat(lines, "\n") .. "\n\\end{tblr}\n\\end{center}"
  return pandoc.RawBlock("latex", latex)
end

-- ── Inline code ─────────────────────────────────────────────────────────────
-- Long paths/URLs in inline code have no spaces to wrap at, so \texttt runs
-- off the page (#64). Let pandoc escape it, then allow breaks after / . : =.
function Code(el)
  if #el.classes > 0 then return nil end -- keep highlighted inline code as-is
  local s = pandoc.write(pandoc.Pandoc({ pandoc.Plain({ el }) }), "latex"):gsub("%s+$", "")
  return pandoc.RawInline("latex", (s:gsub("([/.:=])", "%1\\allowbreak{}")))
end
