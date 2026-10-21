---
sidebar_position: 6
---

# Sharing Templates

Templates are self-contained — everything a recipient needs to render the exact same PDF is in the directory. Share via zip, git submodule, or a package registry.

## Packaging

```bash
# From the repo root
zip -r my-company-template.zip templates/my-template/
```

The zip should contain the template directory at its root:

```
my-company-template.zip
└── my-template/
    ├── report-rex.yaml
    ├── template.tex
    ├── theme.yaml
    ├── reference.docx
    ├── filters/
    │   ├── vuln_filter.lua
    │   └── docx_vuln_filter.lua
    └── assets/
        └── logo.png
```

## Installing a Template

```bash
# Unzip into the templates directory
unzip my-company-template.zip -d templates/

# Reference it in index.md
# template: "my-template"

# Or pass the name (or an absolute path to the extracted directory) at build time
./build.sh --template my-template
rex report build --template my-template
```

`--template` accepts a bare name (resolved under `templates/`) or an absolute
directory path. `.zip` files are not auto-extracted by the build — unzip the
template into `templates/` first, as shown above.

## Including Fonts

Place font files in `assets/fonts/`. The Docker image copies them to `/usr/local/share/fonts/` and runs `fc-cache`. Reference them by family name in `theme.yaml`:

```yaml
mainfont: "My Custom Font"
monofont: "My Custom Mono"
```

## Including Graph Scripts

Place Python or R scripts in `assets/scripts/`. These are user-run scripts (not automatically executed by the build process). A tester would run the script to generate a PNG/SVG and reference it in their markdown:

```bash
python templates/my-template/assets/scripts/cvss-radar.py \
  --findings reports/ \
  --output assets/screenshots/cvss-radar.png
```

```markdown
![CVSS Radar](../assets/screenshots/cvss-radar.png)
```
