# RIT CPTC Template

A bold, high-contrast pentest report template:

- **Indigo brand-band cover**: your logo in the band, the title block, a
  "Prepared by" block with your logo, the client name, and the build date
  (`\today` — the cover always shows when the PDF was compiled)
- **Confidentiality / Document Control** front page
- Numbered sections and findings (`3.1`, `3.2`, …) with an auto-generated,
  hyperlinked table of contents
- **Folded gray corner** accent and a **magenta classification banner** on every page
- A **findings summary table** (ID, title, severity, CVSS) at the top of
  Vulnerability Findings, each row linking to its finding
- Severity-colored **finding panels** — severity chip, finding ID and CVSS on
  top, then CVSS vector, Evaluation (Impact / Sophistication), CWE, OWASP,
  MITRE ATT&CK, Mitigations, Business Risk, Material Impact, and Component with
  its affected assets nested beneath it
- Indigo table headers with severity color coding
  (Critical · High · Medium · Low · Informational)
- `DRAFT` watermark support

## Finding frontmatter

In addition to the standard fields (`cvss_vector`, `status`, `cwe`, `owasp`,
`mitre`, `affected_urls`, …), this template surfaces a few optional fields in the
finding sidebar. All are optional and ignored by other templates:

```yaml
---
title: "Weak Credentials on Mongo Express"
cvss_vector: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"
impact: "Critical"                     # business impact rating
sophistication: "Low"                  # attacker sophistication required
material_impact: "Proprietary Data Loss"  # FAIR-MAM category
mitre: "T1110"                         # ATT&CK technique(s)
mitigations: "M1027, M1032"            # ATT&CK mitigation(s)
affected_urls:
  - "http://10.0.1.11:8081/"
---
```

The finding number (`F-01`, `F-02`, …) is assigned automatically in
severity-sorted order.

## Customization

Override any value from `theme.yaml` in your `index.md` frontmatter:

```yaml
---
title: "All Ports Tours Penetration Test"
client: "All Ports Tours"
company: "RIT CPTC Team"
template: "rit-cptc"
colors:
  primary: "26307C"  # cover band, headings, table headers
  accent:  "C71C64"  # classification banner / links
company_logo: "assets/company-logo-dark.png"   # "Prepared by" block on the cover
company_logo_light: "assets/company-logo.png"  # cover band
headingfont: "Poppins"   # rounded display face (if installed)
---
```

Set `headingfont` to a rounded geometric face (e.g. `TeX Gyre Adventor`,
`Poppins`, `Quicksand`) for a softer look. Without it, the template uses
Liberation Sans (bundled in `assets/fonts/`, SIL OFL), the same face its
`--docx` output embeds — so the PDF and DOCX draw identical glyphs. Changing
`mainfont`/`headingfont`/`monofont` keeps the PDF working but the DOCX keeps
Liberation.

## Sharing this template

Zip the entire `rit-cptc/` directory and share it. Recipients drop it into
their `templates/` directory and reference it by folder name.

```bash
zip -r rit-cptc-template.zip templates/rit-cptc/
```
