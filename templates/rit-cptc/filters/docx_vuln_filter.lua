-- docx_vuln_filter.lua (rit-cptc template)
--   The --docx counterpart of vuln_filter.lua + template.tex: rebuilds the PDF's
--   layout in raw OpenXML (WordprocessingML) wherever pandoc's docx writer has
--   no equivalent — the indigo brand-band cover (its own unnumbered section),
--   the Confidentiality / Document Control page, the table of contents,
--   numbered section headings, the findings summary table, severity finding
--   panels, and \clearpage page breaks. Body text, lists, code, images and
--   plain tables go through pandoc's writer, styled by reference.docx (page
--   geometry, fonts, header/footer page art: core/scripts/make_reference_docx.py).
--
-- Colors default to theme.yaml's values and are overridden at runtime from the
-- report's resolved `colors:` metadata, the same source template.tex reads.

local PRIMARY, ACCENT = "26307C", "C71C64"
local SEV = {
  Critical = "FF3B5C", High = "F5820E", Medium = "F4A81D", Low = "56C4C9", Info = "8B7DF0",
}
local INK, MUTE = "1C2333", "6B7280"
local TEXT_W = 9751 -- twips: A4 minus template.tex's 1.9cm side margins

local function has(v) return v ~= nil and v ~= "" end

-- A zero-width space after each `chars` match: the break opportunities the
-- PDF's \allowbreak gives long URLs, paths and vectors.
local function breakable(s, chars)
  return (s:gsub("(" .. chars .. ")", "%1\u{200B}"))
end

local function str(v)
  if v == nil then return "" end
  if type(v) == "string" then return v end
  return pandoc.utils.stringify(v)
end

local function xml(s)
  return (tostring(s or ""):gsub("&", "&amp;"):gsub("<", "&lt;"):gsub(">", "&gt;"):gsub('"', "&quot;"))
end

-- xcolor's `color!pct!white`.
local function tint(hex, pct)
  local out = {}
  for i = 1, 5, 2 do
    local c = tonumber(hex:sub(i, i + 1), 16)
    out[#out + 1] = string.format("%02X", math.floor(c * pct / 100 + 255 * (100 - pct) / 100 + 0.5))
  end
  return table.concat(out)
end

local function load_colors(meta)
  local c = meta.colors
  if c == nil then return end
  local function get(key, default) return has(str(c[key])) and str(c[key]) or default end
  PRIMARY, ACCENT = get("primary", PRIMARY), get("accent", ACCENT)
  for sev, key in pairs({ Critical = "critical", High = "high", Medium = "medium", Low = "low", Info = "info" }) do
    SEV[sev] = get(key, SEV[sev])
  end
end

local function sev_color(sev) return SEV[sev] or SEV.Info end

-- ── OpenXML primitives ───────────────────────────────────────────────────────

-- o: b, i, caps, color, sz (half-points), mono/sans, shd (fill), style (rStyle).
-- rPr children must follow the schema's order.
local function run(text, o)
  o = o or {}
  local p = {}
  if o.style then p[#p + 1] = '<w:rStyle w:val="' .. o.style .. '"/>' end
  if o.mono then p[#p + 1] = '<w:rFonts w:ascii="Liberation Mono" w:hAnsi="Liberation Mono" w:cs="Liberation Mono"/>' end
  if o.sans then p[#p + 1] = '<w:rFonts w:ascii="Liberation Sans" w:hAnsi="Liberation Sans" w:cs="Liberation Sans"/>' end
  if o.b then p[#p + 1] = "<w:b/><w:bCs/>" end
  if o.i then p[#p + 1] = "<w:i/>" end
  if o.caps then p[#p + 1] = "<w:caps/>" end
  if o.color then p[#p + 1] = '<w:color w:val="' .. o.color .. '"/>' end
  if o.sz then p[#p + 1] = '<w:sz w:val="' .. o.sz .. '"/><w:szCs w:val="' .. o.sz .. '"/>' end
  if o.raise then p[#p + 1] = '<w:position w:val="' .. o.raise .. '"/>' end
  if o.shd then
    -- A same-colored border pads the shading out, like \colorbox's \fboxsep.
    p[#p + 1] = '<w:bdr w:val="single" w:sz="12" w:space="0" w:color="' .. o.shd .. '"/>' ..
      '<w:shd w:val="clear" w:color="auto" w:fill="' .. o.shd .. '"/>'
  end
  return "<w:r>" .. (#p > 0 and ("<w:rPr>" .. table.concat(p) .. "</w:rPr>") or "") ..
    '<w:t xml:space="preserve">' .. xml(text) .. "</w:t></w:r>"
end

-- ppr: raw pPr children. Unstyled paragraphs default to no spacing.
local function para(runs, ppr)
  ppr = ppr or ""
  if not ppr:find("<w:spacing") and not ppr:find("<w:pStyle") then
    ppr = ppr .. '<w:spacing w:before="0" w:after="0"/>'
  end
  return "<w:p><w:pPr>" .. ppr .. "</w:pPr>" .. runs .. "</w:p>"
end

local function spacing(before, after, extra)
  return string.format('<w:spacing w:before="%d" w:after="%d"%s/>', before or 0, after or 0, extra or "")
end

local function raw(x) return pandoc.RawBlock("openxml", x) end

-- A page break whose leftover (empty) line on the next page is ~1pt tall.
local PAGE_BREAK = para('<w:r><w:br w:type="page"/></w:r>',
  spacing(0, 0, ' w:line="20" w:lineRule="exact"') .. '<w:rPr><w:sz w:val="2"/></w:rPr>')

local function field(instr, cached, o)
  return run("", o):gsub("<w:t[^>]*></w:t>", '<w:fldChar w:fldCharType="begin"/>') ..
    run("", o):gsub("<w:t[^>]*></w:t>", '<w:instrText xml:space="preserve"> ' .. instr .. " </w:instrText>") ..
    run("", o):gsub("<w:t[^>]*></w:t>", '<w:fldChar w:fldCharType="separate"/>') ..
    run(cached, o) ..
    run("", o):gsub("<w:t[^>]*></w:t>", '<w:fldChar w:fldCharType="end"/>')
end

local function borders(tag, spec)
  local out = {}
  for _, side in ipairs({ "top", "left", "bottom", "right", "insideH", "insideV" }) do
    local s = spec[side]
    if s then
      out[#out + 1] = string.format('<w:%s w:val="single" w:sz="%d" w:space="0" w:color="%s"/>', side, s[1], s[2])
    else
      out[#out + 1] = string.format('<w:%s w:val="nil"/>', side)
    end
  end
  return "<" .. tag .. ">" .. table.concat(out) .. "</" .. tag .. ">"
end

local function cell(w, content, tcpr)
  return string.format('<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/>%s</w:tcPr>%s</w:tc>', w, tcpr or "", content)
end

local function shd(fill) return '<w:shd w:val="clear" w:color="auto" w:fill="' .. fill .. '"/>' end

-- Filled badge with white text: \sevchip / the cover's classification box.
local function chip(text, fill, sz)
  return run("\u{00A0}" .. text .. "\u{00A0}", { b = true, color = "FFFFFF", shd = fill, sz = sz })
end

-- cols: widths (twips); rows: list of cell XML lists; o.borders/o.mar/o.jc
local function tbl(cols, rows, o)
  o = o or {}
  local grid = {}
  local total = 0
  for _, w in ipairs(cols) do
    grid[#grid + 1] = '<w:gridCol w:w="' .. w .. '"/>'
    total = total + w
  end
  local out = {
    "<w:tbl><w:tblPr>",
    string.format('<w:tblW w:w="%d" w:type="dxa"/>', total),
    o.jc and ('<w:jc w:val="' .. o.jc .. '"/>') or "",
    -- Word hangs cell margins left of the text margin; LaTeX tables start on it.
    o.ind and string.format('<w:tblInd w:w="%d" w:type="dxa"/>', o.ind) or "",
    o.borders or borders("w:tblBorders", {}),
    '<w:tblLayout w:type="fixed"/>',
    o.mar or "",
    '<w:tblLook w:val="0000" w:firstRow="0" w:lastRow="0" w:firstColumn="0" w:lastColumn="0" w:noHBand="1" w:noVBand="1"/>',
    "</w:tblPr><w:tblGrid>", table.concat(grid), "</w:tblGrid>",
  }
  for _, r in ipairs(rows) do out[#out + 1] = "<w:tr>" .. table.concat(r) .. "</w:tr>" end
  out[#out + 1] = "</w:tbl>"
  return table.concat(out)
end

local function cell_mar(top, side)
  return string.format('<w:tblCellMar><w:top w:w="%d" w:type="dxa"/><w:left w:w="%d" w:type="dxa"/>' ..
    '<w:bottom w:w="%d" w:type="dxa"/><w:right w:w="%d" w:type="dxa"/></w:tblCellMar>', top, side, top, side)
end

-- The grid style template.tex/vuln_filter.lua give every styled table:
-- indigo header row, colorPrimary!30 rules, colorPrimary!6 on even rows.
local function grid_borders() return borders("w:tblBorders", {
  top = { 4, tint(PRIMARY, 30) }, left = { 4, tint(PRIMARY, 30) }, bottom = { 4, tint(PRIMARY, 30) },
  right = { 4, tint(PRIMARY, 30) }, insideH = { 4, tint(PRIMARY, 30) }, insideV = { 4, tint(PRIMARY, 30) } })
end

local function today()
  local t = os.date("*t")
  local months = { "January", "February", "March", "April", "May", "June", "July", "August",
    "September", "October", "November", "December" }
  return string.format("%s %d, %d", months[t.month], t.day, t.year)
end

-- ── Vulnerability finding panel (vuln_filter.lua's vulnbox) ──────────────────

local function build_vuln_box(el)
  local a = el.attributes
  local sev = a.severity or "Info"
  local color = sev_color(sev)

  local top = chip(sev, color, 20)
  if has(a.finding) then top = top .. run("\u{00A0}\u{00A0}" .. a.finding, { b = true, color = color, sz = 22 }) end
  if has(a.cvss) then
    top = top .. run("\u{00A0}\u{00A0}\u{00A0}CVSS " .. a.cvss, { b = true, sz = 22 })
    if has(a.cvssversion) then top = top .. run("\u{00A0}v" .. a.cvssversion, { color = MUTE, sz = 18 }) end
  end

  local rows = {}
  -- mono rows keep the PDF's line pitch when they wrap (12pt for 9.8pt mono)
  local function add(label, runs, mono_row, pad)
    if runs then
      rows[#rows + 1] = {
        cell(1998, para(run(label, { b = true, color = MUTE, sz = 16, raise = 2 }), pad and spacing(pad, 0) or nil)),
        cell(7100, para(runs, (mono_row or pad) and spacing(pad or 0, 0, mono_row and
          string.format(' w:line="%d" w:lineRule="exact"', mono_row) or "") or nil)),
      }
    end
  end
  local function mono(v) return has(v) and run(breakable(v, "[/.:,]"), { mono = true, sz = 17 }) or nil end
  local function plain(v) return has(v) and run(v) or nil end

  -- a mono value wider than its column (5.1pt/char) wraps, at the 9.8pt mono
  -- font's 12pt baselineskip
  local wraps = function(v) return has(v) and utf8.len(v) * 5.1 > 343 and 239 or nil end
  add("CVSS Vector", mono(a.vector), wraps(a.vector))
  local eval = {}
  if has(a.impact) then eval[#eval + 1] = run("Impact: ", { b = true }) .. run(a.impact) end
  if has(a.sophistication) then eval[#eval + 1] = run("Sophistication: ", { b = true }) .. run(a.sophistication) end
  add("Evaluation", #eval > 0 and table.concat(eval, run("\u{2003}")) or nil)
  add("CWE", plain(a.cwe))
  add("OWASP", plain(a.owasp))
  add("MITRE ATT&CK", plain(a.mitre))
  add("Mitigations", plain(a.mitigations))
  add("Business Risk", plain(a.business))
  add("Material Impact", plain(a.material))
  add("Component", plain(a.component))
  add(has(a.component) and "\u{2002}\u{00A0}\u{2192}\u{00A0}Affected" or "Affected", mono(a.affected), wraps(a.affected), 22)

  local detail = ""
  if #rows > 0 then
    -- tblr's 6pt colsep: labels sit 6pt in, values 3.1cm + 12pt after them.
    detail = tbl({ 1998, 7100 }, rows, { mar = cell_mar(36, 120), ind = 52 }) ..
      para("", spacing(0, 0, ' w:line="20" w:lineRule="exact"'))
  end

  local box = tbl({ TEXT_W }, { {
    cell(TEXT_W, para(top, spacing(0, 180)) .. detail, shd(tint(color, 5))),
  } }, {
    borders = borders("w:tblBorders", { top = { 8, color }, left = { 32, color }, bottom = { 8, color }, right = { 8, color } }),
    mar = cell_mar(160, 200), ind = 280,
  }):gsub('<w:top w:w="160"', '<w:top w:w="220"', 1):gsub('<w:bottom w:w="160"', '<w:bottom w:w="280"', 1)

  -- the gap below the panel: a wrapped (12pt-pitch) last row leaves less
  local tail = wraps(a.affected) and 63 or 25
  local blocks = pandoc.List({ raw(para("", spacing(142, 0, ' w:line="20" w:lineRule="exact"'))), raw(box), raw(para("", spacing(0, 0, string.format(' w:line="%d" w:lineRule="exact"', tail)))) })
  blocks:extend(el.content)
  return blocks
end

-- ── Findings summary table (before the first finding) ────────────────────────

local function summary_table(entries)
  -- tblr's wd= is the text width; add its 6pt colsep either side.
  local cols = { 1090, 5913, 1601, 1147 }
  local mar = cell_mar(67, 120)
  local function c(i, content, fill, jc)
    return cell(cols[i], para(content, '<w:jc w:val="' .. (jc or "center") .. '"/>'),
      '<w:vAlign w:val="center"/>' .. (fill and shd(fill) or ""))
  end
  local head = {}
  for i, h in ipairs({ "ID", "Finding", "Severity", "CVSS" }) do
    head[i] = c(i, run(h, { b = true, color = "FFFFFF" }), PRIMARY, i == 2 and "left" or "center")
  end
  local rows = { head }
  for n, e in ipairs(entries) do
    local fill = (n % 2 == 1) and tint(PRIMARY, 6) or nil -- tblr row{even}: the header is row 1
    rows[#rows + 1] = {
      c(1, run(e.id), fill),
      c(2, '<w:hyperlink w:anchor="' .. xml(e.anchor) .. '" w:history="1">' .. run(e.title, { color = ACCENT }) ..
        "</w:hyperlink>", fill, "left"),
      c(3, chip(e.sev, sev_color(e.sev)), fill),
      c(4, run(e.cvss), fill),
    }
  end
  return {
    raw(para("", spacing(235, 0, ' w:line="20" w:lineRule="exact"'))),
    raw(tbl(cols, rows, { borders = grid_borders(), mar = mar, ind = 120 })),
  }
end

-- ── Cover (its own section, no page number) ──────────────────────────────────

-- A behind-text rectangle anchored to the page (cm). wps isn't among the
-- namespaces pandoc declares on document.xml, so it is declared inline.
local function rect(id, x, y, w, h, fill)
  local e = function(cm) return math.floor(cm * 360000 + 0.5) end
  return string.format('<w:r><w:drawing><wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" ' ..
    'relativeHeight="%d" behindDoc="1" locked="1" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/>' ..
    '<wp:positionH relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionH>' ..
    '<wp:positionV relativeFrom="page"><wp:posOffset>%d</wp:posOffset></wp:positionV>' ..
    '<wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:wrapNone/>' ..
    '<wp:docPr id="%d" name="RexCover%d"/><wp:cNvGraphicFramePr/><a:graphic><a:graphicData ' ..
    'uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wps:wsp ' ..
    'xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><wps:cNvSpPr/><wps:spPr>' ..
    '<a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom>' ..
    '<a:solidFill><a:srgbClr val="%s"/></a:solidFill><a:ln><a:noFill/></a:ln></wps:spPr><wps:bodyPr/></wps:wsp>' ..
    '</a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>',
    id, e(x), e(y), e(w), e(h), 9000 + id, id, e(w), e(h), fill)
end

local function image_para(path, height, style)
  return pandoc.Div({ pandoc.Para({ pandoc.Image({}, path, "", { height = height }) }) },
    { ["custom-style"] = style })
end

-- A paragraph pinned to a page position (cm), outside the text flow.
local function framed(runs, x, y, w, jc, line)
  local tw = function(cm) return math.floor(cm * 567 + 0.5) end
  return para(runs, string.format('<w:framePr w:w="%d" w:wrap="around" w:hAnchor="page" w:vAnchor="page" ' ..
    'w:x="%d" w:y="%d"/><w:jc w:val="%s"/>', tw(w), tw(x), tw(y), jc or "left") ..
    (line and spacing(0, 0, string.format(' w:line="%d" w:lineRule="exact"', line)) or ""))
end

local function cover_blocks(m)
  local g = function(k) return str(m[k]) end
  local firm = has(g("company")) and g("company") or "Report Rex"
  local center = '<w:jc w:val="center"/>'
  local rule = para("", center .. spacing(283, 0, ' w:line="20" w:lineRule="exact"') ..
    '<w:pBdr><w:bottom w:val="single" w:sz="11" w:space="0" w:color="' .. PRIMARY .. '"/></w:pBdr>' ..
    '<w:ind w:left="488" w:right="488"/>')
  local b = pandoc.List()

  -- Brand band (with a magenta rule beneath it) and the bottom band.
  b:insert(raw(para(rect(1, 0, 0, 21, 4.2, PRIMARY) .. rect(2, 0, 4.2, 21, 0.15, ACCENT) ..
    rect(3, 0, 28.1, 21, 1.6, PRIMARY), spacing(0, 0, ' w:line="20" w:lineRule="exact"'))))
  -- Logo (or firm name) left, firm + tagline right.
  if has(g("company_logo_light")) then
    b:insert(image_para(g("company_logo_light"), "1.5cm", "Cover Band"))
  else
    b:insert(raw(para(run(firm, { b = true, color = "FFFFFF", sz = 28 }), spacing(200, 0))))
  end
  b:insert(raw(framed(run(firm, { color = "D9D9D9", sz = 16 }) .. "<w:r><w:br/></w:r>" ..
    run("Offensive Security Services", { color = "D9D9D9", sz = 16 }), 14.1, 1.68, 5.0, "right")))

  b:insert(raw(para(run(has(g("engagement_type")) and g("engagement_type") or "Penetration Test",
    { b = true, caps = true, color = ACCENT, sz = 16 }), center .. spacing(2140, 0))))
  b:insert(raw(rule:gsub('w:before="283"', 'w:before="443"')))
  b:insert(raw(para(run(g("title"), { b = true, color = PRIMARY, sz = 48 }),
    center .. spacing(223, 0, ' w:line="240" w:lineRule="auto"'))))
  if has(g("subtitle")) then
    b:insert(raw(para(run(g("subtitle"), { color = MUTE, sz = 26 }), center .. spacing(168, 0))))
  end
  b:insert(raw(rule))

  b:insert(raw(para(run("PREPARED BY", { color = MUTE, sz = 16 }), center .. spacing(946, 198))))
  local logo = has(g("company_logo")) and g("company_logo") or (m.logo and str(m.logo.path)) or ""
  if has(logo) then
    b:insert(image_para(logo, "1.6cm", "Cover Center"))
  else
    b:insert(raw(para(run(firm, { b = true, color = INK, sz = 32 }), center)))
  end
  if has(g("client")) then
    b:insert(raw(para(run("PREPARED FOR", { color = MUTE, sz = 16 }), center .. spacing(642, 107))))
    b:insert(raw(para(run(g("client"), { b = true, color = INK, sz = 26 }), center)))
  end
  b:insert(raw(para("", spacing(487, 0, ' w:line="20" w:lineRule="exact"'))))
  b:insert(raw(tbl({ 2000 }, { { cell(2000, para(run(has(g("confidentiality")) and g("confidentiality") or
    "Confidential", { b = true, caps = true, color = "FFFFFF" }), center), shd(ACCENT)) } },
    { jc = "center", mar = cell_mar(90, 80) })))

  b:insert(raw(framed(run("Version:", { b = true, color = MUTE, sz = 16 }) ..
    run("\u{00A0}" .. (has(g("version")) and g("version") or "1.0"), { color = MUTE, sz = 16 }) ..
    "<w:r><w:br/></w:r>" .. run("Date:", { b = true, color = MUTE, sz = 16 }) ..
    run("\u{00A0}" .. today(), { color = MUTE, sz = 16 }), 1.9, 25.19, 8.0, nil, 271)))

  -- Close the cover section: no header/footer art, no page number.
  b:insert(raw(para("", '<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="765" w:right="1077" w:bottom="907" w:left="1077" ' ..
    'w:header="0" w:footer="0" w:gutter="0"/></w:sectPr>' .. spacing(0, 0, ' w:line="20" w:lineRule="exact"'))))
  return b
end

-- Text widths from Helvetica's metrics, which Arial and unkerned TeX Gyre
-- Heros share (both outputs set them identically).
local HELVETICA = { -- advance widths (1/1000 em) for U+0020..U+007E
  278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
  556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
  1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
  667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
  333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556,
  556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
}

local function text_width(s, size)
  local w = 0
  for _, c in utf8.codes(s) do
    w = w + (HELVETICA[c - 31] or (c == 0x2014 and 1000) or (c == 0xA0 and 278) or 556)
  end
  return w * size / 1000
end

-- ── Confidentiality / Document Control page ──────────────────────────────────

local function front_matter(m)
  local g = function(k) return str(m[k]) end
  local firm = has(g("company")) and g("company") or "Report Rex"
  local client = has(g("client")) and g("client") or "the client"
  local b = pandoc.List()
  -- after: the gap down to a paragraph, or (smaller) to a table
  local function heading(text, before, after)
    b:insert(raw(para(run(text, { b = true, color = INK, sz = 23 }), spacing(before, after or 175))))
  end
  local function text(s) b:insert(pandoc.Para({ pandoc.Str(s) })) end

  heading("Confidentiality Notice", 139)
  text("This report contains sensitive and confidential information belonging to " .. client ..
    ". It is intended solely for the use of the client and its authorized personnel.")
  heading("Disclaimer", 534)
  text("The report\u{2019}s results and recommendations reflect the state of the systems and environment at the " ..
    "time of testing and may not account for any changes made after the assessment period. " .. firm ..
    " does not guarantee that the findings represent all potential security issues or that implementing " ..
    "the recommended actions will eliminate all risks.")

  heading("Document Control Details", 534, 144)
  local rows = {}
  local function add(k, v)
    if has(v) then
      rows[#rows + 1] = { cell(2350, para(run(k, { b = true, color = INK }))), cell(7000, para(run(v))) }
    end
  end
  add("Classification:", has(g("confidentiality")) and g("confidentiality") or "Confidential")
  if has(g("client")) then add("Authorized Recipient:", g("client")) end
  add("Prepared By:", firm)
  add("Date Submitted:", today())
  if has(g("contact_name")) then
    add("Point of Contact:", g("contact_name") .. (has(g("contact_email")) and (" <" .. g("contact_email") .. ">") or ""))
  end
  add("Contact Information:", g("contact_phone"))
  add("Report ID:", g("report_id"))
  if has(g("start_date")) then
    add("Testing Window:", g("start_date") .. (has(g("end_date")) and (" – " .. g("end_date")) or ""))
  end
  add("Version:", has(g("version")) and g("version") or "1.0")
  b:insert(raw(tbl({ 2350, 7000 }, rows, { mar = cell_mar(52, 100), ind = 100 })))

  if type(m.authors) == "table" and #m.authors > 0 then
    heading("Assessment Team", 496, 144)
    -- tblr's Q[l] columns are as wide as their text plus 2 x 6pt colsep
    -- (bold header text is ~10% wider than regular).
    local cols = {}
    for c, key in ipairs({ "name", "role" }) do
      local w = text_width(c == 1 and "Name" or "Role", 10) * 1.1
      for _, au in ipairs(m.authors) do w = math.max(w, text_width(str(au[key]), 10)) end
      cols[c] = math.floor((w + 13) * 20 + 0.5) -- +1pt: Word rounds glyph positions up
    end
    local mar = cell_mar(60, 120)
    local trs = { {
      cell(cols[1], para(run("Name", { b = true, color = "FFFFFF" })), shd(PRIMARY)),
      cell(cols[2], para(run("Role", { b = true, color = "FFFFFF" })), shd(PRIMARY)),
    } }
    for _, au in ipairs(m.authors) do
      trs[#trs + 1] = { cell(cols[1], para(run(str(au.name)))), cell(cols[2], para(run(str(au.role)))) }
    end
    b:insert(raw(tbl(cols, trs, { borders = grid_borders(), mar = mar, ind = 120 })))
  end
  return b
end

-- ── Table of contents ────────────────────────────────────────────────────────
-- Pre-rendered entries, each linking to its heading's bookmark, with a PAGEREF
-- field for the page number (marked dirty, so Word fills them in on open).

-- Level-2 entries get LaTeX's dotted leader: a "." every 8.22pt (4.5mu either
-- side) on a grid ending 463.3pt from the margin, starting at the first cell
-- that clears the title. Word's own dot leader is solid, so each dot sits on
-- its own tab stop. Title widths come from Helvetica's metrics, which Arial
-- and unkerned TeX Gyre Heros share.
local function leader_dots(title)
  local title_end, xs = 37.86 + text_width(title, 10), {}
  for k = 0, 60 do
    local x = 463.3 - k * 8.2245
    if x - 2.72 < title_end then break end
    table.insert(xs, 1, x)
  end
  return xs
end

local function toc_blocks(entries)
  local b = pandoc.List()
  b:insert(raw(para(run("Contents", { b = true, color = PRIMARY, sz = 33 }), "<w:pageBreakBefore/>" .. spacing(0, 25))))
  for n, e in ipairs(entries) do
    local o = e.level == 1 and { b = true } or {}
    local dots, tabs = "", ""
    if e.level == 2 then
      for _, dx in ipairs(leader_dots(e.title)) do
        dots = dots .. "<w:r><w:tab/></w:r>" .. run(".", o)
        tabs = tabs .. string.format('<w:tab w:val="left" w:pos="%d"/>', math.floor(dx * 20 + 0.5))
      end
      tabs = "<w:tabs>" .. tabs .. "</w:tabs>"
    end
    local x = '<w:hyperlink w:anchor="' .. xml(e.id) .. '" w:history="1">' ..
      run(e.num, o) .. "<w:r><w:tab/></w:r>" .. run(e.title, o) .. dots .. "<w:r><w:tab/></w:r>" ..
      field("PAGEREF " .. e.mark .. " \\h", "", o):gsub('fldCharType="begin"', 'fldCharType="begin" w:dirty="true"', 1) ..
      "</w:hyperlink>"
    -- the first entry sits closer under "Contents" than later groups do
    b:insert(raw(para(x, '<w:pStyle w:val="TOC' .. e.level .. '"/>' .. tabs ..
      (n == 1 and '<w:spacing w:before="135"/>' or ""))))
  end
  return b
end

-- ── Code blocks ──────────────────────────────────────────────────────────────
-- Written as raw OpenXML rather than left to pandoc, which (inside a list item)
-- gives the paragraph list numbering whose indent overrides the code box's.
-- Tokens come from pandoc's own highlighter (via its HTML output's classes)
-- in template.tex's tango colours. template.tex's fvextra wraps long lines
-- itself: at the last space that fits, else mid-token right at the edge;
-- continuations are 4 columns in behind a small →.
-- Those breaks are made here the same way, from the PDF's geometry: 8.5pt
-- Courier (5.1pt/char) in a box 467.5pt wide, 36pt narrower per list level.

local TANGO = { -- HTML class → { color, bold, italic }
  kw = { "204A87", true }, cf = { "204A87", true }, fu = { "204A87", true }, dt = { "204A87" },
  at = { "204A87" }, dv = { "0000CF" }, bn = { "0000CF" }, fl = { "0000CF" }, ch = { "4E9A06" },
  st = { "4E9A06" }, vs = { "4E9A06" }, ss = { "4E9A06" }, cn = { "8F5902" }, ot = { "8F5902" },
  co = { "8F5902", false, true }, pp = { "8F5902", false, true }, ["do"] = { "8F5902", true, true },
  an = { "8F5902", true, true }, cv = { "8F5902", true, true }, ["in"] = { "8F5902", true, true },
  wa = { "8F5902", true, true }, op = { "CE5C00", true }, sc = { "CE5C00", true },
  va = { "000000" }, al = { "EF2929" }, er = { "A40000", true },
}

local function unescape(t)
  return (t:gsub("&lt;", "<"):gsub("&gt;", ">"):gsub("&quot;", '"'):gsub("&#39;", "'"):gsub("&amp;", "&"))
end

-- Highlighted lines, each a list of { text, class } (class nil for plain).
local function highlight(cb)
  local html = pandoc.write(pandoc.Pandoc({ cb }), "html")
  local code = html:match("<code[^>]*>(.-)</code>") or ""
  local lines = {}
  for line in (code .. "\n"):gmatch("(.-)\n") do
    line = line:gsub('^<span id="[^"]*">', ""):gsub("</span>$", ""):gsub("<a [^>]*></a>", "")
    local toks, pos = {}, 1
    while true do
      local a, b, cls, txt = line:find('<span class="(%w+)">(.-)</span>', pos)
      if not a then break end
      if a > pos then toks[#toks + 1] = { unescape(line:sub(pos, a - 1)) } end
      toks[#toks + 1] = { unescape(txt), cls }
      pos = b + 1
    end
    if pos <= #line then toks[#toks + 1] = { unescape(line:sub(pos)) } end
    lines[#lines + 1] = toks
  end
  return lines
end

-- Break points for one line of `len` chars (fvextra's order of preference):
-- a list of { first, last } character ranges.
local function wrap_points(chars, cap)
  local len, out, start, first = #chars, {}, 1, true
  while len - start + 1 > (first and cap or cap - 4) do
    local c = first and cap or cap - 4
    local lead = start
    while lead <= len and chars[lead] == " " do lead = lead + 1 end
    local brk
    for i = math.min(start + c, len), lead + 1, -1 do
      if chars[i] == " " then brk = i break end
    end
    if brk then
      out[#out + 1] = { start, brk - 1 }
      start = brk + 1
    else
      out[#out + 1] = { start, start + c - 1 }
      start = start + c
    end
    first = false
  end
  out[#out + 1] = { start, len }
  return out
end

local function code_block(cb, depth)
  local ind = 226 + 720 * depth -- twips: list indent + the box's 11.3pt inset
  local cap = math.floor((467.5 - 36 * depth) / 5.1)
  local parts = {}
  for li, toks in ipairs(highlight(cb)) do
    -- per-character token class, so wrapped segments keep their colours
    local chars, cls = {}, {}
    for _, t in ipairs(toks) do
      for _, c in utf8.codes(t[1]) do
        chars[#chars + 1] = utf8.char(c)
        cls[#cls + 1] = t[2] or false
      end
    end
    if li > 1 then parts[#parts + 1] = "<w:r><w:br/></w:r>" end
    for si, seg in ipairs(wrap_points(chars, cap)) do
      if si > 1 then
        parts[#parts + 1] = "<w:r><w:br/></w:r><w:r><w:tab/></w:r>" ..
          run("\u{2192}", { sz = 11, sans = true }) .. "<w:r><w:tab/></w:r>"
      end
      local i = seg[1]
      while i <= seg[2] do
        local j = i
        while j < seg[2] and cls[j + 1] == cls[i] do j = j + 1 end
        local f = TANGO[cls[i]] or {}
        parts[#parts + 1] = run(table.concat(chars, "", i, j), { color = f[1], b = f[2], i = f[3] })
        i = j + 1
      end
    end
  end
  return raw(para(table.concat(parts), string.format(
    '<w:pStyle w:val="SourceCode"/><w:tabs><w:tab w:val="left" w:pos="%d"/><w:tab w:val="left" w:pos="%d"/>' ..
    '</w:tabs><w:ind w:left="%d" w:right="120"/>', ind + 85, ind + 408, ind)))
end

-- pandoc's LaTeX writer sets a list \tightlist only if every item starts with
-- Plain; otherwise all its items get paragraph spacing. The docx writer
-- decides per item (Plain → Compact), so loosen the whole list the same way.
-- A top-level list also ends with the PDF's list/paragraph gap.
-- Bullet lists are written as styled paragraphs with LaTeX's own label glyphs
-- (•, bold –, *, · by itemize level) in the embedded Liberation Sans: pandoc's
-- bullets are Symbol-font glyphs the PDF can't share. The RexBullet*/RexListText*
-- styles (reference.docx) carry the 18bp label / 36bp text indents per depth.
local BULLETS = { { "\u{2022}" }, { "\u{2013}", true }, { "*" }, { "\u{00B7}" } }

local function lists(blocks, depth, bullet_level)
  bullet_level = bullet_level or 0
  local out = pandoc.Blocks({})
  for _, b in ipairs(blocks) do
    if b.t == "BulletList" or b.t == "OrderedList" then
      local tight = true
      for _, item in ipairs(b.content) do tight = tight and item[1] ~= nil and item[1].t == "Plain" end
      local level = bullet_level + (b.t == "BulletList" and 1 or 0)
      local items = {}
      for k, item in ipairs(b.content) do
        item = lists(item, depth + 1, level)
        if not tight and item[1] and item[1].t == "Plain" then item[1] = pandoc.Para(item[1].content) end
        -- a code block ending a loose item: the PDF's item separation follows
        if not tight and #item > 1 and item[#item].t == "RawBlock" and item[#item].text:find('"SourceCode"') then
          item:insert(raw(para("", spacing(0, 0, ' w:line="25" w:lineRule="exact"'))))
        end
        items[k] = item
      end
      local start = #out + 1
      if b.t == "BulletList" then
        local d = math.min(depth + 1, 4)
        local label = BULLETS[math.min(level, 4)]
        for _, item in ipairs(items) do
          for j, blk in ipairs(item) do
            if j == 1 and (blk.t == "Plain" or blk.t == "Para") then
              blk.content:insert(1, pandoc.RawInline("openxml", run(label[1], { b = label[2] }) .. "<w:r><w:tab/></w:r>"))
              out:insert(pandoc.Div({ blk }, { ["custom-style"] = "RexBullet" .. d .. (tight and "Tight" or "Loose") }))
            elseif blk.t == "Plain" or blk.t == "Para" then
              out:insert(pandoc.Div({ blk }, { ["custom-style"] = "RexListText" .. d }))
            else
              out:insert(blk)
            end
          end
        end
      else
        b.content = items
        out:insert(b)
      end
      if depth == 0 then out:insert(raw(para("", spacing(118, 0, ' w:line="20" w:lineRule="exact"')))) end
      -- a loose top-level list opens with LaTeX's list topsep
      if depth == 0 and not tight then
        out:insert(start, raw(para("", spacing(7, 0, ' w:line="20" w:lineRule="exact"'))))
      end
    else
      out:insert(b)
    end
  end
  return out
end

-- Table charts (tabularray, image class .tblr) get the tighter FigureTable
-- spacing; variants for a chart right under a heading (U) or followed by one.
-- Captioned figures are written as their image and caption paragraphs, so a
-- caption followed by another figure can take the PDF's larger gap.
local function heading_level(b)
  if b == nil then return nil end
  if b.t == "Header" then return b.level end
  if b.t == "RawBlock" then return tonumber(b.text:match('"Heading(%d)"')) end
end

local function chart_styles(blocks)
  local out = pandoc.Blocks({})
  for k, b in ipairs(blocks) do
    if b.t == "Figure" and #b.content == 1 and b.content[1].t == "Plain" then
      local nxt = blocks[k + 1]
      out:insert(pandoc.Div({ pandoc.Para(b.content[1].content) }, { ["custom-style"] = "Captioned Figure" }))
      out:insert(pandoc.Div({ pandoc.Para(pandoc.utils.blocks_to_inlines(b.caption.long)) },
        { ["custom-style"] = (nxt and nxt.t == "Figure") and "ImageCaptionF" or "Image Caption" }))
    else
      out:insert(b)
    end
  end
  blocks = out
  for k, d in ipairs(blocks) do
    if d.t == "Div" and d.attributes["custom-style"] == "Figure" then
      local tblr = false
      d:walk({ Image = function(img) tblr = tblr or img.classes:includes("tblr") end })
      local style = (tblr and "FigureTable" or "Figure") .. (heading_level(blocks[k - 1]) and "U" or "")
      local lvl = heading_level(blocks[k + 1])
      if lvl == 2 or lvl == 3 then style = style .. "H" .. lvl end
      d.attributes["custom-style"] = style
      blocks[k] = d
    end
  end
  return blocks
end

local function code_blocks(blocks, depth)
  local out = pandoc.Blocks({})
  for _, b in ipairs(blocks) do
    if b.t == "CodeBlock" then
      b = code_block(b, depth)
    elseif b.t == "BulletList" or b.t == "OrderedList" then
      local items = {}
      for k, item in ipairs(b.content) do items[k] = code_blocks(item, depth + 1) end
      b.content = items
    elseif b.t == "Div" or b.t == "BlockQuote" then
      b.content = code_blocks(b.content, depth)
    end
    out:insert(b)
  end
  return out
end

-- ── Entry point ──────────────────────────────────────────────────────────────

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

-- Inline code is \texttt at body size (~9.3pt); VerbatimChar, which pandoc's
-- highlighted code-block tokens inherit, carries the smaller block size. The
-- font is direct too: LibreOffice lets a table style override VerbatimChar's.
local function inline_code(el)
  return pandoc.RawInline("openxml", '<w:r><w:rPr><w:rStyle w:val="VerbatimChar"/>' ..
    '<w:rFonts w:ascii="Liberation Mono" w:hAnsi="Liberation Mono" w:cs="Liberation Mono"/><w:sz w:val="19"/>' ..
    '<w:szCs w:val="19"/></w:rPr><w:t xml:space="preserve">' .. xml(breakable(el.text, "[/.:=]")) .. "</w:t></w:r>")
end

-- preprocessor.py's `\clearpage` between sections reaches the filter as a
-- paragraph holding a single raw TeX inline (or as a raw block).
local function is_clearpage(b)
  local r = b
  if b.t == "Para" and #b.content == 1 then r = b.content[1] end
  return (r.t == "RawBlock" or r.t == "RawInline") and (r.format == "tex" or r.format == "latex")
    and (r.text:match("^%s*\\clearpage%s*$") or r.text:match("^%s*\\newpage%s*$")) ~= nil
end

function Pandoc(doc)
  doc = doc:walk({ Inlines = Inlines })
  load_colors(doc.meta)
  local m = doc.meta
  -- Read by the page footer's DOCPROPERTY field (pandoc writes unknown
  -- metadata as custom document properties).
  if not has(str(m.confidentiality)) then m.confidentiality = "Confidential" end

  local function is_finding(i)
    local b, nxt = doc.blocks[i], doc.blocks[i + 1]
    return b.t == "Header" and b.identifier:match("^finding%-%d+$") ~= nil and nxt ~= nil and nxt.t == "Div"
      and nxt.classes:includes("vulnerability")
  end
  local summary = {}
  for i, b in ipairs(doc.blocks) do
    if is_finding(i) then
      local a = doc.blocks[i + 1].attributes
      summary[#summary + 1] = {
        id = has(a.finding) and a.finding or tostring(#summary + 1), anchor = b.identifier,
        title = pandoc.utils.stringify(b.content), sev = a.severity or "Info", cvss = has(a.cvss) and a.cvss or "—",
      }
    end
  end

  local blocks, toc = pandoc.List(), {}
  local n1, n2 = 0, 0
  -- A \clearpage is held until the next block: a heading then starts the new
  -- page itself (pageBreakBefore, no space above it, as LaTeX drops it there).
  local brk = has(str(m.title)) -- the front matter ends on a page of its own
  for i, b in ipairs(doc.blocks) do
    if is_finding(i) and summary[1].anchor == b.identifier then
      blocks:extend(summary_table(summary))
      brk = true
    end
    if b.t == "Header" and b.level <= 2 and not b.classes:includes("unnumbered") then
      local num
      if b.level == 1 then
        n1, n2 = n1 + 1, 0
        num = tostring(n1)
      else
        n2 = n2 + 1
        num = n1 .. "." .. n2
      end
      -- A hidden bookmark on the heading line itself for the TOC's PAGEREF
      -- (pandoc's own bookmark spans the whole section).
      local mark = "_RexToc" .. (#toc + 1)
      local title = pandoc.utils.stringify(b.content)
      toc[#toc + 1] = { level = b.level, num = num, title = title, id = b.identifier, mark = mark }
      local bm = function(id, name)
        return string.format('<w:bookmarkStart w:id="%d" w:name="%s"/><w:bookmarkEnd w:id="%d"/>', id, xml(name), id)
      end
      -- After a page break, a heading starts the page with no space above
      -- it; a section heading directly under a chapter one sits closer than
      -- under text (Word spaces paragraphs by the larger of after/before).
      local prev = blocks[#blocks]
      local under_h1 = b.level == 2 and prev ~= nil and ((prev.t == "Header" and prev.level == 1)
        or (prev.t == "RawBlock" and prev.text:find('"Heading1"') ~= nil))
      if brk or under_h1 then
        blocks:insert(raw(para(bm(80000 + #toc, b.identifier) .. bm(90000 + #toc, mark) .. run(num .. "\u{2002}" .. title),
          '<w:pStyle w:val="Heading' .. b.level .. '"/>' .. (brk and '<w:pageBreakBefore/><w:spacing w:before="0"/>'
            or '<w:spacing w:before="89"/>'))))
      else
        b.content:insert(1, pandoc.RawInline("openxml", bm(90000 + #toc, mark)))
        b.content:insert(2, pandoc.Str(num .. "\u{2002}"))
        blocks:insert(b)
      end
      brk = false
    elseif is_clearpage(b) then
      brk = true
    else
      if brk then blocks:insert(raw(PAGE_BREAK)) end
      brk = false
      if b.t == "Div" and b.classes:includes("vulnerability") then
        blocks:extend(build_vuln_box(b))
      else
        blocks:insert(b)
      end
    end
  end

  -- Plain tables span the text width, split like vuln_filter.lua's X[coef]
  -- columns: coef = width x 12 rounded (min 1), or 1 each without widths.
  -- tabularray shares out the width left after each column's 2 x 6pt colsep
  -- and the 0.4pt rules between them (which Word draws without taking room).
  blocks = pandoc.Blocks(blocks):walk({
    Table = function(t)
      local coefs, sum = {}, 0
      for k, cs in ipairs(t.colspecs) do
        coefs[k] = (cs[2] and cs[2] > 0) and math.max(1, math.floor(cs[2] * 12 + 0.5)) or 1
        sum = sum + coefs[k]
      end
      local any = false
      for _, cs in ipairs(t.colspecs) do any = any or (cs[2] and cs[2] > 0) end
      local W, n, r = TEXT_W / 20, #coefs, 0.4
      for k, cs in ipairs(t.colspecs) do
        local share = any and coefs[k] / sum or 1 / n
        t.colspecs[k] = { cs[1], (r + 12 + share * (W - 12 * n - (n + 1) * r)) / W }
      end
      return t
    end,
  })
  -- vuln_filter.lua's cells allow a break after every . / : (hostnames, URLs).
  blocks = pandoc.Blocks(blocks):walk({
    Table = function(t)
      return t:walk({ Str = function(s) return pandoc.Str(breakable(s.text, "[./:]")) end })
    end,
  })
  blocks = pandoc.Blocks(blocks):walk({ Code = inline_code })
  -- code blocks first: they need the list nesting that lists() flattens
  blocks = chart_styles(lists(code_blocks(blocks, 0), 0))
  -- The PDF's center environment (8pt) and parskip around every table; less
  -- above one that directly follows a heading (titlesec's smaller after-skip).
  local spaced = pandoc.Blocks({})
  for k, t in ipairs(blocks) do
    if t.t == "Table" then
      local prev = blocks[k - 1]
      local lvl = heading_level(prev)
      spaced:insert(raw(para("", spacing(lvl == 2 and 223 or lvl and 159 or 255, 0, ' w:line="20" w:lineRule="exact"'))))
      spaced:insert(t)
      -- space *after* (Word takes the larger of it and a heading's before)
      spaced:insert(raw(para("", spacing(0, 325, ' w:line="20" w:lineRule="exact"'))))
    else
      spaced:insert(t)
    end
  end
  blocks = spaced

  if has(str(m.title)) then
    local front = cover_blocks(m)
    front:extend(front_matter(m))
    if #toc > 0 then front:extend(toc_blocks(toc)) end
    front:extend(blocks)
    blocks = front
    m.title, m.subtitle, m.date = nil, nil, nil
  end
  doc.blocks = blocks
  doc.meta = m
  return doc
end
