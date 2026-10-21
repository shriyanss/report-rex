# Default Template

Professional penetration test report with:

- Cover page with logo, client name, version, date, and authors table
- Auto-generated table of contents
- Color-coded vulnerability severity boxes (Critical / High / Medium / Low / Info)
- Branded header/footer on every page
- `DRAFT` watermark support

## Customization

Override any value from `theme.yaml` in your `index.md` frontmatter:

```yaml
---
title: "Web Application Pentest"
client: "ACME Corp"
template: "rex-default"
colors:
  primary: "2C3E50"
  accent:  "3498DB"
logo:
  path: "assets/logo.png"
  width: "3cm"
---
```

## Sharing this template

Zip the entire `default/` directory and share it. Recipients drop it into their `templates/` directory and reference it by folder name.

```bash
zip -r my-company-template.zip templates/rex-default/
```
