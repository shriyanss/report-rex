// @ts-check
const { themes: prismThemes } = require('prism-react-renderer');

// GitHub Pages serves this site at /report-rex/ (docs.yml sets DOCS_BASE_URL);
// rex-infra's nginx serves it at the root, so default to '/'.
const siteBaseUrl = process.env.DOCS_BASE_URL || '/';
const projectUrl = 'https://github.com/shriyanss/report-rex';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Report Rex',
  tagline: 'Professional pentest reports from Markdown',
  favicon: 'img/favicon.png',

  url: 'https://shriyanss.github.io',
  baseUrl: siteBaseUrl,

  organizationName: 'shriyanss',
  projectName: 'report-rex',

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          routeBasePath: '/',
          sidebarPath: require.resolve('./sidebars.js'),
          editUrl: `${projectUrl}/edit/main/docs/`,
        },
        blog: false,
        theme: {
          customCss: require.resolve('./src/css/custom.css'),
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      navbar: {
        title: 'Report Rex',
        logo: {
          alt: 'Report Rex Logo',
          src: 'img/logo.png',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docs',
            position: 'left',
            label: 'Documentation',
          },
          {
            href: projectUrl,
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Docs',
            items: [
              { label: 'Getting Started', to: '/getting-started/cli' },
              { label: 'Creating Templates', to: '/creating-templates/overview' },
            ],
          },
          {
            title: 'Project',
            items: [
              { label: 'GitHub', href: projectUrl },
              { label: 'Issues', href: `${projectUrl}/issues` },
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} Report Rex. Built with Docusaurus.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ['bash', 'yaml', 'latex', 'lua'],
      },
    }),
};

module.exports = config;
