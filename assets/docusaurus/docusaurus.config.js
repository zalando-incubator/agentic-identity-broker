// @ts-check
// `@type` JSDoc annotations allow editor autocompletion and type checking
// (when paired with `@ts-check`).
// See: https://docusaurus.io/docs/api/docusaurus-config

import {themes as prismThemes} from 'prism-react-renderer';

const siteUrl = process.env.DOCS_SITE_URL ?? 'https://agenticidentitybroker.dev';
const baseUrl = process.env.DOCS_BASE_URL ?? '/';
const githubOrg = process.env.DOCS_GITHUB_ORG ?? 'zalando-incubator';
const githubRepo = process.env.DOCS_GITHUB_REPO ?? 'agentic-identity-broker';
const githubBranch = process.env.DOCS_GITHUB_BRANCH ?? 'main';
const githubRepoUrl = `https://github.com/${githubOrg}/${githubRepo}`;
const githubIssuesUrl = `${githubRepoUrl}/issues`;

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'Agentic Identity Broker',
  tagline: 'OAuth2 delegation and consent for AI agents',
  favicon: 'img/favicon.svg',

  // Pin the v4 future flags that were enabled before 3.10 added new defaults.
  future: {
    v4: {
      removeLegacyPostBuildHeadAttribute: true,
      useCssCascadeLayers: true,
    },
  },

  // GitHub Pages + custom-domain metadata, overridable via environment variables
  // so org moves or fork deployments need no code changes.
  url: siteUrl,
  baseUrl,
  organizationName: githubOrg,
  projectName: githubRepo,
  trailingSlash: false,

  onBrokenLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  markdown: {
    mermaid: true,
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  themes: ['@docusaurus/theme-mermaid'],

  plugins: [
    // Client-side full-text search (no external service required).
    [
      'docusaurus-lunr-search',
      {
        indexBaseUrl: true,
      },
    ],
  ],

  presets: [
    // Render the canonical OpenAPI contracts directly with Redoc so the API
    // reference is generated from the specs and can never drift from the code.
    [
      'redocusaurus',
      /** @type {import('redocusaurus').PresetEntry} */
      {
        specs: [
          {
            id: 'enduser',
            spec: '../../api/enduser/openapi.yaml',
            route: '/api/enduser/',
          },
          {
            id: 'admin',
            spec: '../../api/admin/openapi.yaml',
            route: '/api/admin/',
          },
        ],
        theme: {
          primaryColor: '#2563eb',
        },
      },
    ],
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          path: '../../docs',
          sidebarPath: './sidebars.js',
          routeBasePath: 'docs',
          editUrl: ({version, versionDocsDirPath, docPath}) =>
            version === 'current'
              ? `${githubRepoUrl}/edit/${githubBranch}/docs/${docPath}`
              : `${githubRepoUrl}/edit/${githubBranch}/assets/docusaurus/${versionDocsDirPath}/${docPath}`,
          lastVersion: '0.1',
          versions: {
            current: {
              label: 'next 🚧',
              path: 'next',
              banner: 'unreleased',
              // Keep unreleased docs out of search engines so results point at the
              // released series rather than splitting across both versions.
              noIndex: true,
            },
          },
          onlyIncludeVersions: process.env.DOCS_ONLY_INCLUDE_VERSIONS
            ?.split(',').map((version) => version.trim()),
          // Repository-internal maintainer references live under docs/ but are
          // not part of the public site. Keep the classic defaults and add them.
          exclude: [
            '**/_*.{js,jsx,ts,tsx,md,mdx}',
            '**/_*/**',
            '**/*.test.{js,jsx,ts,tsx}',
            '**/__tests__/**',
            'ENCRYPTION_INTEGRATION_GUIDE.md',
            'STORAGE_EXTENSION_GUIDE.md',
            'STORAGE_TROUBLESHOOTING.md',
            'docker-compose-setup.md',
            'deployment/kubernetes.md',
            'configuration/consent-spa.md',
          ],
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      image: 'img/docusaurus-social-card.jpg',
      colorMode: {
        respectPrefersColorScheme: true,
      },
      docs: {
        sidebar: {
          hideable: true,
          autoCollapseCategories: true,
        },
      },
      navbar: {
        title: 'Agentic Identity Broker',
        logo: {
          alt: 'Agentic Identity Broker logo',
          src: 'img/logo.svg',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docs',
            position: 'left',
            label: 'Documentation',
          },
          {
            type: 'dropdown',
            label: 'API',
            position: 'left',
            items: [
              {label: 'End-user API', to: '/api/enduser'},
              {label: 'Admin API', to: '/api/admin'},
            ],
          },
          {type: 'docsVersionDropdown', position: 'right'},
          {
            href: githubRepoUrl,
            label: 'GitHub',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Learn',
            items: [
              {label: 'Introduction', to: '/docs/introduction'},
              {label: 'Concepts', to: '/docs/concepts'},
              {label: 'Get started', to: '/docs/get-started'},
            ],
          },
          {
            title: 'Operate',
            items: [
              {label: 'Guides', to: '/docs/guides/deploy-on-kubernetes'},
              {label: 'Configuration', to: '/docs/configuration'},
              {label: 'API reference', to: '/api/enduser'},
            ],
          },
          {
            title: 'Community',
            items: [
              {label: 'GitHub', href: githubRepoUrl},
              {label: 'Issues', href: githubIssuesUrl},
              {label: 'Public relations', to: '/docs/resources/public-relations'},
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} Agentic Identity Broker contributors. Built with Docusaurus.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ['bash', 'json', 'yaml', 'go', 'hcl'],
      },
    }),
};

export default config;
