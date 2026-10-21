"""Regenerate templates/<name>/reference.docx — the branded style sheet pandoc's
DOCX writer uses (`--reference-doc`) for everything the template's
docx_vuln_filter.lua doesn't emit as raw OpenXML: body text, headings, plain
tables, inline code, links.

Starts from pandoc's own default reference.docx and patches its styles with
the template's theme.yaml colors, swapping the Office-only theme fonts
(Aptos/Consolas) for Arial/Courier New so Pages/LibreOffice don't report
missing fonts. Rerun after changing a template's theme.yaml colors:

    python3 core/scripts/make_reference_docx.py
"""

import io
import re
import subprocess
import uuid
import zipfile
from pathlib import Path

import yaml

TEMPLATES = Path(__file__).resolve().parents[2] / "templates"
SANS, MONO = "Arial", "Courier New"
GRID, BAND = "D0D0D0", "F5F7FA"


def _style(xml: str, style_id: str, fn) -> str:
    pat = re.compile(rf'<w:style [^>]*w:styleId="{style_id}".*?</w:style>', re.S)
    return pat.sub(lambda m: fn(m.group(0)), xml, count=1)


def _table_style(primary: str) -> str:
    border = "".join(
        f'<w:{side} w:val="single" w:sz="4" w:space="0" w:color="{GRID}"/>'
        for side in ("top", "left", "bottom", "right", "insideH", "insideV")
    )
    margins = (
        '<w:tblCellMar><w:top w:w="40" w:type="dxa"/><w:left w:w="108" w:type="dxa"/>'
        '<w:bottom w:w="40" w:type="dxa"/><w:right w:w="108" w:type="dxa"/></w:tblCellMar>'
    )
    return (
        '<w:style w:type="table" w:default="1" w:styleId="Table">'
        '<w:name w:val="Table"/><w:basedOn w:val="TableNormal"/><w:qFormat/>'
        f'<w:tblPr><w:tblStyleRowBandSize w:val="1"/><w:tblInd w:w="0" w:type="dxa"/>'
        f"<w:tblBorders>{border}</w:tblBorders>{margins}</w:tblPr>"
        '<w:tblStylePr w:type="firstRow"><w:rPr><w:b/><w:color w:val="FFFFFF"/></w:rPr>'
        f'<w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="{primary}"/></w:tcPr></w:tblStylePr>'
        '<w:tblStylePr w:type="band2Horz">'
        f'<w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="{BAND}"/></w:tcPr></w:tblStylePr>'
        "</w:style>"
    )


def patch_styles(xml: str, primary: str, accent: str) -> str:
    # Theme-font references (→ Aptos) become explicit, universally installed fonts.
    xml = re.sub(
        r"<w:rFonts [^>]*Theme=[^>]*/>",
        f'<w:rFonts w:ascii="{SANS}" w:hAnsi="{SANS}" w:eastAsia="{SANS}" w:cs="{SANS}"/>',
        xml,
    )
    xml = xml.replace('"Consolas"', f'"{MONO}"')
    # Headings (and Title, used by the one-pager templates): bold, brand primary.
    xml = re.sub(
        r'<w:color w:val="0F4761" w:themeColor="accent1"\s+w:themeShade="BF" />',
        f'<w:b/><w:color w:val="{primary}"/>',
        xml,
    )
    xml = _style(xml, "Title", lambda s: s.replace("<w:sz ", f'<w:b/><w:color w:val="{primary}"/><w:sz ', 1))
    xml = _style(
        xml, "Hyperlink",
        lambda s: re.sub(r"<w:color [^>]*/>", f'<w:color w:val="{accent}"/>', s),
    )
    xml = _style(
        xml, "VerbatimChar",
        lambda s: s.replace("</w:rPr>", f'<w:shd w:val="clear" w:color="auto" w:fill="{BAND}"/></w:rPr>'),
    )
    return _style(xml, "Table", lambda _: _table_style(primary))


# ── rit-cptc: page geometry, styles and page art matching template.tex ────────
# Lengths: twips (1/20 pt) for page/paragraph, EMU for drawings, half-points
# for font sizes. The PDF's TeX Gyre Heros at Scale=0.92 on an 11pt class is
# ~10pt Arial; \LARGE/\large/\small/\footnotesize scale the same way.

PAGE_W, PAGE_H = 21.0, 29.7  # cm, A4
TEXT_W = 9751  # twips: 21cm - 2 x 1.9cm
EMU = 360000  # per cm
INK, MUTE, CODE_BG, CODE_FRAME = "1C2333", "6B7280", "F5F7FA", "DEE3EA"
W_NS = (
    'xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" '
    'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" '
    'xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" '
    'xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" '
    'xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"'
)
BODY_HEADER_RID, BODY_FOOTER_RID = "rIdRexHeader", "rIdRexFooter"


def mix(color: str, pct: float, base: str = "FFFFFF") -> str:
    """xcolor's `color!pct!base`: pct% of color blended into base."""
    c, b = (bytes.fromhex(x) for x in (color, base))
    return "".join(f"{round(x * pct / 100 + y * (100 - pct) / 100):02X}" for x, y in zip(c, b))


def _shape(sid: int, x: float, y: float, pts: list, fill: str, text: str = "") -> str:
    """A page-anchored, behind-text polygon (cm, page coordinates). With `text`,
    it is a transparent text box instead, the polygon's bounding box."""
    xs, ys = [p[0] for p in pts], [p[1] for p in pts]
    w, h = max(xs), max(ys)
    path = "".join(
        f'<a:{"moveTo" if i == 0 else "lnTo"}><a:pt x="{round(px * EMU)}" y="{round(py * EMU)}"/>'
        f'</a:{"moveTo" if i == 0 else "lnTo"}>'
        for i, (px, py) in enumerate(pts)
    )
    if text:
        body = (f"<a:prstGeom prst=\"rect\"><a:avLst/></a:prstGeom><a:noFill/><a:ln><a:noFill/></a:ln></wps:spPr>"
                f"<wps:txbx><w:txbxContent>{text}</w:txbxContent></wps:txbx>"
                '<wps:bodyPr lIns="0" tIns="0" rIns="0" bIns="0" anchor="ctr"><a:noAutofit/></wps:bodyPr>')
    else:
        body = (f'<a:custGeom><a:avLst/><a:gdLst/><a:ahLst/><a:cxnLst/><a:rect l="0" t="0" r="r" b="b"/>'
                f'<a:pathLst><a:path w="{round(w * EMU)}" h="{round(h * EMU)}">{path}<a:close/></a:path></a:pathLst>'
                f'</a:custGeom><a:solidFill><a:srgbClr val="{fill}"/></a:solidFill><a:ln><a:noFill/></a:ln>'
                "</wps:spPr><wps:bodyPr/>")
    return (
        f'<w:r><w:drawing><wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" '
        f'relativeHeight="{sid}" behindDoc="{0 if text else 1}" locked="1" layoutInCell="1" allowOverlap="1">'
        '<wp:simplePos x="0" y="0"/>'
        f'<wp:positionH relativeFrom="page"><wp:posOffset>{round(x * EMU)}</wp:posOffset></wp:positionH>'
        f'<wp:positionV relativeFrom="page"><wp:posOffset>{round(y * EMU)}</wp:posOffset></wp:positionV>'
        f'<wp:extent cx="{round(w * EMU)}" cy="{round(h * EMU)}"/><wp:effectExtent l="0" t="0" r="0" b="0"/>'
        f'<wp:wrapNone/><wp:docPr id="{sid}" name="RexArt{sid}"/><wp:cNvGraphicFramePr/>'
        '<a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape">'
        f'<wps:wsp><wps:cNvSpPr/><wps:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="{round(w * EMU)}" '
        f'cy="{round(h * EMU)}"/></a:xfrm>{body}</wps:wsp></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r>'
    )


def _rect(sid: int, x: float, y: float, w: float, h: float, fill: str) -> str:
    return _shape(sid, x, y, [(0, 0), (w, 0), (w, h), (0, h)], fill)


def _hdrftr(tag: str, runs: str) -> str:
    # One tiny paragraph carrying the page-anchored art, so the header/footer
    # itself takes (almost) no room from the page margins.
    return (f'<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:{tag} {W_NS}>'
            '<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="20" w:lineRule="exact"/>'
            f'<w:rPr><w:sz w:val="2"/></w:rPr></w:pPr>{runs}</w:p></w:{tag}>')


def _field(instr: str, cached: str) -> str:
    return (f'<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> {instr} '
            f'</w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>{cached}</w:t></w:r>'
            '<w:r><w:fldChar w:fldCharType="end"/></w:r>')


def _rit_cptc_art(primary: str, accent: str) -> dict:
    """header/footer parts: every body page's folded corner + bottom
    classification banner (template.tex \\rrPageDecor). The cover's bands are
    drawn by the docx filter, in the cover's own section."""
    W, H = PAGE_W, PAGE_H
    corner = _hdrftr("hdr", "".join([
        _shape(1, W - 2.9, 0, [(0, 0), (2.9, 0), (2.9, 2.9)], "ECEEF1"),
        _shape(2, W - 1.7, 0, [(0, 0), (1.7, 0), (1.7, 1.7)], "C9CDD4"),
    ]))
    label = (
        '<w:p><w:pPr><w:jc w:val="right"/><w:spacing w:before="0" w:after="0"/>'
        '<w:rPr><w:b/><w:caps/><w:color w:val="FFFFFF"/><w:sz w:val="16"/></w:rPr></w:pPr>'
        + _field("DOCPROPERTY confidentiality", "Confidential").replace(
            "<w:r>", '<w:r><w:rPr><w:b/><w:caps/><w:color w:val="FFFFFF"/><w:sz w:val="16"/></w:rPr>')
        + '<w:r><w:rPr><w:b/><w:color w:val="FFFFFF"/><w:sz w:val="16"/></w:rPr>'
          '<w:t xml:space="preserve"> | </w:t></w:r>'
        + _field("PAGE", "1").replace(
            "<w:r>", '<w:r><w:rPr><w:b/><w:color w:val="FFFFFF"/><w:sz w:val="16"/></w:rPr>')
        + "</w:p>"
    )
    banner = _hdrftr("ftr", "".join([
        _rect(3, 0, H - 0.32, W, 0.32, primary),
        _shape(4, W - 5.6, H - 1.35, [(1.2, 0), (5.6, 0), (5.6, 1.03), (0, 1.03)], accent),
        _shape(5, W - 1.1, H - 1.35, [(0, 0), (1.1, 0), (1.1, 0.8)], mix(primary, 85, "000000")),
        _shape(6, W - 6.35, H - 1.12, [(0, 0), (5.0, 0), (5.0, 0.6), (0, 0.6)], "", label),
    ]))
    return {"word/header1.xml": corner, "word/footer1.xml": banner}


def _set_style(xml: str, style: str) -> str:
    """Replace the style with `style`'s styleId, or append it."""
    sid = re.search(r'w:styleId="([^"]+)"', style).group(1)
    pat = re.compile(rf'<w:style [^>]*w:styleId="{sid}".*?</w:style>', re.S)
    return pat.sub(lambda _: style, xml, count=1) if pat.search(xml) else xml.replace("</w:styles>", style + "</w:styles>")


def _pstyle(sid: str, name: str, ppr: str = "", rpr: str = "", based: str = "Normal", extra: str = "") -> str:
    return (f'<w:style w:type="paragraph" w:customStyle="1" w:styleId="{sid}"><w:name w:val="{name}"/>'
            f'<w:basedOn w:val="{based}"/>{extra}<w:qFormat/><w:pPr>{ppr}</w:pPr><w:rPr>{rpr}</w:rPr></w:style>')


def _rit_cptc_styles(xml: str, primary: str, accent: str) -> str:
    grid, band = mix(primary, 30), mix(primary, 6)
    font = f'<w:rFonts w:ascii="{SANS}" w:hAnsi="{SANS}" w:eastAsia="{SANS}" w:cs="{SANS}"/>'
    mono = f'<w:rFonts w:ascii="{MONO}" w:hAnsi="{MONO}" w:eastAsia="{MONO}" w:cs="{MONO}"/>'
    xml = re.sub(r'(<w:rPrDefault>\s*<w:rPr>.*?)<w:sz w:val="24" />\s*<w:szCs w:val="24" />',
                 r'\g<1><w:sz w:val="20"/><w:szCs w:val="20"/>', xml, flags=re.S)
    xml = re.sub(r'(<w:pPrDefault>\s*<w:pPr>\s*)<w:spacing w:after="200" />',
                 r'\1<w:spacing w:after="0" w:line="283" w:lineRule="auto"/>', xml)

    def heading(lvl, sz, ppr):
        return _pstyle(
            f"Heading{lvl}", f"heading {lvl}",
            f'<w:keepNext/><w:keepLines/>{ppr}<w:outlineLvl w:val="{lvl - 1}"/>',
            f'{font}<w:b/><w:bCs/><w:color w:val="{primary}"/><w:sz w:val="{sz}"/><w:szCs w:val="{sz}"/>',
            extra='<w:next w:val="BodyText"/>').replace(' w:customStyle="1"', "")

    tbl_border = "".join(f'<w:{s} w:val="single" w:sz="4" w:space="0" w:color="{grid}"/>'
                         for s in ("top", "left", "bottom", "right", "insideH", "insideV"))
    code_border = "".join(f'<w:{s} w:val="single" w:sz="5" w:space="{sp}" w:color="{CODE_FRAME}"/>'
                          for s, sp in (("top", 5), ("left", 8), ("bottom", 6), ("right", 8)))
    for style in [
        # Unstyled paragraphs (pandoc's loose-list items) get parskip too.
        '<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/>'
        '<w:pPr><w:spacing w:after="136"/></w:pPr></w:style>',
        # Text lines sit on TeX's fixed 13.55bp baselineskip, whatever fonts
        # (inline code) a line holds.
        _pstyle("BodyText", "Body Text", '<w:jc w:val="both"/><w:spacing w:before="0" w:after="136" w:line="271" w:lineRule="exact"/>',
                extra='<w:link w:val="BodyTextChar"/>').replace(' w:customStyle="1"', ""),
        _pstyle("Compact", "Compact", '<w:jc w:val="left"/><w:spacing w:before="0" w:after="40" w:line="271" w:lineRule="exact"/>', based="BodyText"),
        heading(1, 33, '<w:spacing w:before="0" w:after="40"/>'),
        heading(2, 23, '<w:spacing w:before="179" w:after="64" w:line="279" w:lineRule="exact"/>'),
        heading(3, 21, f'<w:pBdr><w:left w:val="single" w:sz="24" w:space="6" w:color="{accent}"/></w:pBdr>'
                       '<w:spacing w:before="123" w:after="2"/><w:ind w:left="150"/>'),
        heading(4, 20, '<w:spacing w:before="200" w:after="60"/>'),
        _pstyle("SourceCode", "Source Code",
                f'<w:pBdr>{code_border}</w:pBdr><w:shd w:val="clear" w:color="auto" w:fill="{CODE_BG}"/>'
                '<w:wordWrap w:val="0"/><w:spacing w:before="157" w:after="160" w:line="234" w:lineRule="exact"/>'
                '<w:ind w:left="226" w:right="120"/>',
                f'{mono}<w:sz w:val="17"/>', extra='<w:link w:val="VerbatimChar"/>'),
        '<w:style w:type="character" w:customStyle="1" w:styleId="VerbatimChar"><w:name w:val="Verbatim Char"/>'
        f'<w:basedOn w:val="BodyTextChar"/><w:rPr>{mono}<w:sz w:val="17"/></w:rPr></w:style>',
        '<w:style w:type="character" w:styleId="Hyperlink"><w:name w:val="Hyperlink"/>'
        f'<w:basedOn w:val="BodyTextChar"/><w:rPr><w:color w:val="{accent}"/></w:rPr></w:style>',
        _pstyle("Figure", "Figure", '<w:jc w:val="center"/><w:spacing w:before="249" w:after="249" w:line="240" w:lineRule="auto"/>'),
        # tabularray charts sit tighter in the PDF than TikZ ones
        _pstyle("FigureTable", "FigureTable", '<w:spacing w:before="196" w:after="196"/>', based="Figure"),
        # followed by a heading: LaTeX stacks the heading's space on the chart's
        # variants by neighbours: right under a heading (U: no parskip above,
        # as in LaTeX), and followed by an H2/H3 (its space stacks on the chart's)
        *[_pstyle(f"{base}{under}{nxt}", f"{base}{under}{nxt}",
                  f'<w:spacing w:before="{before - (95 if under else 0)}" w:after="{after + extra}"/>', based=base)
          for base, before, after in (("Figure", 249, 249), ("FigureTable", 196, 196))
          for under in ("", "U") for nxt, extra in (("", 0), ("H2", 83), ("H3", 66)) if under or nxt],
        _pstyle("ImageCaptionF", "ImageCaptionF", '<w:spacing w:after="501"/>', based="ImageCaption"),
        _pstyle("CaptionedFigure", "Captioned Figure", '<w:keepNext/><w:spacing w:before="230" w:after="0" w:line="240" w:lineRule="auto"/>',
                based="Figure"),
        _pstyle("ImageCaption", "Image Caption", '<w:jc w:val="center"/><w:spacing w:before="100" w:after="440"/>',
                '<w:i/><w:iCs/><w:sz w:val="18"/>'),
        _pstyle("FootnoteText", "footnote text", '<w:spacing w:after="0" w:line="219" w:lineRule="exact"/><w:ind w:left="220"/>',
                '<w:sz w:val="16"/>').replace(' w:customStyle="1"', ""),
        _pstyle("TOC1", "toc 1", f'<w:tabs><w:tab w:val="left" w:pos="300"/><w:tab w:val="right" w:pos="{TEXT_W}"/>'
                '</w:tabs><w:spacing w:before="201" w:after="0"/>',
                f'<w:b/><w:color w:val="{primary}"/>').replace(' w:customStyle="1"', ""),
        _pstyle("TOC2", "toc 2", f'<w:tabs><w:tab w:val="left" w:pos="760"/>'
                f'<w:tab w:val="right" w:pos="{TEXT_W}"/></w:tabs>'
                '<w:spacing w:before="0" w:after="0"/><w:ind w:left="300"/>',
                f'<w:color w:val="{primary}"/>').replace(' w:customStyle="1"', ""),
        *[_pstyle(f"RexBullet{d}{kind}", f"RexBullet{d}{kind}",
                  f'<w:jc w:val="{jc}"/><w:spacing w:before="0" w:after="{after}" w:line="271" w:lineRule="exact"/>'
                  f'<w:ind w:left="{720 * d}" w:hanging="360"/>')
          for d in range(1, 5) for kind, after, jc in (("Tight", 40, "left"), ("Loose", 136, "both"))],
        *[_pstyle(f"RexListText{d}", f"RexListText{d}",
                  '<w:jc w:val="both"/><w:spacing w:before="0" w:after="136" w:line="271" w:lineRule="exact"/>'
                  f'<w:ind w:left="{720 * d}"/>') for d in range(1, 5)],
        _pstyle("CoverCenter", "Cover Center", '<w:jc w:val="center"/><w:spacing w:before="0" w:after="0" w:line="240" w:lineRule="auto"/>'),
        _pstyle("CoverBand", "Cover Band", '<w:spacing w:before="0" w:after="0" w:line="240" w:lineRule="auto"/>'),
        '<w:style w:type="table" w:default="1" w:styleId="Table"><w:name w:val="Table"/>'
        '<w:basedOn w:val="TableNormal"/><w:qFormat/><w:tblPr><w:tblStyleRowBandSize w:val="1"/>'
        f'<w:tblInd w:w="128" w:type="dxa"/><w:tblBorders>{tbl_border}</w:tblBorders><w:tblCellMar>'
        '<w:top w:w="60" w:type="dxa"/><w:left w:w="120" w:type="dxa"/><w:bottom w:w="20" w:type="dxa"/>'
        '<w:right w:w="120" w:type="dxa"/></w:tblCellMar></w:tblPr>'
        '<w:tblStylePr w:type="firstRow"><w:rPr><w:b/><w:color w:val="FFFFFF"/></w:rPr>'
        f'<w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="{primary}"/></w:tcPr></w:tblStylePr>'
        '<w:tblStylePr w:type="band1Horz">'
        f'<w:tcPr><w:shd w:val="clear" w:color="auto" w:fill="{band}"/></w:tcPr></w:tblStylePr></w:style>',
    ]:
        xml = _set_style(xml, style)
    return xml


# Embedded fonts: template.tex typesets in Liberation Sans/Mono (shipped in
# assets/fonts), and embedding the very same files makes Word and LibreOffice
# draw the DOCX with identical glyphs, installed or not.
FONT_FACES = {"Regular": "embedRegular", "Bold": "embedBold", "Italic": "embedItalic", "BoldItalic": "embedBoldItalic"}


def _embed_fonts(files: dict, font_dir: Path, families: tuple) -> None:
    rel_ns = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
    fonts, rels = [], []
    for family in families:
        embeds = []
        for face, tag in FONT_FACES.items():
            ttf = font_dir / f"{family.replace(' ', '')}-{face}.ttf"
            # Word's font obfuscation: XOR the first 32 bytes with the GUID key
            # (deterministic per file, so regenerating is byte-stable).
            guid = str(uuid.uuid5(uuid.NAMESPACE_URL, ttf.name)).upper()
            key = bytes.fromhex(guid.replace("-", ""))[::-1]
            data = bytearray(ttf.read_bytes())
            for i in range(32):
                data[i] ^= key[i % 16]
            rid, part = f"rIdRexFont{len(rels) + 1}", f"fonts/{ttf.stem}.odttf"
            files[f"word/{part}"] = bytes(data)
            rels.append(f'<Relationship Id="{rid}" Type="{rel_ns}/font" Target="{part}"/>')
            embeds.append(f'<w:{tag} r:id="{rid}" w:fontKey="{{{guid}}}"/>')
        fonts.append(f'<w:font w:name="{family}">{"".join(embeds)}</w:font>')
    table = files["word/fontTable.xml"]
    if "xmlns:r=" not in table[: table.index(">", table.index("<w:fonts")) + 1]:
        table = table.replace("<w:fonts ", f'<w:fonts xmlns:r="{rel_ns}" ', 1)
    files["word/fontTable.xml"] = table.replace("</w:fonts>", "".join(fonts) + "</w:fonts>")
    files["word/_rels/fontTable.xml.rels"] = (
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships '
        f'xmlns="http://schemas.openxmlformats.org/package/2006/relationships">{"".join(rels)}</Relationships>')
    files["[Content_Types].xml"] = files["[Content_Types].xml"].replace(
        "</Types>", '<Default Extension="odttf" ContentType="application/vnd.openxmlformats-officedocument.obfuscatedFont"/>'
        "</Types>")
    files["word/settings.xml"] = re.sub(r"(<w:settings[^>]*>)", r"\1<w:embedTrueTypeFonts/>", files["word/settings.xml"],
                                        count=1)


def _rit_cptc(files: dict, primary: str, accent: str) -> None:
    files.update(_rit_cptc_art(primary, accent))
    files["word/styles.xml"] = _rit_cptc_styles(files["word/styles.xml"], primary, accent)
    for part in ("word/styles.xml", "word/theme/theme1.xml"):
        files[part] = files[part].replace(f'"{SANS}"', '"Liberation Sans"').replace(f'"{MONO}"', '"Liberation Mono"')
    _embed_fonts(files, TEMPLATES / "rit-cptc" / "assets" / "fonts", ("Liberation Sans", "Liberation Mono"))
    rel = 'http://schemas.openxmlformats.org/officeDocument/2006/relationships/'
    files["word/_rels/document.xml.rels"] = files["word/_rels/document.xml.rels"].replace("</Relationships>", "".join(
        f'<Relationship Type="{rel}{kind}" Id="{rid}" Target="{target}"/>'
        for kind, rid, target in (("header", BODY_HEADER_RID, "header1.xml"), ("footer", BODY_FOOTER_RID, "footer1.xml"))) + "</Relationships>")
    ct = "application/vnd.openxmlformats-officedocument.wordprocessingml."
    files["[Content_Types].xml"] = files["[Content_Types].xml"].replace("</Types>", "".join(
        f'<Override PartName="/word/{p}.xml" ContentType="{ct}{k}+xml"/>'
        for p, k in (("header1", "header"), ("footer1", "footer"))) + "</Types>")
    # Body pages: A4 with template.tex's geometry, numbered from 1 (the cover is
    # its own unnumbered section, closed by the docx filter).
    files["word/document.xml"] = re.sub(r"<w:sectPr>.*?</w:sectPr>", (
        f'<w:sectPr><w:headerReference w:type="default" r:id="{BODY_HEADER_RID}"/>'
        f'<w:footerReference w:type="default" r:id="{BODY_FOOTER_RID}"/>'
        '<w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1134" w:right="1077" w:bottom="1247" w:left="1077" '
        'w:header="0" w:footer="0" w:gutter="0"/><w:pgNumType w:start="1"/></w:sectPr>'), files["word/document.xml"],
        flags=re.S)


def build(base: bytes, theme: dict, name: str = "") -> bytes:
    colors = theme.get("colors") or {}
    primary, accent = str(colors["primary"]), str(colors["accent"])
    with zipfile.ZipFile(io.BytesIO(base)) as zin:
        items = zin.infolist()
        files = {i.filename: zin.read(i.filename).decode("utf-8") for i in items}
    files["word/styles.xml"] = patch_styles(files["word/styles.xml"], primary, accent)
    files["word/theme/theme1.xml"] = re.sub(r'typeface="Aptos[^"]*"', f'typeface="{SANS}"', files["word/theme/theme1.xml"])
    files["word/fontTable.xml"] = re.sub(r'<w:font w:name="Aptos[^"]*">.*?</w:font>', "", files["word/fontTable.xml"],
                                         flags=re.S)
    if name == "rit-cptc":
        _rit_cptc(files, primary, accent)
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as zout:
        for item in items:
            zout.writestr(item, files.pop(item.filename))
        for fname, data in files.items():
            zout.writestr(fname, data)
    return out.getvalue()


def main() -> None:
    base = subprocess.run(
        ["pandoc", "-o", "/dev/stdout", "--print-default-data-file", "reference.docx"],
        capture_output=True, check=True,
    ).stdout
    for theme_yaml in sorted(TEMPLATES.glob("*/theme.yaml")):
        target = theme_yaml.parent / "reference.docx"
        target.write_bytes(build(base, yaml.safe_load(theme_yaml.read_text()), theme_yaml.parent.name))
        print(f"wrote {target.relative_to(TEMPLATES.parent)}")


if __name__ == "__main__":
    main()
