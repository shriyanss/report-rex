# docs/ — Claude Guidance

## What This Is

Docusaurus 3.x documentation site. Deployed via GitHub Pages on every push to `main` (`.github/workflows/docs.yml` sets `DOCS_BASE_URL=/report-rex/`); also rsynced by rex-infra's CI onto its own nginx at the site root, baseUrl `/`. Locally available at http://localhost:3000/.

## Local Development

```bash
cd docs
npm install
npm start       # dev server with hot reload at http://localhost:3000/
npm run build   # production build to docs/build/
```

## Structure

```
docs/
├── docusaurus.config.js    Site config: baseUrl, navbar, footer, prism langs
├── sidebars.js             Sidebar tree — must match files in docs/docs/
├── package.json            Node dependencies (Docusaurus 3.5.2)
├── src/css/custom.css      Color overrides for the Docusaurus theme
├── static/
│   └── img/               Static images served at /img/ (/report-rex/img/ on GitHub Pages)
│       └── screenshots/   PDF page screenshots for documentation
└── docs/
    ├── intro.md                          Landing page (slug: /)
    ├── getting-started/
    │   ├── cli.md                         Install rex + build your first report
    │   ├── server.md                      rex-svr + rex sync collaboration setup
    │   ├── handoffs.md                    rex handoffs user guide
    │   └── ide/
    │       ├── neovim.md                  rex editor-conf --ide nvim
    │       └── vscode.md                  cli/editors/vscode extension
    ├── reporting-structure/
    │   ├── index-md.md                   index.md schema reference
    │   ├── vulnerability-files.md        vuln frontmatter schema
    │   ├── sections-appendices.md
    │   ├── charts.md                      :::findings_graph::: directive
    │   └── markdown-features.md           cross-refs, footnotes, REX-STUB, etc.
    ├── creating-templates/
    │   ├── overview.md
    │   ├── template-yaml.md
    │   ├── latex-template.md
    │   ├── lua-filters.md
    │   ├── bundled-filters.md            per-template Lua filter catalog
    │   ├── sharing.md
    │   └── extending.md
    └── reference/
        ├── build-sh.md
        ├── cli.md
        ├── go-cli.md                      rex Go CLI command reference
        ├── sync.md                        rex-svr reference
        ├── handoffs.md                    rex handoffs reference
        ├── ci-cd.md
        ├── output-formats.md              PDF + --docx output
        ├── mcp.md                         rex mcp server
        └── overrides.md                   Index of all override mechanisms
```

## Adding a Page

1. Create a `.md` file in the appropriate `docs/docs/` subdirectory
2. Add a `sidebar_position: N` frontmatter key to control ordering
3. Add the file path (without `.md`) to the correct place in `sidebars.js`

## Adding PDF Screenshots

Screenshot images live in `docs/static/img/screenshots/`. Reference in markdown as:

```markdown
![Cover page](../../../static/img/screenshots/cover-page.png)
```

Use a relative path, not an absolute one starting with `/` — baseUrl
differs between deployments (`/` on rex-infra's nginx, `/report-rex/` on
GitHub Pages), so an absolute path only works on one of them.

## Updating the Config

`docusaurus.config.js` — important settings to update for your deployment:

- `baseUrl` — `DOCS_BASE_URL` env var, default `/` (rex-infra's CI, local dev)
- `url` / `projectUrl` / `organizationName` / `projectName` — the GitHub
  Pages host and repository; change these in a fork

## Sidebar Must Match Files

If `sidebars.js` references a file that doesn't exist, the build fails. Always add both the file AND the sidebar entry together.
