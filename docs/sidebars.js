/** @type {import('@docusaurus/plugin-content-docs').SidebarsConfig} */
const sidebars = {
  docs: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/cli',
        'getting-started/server',
        'getting-started/handoffs',
        {
          type: 'category',
          label: 'IDE',
          items: [
            'getting-started/ide/neovim',
            'getting-started/ide/vscode',
          ],
        },
      ],
    },
    {
      type: 'category',
      label: 'Reporting Structure',
      items: [
        'reporting-structure/index-md',
        'reporting-structure/vulnerability-files',
        'reporting-structure/sections-appendices',
        'reporting-structure/charts',
        'reporting-structure/markdown-features',
      ],
    },
    {
      type: 'category',
      label: 'Templates',
      items: [
        'creating-templates/overview',
        'creating-templates/template-yaml',
        'creating-templates/latex-template',
        'creating-templates/lua-filters',
        'creating-templates/bundled-filters',
        'creating-templates/sharing',
        'creating-templates/extending',
      ],
    },
    {
      type: 'category',
      label: 'Reference',
      items: [
        'reference/build-sh',
        'reference/cli',
        'reference/go-cli',
        'reference/sync',
        'reference/handoffs',
        'reference/ci-cd',
        'reference/output-formats',
        'reference/mcp',
        'reference/overrides',
      ],
    },
  ],
};

module.exports = sidebars;
