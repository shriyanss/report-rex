-- docx_vuln_filter.lua (rex-default template)
--   * Converts ::: {.vulnerability ...} divs into a Word-native severity panel
--     (colored header bar + a label/value properties table), mirroring
--     filters/vuln_filter.lua's tcolorbox layout for the PDF path.
--   * Prepends a branded cover block + an auto-updating TOC field.
--
-- DOCX has no LaTeX renderer, so this emits raw OpenXML (WordprocessingML)
-- directly wherever the PDF path would reach for a tcolorbox — pandoc's docx
-- writer passes `pandoc.RawBlock("openxml", ...)` straight through into
-- word/document.xml. Everything else (headings, paragraphs, lists, plain
-- tables, images incl. rasterized charts) still goes through pandoc's
-- ordinary docx writer untouched.
--
-- Colors default to templates/rex-default/theme.yaml's values but are overridden
-- at runtime from the report's resolved `colors:` metadata (index.md override
-- merged over theme.yaml), the same source the LaTeX template reads from —
-- so this filter never hardcodes a color that could drift out of sync.

local SEVERITY_COLORS = {
  Critical = "B71C1C", High = "E65100", Medium = "F57F17",
  Low = "1565C0", Info = "37474F", None = "37474F",
}
local PRIMARY = "1A1A2E"
local ACCENT = "0F3460"

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
  PRIMARY = get("primary", PRIMARY)
  ACCENT = get("accent", ACCENT)
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

-- A shaded, full-width single-cell bar (severity header / accent rule).
local function shaded_bar(text, fill, size)
  return table.concat({
    "<w:tbl><w:tblPr>",
    '<w:tblW w:w="5000" w:type="pct"/>',
    "<w:tblBorders>",
    '<w:top w:val="single" w:sz="2" w:color="', fill, '"/>',
    '<w:bottom w:val="single" w:sz="2" w:color="', fill, '"/>',
    '<w:left w:val="single" w:sz="2" w:color="', fill, '"/>',
    '<w:right w:val="single" w:sz="2" w:color="', fill, '"/>',
    "</w:tblBorders></w:tblPr>",
    "<w:tr><w:tc>",
    '<w:tcPr><w:tcW w:w="5000" w:type="pct"/>',
    '<w:shd w:val="clear" w:color="auto" w:fill="', fill, '"/>',
    '<w:tcMar><w:top w:w="60" w:type="dxa"/><w:bottom w:w="60" w:type="dxa"/>',
    '<w:left w:w="120" w:type="dxa"/><w:right w:w="120" w:type="dxa"/></w:tcMar></w:tcPr>',
    "<w:p>", run(text, { bold = true, color = "FFFFFF", size = size or "22" }), "</w:p>",
    "</w:tc></w:tr></w:tbl>",
  })
end

-- A two-column properties table (label | value). rows = { {label, value}, ... }
local function props_table(rows)
  local out = {
    "<w:tbl><w:tblPr>",
    '<w:tblW w:w="5000" w:type="pct"/>',
    "<w:tblBorders>",
    '<w:top w:val="single" w:sz="2" w:color="D0D0D0"/>',
    '<w:bottom w:val="single" w:sz="2" w:color="D0D0D0"/>',
    '<w:left w:val="single" w:sz="2" w:color="D0D0D0"/>',
    '<w:right w:val="single" w:sz="2" w:color="D0D0D0"/>',
    '<w:insideH w:val="single" w:sz="2" w:color="D0D0D0"/>',
    '<w:insideV w:val="single" w:sz="2" w:color="D0D0D0"/>',
    "</w:tblBorders></w:tblPr>",
    '<w:tblGrid><w:gridCol w:w="2200"/><w:gridCol w:w="7000"/></w:tblGrid>',
  }
  for _, r in ipairs(rows) do
    out[#out + 1] = table.concat({
      "<w:tr><w:tc>",
      '<w:tcPr><w:tcW w:w="1200" w:type="pct"/>',
      '<w:shd w:val="clear" w:color="auto" w:fill="F2F2F2"/>',
      '<w:tcMar><w:top w:w="40" w:type="dxa"/><w:bottom w:w="40" w:type="dxa"/>',
      '<w:left w:w="100" w:type="dxa"/><w:right w:w="100" w:type="dxa"/></w:tcMar></w:tcPr>',
      "<w:p>", run(r[1], { bold = true, color = PRIMARY, size = "18" }), "</w:p></w:tc>",
      "<w:tc>",
      '<w:tcPr><w:tcW w:w="3800" w:type="pct"/>',
      '<w:tcMar><w:top w:w="40" w:type="dxa"/><w:bottom w:w="40" w:type="dxa"/>',
      '<w:left w:w="100" w:type="dxa"/><w:right w:w="100" w:type="dxa"/></w:tcMar></w:tcPr>',
      "<w:p>", run(r[2], { size = "18" }), "</w:p></w:tc></w:tr>",
    })
  end
  out[#out + 1] = "</w:tbl>"
  return table.concat(out)
end

local function empty_para() return "<w:p/>" end

-- ── Vulnerability finding panel ──────────────────────────────────────────────

local function build_vuln_box(el)
  local a = el.attributes
  local sev = a.severity or "Info"
  local fill = SEVERITY_COLORS[sev] or SEVERITY_COLORS.Info

  local bar = string.upper(sev)
  if a.cvss and a.cvss ~= "" then
    bar = bar .. "   |   CVSS " .. a.cvss
    if a.cvssversion and a.cvssversion ~= "" then bar = bar .. " (v" .. a.cvssversion .. ")" end
  end
  bar = bar .. "   |   Status: " .. (a.status ~= "" and a.status or "Open")

  local rows = {}
  local function add(label, val) if val and val ~= "" then rows[#rows + 1] = { label, val } end end
  add("CVSS Vector", a.vector)
  add("CWE", a.cwe)
  add("OWASP", a.owasp)
  add("MITRE ATT&CK", a.mitre)
  add("Business Risk", a.business)
  add("Affected", a.affected)
  add("Component", a.component)

  local blocks = pandoc.List()
  blocks:insert(pandoc.RawBlock("openxml", empty_para()))
  blocks:insert(pandoc.RawBlock("openxml", shaded_bar(bar, fill)))
  if #rows > 0 then
    blocks:insert(pandoc.RawBlock("openxml", props_table(rows)))
  end
  blocks:insert(pandoc.RawBlock("openxml", empty_para()))
  for _, b in ipairs(el.content) do blocks:insert(b) end
  blocks:insert(pandoc.RawBlock("openxml", empty_para()))
  return blocks
end

-- ── Cover / TOC (full-report layout) ─────────────────────────────────────────

local function build_cover(doc)
  local m = doc.meta
  local function g(k) return meta_to_str(m[k]) end
  local title = g("title")
  if title == "" then return nil end

  local cover = pandoc.List()
  cover:insert(pandoc.RawBlock("openxml", empty_para()))
  cover:insert(pandoc.RawBlock("openxml",
    "<w:p><w:pPr><w:spacing w:after='120'/></w:pPr>" ..
    run(title, { bold = true, color = PRIMARY, size = "56" }) .. "</w:p>"))
  local subtitle = g("subtitle")
  if subtitle ~= "" then
    cover:insert(pandoc.RawBlock("openxml",
      "<w:p><w:pPr><w:spacing w:after='240'/></w:pPr>" ..
      run(subtitle, { color = ACCENT, size = "32" }) .. "</w:p>"))
  end
  cover:insert(pandoc.RawBlock("openxml", shaded_bar("", ACCENT, "2")))
  cover:insert(pandoc.RawBlock("openxml", empty_para()))

  local rows = {}
  local function add(label, key)
    local v = g(key)
    if v ~= "" then rows[#rows + 1] = { label, v } end
  end
  add("Client", "client")
  add("Prepared by", "company")
  add("Engagement", "engagement_type")
  add("Report ID", "report_id")
  add("Version", "version")
  add("Report date", "date")
  local sd, ed = g("start_date"), g("end_date")
  if sd ~= "" and ed ~= "" then rows[#rows + 1] = { "Testing window", sd .. "  –  " .. ed } end
  add("Confidentiality", "confidentiality")
  add("Contact", "contact_name")
  add("Contact email", "contact_email")

  if m.authors and m.authors.t == "MetaList" then
    local names = {}
    for _, au in ipairs(m.authors) do
      local nm, role = meta_to_str(au.name), meta_to_str(au.role)
      if nm ~= "" then names[#names + 1] = (role ~= "" and (nm .. " (" .. role .. ")") or nm) end
    end
    if #names > 0 then rows[#rows + 1] = { "Authors", table.concat(names, "; ") } end
  end
  if #rows > 0 then cover:insert(pandoc.RawBlock("openxml", props_table(rows))) end

  local nheads = 0
  for _, b in ipairs(doc.blocks) do
    if b.t == "Header" and b.level and b.level <= 2 then nheads = nheads + 1 end
  end

  cover:insert(pandoc.RawBlock("openxml", '<w:p><w:r><w:br w:type="page"/></w:r></w:p>'))
  if nheads >= 3 then
    cover:insert(pandoc.Header(1, { pandoc.Str("Table of Contents") }))
    cover:insert(pandoc.RawBlock("openxml", table.concat({
      "<w:p>",
      '<w:r><w:fldChar w:fldCharType="begin" w:dirty="true"/></w:r>',
      '<w:r><w:instrText xml:space="preserve"> TOC \\o "1-2" \\h \\z \\u </w:instrText></w:r>',
      '<w:r><w:fldChar w:fldCharType="separate"/></w:r>',
      run("Right-click and choose \"Update Field\" to build the table of contents.",
          { italic = true, color = "808080", size = "18" }),
      '<w:r><w:fldChar w:fldCharType="end"/></w:r>',
      "</w:p>",
    })))
    cover:insert(pandoc.RawBlock("openxml", '<w:p><w:r><w:br w:type="page"/></w:r></w:p>'))
  end

  m.title, m.subtitle, m.date = nil, nil, nil
  doc.meta = m
  return cover
end

-- ── Entry point ──────────────────────────────────────────────────────────────

function Pandoc(doc)
  load_colors(doc.meta)

  doc.blocks = doc.blocks:walk({
    Div = function(el)
      if not el.classes:includes("vulnerability") then return nil end
      return build_vuln_box(el)
    end,
  })

  local cover = build_cover(doc)
  if cover then
    local blocks = pandoc.List()
    for _, b in ipairs(cover) do blocks:insert(b) end
    for _, b in ipairs(doc.blocks) do blocks:insert(b) end
    doc.blocks = blocks
  end

  return doc
end
