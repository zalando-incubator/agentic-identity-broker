# Phase 0: Research & Technology Selection

**Feature**: End-User Documentation
**Date**: 2025-12-14
**Status**: Complete

## Overview

This document consolidates research findings for implementing comprehensive end-user documentation for the Agentic Identity Broker project. All technical decisions and unknowns from the Technical Context section have been researched and resolved.

## Research Task 1: Docusaurus vs. Alternatives

### Decision

**Selected**: Docusaurus 3.x

### Rationale

Docusaurus 3.x provides the best balance of features, developer experience, and ecosystem fit for an OSS identity project:

1. **React/MDX Ecosystem**: Built on React 18+, allowing interactive components and enhanced markdown
2. **Versioning**: Built-in documentation versioning for future multi-version support
3. **Search**: Multiple options (Algolia DocSearch, local search plugin)
4. **Performance**: Static site generation with modern optimizations (code splitting, prefetching)
5. **Community**: Large OSS community, maintained by Meta (Facebook)
6. **Deployment**: Simple deployment to any static host (GitHub Pages, Netlify, Vercel)
7. **i18n**: Built-in internationalization support for future localization
8. **Customization**: Full React customization without ejecting

### Alternatives Considered

| Alternative | Pros | Cons | Why Rejected |
|-------------|------|------|--------------|
| **MkDocs** | Simple Python-based, Material theme | Limited interactivity, Python dependency | Less flexible for advanced features, smaller ecosystem |
| **VitePress** | Fast (Vite-based), Vue ecosystem | Newer, smaller community, less mature versioning | Less mature than Docusaurus, fewer OSS examples |
| **Sphinx** | Industry standard for Python projects | Complex setup, Python-focused, dated UI | Not ideal for non-Python project, steeper learning curve |
| **GitBook** | Beautiful UI, hosted solution | Paid for private repos, vendor lock-in | Vendor dependency, less control over infrastructure |

### Version Selection

**Selected**: Docusaurus 3.6.x (latest stable 3.x)

**Rationale**:
- Docusaurus 3.x is production-ready (released Oct 2023)
- Major improvements over 2.x: faster builds, better MDX support, improved React Fast Refresh
- Stable API with clear migration path for future versions
- Active maintenance and security updates

### Implementation Impact

- **Dependencies**: Requires Node.js 18+ and npm
- **Build Time**: ~10-30 seconds for initial build, <1s for hot reload
- **Bundle Size**: ~300KB gzipped for baseline site
- **Learning Curve**: Low for developers familiar with React/Markdown

## Research Task 2: Docusaurus Configuration Best Practices

### Decision

Use standard Docusaurus project structure with customizations for identity/security domain.

### Recommended Structure

```text
assets/docusaurus/
├── docusaurus.config.js       # Main configuration
├── sidebars.js                # Navigation structure
├── src/
│   ├── components/            # Custom React components
│   │   ├── HomepageFeatures/  # Landing page features
│   │   └── SecurityBadge/     # Security-related UI elements
│   ├── css/
│   │   └── custom.css         # Theme customization
│   └── pages/
│       ├── index.js           # Landing page
│       └── about.md           # About page
├── static/
│   ├── img/                   # Images and logos
│   └── fonts/                 # Custom fonts (if needed)
└── package.json
```

### Key Configuration Decisions

#### 1. Theme Configuration

```javascript
// docusaurus.config.js
themeConfig: {
  colorMode: {
    defaultMode: 'light',
    respectPrefersColorScheme: true,  // Respect OS dark mode preference
  },
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
        to: '/api',
        label: 'API',
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
        title: 'Docs',
        items: [
          { label: 'Introduction', to: '/docs/introduction' },
          { label: 'Quick Start', to: '/docs/quick-start' },
          { label: 'API Reference', to: '/docs/api' },
        ],
      },
      {
        title: 'Community',
        items: [
          { label: 'GitHub', href: 'https://github.com/[org]/agentic-identity-broker' },
          { label: 'Discussions', href: 'https://github.com/[org]/agentic-identity-broker/discussions' },
        ],
      },
    ],
  },
}
```

#### 2. Plugins Required

| Plugin | Purpose | Installation |
|--------|---------|--------------|
| `@docusaurus/preset-classic` | Core documentation features (included by default) | Pre-installed |
| `@docusaurus/plugin-content-docs` | Multi-instance docs support (included in preset) | Pre-installed |
| `@docusaurus/plugin-client-redirects` | URL redirects for moved pages | `npm install --save @docusaurus/plugin-client-redirects` |
| `docusaurus-lunr-search` | Local search (alternative to Algolia) | `npm install --save docusaurus-lunr-search` |

**Note**: Start with local search plugin; apply for Algolia DocSearch once site is public and has traffic.

#### 3. Sidebar Configuration Pattern

Use auto-generated sidebars with manual overrides for critical sections:

```javascript
// sidebars.js
module.exports = {
  docs: [
    'introduction/index',
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
    // ... more categories
  ],
};
```

### Best Practices from OSS Identity Projects

Studied documentation from:
- **Keycloak**: Comprehensive guides, clear navigation hierarchy
- **Auth0 Docs**: Excellent quick starts, SDK-specific guides
- **Ory Kratos**: Open-source focused, strong API reference
- **Okta Developer**: Clear conceptual vs. how-to separation

**Key Takeaways**:
1. **Progressive Disclosure**: Start simple (quick start), gradually introduce complexity
2. **Use Case Driven**: Organize by user goals, not system architecture
3. **Code Examples First**: Show working code before explaining theory
4. **Clear Differentiation**: Explicitly state what this project is NOT
5. **Security Prominent**: Security considerations visible throughout, not hidden

## Research Task 3: Justfile Integration

### Decision

Use Justfile for cross-platform build automation with clear, self-documenting commands.

### Justfile Syntax and Patterns

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

# Full docs workflow: install, build, serve
docs: docs-install docs-build docs-preview

# Run markdown linting
docs-lint:
    npx markdownlint-cli2 "docs/**/*.md"

# Check for broken links
docs-check-links: docs-build
    npx broken-link-checker http://localhost:3000

# Deploy to GitHub Pages
docs-deploy: docs-build
    cd assets/docusaurus && npm run deploy
```

### Cross-Platform Compatibility

**Just** is available on all platforms:
- **macOS**: `brew install just`
- **Linux**: `cargo install just` or package managers (apt, dnf, pacman)
- **Windows**: `cargo install just` or `scoop install just`

**Rationale for Justfile vs. Alternatives**:

| Tool | Pros | Cons | Decision |
|------|------|------|----------|
| **Justfile** | Simple syntax, cross-platform, self-documenting | Requires separate install | ✅ **Selected** - Best DX |
| **Make** | Ubiquitous, no install | Complex syntax, platform quirks | ❌ Poor DX for modern projects |
| **npm scripts** | No extra install (Node.js only) | Limited to Node.js projects | ❌ Couples docs to Node |
| **Bash scripts** | Ubiquitous on Unix | Not cross-platform (Windows) | ❌ Windows compatibility issues |

### Implementation Impact

- Justfile will be created at repository root
- Documentation in README will reference `just docs` commands
- CI/CD will use `just docs-build` for consistency

## Research Task 4: Content Structure Patterns

### Decision

Use **task-oriented** documentation structure with progressive disclosure.

### Information Architecture

```text
Documentation Flow:
1. Introduction (What/Why) → 2. Quick Start (Get Running) → 3. Features (What's Possible) → 4. API (Integration) → 5. Advanced (Concepts/Architecture)
```

### Structure Pattern from Identity Projects

Based on analysis of Keycloak, Auth0, Ory, and Okta documentation:

#### Pattern 1: Hub-and-Spoke Navigation

- **Hub**: Each major section has an index page (landing page)
- **Spokes**: Detailed pages branch from hub
- **Cross-links**: Related content linked bidirectionally

**Applied to Agentic Identity Broker**:
```text
Introduction (Hub)
  ├─ What is AIB? (landing page)
  ├─ AIB vs Traditional IdP
  └─ Use Cases

Quick Start (Hub)
  ├─ Prerequisites
  ├─ Installation
  └─ Verification
```

#### Pattern 2: Layered Complexity

- **Layer 1**: Quick start (get running in 15 min)
- **Layer 2**: Common use cases (80% of users)
- **Layer 3**: Advanced configuration (20% power users)
- **Layer 4**: Architecture internals (contributors/debuggers)

**Implementation**:
- Quick Start: Layer 1
- Features: Layer 2
- API + Security: Layer 2-3
- Architecture: Layer 4
- Contributing: Layer 4

#### Pattern 3: Code-First Examples

Every feature page structure:
1. **What it does** (1-2 sentences)
2. **Quick example** (code snippet)
3. **How it works** (explanation)
4. **Configuration options** (reference)
5. **Advanced use cases** (optional)

### Navigation Recommendations

1. **Persistent sidebar**: Always visible on desktop
2. **Breadcrumbs**: Show location in hierarchy
3. **Table of contents**: Right sidebar for long pages
4. **Next/Previous**: Bottom of each page
5. **Search**: Prominent in header

### Content Templates

Created templates for consistency:

**Feature Page Template**:
```markdown
---
title: [Feature Name]
description: [One-line description]
---

# [Feature Name]

[1-2 sentence overview]

## Quick Example

```[language]
[Working code example]
```

## Overview

[Detailed explanation of what the feature does and why it's useful]

## Configuration

[Configuration options with defaults]

## Common Use Cases

### [Use Case 1]
[Example with code]

### [Use Case 2]
[Example with code]

## Advanced

[Advanced scenarios, edge cases]

## Related

- [Link to related feature 1]
- [Link to related feature 2]
```

## Research Task 5: Technical Writer Agent Integration

### Decision

Use structured prompts with comprehensive context for technical-writer agent to generate high-quality content.

### Content Generation Workflow

```text
1. Define Content Scope
   ↓
2. Prepare Context Package
   ↓
3. Generate Draft via Agent
   ↓
4. Human Review & Refinement
   ↓
5. Publish to Docs
```

### Context Package for Technical Writer Agent

For each documentation page, provide:

```yaml
page_context:
  title: "[Page Title]"
  section: "[Introduction|Quick Start|Features|API|etc.]"
  audience: "Developers with basic identity system knowledge"
  tone: "Technical but approachable, security-focused"

  project_context:
    name: "Agentic Identity Broker"
    purpose: "Identity brokering specifically designed for agentic systems"
    differentiation: "Unlike traditional IdPs (Keycloak, Auth0), focuses on agent-to-agent identity"
    security_posture: "Security-first, fail-closed, library-based crypto"

  content_requirements:
    format: "Docusaurus MDX"
    length: "[target word count]"
    code_examples: true
    include_sections: ["Overview", "Quick Example", "Configuration", "Use Cases"]

  domain_concepts:
    # Glossary terms relevant to this page
    - term: "Agentic Identity"
      definition: "[from ARCHITECTURE.md glossary]"

  references:
    - constitution: ".specify/memory/constitution.md"
    - architecture: "ARCHITECTURE.md"
    - spec: "specs/20251214-120000-end-user-docs/spec.md"
```

### Agent Prompt Template

```text
You are a technical writer creating documentation for the Agentic Identity Broker, an open-source identity brokering system designed specifically for agentic systems (AI agents, autonomous systems, etc.) rather than traditional human users.

Context:
- Target audience: [from context package]
- Tone: [from context package]
- Project details: [from context package]

Task:
Write a documentation page for: [page title]

Requirements:
1. Follow the feature page template structure
2. Include working code examples (use placeholder APIs if actual implementation doesn't exist yet)
3. Clearly differentiate from traditional identity providers where relevant
4. Emphasize security-first principles throughout
5. Use progressive disclosure (simple → advanced)
6. Include "Coming Soon" markers for unimplemented features
7. Cross-link to related documentation pages
8. Use Docusaurus MDX format with frontmatter

Output the complete markdown content for this page.
```

### Quality Assurance Checklist

After agent generates content, human reviewer checks:

- [ ] Technical accuracy (no false claims)
- [ ] Code examples work (or marked as placeholder)
- [ ] Appropriate for target audience (not too simple/complex)
- [ ] Follows Docusaurus conventions (frontmatter, admonitions, code blocks)
- [ ] Links are valid (no broken internal links)
- [ ] Security guidance is consistent with Constitution
- [ ] Grammar and spelling are correct
- [ ] Tone is consistent with project voice

### Revision Strategy

- **First pass**: Generate all pages with agent (establish baseline)
- **Second pass**: Human review and refinement (technical accuracy, voice)
- **Third pass**: User testing with external developers (validate clarity)
- **Ongoing**: Continuous updates as features evolve

## Research Decisions Summary

| Decision Point | Resolution | Rationale |
|----------------|------------|-----------|
| Documentation framework | Docusaurus 3.x | Best ecosystem fit, versioning, React/MDX |
| Version | 3.6.x (latest stable) | Production-ready, active maintenance |
| Search | Local search plugin (docusaurus-lunr-search) | No application required, works offline |
| Build automation | Justfile | Best DX, cross-platform, self-documenting |
| Navigation pattern | Hub-and-spoke with progressive disclosure | Proven pattern from successful identity docs |
| Content structure | Task-oriented with code-first examples | User goal-driven, reduces time-to-value |
| Agent workflow | Structured context → generate → review | Balances automation with quality control |
| Deployment target | GitHub Pages (initially) | Free, simple, version controlled |

## Resolved Unknowns from Technical Context

All items marked "NEEDS CLARIFICATION" in the Technical Context have been resolved:

| Unknown | Resolution |
|---------|------------|
| Language/Version | Node.js 18+ for Docusaurus, Markdown for content |
| Primary Dependencies | Docusaurus 3.6.x, React 18+, docusaurus-lunr-search |
| Testing | Docusaurus built-in validation, markdownlint, broken-link-checker |
| Performance Goals | <2s initial load, <500ms navigation (Docusaurus default) |
| Constraints | Offline-first via local dev server, versioning via Docusaurus |
| Scale/Scope | 10-15 pages initially, expandable to 50+ as features grow |

## Next Steps (Phase 1)

With all research complete, proceed to Phase 1:

1. Generate `contracts/docs-structure.yaml` (documentation structure contract)
2. Create `quickstart.md` (Docusaurus setup guide)
3. Create ADR `adrs/001-docusaurus-for-documentation.md`
4. Update agent context via `.specify/scripts/bash/update-agent-context.sh`

---

**Research Status**: ✅ COMPLETE
**All unknowns resolved**: Yes
**Ready for Phase 1**: Yes
