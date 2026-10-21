-- docx_vuln_filter.lua (rit-cptc-comm template)
--   * Converts ::: {.vulnerability ...} divs into a compact severity card
--     (title inside the card, since quick communications have no per-finding
--     heading), surfacing the rit-cptc fields (Impact/Sophistication) inline
--     when present — echoing filters/vuln_filter.lua's vulnbox layout for
--     the PDF path.
--   * No cover page or TOC — this is a one/two-pager, same as the PDF.
--
-- DOCX has no LaTeX renderer, so this emits raw OpenXML (WordprocessingML)
-- directly for the severity card — pandoc's docx writer passes
-- `pandoc.RawBlock("openxml", ...)` straight through into word/document.xml.
-- Everything else still goes through pandoc's ordinary docx writer untouched.
--
-- Colors default to templates/rit-cptc-comm/theme.yaml's values but are
-- overridden at runtime from the report's resolved `colors:` metadata, the
-- same source the LaTeX template reads from.

local SEVERITY_COLORS = {
  Critical = "B71C1C", High = "E65100", Medium = "F57F17",
  Low = "1565C0", Info = "37474F", None = "37474F",
}

local function xml_escape(s)
  s = tostring(s or "")
  s = s:gsub("&", "&amp;"):gsub("<", "&lt;"):gsub(">", "&gt;"):gsub('"', "&quot;")
  return s
end

local function meta_to_str(v)
  if v == nil then return "" end
  if type(v) == "string" then return v end
  return pandoc.utils.stringify(v)
end

local function load_colors(meta)
  local c = meta.colors
  if c == nil then return end
  local function get(key, default)
    local v = c[key]
    if v then return meta_to_str(v) end
    return default
  end
  SEVERITY_COLORS.Critical = get("critical", SEVERITY_COLORS.Critical)
  SEVERITY_COLORS.High = get("high", SEVERITY_COLORS.High)
  SEVERITY_COLORS.Medium = get("medium", SEVERITY_COLORS.Medium)
  SEVERITY_COLORS.Low = get("low", SEVERITY_COLORS.Low)
  SEVERITY_COLORS.Info = get("info", SEVERITY_COLORS.Info)
  SEVERITY_COLORS.None = SEVERITY_COLORS.Info
end

-- ── OpenXML primitives ───────────────────────────────────────────────────────

local function run(text, opts)
  opts = opts or {}
  local rpr = {}
  if opts.bold then rpr[#rpr + 1] = "<w:b/>" end
  if opts.italic then rpr[#rpr + 1] = "<w:i/>" end
  if opts.color then rpr[#rpr + 1] = '<w:color w:val="' .. opts.color .. '"/>' end
  if opts.size then rpr[#rpr + 1] = '<w:sz w:val="' .. opts.size .. '"/>' end
  local rprxml = #rpr > 0 and ("<w:rPr>" .. table.concat(rpr) .. "</w:rPr>") or ""
  return "<w:r>" .. rprxml .. '<w:t xml:space="preserve">' .. xml_escape(text) .. "</w:t></w:r>"
end

local function tab() return '<w:r><w:tab/></w:r>' end

-- A single bordered card: a colored top accent, light gray sides/bottom,
-- containing one or more already-built <w:p>...</w:p> paragraph strings.
local function card(paras, fill)
  return table.concat({
    "<w:tbl><w:tblPr>",
    '<w:tblW w:w="5000" w:type="pct"/>',
    "<w:tblBorders>",
    '<w:top w:val="single" w:sz="16" w:color="', fill, '"/>',
    '<w:bottom w:val="single" w:sz="4" w:color="D0D0D0"/>',
    '<w:left w:val="single" w:sz="4" w:color="D0D0D0"/>',
    '<w:right w:val="single" w:sz="4" w:color="D0D0D0"/>',
    "</w:tblBorders></w:tblPr>",
    "<w:tr><w:tc>",
    '<w:tcPr><w:tcW w:w="5000" w:type="pct"/>',
    '<w:tcMar><w:top w:w="120" w:type="dxa"/><w:bottom w:w="120" w:type="dxa"/>',
    '<w:left w:w="150" w:type="dxa"/><w:right w:w="150" w:type="dxa"/></w:tcMar></w:tcPr>',
    table.concat(paras),
    "</w:tc></w:tr></w:tbl>",
  })
end

local function empty_para() return "<w:p/>" end

-- ── Vulnerability finding card ───────────────────────────────────────────────

local function build_vuln_card(el)
  local a = el.attributes
  local sev = a.severity or "Info"
  local fill = SEVERITY_COLORS[sev] or SEVERITY_COLORS.Info
  local title = (a.title and a.title ~= "") and a.title or "Finding"

  local title_p = "<w:p><w:pPr><w:spacing w:after='80'/></w:pPr>" ..
    run(title, { bold = true, size = "28" }) .. "</w:p>"

  local chips = "<w:p>" .. run(string.upper(sev), { bold = true, color = fill, size = "20" })
  if a.cvss and a.cvss ~= "" then
    chips = chips .. tab() .. run("CVSS " .. a.cvss, { bold = true, size = "18" })
  end
  if a.impact and a.impact ~= "" then
    chips = chips .. tab() .. run("Impact: " .. a.impact, { size = "18" })
  end
  if a.sophistication and a.sophistication ~= "" then
    chips = chips .. tab() .. run("Sophistication: " .. a.sophistication, { size = "18" })
  end
  chips = chips .. tab() .. run("Status: " .. (a.status ~= "" and a.status or "Open"), { size = "18" }) .. "</w:p>"

  local paras = { title_p, chips }
  if a.affected and a.affected ~= "" then
    paras[#paras + 1] = "<w:p><w:pPr><w:spacing w:before='80'/></w:pPr>" ..
      run("Affected: ", { bold = true, size = "16" }) ..
      run(a.affected, { size = "16" }) .. "</w:p>"
  end

  local blocks = pandoc.List()
  blocks:insert(pandoc.RawBlock("openxml", empty_para()))
  blocks:insert(pandoc.RawBlock("openxml", card(paras, fill)))
  blocks:insert(pandoc.RawBlock("openxml", empty_para()))
  for _, b in ipairs(el.content) do blocks:insert(b) end
  blocks:insert(pandoc.RawBlock("openxml", empty_para()))
  return blocks
end

function Pandoc(doc)
  load_colors(doc.meta)
  doc.blocks = doc.blocks:walk({
    Div = function(el)
      if not el.classes:includes("vulnerability") then return nil end
      return build_vuln_card(el)
    end,
  })
  return doc
end
