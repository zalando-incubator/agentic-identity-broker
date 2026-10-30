# Implementation Plan: Request Principal Extraction

**Branch**: `20251216-120000-session-management` | **Date**: 2025-12-16 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20251216-120000-session-management/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implement request principal extraction middleware to extract authenticated user principals (username, email, user ID) from HTTP headers set by a trusted reverse proxy. The middleware will use chi router v5 route groups to enable per-route control, propagate principals via standard Go context, and validate input according to security requirements. No database storage is required—context is scoped to individual requests.

**Technical Approach**: Implement HTTP middleware using chi v5 route groups with type-safe Go context propagation. Create `RequirePrincipalMiddleware` for protected routes (reject 401 if missing) and `OptionalPrincipalMiddleware` for routes that benefit from but don't require principals. Use middleware factory pattern with configuration for header name. Follow hexagonal architecture with middleware in adapters/http and context utilities in a dedicated package.

## Technical Context

**Language/Version**: Go 1.24.0 (already configured in go.mod)
**Primary Dependencies**: chi/v5 v5.2.3 (HTTP router), viper v1.21.0 (configuration), slog (structured logging)
**Storage**: N/A (per-request context only, no persistence)
**Testing**: Go standard library testing, testify v1.11.1 for assertions, httptest for HTTP handler testing, table-driven tests
**Target Platform**: Linux server (containerized deployment)
**Project Type**: Single backend service (Go HTTP server)
**Performance Goals**: < 1ms added latency at p95 (50-100ns context lookups, validated via benchmarks)
**Constraints**: < 200ms total request latency at p95, fail closed security (reject invalid requests)
**Scale/Scope**: Middleware applied to HTTP endpoints requiring user context, supporting 1000+ req/s throughput

## Configuration Structure

To support future extensibility (particularly JWT authentication), the configuration uses a nested structure:

```yaml
servers:
  admin:
    authentication:
      preauth:
        principal_header_name: "X-Remote-User"
      # Future: jwt section can be added here without breaking changes
      # jwt:
      #   enabled: false
      #   public_key_path: ""
  api:
    authentication:
      preauth:
        principal_header_name: "X-Remote-User"
```

**Rationale**:
- `authentication` namespace groups all authentication methods
- `preauth` specifically identifies reverse proxy pre-authentication
- Future `jwt` section can be added as a sibling to `preauth`
- Backwards-compatible: new features don't break existing config
- Clear separation: each auth method has its own configuration scope

**Default Values**:
- `servers.admin.authentication.preauth.principal_header_name`: `"X-Remote-User"`
- `servers.api.authentication.preauth.principal_header_name`: `"X-Remote-User"`

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

- [x] **Security-First**: ✅ Yes. Protected routes reject requests with missing/invalid principals (401/400). Middleware fails closed by default. No optional security bypasses.
- [x] **Architecture Docs**: ✅ Yes. ARCHITECTURE.md will be updated to document principal extraction middleware and request context patterns. Glossary will include Principal and Request Context.
- [x] **ADRs**: ⚠️ Minor decision. Does not require ADR—follows established hexagonal architecture patterns and chi middleware conventions. No new architectural patterns introduced.
- [x] **Library-First Security**: ✅ N/A. No cryptographic operations. Uses standard Go HTTP header validation and context propagation.
- [x] **API Documentation**: ✅ Yes. docs/ will be updated with middleware usage, configuration options, and per-route principal extraction patterns.
- [x] **Domain Model**: ✅ Yes. Principal and Request Context will be documented in ARCHITECTURE.md Glossary section.
- [x] **Hexagonal Architecture**: ✅ Yes. Middleware lives in internal/adapters/http. Context utilities are adapter-agnostic. Domain services depend on context.Context interface, not concrete implementations.

**Constitution Compliance**: ✅ PASS. All checks satisfied. No violations requiring justification.

## Project Structure

### Documentation (this feature)

```text
specs/20251216-120000-session-management/
├── spec.md              # Feature specification
├── plan.md              # This file (implementation plan)
├── research.md          # Phase 0 output (chi patterns, context best practices)
├── data-model.md        # Phase 1 output (Principal, Request Context entities)
├── quickstart.md        # Phase 1 output (implementation guide)
└── contracts/           # Phase 1 output (HTTP middleware contracts)
    └── middleware.md    # Middleware behavior specification
```

### Source Code (repository root)

This is a single Go backend service following hexagonal architecture:

```text
internal/
├── adapters/
│   ├── http/
│   │   ├── server.go              # Existing HTTP server adapter
│   │   ├── middleware.go          # Existing middleware (logging, recovery)
│   │   ├── principal_middleware.go  # NEW: RequirePrincipalMiddleware, OptionalPrincipalMiddleware
│   │   ├── principal_middleware_test.go  # NEW: Middleware unit tests
│   │   └── health.go              # Existing health check handler
│   └── storage/
│       └── [existing storage adapters]
│
├── ports/
│   ├── config.go                  # Existing configuration port
│   ├── server.go                  # Existing server port
│   └── storage.go                 # Existing storage port
│
├── domain/
│   ├── config/                    # Existing config domain
│   ├── server/                    # Existing server domain
│   ├── storage/                   # Existing storage domain
│   └── principal/                 # NEW: Principal domain package
│       ├── context.go             # NEW: Context key, WithPrincipal, FromContext functions
│       ├── context_test.go        # NEW: Context propagation tests
│       └── errors.go              # NEW: Principal validation errors
│
└── config/
    └── schema.go                  # UPDATE: Add authentication.preauth.principal_header_name field

cmd/
└── agentic-identity-broker/
    └── main.go                    # UPDATE: Configure principal middleware

tests/
├── integration/
│   └── principal_middleware_test.go  # NEW: End-to-end middleware tests
└── [existing test directories]

docs/
├── configuration.md               # UPDATE: Document authentication.preauth.principal_header_name config
├── architecture.md                # UPDATE: Document middleware patterns
└── api/
    └── middleware.md              # NEW: Middleware usage guide
```

**Structure Decision**: Single Go backend service (existing structure). New code follows established hexagonal architecture:
- **Adapters**: HTTP middleware in `internal/adapters/http/principal_middleware.go`
- **Domain**: Context utilities in `internal/domain/principal/` (adapter-agnostic)
- **Ports**: Configuration integrated into existing `ports.ServerInstanceConfig`
- **Testing**: Unit tests colocated with source, integration tests in `tests/integration/`

No new directories required beyond `internal/domain/principal/` for context utilities.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

N/A - No constitutional violations. All checks passed.

## Phase 0: Research (Completed)

**Output**: [research.md](research.md)

The golang-pro agent researched chi v5 route groups, Go context propagation, HTTP middleware patterns, error handling, testing strategies, and configuration integration. Key findings:

1. **Chi Router Route Groups**: Use `router.Group(func(r chi.Router) { r.Use(middleware); r.Get(...) })` for per-route middleware control
2. **Context Propagation**: Type-safe context keys using unexported types, `WithPrincipal/FromContext` functions
3. **Middleware Pattern**: Factory pattern with configuration closure, fail closed security
4. **Error Handling**: Structured JSON responses, 401 for missing principals, 400 for malformed principals
5. **Testing**: Table-driven tests with httptest, context propagation tests, integration tests with chi router
6. **Configuration**: Middleware factory accepting header name, integration with existing config system

## Phase 1: Design & Contracts

### Phase 1.1: Data Model

**Task**: Extract entities from feature spec and document in `data-model.md`

**Entities to Document**:
1. **Principal**: String identifier extracted from HTTP header (username, email, user ID)
   - Fields: value (string), source header (string)
   - Validation: trimmed, non-empty, ≤ 200 characters
   - Lifecycle: Request-scoped, not persisted

2. **Request Context**: Go context.Context with embedded principal
   - Fields: principal (Principal), request metadata
   - Validation: Context must contain principal for protected routes
   - Lifecycle: Created by middleware, passed to all downstream handlers

**Output**: `data-model.md` with entity definitions, relationships, validation rules

### Phase 1.2: API Contracts

**Task**: Generate middleware behavior specification in `contracts/middleware.md`

**Contracts to Document**:
1. **RequirePrincipalMiddleware Contract**:
   - Input: HTTP request with configured header
   - Output: Context with principal OR HTTP 401/400 response
   - Behavior: Extract, validate, reject if missing/invalid

2. **OptionalPrincipalMiddleware Contract**:
   - Input: HTTP request with optional configured header
   - Output: Context with principal (if present and valid)
   - Behavior: Extract if present, continue if missing

3. **Context Propagation Contract**:
   - Input: Context from middleware
   - Output: Principal value accessible via `principal.FromContext(ctx)`
   - Behavior: Type-safe retrieval, error handling for missing principal

**Output**: `contracts/middleware.md` with behavior specifications, error conditions, examples

### Phase 1.3: Quickstart Guide

**Task**: Generate implementation guide in `quickstart.md`

**Sections**:
1. Overview: Principal extraction pattern
2. Configuration: Setting `authentication.preauth.principal_header_name` in config (extensible for future JWT support)
3. Middleware Setup: Applying to chi route groups
4. Context Usage: Retrieving principal in handlers
5. Testing: Unit and integration test examples
6. Common Patterns: Protected routes, optional routes, error handling

**Output**: `quickstart.md` with step-by-step implementation guide

### Phase 1.4: Agent Context Update

**Task**: Run `.specify/scripts/bash/update-agent-context.sh claude` to update AGENT.md

**Updates**:
- Add chi v5 middleware patterns
- Add Go context propagation patterns
- Document principal extraction as implemented feature

**Output**: Updated AGENT.md

## Phase 2: Constitution Re-Check

After Phase 1 design artifacts are complete, re-evaluate Constitution Check to ensure:
- Architecture documentation requirements are met
- API documentation requirements are met
- Domain model is properly documented
- Hexagonal architecture is maintained

**Expected Result**: All constitution checks remain passing after design phase.

## Next Steps (Post-Planning)

After this plan is approved and Phase 1 design artifacts are generated:

1. Run `/speckit.tasks` to generate `tasks.md` from this plan
2. Implement per tasks.md (Phase 3: Implementation)
3. Run the verification gate: `just verify`
4. Update documentation: `docs/configuration.md`, `ARCHITECTURE.md`
5. Create PR for review

## Dependencies

- Existing chi router setup in `internal/adapters/http/server.go`
- Existing configuration system in `internal/config/schema.go`
- Existing structured logging with slog

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance overhead from context lookups | Medium | Benchmarks show <100ns per lookup, well within <1ms budget |
| Context key collisions | Low | Use unexported type for context key (Go best practice) |
| Header name case-sensitivity issues | Medium | Use `http.Header.Get()` for case-insensitive lookup |
| Reverse proxy misconfiguration | High | Clear documentation, fail closed design, structured logging |

## Success Criteria

Implementation will be considered successful when:

1. ✅ Protected routes reject requests without valid principals (401)
2. ✅ Protected routes reject principals >200 chars (400)
3. ✅ Principal is accessible via context in all downstream handlers
4. ✅ Optional routes work with or without principals
5. ✅ Performance: <1ms latency added at p95 (validated via benchmarks)
6. ✅ Test coverage: >90% for middleware and context utilities
7. ✅ Documentation: Configuration, architecture, and API docs updated
8. ✅ Integration tests: End-to-end tests with chi router pass
