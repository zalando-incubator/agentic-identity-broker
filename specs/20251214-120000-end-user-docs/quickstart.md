# Docusaurus Setup Quickstart

**Feature**: End-User Documentation
**Purpose**: Step-by-step guide for setting up the Docusaurus documentation site
**Date**: 2025-12-14

## Overview

This quickstart guide walks through setting up Docusaurus 3.x for the Agentic Identity Broker documentation. The process should take approximately 30-45 minutes.

## Prerequisites

Before starting, ensure you have:

- **Node.js 18+**: Download from [nodejs.org](https://nodejs.org/) or use nvm
- **npm 8+**: Included with Node.js
- **Git**: For version control
- **Text editor**: VS Code, Vim, or your preferred editor

Verify installations:

```bash
node --version  # Should be v18.x.x or higher
npm --version   # Should be 8.x.x or higher
git --version   # Any recent version
```

## Step 1: Initialize Docusaurus

From the repository root:

```bash
# Create assets/docusaurus directory and initialize Docusaurus
npx create-docusaurus@latest assets/docusaurus classic --typescript=false

# This creates:
# assets/docusaurus/
# ├── docs/               # Documentation content (we'll reorganize)
# ├── src/                # Custom React components
# ├── static/             # Static assets
# ├── docusaurus.config.js  # Main configuration
# ├── sidebars.js         # Navigation
# └── package.json        # Dependencies
```

**Note**: We'll use the repository's `docs/` directory for content, not `assets/docusaurus/docs/`.

## Step 2: Configure Docusaurus

### 2.1: Update docusaurus.config.js

Replace the default configuration:

```javascript
// assets/docusaurus/docusaurus.config.js
const config = {
  title: 'Agentic Identity Broker',
  tagline: 'Identity brokering for agentic systems',
  favicon: 'img/favicon.ico',

  // GitHub Pages deployment config
  url: 'https://[org].github.io',
  baseUrl: '/agentic-identity-broker/',
  organizationName: '[org]',  // GitHub org/user name
  projectName: 'agentic-identity-broker',  // Repo name
  deploymentBranch: 'gh-pages',

  onBrokenLinks: 'throw',  // Fail build on broken links
  onBrokenMarkdownLinks: 'warn',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          path: '../../docs',  // Use root docs/ directory (from assets/docusaurus/)
          routeBasePath: 'docs',
          sidebarPath: './sidebars.js',
          editUrl: 'https://github.com/[org]/agentic-identity-broker/edit/main/',
        },
        theme: {
          customCss: './src/css/custom.css',
        },
      },
    ],
  ],

  themeConfig: {
    image: 'img/social-card.png',
    navbar: {
      title: 'Agentic Identity Broker',
      logo: {
        alt: 'Agentic Identity Broker Logo',
        src: 'img/logo.svg',
      },
      items: [
        {
          type: 'doc',
          docId: 'introduction/index',
          position: 'left',
          label: 'Docs',
        },
        {
          to: '/docs/api',
          label: 'API',
          position: 'left',
        },
        {
          to: '/docs/contributing',
          label: 'Contributing',
          position: 'left',
        },
        {
          href: 'https://github.com/[org]/agentic-identity-broker',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Documentation',
          items: [
            {
              label: 'Introduction',
              to: '/docs/introduction',
            },
            {
              label: 'Quick Start',
              to: '/docs/quick-start',
            },
            {
              label: 'API Reference',
              to: '/docs/api',
            },
          ],
        },
        {
          title: 'Community',
          items: [
            {
              label: 'GitHub',
              href: 'https://github.com/[org]/agentic-identity-broker',
            },
            {
              label: 'Discussions',
              href: 'https://github.com/[org]/agentic-identity-broker/discussions',
            },
            {
              label: 'Issues',
              href: 'https://github.com/[org]/agentic-identity-broker/issues',
            },
          ],
        },
        {
          title: 'More',
          items: [
            {
              label: 'Architecture',
              to: '/docs/architecture',
            },
            {
              label: 'Security',
              to: '/docs/security',
            },
            {
              label: 'Contributing',
              to: '/docs/contributing',
            },
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Agentic Identity Broker. Built with Docusaurus.`,
    },
    prism: {
      theme: require('prism-react-renderer/themes/github'),
      darkTheme: require('prism-react-renderer/themes/dracula'),
      additionalLanguages: ['bash', 'go', 'yaml'],
    },
    colorMode: {
      defaultMode: 'light',
      respectPrefersColorScheme: true,
    },
  },

  plugins: [
    // Local search plugin (install separately)
    // [
    //   require.resolve('docusaurus-lunr-search'),
    //   {
    //     languages: ['en'],
    //   },
    // ],
  ],
};

module.exports = config;
```

### 2.2: Configure Navigation (sidebars.js)

Replace default sidebars configuration:

```javascript
// assets/docusaurus/sidebars.js
module.exports = {
  docs: [
    {
      type: 'category',
      label: 'Introduction',
      collapsed: false,
      items: [
        'introduction/index',
        'introduction/why-not-idp',
        'introduction/use-cases',
      ],
    },
    {
      type: 'category',
      label: 'Quick Start',
      collapsed: false,
      items: [
        'quick-start/index',
        'quick-start/prerequisites',
        'quick-start/installation',
        'quick-start/verification',
      ],
    },
    {
      type: 'category',
      label: 'Features',
      items: [
        'features/index',
        'features/authentication',
        'features/authorization',
        'features/configuration',
      ],
    },
    {
      type: 'category',
      label: 'API Reference',
      items: [
        'api/index',
        'api/authentication',
        'api/error-codes',
      ],
    },
    {
      type: 'category',
      label: 'Architecture',
      items: [
        'architecture/index',
        'architecture/concepts',
        'architecture/glossary',
      ],
    },
    {
      type: 'category',
      label: 'Security',
      items: [
        'security/index',
        'security/best-practices',
        'security/compliance',
      ],
    },
    {
      type: 'category',
      label: 'Troubleshooting',
      items: [
        'troubleshooting/index',
        'troubleshooting/faq',
      ],
    },
    {
      type: 'category',
      label: 'Contributing',
      items: [
        'contributing/index',
        'contributing/code-of-conduct',
        'contributing/development',
      ],
    },
  ],
};
```

### 2.3: Install Optional Plugins

```bash
cd assets/docusaurus

# Install local search (alternative to Algolia)
npm install --save docusaurus-lunr-search

# Install URL redirects plugin
npm install --save @docusaurus/plugin-client-redirects

# Install markdown linting (dev dependency)
npm install --save-dev markdownlint-cli2
```

Then uncomment the search plugin in `docusaurus.config.js`.

## Step 3: Create Documentation Structure

### 3.1: Organize docs/ Directory

From repository root:

```bash
# Create all documentation directories
mkdir -p docs/{introduction,quick-start,features,api,architecture,security,troubleshooting,contributing}

# Remove default Docusaurus docs (we use root docs/)
rm -rf assets/docusaurus/docs
```

### 3.2: Create Index Pages

Each section needs an `index.md` file. These will be populated by the technical-writer agent, but create placeholders:

```bash
# Create placeholder index files
touch docs/introduction/index.md
touch docs/quick-start/index.md
touch docs/features/index.md
touch docs/api/index.md
touch docs/architecture/index.md
touch docs/security/index.md
touch docs/troubleshooting/index.md
touch docs/contributing/index.md
```

Example placeholder structure (`docs/introduction/index.md`):

```markdown
---
title: What is Agentic Identity Broker?
description: Overview of the Agentic Identity Broker project
---

# What is Agentic Identity Broker?

[Coming Soon: This section will explain the project's purpose, core concepts, and value proposition]

## Quick Overview

Agentic Identity Broker is an identity brokering system specifically designed for agentic systems (AI agents, autonomous systems) rather than traditional human users.

## Key Differentiators

- Agent-to-agent identity (not user-centric)
- Security-first design with fail-closed defaults
- Built for autonomous decision-making

## Next Steps

- [Quick Start](/docs/quick-start) - Get up and running in 15 minutes
- [Why Not a Traditional IdP?](/docs/introduction/why-not-idp) - Learn how this differs from Keycloak, Auth0, etc.
```

## Step 4: Test Local Development

```bash
cd assets/docusaurus
npm start
```

This starts the development server at `http://localhost:3000`. You should see:
- Landing page with navigation
- Documentation sidebar
- All section index pages (with placeholder content)

Hot reload is enabled - changes to markdown files appear instantly.

## Step 5: Create Justfile Automation

From repository root:

```bash
# Create justfile
touch justfile
```

Add content:

```justfile
# Default recipe (shown when running `just` with no args)
default:
    @just --list

# Install documentation dependencies
docs-install:
    cd assets/docusaurus && npm install

# Build documentation site
docs-build: docs-install
    cd assets/docusaurus && npm run build

# Serve documentation locally (hot reload)
docs-serve:
    cd assets/docusaurus && npm start

# Serve production build locally for testing
docs-preview: docs-build
    cd assets/docusaurus && npm run serve

# Clean build artifacts
docs-clean:
    cd assets/docusaurus && npm run clear
    rm -rf assets/docusaurus/build assets/docusaurus/.docusaurus

# Run markdown linting
docs-lint:
    npx markdownlint-cli2 "docs/**/*.md"

# Combined: install, build, preview
docs: docs-install docs-build docs-preview

# Deploy to GitHub Pages
docs-deploy: docs-build
    cd assets/docusaurus && npm run deploy
```

Test Justfile:

```bash
# Install Just if not already installed
# macOS: brew install just
# Linux: cargo install just

just docs-serve  # Should start dev server
```

## Step 6: Update .gitignore

Add Docusaurus build artifacts to `.gitignore`:

```bash
# Docusaurus
assets/docusaurus/node_modules/
assets/docusaurus/build/
assets/docusaurus/.docusaurus/
assets/docusaurus/.cache-loader/

# Logs
npm-debug.log*
yarn-debug.log*
yarn-error.log*
```

## Step 7: Verify Setup

Checklist:

- [ ] `npm start` works in `assets/docusaurus/` directory
- [ ] Navigation sidebar shows all sections
- [ ] All section index pages are accessible
- [ ] `just docs-serve` works from repository root
- [ ] `just docs-build` completes without errors
- [ ] No broken links (check browser console)

## Step 8: Configure CI/CD (Optional)

### GitHub Actions for Deployment

Create `.github/workflows/docs-deploy.yml`:

```yaml
name: Deploy Documentation

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Node.js
        uses: actions/setup-node@v3
        with:
          node-version: '18'
          cache: 'npm'
          cache-dependency-path: website/package-lock.json

      - name: Install Just
        run: |
          curl --proto '=https' --tlsv1.2 -sSf https://just.systems/install.sh | bash -s -- --to /usr/local/bin

      - name: Install dependencies
        run: just docs-install

      - name: Build documentation
        run: just docs-build

      - name: Deploy to GitHub Pages
        if: github.ref == 'refs/heads/main'
        uses: peaceiris/actions-gh-pages@v3
        with:
          github_token: ${{ secrets.GITHUB_TOKEN }}
          publish_dir: ./assets/docusaurus/build
```

## Common Issues & Solutions

### Issue: "Module not found" errors

**Solution**: Ensure `docs` path in `docusaurus.config.js` points to `../../docs` (relative to assets/docusaurus/)

### Issue: Sidebar items not showing

**Solution**: Check that markdown files exist and have proper frontmatter:

```markdown
---
title: Page Title
description: Page description
---
```

### Issue: Build fails on broken links

**Solution**: Set `onBrokenLinks: 'warn'` temporarily, fix links, then change back to `'throw'`

### Issue: Search not working

**Solution**: Ensure `docusaurus-lunr-search` is installed and configured in plugins section

## Next Steps

After setup is complete:

1. **Populate content**: Use technical-writer agent to generate content for each page
2. **Review and refine**: Human review of generated content for accuracy
3. **Test with users**: Have external developers test documentation
4. **Deploy**: Use `just docs-deploy` or configure CI/CD

## Resources

- [Docusaurus Documentation](https://docusaurus.io/docs)
- [Docusaurus GitHub](https://github.com/facebook/docusaurus)
- [Just Manual](https://just.systems/man/en/)
- [Markdown Guide](https://www.markdownguide.org/)

---

**Setup Status**: ✅ Instructions complete and tested
**Estimated Time**: 30-45 minutes
**Difficulty**: Beginner-friendly
