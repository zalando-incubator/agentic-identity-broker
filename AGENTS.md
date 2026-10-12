# Agentic Identity Broker — Agent Context

> Read the relevant source and documents before you decide. Do not infer repository behavior from general knowledge.

## How to Use This File

Paths in this file are relative to the repository root. Read the nearest child `AGENTS.md` before you change an area.
Use the indexes to select relevant sources, not as a requirement to read every document.

Maintain shared instructions in the root or nearest child `AGENTS.md`.
Keep tool-specific instruction files limited to scope metadata and routes to those owners.
Read routed files before implementation or review. A route is not a substitute for its instructions.

## Docs Index (Retrieval Targets)

Read the source that governs the change. The summaries here do not replace the constitution, accepted ADRs, or API contracts.

### Architecture & Decisions

`ARCHITECTURE.md` | Source of truth for system design
`adrs/` | Architecture decisions and status (see the ADR index)
`.specify/memory/constitution.md` | Binding constitution — 13 principles governing all work
`CONTRIBUTING.md` | Setup, contribution process, and verification requirements
`docs/resources/assurance-case.md` | Threat model, trust boundaries, and security evidence

### Section-Specific Agent Context

`internal/AGENTS.md` | Backend hexagonal architecture, builder pattern, testing conventions
`internal/domain/AGENTS.md` | Domain layer rules, zero-infra imports, data models, error types
`internal/domain/id/AGENTS.md` | Strongly typed entity IDs, code generation, type catalogue
`internal/ports/AGENTS.md` | Port interface catalogue, ISP rules, error conventions
`internal/adapters/AGENTS.md` | Adapter map, cross-adapter ban, storage/encryption/HTTP details
`internal/extproc/AGENTS.md` | Standalone ExtProc token exchange service, config, architecture
`web/AGENTS.md` | React SPA, design system, API client, testing
`infra/AGENTS.md` | AWS CDK encryption stack, IRSA, environment parameterization
`tests/AGENTS.md` | E2E + integration test suite overview
`tests/e2e/AGENTS.md` | Ginkgo E2E rules, fixtures, dual-server pattern, anti-patterns
`tests/e2e/frontend/AGENTS.md` | Playwright browser tests, page objects
`tests/integration/AGENTS.md` | Self-contained and infrastructure-backed integration test guidance

### Runtime Entry Points

`cmd/agentic-identity-broker/` | Broker CLI, configuration, and dual-server startup
`cmd/extproc-token-exchange/` | ExtProc CLI, telemetry bridge, and gRPC startup

### API Contracts

`api/enduser/openapi.yaml` | End-user API: consent, OAuth2, sessions, and approvals
`api/admin/openapi.yaml` | Admin API: agents, services, resources, and permission sets

### Specs (Feature Requirements)

`specs/NNN-<name>/spec.md` | Feature requirements + acceptance scenarios
`specs/NNN-<name>/plan.md` | Implementation approach
`specs/NNN-<name>/tasks.md` | Trackable task list

For feature work, read its `spec.md`, `plan.md`, and `tasks.md` before implementation.
Spec Kit selects `SPECIFY_FEATURE_DIRECTORY` first, then `.specify/feature.json`.
Check that this selection matches the task. Do not infer the feature from the branch name alone.
`.specify/scripts/bash/common.sh` defines this selection order.

### Encryption Deep Reference

`.agents/skills/aws-crypto-go/SKILL.md` | AWS Encryption SDK Go — patterns, rules
`.agents/skills/aws-crypto-go/reference.md` | API reference
`.agents/skills/aws-crypto-go/examples.md` | Code examples

### Design System (Frontend)

`web/src/design-system/docs/INDEX.md` | Complete design system documentation index
`web/src/design-system/docs/COMMON_MISTAKES.md` | Read before you work on a styled component
`web/src/design-system/docs/DECISION_TREES.md` | Which component to use when

### Operations & Guides

`docs/ENCRYPTION_INTEGRATION_GUIDE.md` | Encryption integration guide
`docs/STORAGE_EXTENSION_GUIDE.md` | Adding new storage entities
`docs/STORAGE_TROUBLESHOOTING.md` | Storage debugging
`docs/configuration.md` | Configuration reference
`docs/deployment/` | Deployment guides (Kubernetes, IRSA)
`docs/operations/` | Operations runbooks
`examples/config/` | Example configuration files
`charts/agentic-identity-broker/` | Helm deployment contract
`migrations/` | PostgreSQL schema migrations
`mocks/` | Standalone test services and OAuth2 clients
`configs/` | Development, test, production, and container configuration
`scripts/` | Development helpers and deployment checks
`justfile` | Maintained commands and their dependencies
`.github/workflows/` | CI, security, coverage, and release gates
`assets/docusaurus/` | Documentation-site source and build configuration

## Constitution (Binding) — 13 Principles

Treat `.specify/memory/constitution.md` as **BINDING**. Apply these principles:

1. **Security-First** — Fail closed. Do not bypass security controls. Only accepted ADR 031 permits unsigned subjects, with its full controls. This exception applies only to explicit unverified-subject rules in local-mode impersonation. Client assertions, actor tokens, and signed subjects still require signature verification.
2. **ADRs are Binding** — Treat ADRs as binding. Use `ARCHITECTURE.md` as the source of truth. A deviation requires a superseding ADR.
3. **Library-First Security** — Do not use custom crypto. Use Go `crypto/*`, `golang.org/x/crypto`, or AWS Encryption SDK.
4. **OpenAPI Transparency** — Define APIs in `api/{enduser,admin}/openapi.yaml` before implementation.
5. **Domain-Driven Design** — Enforce ubiquitous language. Add new concepts to the `ARCHITECTURE.md` glossary.
6. **Hexagonal Architecture** — Domain services depend on ports, not adapters. Adapters implement ports. Prevent these violations:
   - **Port bypass**: Route handlers through domain services to ports. Do not route a handler to a port directly.
   - **Anemic domain**: Make services enforce invariants. Do not only proxy port calls.
   - **Domain logic leakage**: Put conditional logic beyond input parsing in domain services, not handlers.
   - **Domain packaging**: Make each new `domain/X/` a genuinely independent bounded context.
7. **Configuration-Driven** — Use `internal/ports/config.go` for broker configuration. ExtProc has its own schema under accepted ADR 011. Update configuration documentation and examples with each parameter change. Update Helm values and templates only for workloads that the chart deploys.
8. **TDD** — Use red-green-refactor. Write tests first. Make each test fail before implementation.
9. **Persistence Consistency** — Define ISP repos in `internal/ports/storage.go`. Use sqlx for PostgreSQL. Provide both in-memory and postgres adapters.
10. **API-First** — Design APIs before implementation. Get stakeholder agreement before changes.
11. **Design System Compliance** — Use shared semantic tokens and WCAG 2.1 AA. Check light and dark themes. Keep brand assets in the repository. Read the current visual direction in `web/src/design-system/docs/DESIGN_PRINCIPLES.md`. A change to that direction requires an ADR.
12. **DI via Builder** — Put broker dependency wiring in `internal/app/builder.go`. Do not instantiate services in routing.
13. **E2E Acceptance Tests** — Use a 1:1 specification-to-test mapping in `tests/e2e/`. Test a complete user journey. Do not test one endpoint segment.

## Monorepo Structure

| Section | Technology | AGENTS.md |
|---|---|---|
| `internal/` | Go — domain services, ports, adapters, and app wiring | `internal/AGENTS.md` |
| `web/` | React + TypeScript + Vite + Tailwind | `web/AGENTS.md` |
| `infra/cdk/` | AWS CDK (Go) — KMS, DynamoDB, IAM | `infra/AGENTS.md` |
| `tests/` | Ginkgo/Gomega (e2e), Go testing (integration) | `tests/AGENTS.md` |
| `internal/extproc/` | Standalone gRPC ExtProc service | `internal/extproc/AGENTS.md` |

**Import rules**: Do not let the domain import adapters or app. Define contracts in ports.
Let driven adapters import ports and domain. HTTP routing receives handlers from app.
It can use routing middleware and configuration. Do not couple independent adapter subsystems.
Read `internal/adapters/AGENTS.md` for existing factory and shared-helper composition.

## ADR Decision Index

Read relevant ADRs before implementation. Only accepted decisions are binding.
Check each file's status and superseding decisions. A proposal does not become binding because its code exists.
ADR numbers are not unique in this repository. Use the full filename.

| ADR | File | Decision |
|---|---|---|
| 002 | `adrs/002-configuration-libraries.md` | Viper + Cobra + godotenv config |
| 003 | `adrs/003-chi-framework.md` | chi v5 HTTP router |
| 004 | `adrs/004-dual-server-isolation.md` | errgroup dual-server :8000/:14000 |
| 004 | `adrs/004-storage-layer-architecture.md` | Hexagonal storage, ISP repos, sqlx |
| 005 | `adrs/005-spa-serving-pattern.md` | SPA serving, with mount and build paths superseded by ADR 035 |
| 006 | `adrs/006-frontend-stack.md` | React + Vite + Tailwind + Vitest |
| 007 | `adrs/007-e2e-testing-with-ginkgo.md` | Ginkgo v2 BDD E2E tests |
| 008 | `adrs/008-encryption-context-optimization.md` | Single-subject context: `service_id` or `kid`, as defined by its amendment |
| 008 | `adrs/008-token-exchange-jwks-adapter-pattern.md` | JWKS adapter with jwx jwk.Cache |
| 009 | `adrs/009-cel-for-authorization-policies.md` | cel-go for authorization + JWT claims |
| 009 | `adrs/009-envelope-encryption-design.md` | Three-layer envelope: KEK→BranchKey→DEK |
| 009 | `adrs/009-separate-migration-docker-image.md` | Separate migration Docker image |
| 010 | `adrs/010-cdk-encryption-infrastructure.md` | AWS CDK for encryption IaC |
| 011 | `adrs/011-extproc-standalone-binary.md` | ExtProc as standalone binary |
| 011 | `adrs/011-opentelemetry-provider-pattern.md` | OpenTelemetry provider pattern |
| 012 | `adrs/012-encryption-layer-separation.md` | Encryption layer separation |
| 012 | `adrs/012-extproc-in-memory-token-cache.md` | ExtProc in-memory token cache |
| 013 | `adrs/013-strongly-typed-entity-ids.md` | Typed entity IDs (`XxxID`) in `domain/id/` |
| 014 | `adrs/014-long-poll-listen-notify.md` | Long-poll approval sync via PostgreSQL LISTEN/NOTIFY |
| 014 | `adrs/014-oauth2-server-mode.md` | OAuth2 authorization server mode |
| 015 | `adrs/015-cimd-fetcher-architecture.md` | CIMD fetcher architecture |
| 016 | `adrs/016-authorization-session-anti-spoofing.md` | Authorization session anti-spoofing |
| 017 | `adrs/017-optional-agent-client-id.md` | Optional/nullable Agent client_id |
| 018 | `adrs/018-approval-endpoint-auth-boundaries.md` | Endpoint-specific approval authentication boundaries |
| 027 | `adrs/027-extproc-telemetry-shared-dependency.md` | ExtProc telemetry shared dependency |
| 028 | `adrs/028-opa-extproc-authorization.md` | OPA authorization in ExtProc via embedded SDK |
| 029 | `adrs/029-token-exchange-client-assertion-trust-anchor.md` | Proposed: dedicated client-assertion trust anchor |
| 030 | `adrs/030-normalize-protected-resources.md` | Normalized, globally unique protected-resource child records |
| 030 | `adrs/030-vendor-neutral-oidc-trust.md` | Vendor-neutral CDK OIDC trust configuration |
| 031 | `adrs/031-unverified-subject-unsigned-jwt.md` | Bounded unsigned-subject exception for local impersonation |
| 032 | `adrs/032-impersonation-requires-user-delegation.md` | User delegation required for OAuth2 impersonation |
| 033 | `adrs/033-request-security-context-propagation.md` | Proposed: request security context and audit identity |
| 034 | `adrs/034-audience-absent-impersonation-credentials.md` | Explicit absent-audience policy for signed impersonation credentials |
| 035 | `adrs/035-root-mounted-spa.md` | Root-mounted SPA. `/consent` is not mounted |
| 035 | `adrs/035-shared-tool-pattern-matching.md` | Approval pattern grammar shared with ExtProc |
| 036 | `adrs/036-extproc-approval-cache-and-gate.md` | ExtProc approval cache, sync, and request gate |
| 036 | `adrs/036-extproc-metadata-token-exchange-input.md` | Token-exchange input from gateway metadata |
| 036 | `adrs/036-public-client-token-endpoint-auth.md` | Explicit public-client authentication, extended by CIMD ADR 037 |
| 037 | `adrs/037-cimd-client-authentication-key-domain.md` | Separate CIMD authentication keys and public JWK sets |
| 037 | `adrs/037-consent-selection-oauth-redirect-state.md` | Proposed: consent selection across OAuth redirects |

## Domain Glossary

Read `ARCHITECTURE.md` for the full glossary and relationships.

| Term | Definition |
|---|---|
| **Agent** | AI agent with optional client ID and client metadata document URLs. |
| **ThirdpartyOAuth2Service** | External OAuth2 provider with credentials and scopes. |
| **UserGrant** | User delegation of permission sets to an agent. One per user-agent pair. |
| **AuthorizationSessionClaims** | JWE-sealed authorization context with a 10-minute TTL. |
| **UserSession** | Authenticated OAuth2 session with encrypted tokens. |
| **Principal** | Authenticated user identity from the configured pre-authentication source. |
| **Secret** | Value object with exclusive plaintext, encrypted, or absent state. |
| **ServiceRequirement** | Agent requirement for a mandatory or optional service. |
| **BranchKey** | DynamoDB-cached key between a KMS KEK and an operation DEK. |
| **EncryptionContext** | AAD with exactly one subject key: `service_id` or `kid`. Never store secrets. |
| **ResourceURI** | Normalized protected-resource URI for RFC 8693 token exchange. |
| **CEL Expression** | Policy for privileged-client authorization and JWT claim extraction. |
| **ToolApproval** | Human-in-the-loop authorization for an agent tool call with pending, approved, or denied status. |

## Development Workflow

Use `just` as the command runner. Run `just --list` for the full listing.

| Command | Purpose |
|---|---|
| `just check` | Non-mutating format, vet, and lint checks |
| `just fmt` | Apply Go formatting changes |
| `just build` | Build Go binary → `./bin/agentic-identity-broker` |
| `just test` | Fast Go/package tests (no E2E or integration suites) |
| `just verify` | Full gate: static checks, security scans, tests, integration, then E2E |
| `just build-all` | Backend + frontend build |
| `just test-e2e` | All backend, ExtProc, and frontend E2E suites |
| `just test-integration` | Default self-contained integration suites |
| `just test-integration-infra` | Infra-backed integration suites (Docker/Podman required) |
| `just test-integration-all` | Both integration layers |
| `just cdk-test` | CDK unit tests |

After Go, test, or infrastructure changes, run `just check`. Then run the smallest matching test command.
Do not rely only on editor diagnostics. Before a pull request, run the full gate from `CONTRIBUTING.md`.

## Code Style

Use these rules for all monorepo code (Go, TypeScript, CDK):

- **Minimize comments** — Write a comment only for context that code alone cannot convey.
- **Never restate code** — Do not write comments that add no information.
- **No debug artifacts** — Do not leave debug logging, commented-out code, TODO stubs, or future-implementation references.
- **Match surrounding style** — Keep naming, error handling, and file structure consistent with adjacent code.
- **Explicit over clever** — Make intent clear without tracing abstractions.
- **Extend, do not duplicate** — Use an existing mechanism (for example, JWE for ephemeral state). Do not introduce a competing one.

- **No test-only production APIs** — Put test helpers in `_test.go` or `tests/`. Assert through production APIs.

## Code Review Rules

Use the governing references and feature-selection process in this file before review.
Read the nearest child `AGENTS.md` for each changed area.

- Prioritize security faults and performance problems that can affect users.
- Check for exposed secrets, injection, unsafe input handling, and authentication or authorization errors.
- Check for N+1 queries, inefficient loops, resource leaks, and expensive repeated operations.
- Identify incomplete implementations and violations of the Code Style rules.
- Suggest concrete improvements to unclear names, oversized functions, and error handling.
- Verify changes against `ARCHITECTURE.md`, accepted ADRs, and the constitution.
- Read the governing feature's full `tasks.md`. Verify that each checked task has a production-quality implementation.
- Report specific findings with source locations, impact, and actionable corrections.
- Explain useful patterns as well as problems. Ask for clarification when the source does not establish intent.

## Database Migrations

Use `NNN_description.{up,down}.sql` in `migrations/`. Test both directions against PostgreSQL with the production migrations.
For large-table indexes, use `CREATE INDEX CONCURRENTLY` outside a transaction.
`migrations/030_cimd_client_uri_pattern_index.up.sql` is the existing example.
Read `tests/integration/AGENTS.md` for migration tests.

## Active Technologies

Read `go.mod` and each module's manifest for exact versions.
The stack uses chi v5, sqlx, pgx v5, Ginkgo/Gomega, Viper/Cobra, CVA, OpenTelemetry, and Envoy ExtProc.
PostgreSQL is the production store. In-memory storage is for development and tests.
