# Implementation Plan: Third-Party OAuth2 Session Management

**Branch**: `20251222-120000-thirdparty-oauth2-sessions` | **Date**: 2025-12-23 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20251222-120000-thirdparty-oauth2-sessions/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Users need to authenticate with third-party OAuth2 services (e.g., GitHub, Google) through the identity broker. The broker orchestrates OAuth2 Authorization Code flows with PKCE, securely stores encrypted access/refresh tokens, and provides a UI for users to view, establish, and terminate their sessions. JWE state tokens bind OAuth2 flows to authenticated users, preventing CSRF attacks.

**Technical Approach**: 
- New `OAuth2SessionService` domain service orchestrating OAuth2 flows using Go's standard `golang.org/x/oauth2` library
- JWE state tokens using `github.com/lestrrat-go/jwx/v3` for encryption/decryption
- New `UserSession` entity with encrypted token storage using existing `EncryptionPort`
- HTTP handlers for authorize, callback, list sessions, and terminate session endpoints
- React frontend for displaying and managing third-party sessions

## Technical Context

**Language/Version**: Go 1.21+  
**Primary Dependencies**: 
- `golang.org/x/oauth2` (OAuth2 client flows)
- `github.com/lestrrat-go/jwx/v3` (JWE state tokens)
- Chi router (HTTP handlers)
- sqlx (PostgreSQL adapter)
- Viper/Cobra (configuration)  
**Storage**: PostgreSQL (user_sessions table) + In-memory adapter for testing  
**Testing**: Go standard library testing + testify assertions + testcontainers for PostgreSQL  
**Target Platform**: Linux server (containerized)  
**Project Type**: Web application (Go backend + React frontend)  
**Performance Goals**: OAuth2 flow completion < 60 seconds; Session page load < 2 seconds  
**Constraints**: OAuth2 tokens encrypted at rest; State tokens TTL ≤ 15 minutes; PKCE required  
**Scale/Scope**: Multi-user SaaS; ~10k concurrent sessions expected

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Have entities, aggregates, value objects been identified and documented?
  - UserSession entity (aggregate root)
  - OAuth2StateToken value object (JWE-encrypted flow state)
  - EncryptedToken value object (encrypted OAuth2 tokens)
  - OAuth2SessionService domain service (orchestrates OAuth2 flows)
- [x] **Domain Concepts**: Will new domain terms be added to ARCHITECTURE.md Glossary?
  - UserSession, OAuth2StateToken, OAuth2SessionService, Token Vault, Session Termination
- [x] **Configuration Design**: Have all config requirements been identified with YAML examples?
  - third_party_oauth2.jwe_signing_key, state_token_ttl, pkce_verifier_length
- [x] **Config Examples**: Will example YAML snippets be added to examples/config/?
  - examples/config/third-party-oauth2.yaml
- [x] **API Design First**: Will APIs be designed (OpenAPI spec) and confirmed BEFORE implementation?
  - OpenAPI specs in /api/enduser/openapi.yaml for session endpoints
- [x] **API Documentation**: Will OpenAPI specs be created in `/api/enduser/` or `/api/admin/` as applicable?
  - End-user APIs: authorize, callback, list sessions, terminate session
- [x] **API Changes**: Are all API changes confirmed by user/stakeholder (document in PR)?
  - Confirmed via feature spec.md user stories
- [x] **Database Design**: Will all schema changes use go-migrate naming in `/migrations/`?
  - 004_create_user_sessions.up.sql and 004_create_user_sessions.down.sql

**Implementation Considerations**:

- [x] **Security-First**: Are security features enabled by default? No bypasses or optional security?
  - PKCE mandatory, JWE state tokens mandatory, encryption mandatory, no bypasses
- [x] **Architecture Docs**: Will ARCHITECTURE.md be updated if this touches architecture?
  - New domain service documented in architecture
- [ ] **ADRs**: Does this require an ADR in adrs/ for major decisions?
  - Not required: follows existing hexagonal patterns, uses established libraries
- [x] **Library-First Security**: Are we using vetted libraries for crypto/security (no custom implementations)?
  - golang.org/x/oauth2 for OAuth2 flows
  - github.com/lestrrat-go/jwx/v3 for JWE (RFC 7516)
  - Existing EncryptionPort for token storage encryption
- [x] **Zalando Guidelines**: Will APIs follow Zalando RESTful API and Event Guidelines?
  - Resource-oriented endpoints, consistent error format, proper HTTP status codes
- [x] **End-User Docs**: Will API documentation be rendered in `docs/api/` with examples?
  - OAuth2 session management documentation
- [x] **Migration Testing**: Will migrations be tested (apply/rollback) in PostgreSQL integration tests?
  - Integration tests for user_sessions table
- [x] **Hexagonal Architecture**: Does domain logic use ports (interfaces) with clear adapter separation?
  - OAuth2SessionService in domain, UserSessionRepository port, adapters for memory/postgres
- [x] **Persistence Patterns**: If adding persistence, will it follow specs/20251215-120100-persistence-layer/quickstart.md?
  - UserSessionRepository interface, memory + postgres adapters, StorageError wrapping

*All BLOCKING preconditions verified. Implementation can proceed.*

## Project Structure

### Documentation (this feature)

```text
specs/20251222-120000-thirdparty-oauth2-sessions/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── oauth2-sessions.yaml  # OpenAPI contract for session endpoints
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
# Backend (Go - Hexagonal Architecture)
internal/
├── domain/
│   └── oauth2session/           # NEW: OAuth2 session domain
│       ├── service.go           # OAuth2SessionService (orchestrates flows)
│       ├── service_test.go      # Unit tests for service
│       ├── state_token.go       # OAuth2StateToken JWE handling
│       ├── state_token_test.go  # Unit tests for state tokens
│       └── errors.go            # Domain errors
├── domain/storage/
│   └── user_session.go          # NEW: UserSession entity
├── ports/
│   └── storage.go               # EXTEND: UserSessionRepository interface
├── adapters/
│   ├── http/
│   │   └── oauth2_sessions/     # NEW: HTTP handlers
│   │       ├── handler.go       # Authorize, callback, list, terminate handlers
│   │       └── handler_test.go  # Handler unit tests
│   └── storage/
│       ├── memory/
│       │   └── user_session.go  # NEW: In-memory adapter
│       └── postgres/
│           └── user_session.go  # NEW: PostgreSQL adapter

# Database migrations
migrations/
├── 004_create_user_sessions.up.sql    # NEW: Create user_sessions table
└── 004_create_user_sessions.down.sql  # NEW: Drop user_sessions table

# Frontend (React + TypeScript)
web/src/
├── pages/
│   └── ThirdPartySessionsPage.tsx     # NEW: Session management page
├── components/
│   └── sessions/                      # NEW: Session components
│       ├── SessionCard.tsx            # Session card with status
│       └── TerminationDialog.tsx      # Confirmation dialog
├── hooks/
│   └── useSessions.ts                 # NEW: Session API hooks
└── services/api/
    └── sessions.ts                    # NEW: Session API client

# Configuration
examples/config/
└── third-party-oauth2.yaml            # NEW: OAuth2 session config example

# Tests
tests/
├── integration/
│   └── user_sessions_test.go          # NEW: PostgreSQL integration tests
└── unit/
    └── oauth2session/                 # NEW: Domain service unit tests
```

**Structure Decision**: Web application structure following existing hexagonal architecture patterns. Backend in `internal/` with domain/ports/adapters separation. Frontend in `web/src/` following existing React component patterns. Migrations in `/migrations/` using go-migrate naming conventions.

## Complexity Tracking

> No constitution violations requiring justification. All patterns follow established conventions.
