# Implementation Plan: OAuth2 Authorization Server Proxy

**Branch**: `20251222-120100-oauth2-auth-server` | **Date**: 2025-12-22 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20251222-120100-oauth2-auth-server/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

The identity broker will act as an OAuth2 Authorization Server proxy, exposing RFC 6749 compliant `/oauth2/authorize` and `/oauth2/token` endpoints on the enduser server. The broker validates client_id against registered Agents, checks user consent via the existing grant system, and proxies authorized requests to an upstream OAuth2 server. Authorization requests redirect the user's browser to the upstream server, while token requests use server-to-server HTTP POST. The broker also exposes `/.well-known/oauth-authorization-server` for OAuth2 metadata discovery.

## Technical Context

**Language/Version**: Go 1.23.0+ (established in go.mod)
**Primary Dependencies**:
- chi/v5 v5.2.3 (HTTP router for OAuth2 endpoints)
- viper v1.21.0 (configuration management)
- slog (structured logging for audit trails)
- net/http (HTTP client for upstream communication)

**Storage**: PostgreSQL + in-memory (existing agent registry and grant store, no new persistence required)
**Testing**: Go standard library testing + testify assertions, testcontainers for integration tests
**Target Platform**: Linux server (enduser HTTP server on port 8000)
**Project Type**: Single/web (backend Go service)
**Performance Goals**:
- Authorization requests with valid consent proxy within 500ms
- Token endpoint proxies requests within 500ms
- Metadata endpoint responds within 100ms
- Support 100 concurrent OAuth2 authorization flows

**Constraints**:
- No token issuance/validation (broker is proxy only)
- Upstream OAuth2 server must be RFC 6749 compliant
- TLS certificate validation required (no self-signed certificates)
- Audit logging only for authorization endpoint (token logging delegated to upstream)

**Scale/Scope**:
- 3 new HTTP endpoints (/oauth2/authorize, /oauth2/token, /.well-known/oauth-authorization-server)
- Integration with existing agent registry (006) and grant store (006)
- Integration with consent UI (007)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: No new entities required; uses existing Agent and User Grant entities
- [x] **Domain Concepts**: No new domain terms; uses standard OAuth2 RFC terminology
- [x] **Configuration Design**: Configuration requirements defined with `oauth2_authorization_server` block
- [x] **Config Examples**: Will add examples/config/oauth2-authorization-server.yaml
- [x] **API Design First**: OAuth2 endpoints follow RFC 6749/8414; will document in /api/enduser/openapi.yaml
- [x] **API Documentation**: Will document OAuth2 endpoints in `/api/enduser/openapi.yaml`
- [x] **API Changes**: OAuth2 endpoints are new (not changes); user confirmed proxy architecture via clarifications
- [x] **Database Design**: No database schema changes required

**Implementation Considerations**:

- [x] **Security-First**:
  - TLS certificate validation enabled by default (reject self-signed/expired)
  - OAuth2 error responses on security failures (fail closed)
  - No sensitive data logging (SR-006)
  - Rate limiting delegated to upstream (clarification confirmed)
- [x] **Architecture Docs**: Will update ARCHITECTURE.md with OAuth2 proxy flow diagram
- [x] **ADRs**: No major architectural decisions requiring new ADR (follows existing hexagonal architecture)
- [x] **Library-First Security**: Using Go standard library net/http for TLS validation (crypto/tls), no custom crypto
- [x] **Zalando Guidelines**: OAuth2 endpoints follow RFC 6749/8414 (industry standard for OAuth2)
- [x] **End-User Docs**: Will add OAuth2 flow examples to docs/configuration.md
- [x] **Migration Testing**: N/A (no database migrations)
- [x] **Hexagonal Architecture**:
  - Domain service uses AgentRepository and GrantRepository ports
  - HTTP handlers are driving adapters
  - HTTP client to upstream is driven adapter
- [x] **Persistence Patterns**: N/A (no new persistence entities)

*All BLOCKING checks pass. Implementation may proceed after Phase 0 research.*

## Project Structure

### Documentation (this feature)

```text
specs/20251222-120100-oauth2-auth-server/
├── spec.md              # Feature specification (/speckit.specify output)
├── plan.md              # This file (/speckit.plan output)
├── research.md          # Phase 0 output (generated below)
├── data-model.md        # Phase 1 output (generated below)
├── quickstart.md        # Phase 1 output (generated below)
├── contracts/           # Phase 1 output (OpenAPI specs)
│   ├── openapi.yaml     # OAuth2 endpoints (will merge into /api/enduser/openapi.yaml)
│   └── README.md        # Contract documentation
├── checklists/
│   └── requirements.md  # Spec quality checklist (already created)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Backend (Go hexagonal architecture)
internal/
├── ports/
│   ├── oauth2.go                 # OAuth2 service port interface (NEW)
│   └── storage.go                # Existing: AgentRepository, GrantRepository
├── domain/
│   ├── oauth2/
│   │   ├── authorization.go      # Authorization request domain logic (NEW)
│   │   ├── token.go              # Token request domain logic (NEW)
│   │   ├── metadata.go           # Metadata discovery logic (NEW)
│   │   └── errors.go             # OAuth2-specific errors (NEW)
│   └── storage/
│       ├── agent.go              # Existing: Agent entity
│       └── grant.go              # Existing: User Grant entity
├── adapters/
│   ├── http/
│   │   ├── enduser/
│   │   │   ├── oauth2_authorize.go       # /oauth2/authorize handler (NEW)
│   │   │   ├── oauth2_token.go           # /oauth2/token handler (NEW)
│   │   │   ├── oauth2_metadata.go        # /.well-known handler (NEW)
│   │   │   └── middleware/
│   │   │       └── oauth2_audit.go       # Audit logging middleware (NEW)
│   │   └── upstream/
│   │       └── oauth2_client.go          # HTTP client for upstream OAuth2 (NEW)
│   └── storage/
│       ├── memory/                       # Existing: in-memory adapters
│       └── postgres/                     # Existing: PostgreSQL adapters
└── config/
    └── schema.go                         # Add oauth2_authorization_server block (MODIFY)

# Configuration examples
examples/config/
└── oauth2-authorization-server.yaml      # Configuration example (NEW)

# API Documentation
api/enduser/
└── openapi.yaml                          # Add OAuth2 endpoints (MODIFY)

# Tests
internal/adapters/http/enduser/
├── oauth2_authorize_test.go              # Authorization endpoint unit tests (NEW)
├── oauth2_token_test.go                  # Token endpoint unit tests (NEW)
└── oauth2_metadata_test.go               # Metadata endpoint unit tests (NEW)

internal/domain/oauth2/
├── authorization_test.go                 # Authorization logic unit tests (NEW)
├── token_test.go                         # Token logic unit tests (NEW)
└── metadata_test.go                      # Metadata logic unit tests (NEW)

tests/integration/
└── oauth2_flow_test.go                   # End-to-end OAuth2 flow integration tests (NEW)
```

**Structure Decision**: Single backend project following established hexagonal architecture. OAuth2 functionality is organized in dedicated `internal/domain/oauth2/` package with HTTP handlers in `internal/adapters/http/enduser/` and upstream HTTP client in `internal/adapters/http/upstream/`. No new storage adapters required; uses existing agent registry and grant store via ports.

## Complexity Tracking

No Constitution violations requiring justification. The feature follows established patterns:
- Uses existing hexagonal architecture (domain logic via ports/adapters)
- No custom cryptography (uses Go standard library TLS validation)
- No new persistence entities (reuses existing repositories)
- Configuration via unified viper-based system
- API documentation in established OpenAPI location

---

# Phase 0: Research & Decisions

## Research Topics

1. **OAuth2 RFC 6749 Authorization Code Flow**: Understand parameter validation, error codes, and redirect semantics
2. **OAuth2 RFC 8414 Metadata Discovery**: Understand required and optional metadata fields
3. **Go net/http Client TLS Configuration**: Research proper TLS certificate validation configuration
4. **HTTP Redirect Best Practices**: Research proper HTTP 302 vs 303 usage for OAuth2 flows
5. **OAuth2 Error Response Format**: Understand RFC 6749 error response structure
6. **Go chi Router Integration**: Research chi middleware patterns for audit logging
7. **Upstream HTTP Proxy Patterns**: Research best practices for proxying HTTP requests (headers, timeouts, status codes)
8. **OAuth2 Content-Type Validation**: Research token endpoint CSRF protection via Content-Type validation

## Research Execution

Running research tasks to resolve technical unknowns...

