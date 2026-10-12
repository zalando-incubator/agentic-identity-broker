# Contributing to Agentic Identity Broker

Thank you for contributing! This document provides guidelines for maintaining our governance principles and code quality standards.

## Quick Start

Install Go for [go.mod](go.mod), Node.js for [web/package.json](web/package.json), npm, and `just`.
The [justfile](justfile) defines the recipes in this guide and installs the test tools.
Infrastructure-backed verification also needs Docker or Podman, as described in the [storage integration guide](tests/integration/storage/README.md).

1. Clone the repository:
   ```bash
   git clone https://github.com/zalando-incubator/agentic-identity-broker.git
   cd agentic-identity-broker
   ```
2. Install dependencies and tools:
   ```bash
   just deps
   just install-tools
   just web-install
   ```
3. Select the feature specification before implementation. Use the workflow in [Specification-Driven Development](#specification-driven-development).
4. Start the broker from the repository root:
   ```bash
   just run
   ```
5. Start Vite in another terminal:
   ```bash
   just web-dev
   ```

The [run recipe](justfile) builds the broker and supplies temporary JWE and memory-encryption keys.
The root [config.yaml](config.yaml) selects local development configuration and an upstream OAuth2 server on port 9001.
Vite requests port 3000 and proxies backend namespaces to port 8000 through [vite.config.ts](web/vite.config.ts).
OAuth2 journeys also need the configured providers. The [Compose guide](docs/docker-compose-setup.md) describes the multi-service environment.
This configuration trusts a proxy header and permits local HTTP providers. It is not a production deployment.

For hot reload, supply both keys to the Air process:

```bash
export IDENTITY_BROKER_JWE_SIGNING_KEY="$(./scripts/generate-jwe-key.sh)"
export IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="$(./scripts/generate-jwe-key.sh)"
just dev
```

The [Air configuration](.air.toml) runs [start-broker.sh](scripts/start-broker.sh), which also attempts to seed development data.
Do not run `just run` and `just dev` on the same ports.

## Governance Principles

All contributions must comply with our specification-driven development approach:

- **API-First**: Document all public APIs in OpenAPI and get user confirmation before implementation
- **Security-First**: Security controls enabled by default, never optional
- **Architecture**: Follow hexagonal architecture patterns and ADRs
- **Domain-Driven Design**: Clear ubiquitous language and domain isolation
- **Design System**: Frontend components use `/web/src/design-system/`
- **Database**: Schema changes use go-migrate naming conventions

Read the current [constitution](.specify/memory/constitution.md) for binding rules and its version history.
The [ADR index](ARCHITECTURE.md#10-architecture-decision-records-adrs) identifies decisions by full filename, status, and partial supersession.
Proposed ADRs do not become accepted because their implementation exists.

## Before Committing

Run the gates defined in the [justfile](justfile):
```bash
just check    # Non-mutating format, vet, and lint checks
just verify   # Security scanning plus test, integration, and E2E checks
```

`just check` does not change source files. Use `just fmt` to apply Go formatting fixes.
`just security` runs the focused gosec, govulncheck, and OSV-Scanner scans.
`just verify` includes static checks, security scans, Go and frontend tests, CDK and mock tests, both integration layers, and all E2E suites.

The frontend and documentation-site manifests use npm overrides for transitive security fixes.
Commit the corresponding lockfile when changing an override. Verify both site builds and the
instrumented frontend build (`VITE_COVERAGE=1 npm run build --prefix web`). The frontend's
js-yaml 4 override removes the unpatched sprintf-js dependency from coverage tooling.

The root `osv-scanner.toml` temporarily excludes `GHSA-vfj7-8cjw-p6xm` until 2026-11-07.
No patched braces release exists, and Docusaurus requires Chokidar 3's glob support. Its
patterns are repository-controlled; the deployed documentation is static. Reassess this
exception at expiry or if documentation tooling starts accepting untrusted patterns.

Both commands must pass before you open a pull request.

## Making Changes

### Repository Root

Root contains only files that a tool requires there or that GitHub/Zalando OSS renders. Variants go in a subdirectory.

### Specification-Driven Development

For new features, use the [Spec Kit templates](.specify/templates/) and the [constitution](.specify/memory/constitution.md).
These paths are naming patterns, not literal directories to copy:

1. Create `specs/NNN-feature-name/spec.md` with requirements.
2. Create `specs/NNN-feature-name/plan.md` with the design.
3. Create `specs/NNN-feature-name/tasks.md` with ordered tasks.
4. Obtain the required API and architecture approvals before implementation.
5. Implement the plan with red-green-refactor tests and acceptance-scenario traceability.
6. Update [ARCHITECTURE.md](ARCHITECTURE.md) when the design changes.

The [selection script](.specify/scripts/bash/common.sh) checks `SPECIFY_FEATURE_DIRECTORY` before [.specify/feature.json](.specify/feature.json).
Verify that the selected feature matches the task. Do not infer it only from the branch name.

### Backend

Use the boundaries defined by [Constitution VI](.specify/memory/constitution.md#vi-hexagonal-architecture--clean-boundaries):

- **Domain:** [internal/domain/](internal/domain/) owns business invariants.
- **Ports:** [internal/ports/](internal/ports/) defines infrastructure contracts.
- **Adapters:** [internal/adapters/](internal/adapters/) implements HTTP, storage, and external-service boundaries.
- **Configuration:** [internal/config/](internal/config/) implements the broker loader.
- **Composition:** [internal/app/builder.go](internal/app/builder.go) wires broker dependencies under Constitution XII.

### Frontend

Use the [design system](web/src/design-system/docs/INDEX.md), as required by [Constitution XI](.specify/memory/constitution.md#xi-design-system-compliance--consistency).
Read its component guidance before choosing an import or variant.
The [Vite aliases](web/vite.config.ts) and [existing pages](web/src/pages/) show current import paths.
The constitution requires semantic tokens, self-hosted brand assets, and accessibility evidence in light and dark themes.

### Tests

The [justfile](justfile) defines the following suite boundaries.
Go package tests generally live next to their sources. Frontend test locations follow [vitest.config.ts](web/vitest.config.ts).

- Fast Go/package tests: `just test`
- ExtProc unit tests with race detection: `just extproc-test`. These include checks that
  disabled local tracing neither creates child spans nor modifies inherited spans.
- Self-contained integration suites: `just test-integration`
- Infrastructure-backed integration suites: `just test-integration-infra`
- Both integration layers: `just test-integration-all`
- All E2E suites: `just test-e2e`
- Dedicated E2E performance measurement: `just test-e2e-performance` (manual; normal E2E commands exclude performance-labelled specs)
- Full verification gate, including security scanning: `just verify`
- Coverage for the fast Go/package suite: `just test-coverage`
- Coverage summary for fast Go/package tests: `just test-coverage-summary`
- Frontend coverage report: `just web-test-coverage`

Read the [E2E guide](tests/e2e/README.md) and [storage integration guide](tests/integration/storage/README.md) for prerequisites and cleanup limits.
Browser tests are implemented, not future work. `just test-e2e-frontend` builds and tests the production frontend bundle.
The [development-mode conflict](specs/agentic-engineering-improvements.md#4-make-frontend-development-tests-safe-by-default) remains open.
Use the guide's serial-worker override for development mode, not the current recipe default.

The [Coverage workflow](https://github.com/zalando-incubator/agentic-identity-broker/actions/workflows/scheduled-coverage.yml) runs manually from the GitHub Actions UI and weekly on `main`. It sends coverage from the Go unit, integration, and functional backend, ExtProc, and frontend E2E suites, plus Vitest and instrumented Playwright browser runs, to [Coveralls](https://coveralls.io/github/zalando-incubator/agentic-identity-broker?branch=main). Go coverage measures `cmd/` and `internal/`; browser coverage measures `web/src/`. The separate performance measurement is excluded. The README badge shows the latest main-branch result.

To reproduce the frontend report locally after installing the test tools, run:

```bash
npm run test:coverage --prefix web
VITE_COVERAGE=1 E2E_WEB_COVERAGE_DIR="$PWD/web/coverage/browser" just test-e2e-frontend-coverage
E2E_WEB_COVERAGE_DIR="$PWD/web/coverage/browser" npm run coverage:merge --prefix web
```

The last command writes the combined browser and Vitest report to `web/coverage/lcov.info`. The Go E2E coverage recipes produce separate profiles; Coveralls merges them with the Go unit/integration profile across flagged uploads.

## Pull Request Process

### Before Opening a PR

1. Run `just check` and `just verify`, and ensure both pass
2. Update [ARCHITECTURE.md](ARCHITECTURE.md) if design changed
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
- [ARCHITECTURE.md](ARCHITECTURE.md) - System design
- [Constitution](.specify/memory/constitution.md) - Binding governance and version history
- [MAINTAINERS](MAINTAINERS) - Maintainer information
- [Speckit](https://speckit.org/) - Specification-driven development

---

**Thank you for contributing!**
