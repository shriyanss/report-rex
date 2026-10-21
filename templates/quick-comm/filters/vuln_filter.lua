-- vuln_filter.lua (quick-comm template)
--   * Converts ::: {.vulnerability ...} divs into a compact severity panel.
--     Unlike the default template, the title is shown inside the panel because
--     quick communications have no per-finding heading.
--   * Restyles pandoc tables as full-width tabularray tables.

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

function Div(el)
  if not el.classes:includes("vulnerability") then return nil end
  local a = el.attributes
  local sev = a.severity or "Info"
  local color = severity_color[sev] or "colorInfo"

  local head = string.format("{\\large\\bfseries %s}\\\\[4pt]", esc(a.title or "Finding"))
  local line = string.format("\\sevchip{%s}{%s}", color, esc(sev))
  if a.cvss and a.cvss ~= "" then
    line = line .. string.format("\\hspace{10pt}\\textbf{CVSS %s}", esc(a.cvss))
  end
  line = line .. string.format("\\hfill\\metalabel{Status}~\\textbf{%s}", esc(a.status or "Open"))

  local affected = ""
  if a.affected and a.affected ~= "" then
    affected = string.format("\\\\[5pt]\\metalabel{Affected}~\\texttt{\\small %s}", esc(a.affected))
  end

  local latex = string.format(
    "\\begin{vulnbox}{%s}\n%s%s%s\n\\end{vulnbox}", color, head, line, affected)
  return pandoc.RawBlock("latex", latex)
end

-- ── Table restyling (shared behaviour with the default template) ────────────
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
  return (s:gsub("([%./:])", "%1\\allowbreak{}"))
end

local function render_row(row)
  local out = {}
  for _, cell in ipairs(row.cells) do
    out[#out + 1] = cell_to_latex(cell)
  end
  return table.concat(out, " & ") .. " \\\\"
end

function Table(el)
  local cols = {}
  local has_width = false
  for _, cs in ipairs(el.colspecs) do
    if cs[2] and cs[2] > 0 then has_width = true end
  end
  for _, cs in ipairs(el.colspecs) do
    local al = align_map[cs[1]] or "l"
    local coef = 1
    if has_width and cs[2] and cs[2] > 0 then
      coef = math.max(1, math.floor(cs[2] * 12 + 0.5))
    end
    cols[#cols + 1] = string.format("X[%d,%s]", coef, al)
  end

  local lines = {}
  for _, row in ipairs(el.head.rows) do lines[#lines + 1] = render_row(row) end
  for _, body in ipairs(el.bodies) do
    for _, row in ipairs(body.body) do lines[#lines + 1] = render_row(row) end
  end

  local opts = string.format(
    "colspec={%s}, width=\\linewidth, rowsep=3pt, " ..
    "row{1}={bg=colorPrimary, fg=white, font=\\bfseries}, " ..
    "row{even}={bg=colorPrimary!5}, hlines={colorPrimary!12}", table.concat(cols, " "))
  return pandoc.RawBlock("latex",
    "\\begin{center}\n\\begin{tblr}{" .. opts .. "}\n" ..
    table.concat(lines, "\n") .. "\n\\end{tblr}\n\\end{center}")
end

-- ── Inline code ─────────────────────────────────────────────────────────────
-- Long paths/URLs in inline code have no spaces to wrap at, so \texttt runs
-- off the page (#64). Let pandoc escape it, then allow breaks after / . : =.
function Code(el)
  if #el.classes > 0 then return nil end -- keep highlighted inline code as-is
  local s = pandoc.write(pandoc.Pandoc({ pandoc.Plain({ el }) }), "latex"):gsub("%s+$", "")
  return pandoc.RawInline("latex", (s:gsub("([/.:=])", "%1\\allowbreak{}")))
end
