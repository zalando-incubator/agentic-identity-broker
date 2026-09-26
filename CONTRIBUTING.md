# Contributing to Agentic Identity Broker

Thank you for contributing! This document provides guidelines for maintaining our governance principles and code quality standards.

## Quick Start

1. **Fork and clone**:
   ```bash
   git clone https://github.com/zalando-incubator/agentic-identity-broker.git
   cd agentic-identity-broker
   ```

2. **Setup**:
   ```bash
   just deps
   just install-tools
   ```

3. **Create a branch**:
   ```bash
   git checkout -b feature/your-feature-name
   ```

4. **Develop**:
   ```bash
   just dev              # Backend with hot-reload
   just web-dev          # Frontend (optional, separate terminal)
   ```

## Governance Principles

All contributions must comply with our specification-driven development approach:

- **API-First**: Document all public APIs in OpenAPI and get user confirmation before implementation
- **Security-First**: Security controls enabled by default, never optional
- **Architecture**: Follow hexagonal architecture patterns and ADRs
- **Domain-Driven Design**: Clear ubiquitous language and domain isolation
- **Design System**: Frontend components use `/web/src/design-system/`
- **Database**: Schema changes use go-migrate naming conventions

See [Constitution v1.4.0](.specify/memory/constitution.md) for details.

## Before Committing

Run static checks and the full verification gate:
```bash
just check    # Non-mutating format, vet, and lint checks
just verify   # Security scanning plus test, integration, and E2E checks
```

`just check` does not change source files. Use `just fmt` to apply Go formatting fixes.
`just security` runs the focused gosec, govulncheck, and OSV-Scanner scans.
`just verify` runs security scanning before the existing test, integration, and E2E gate.

Both commands must pass before you open a pull request.

## Making Changes

### Repository Root

Root contains only files that a tool requires there or that GitHub/Zalando OSS renders. Variants go in a subdirectory.

### Specification-Driven Development

For new features, follow this workflow using [Speckit](https://speckit.org/):

1. Create `specs/NNN-feature-name/spec.md` with requirements
2. Create `specs/NNN-feature-name/plan.md` with design
3. Create `specs/NNN-feature-name/tasks.md` with tasks
4. Implement according to the plan
5. Update [ARCHITECTURE.md](docs/ARCHITECTURE.md) if needed

### Backend

Follow hexagonal architecture:
- **Domain** (`internal/domain/`): Pure business logic
- **Ports** (`internal/ports/`): Interface boundaries
- **Adapters** (`internal/adapters/`): HTTP, storage, config

### Frontend

All components must use the design system at `/web/src/design-system/`:

```typescript
import { Button, Card } from '@/components/design-system';

export function MyComponent() {
  return (
    <Card>
      <Button variant="primary">Submit</Button>
    </Card>
  );
}
```

### Tests

- Tests live next to source files (`*_test.go` or `*.test.tsx`)
- New features need >80% test coverage
- Fast Go/package tests: `just test`
- Integration suites: `just test-integration`
- All E2E suites: `just test-e2e`
- Dedicated E2E performance measurement: `just test-e2e-performance` (manual; normal E2E commands exclude performance-labelled specs)
- Full verification gate, including security scanning: `just verify`
- Coverage for the fast Go/package suite: `just test-coverage`

## Pull Request Process

### Before Opening a PR

1. Run `just check` and `just verify`, and ensure both pass
2. Update [ARCHITECTURE.md](docs/ARCHITECTURE.md) if design changed
3. Write clear commit messages using [Conventional Commits](https://www.conventionalcommits.org/)

### Submitting a PR

1. Push your branch
2. Open a PR with:
   - Clear description of changes
   - Rationale for changes
   - Reference to related issues (`Closes #123`)
   - Testing approach

### Requirements

- ✅ One maintainer review required
- ✅ Static quality checks passing (`just check`)
- ✅ Full verification gate passing (`just verify`)
- ✅ Architecture changes require both maintainers

## Commit Guidelines

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(scope): description

[optional body explaining why]

Closes #123
```

**Types**: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`

## Security

⚠️ **Never open public issues for security vulnerabilities!**

Disclose responsibly:
- Email maintainers with details
- Use GitHub Security Advisories
- Allow time for fix before disclosure

**Security guidelines**:
- Never commit secrets or API keys
- Validate input at system boundaries
- Enable security controls by default
- Escalate rather than bypass security issues

## Questions?

- **General**: Create a GitHub discussion
- **Bugs**: Open an issue with reproduction steps
- **Features**: Start a discussion first
- **Process**: Check [MAINTAINERS](MAINTAINERS) or [README](README.md)

## Resources

- [README](README.md) - Overview and quick start
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) - System design
- [Constitution v1.4.0](.specify/memory/constitution.md) - Governance
- [MAINTAINERS](MAINTAINERS) - Maintainer information
- [Speckit](https://speckit.org/) - Specification-driven development

---

**Thank you for contributing!**
