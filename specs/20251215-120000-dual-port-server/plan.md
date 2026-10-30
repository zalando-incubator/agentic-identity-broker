# Implementation Plan: Dual-Port HTTP Server

**Branch**: `20251215-120000-dual-port-server` | **Date**: 2025-12-15 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20251215-120000-dual-port-server/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Implement two independent HTTP servers (end-user on port 8000, admin on port 14000) using the chi framework, supporting dual-stack IPv4/IPv6 with atomic startup, independent health endpoints, and graceful shutdown. Configuration through YAML/environment/CLI with the existing flexible configuration system.

## Technical Context

**Language/Version**: Go 1.23.0+
**Primary Dependencies**:
- chi v5 (HTTP routing framework - lightweight, idiomatic, stdlib-compatible)
- Existing: Viper, Cobra, godotenv (configuration system from 20251214-120100-flexible-configuration)
- Existing: Standard library crypto/* packages

**Storage**: N/A (server infrastructure only, no persistence in this feature)
**Testing**: Go standard library testing, table-driven tests for configuration validation
**Target Platform**: Linux/macOS/Windows server environments, Docker containers
**Project Type**: Single project (Go backend service)
**Performance Goals**:
- Server startup within 5 seconds
- Support 1000+ concurrent connections per server
- Graceful shutdown completing 99% of in-flight requests within timeout

**Constraints**:
- Atomic startup (both servers must succeed or neither starts)
- No TLS support (deferred to future feature)
- No custom timeout configuration (using Go stdlib defaults)
- HTTP only (no HTTPS in this feature)

**Scale/Scope**:
- 2 independent HTTP server instances
- 4 prioritized user stories
- 22 functional requirements + 6 security requirements
- IPv4 and IPv6 dual-stack support with automatic detection

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

- [x] **Security-First**: Security features enabled by default - health endpoints return appropriate status codes, structured logging for all security events, no optional security bypasses
- [x] **Architecture Docs**: ARCHITECTURE.md will be updated to document the dual-server architecture, server lifecycle, and hexagonal boundaries
- [x] **ADRs**: Will create ADR for chi framework selection and dual-server isolation strategy
- [x] **Library-First Security**: Using Go stdlib crypto/* for any security operations, chi framework for HTTP handling (no custom implementations)
- [x] **API Documentation**: docs/api.md will document health check endpoints and server behavior
- [x] **Domain Model**: New domain concepts (EndUserServer, AdminServer, ServerConfig, BindAddress) will be added to ARCHITECTURE.md Glossary
- [x] **Hexagonal Architecture**: Servers will be adapters (driving/inbound), configuration through existing config port, domain logic isolated via interfaces
- [x] **Configuration-Driven**: Using existing internal/ports/config.go system, extending Config struct, documentation in docs/configuration.md
- [x] **Test-Driven Development**: Automated tests for configuration validation, server startup/shutdown, health endpoints - no Bash validation for correctness

*All checks pass. This feature aligns with constitution principles.*

## Project Structure

### Documentation (this feature)

```text
specs/20251215-120000-dual-port-server/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   └── health-api.yaml  # OpenAPI spec for health check endpoints
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── ports/
│   ├── config.go         # EXISTING - extend Config struct for server settings
│   └── server.go         # NEW - port interface for HTTP server operations
├── adapters/
│   ├── http/             # NEW - HTTP server adapters
│   │   ├── server.go     # HTTP server implementation using chi
│   │   ├── health.go     # Health check handlers
│   │   └── middleware.go # Common middleware (logging, recovery)
│   └── config/           # EXISTING - configuration adapter (Viper)
│       └── loader.go     # UPDATE - add server configuration loading
├── domain/
│   └── server/           # NEW - domain logic for server lifecycle
│       ├── lifecycle.go  # Startup, shutdown coordination
│       └── manager.go    # Dual-server management, atomic startup
└── cmd/
    └── agentic-identity-broker/
        └── main.go       # UPDATE - initialize and start dual servers

tests/
├── integration/
│   ├── server_test.go    # End-to-end server startup/shutdown tests
│   └── health_test.go    # Health endpoint integration tests
└── unit/
    ├── config_test.go    # Configuration validation tests
    └── lifecycle_test.go # Server lifecycle logic tests

docs/
├── configuration.md      # UPDATE - add server configuration section
└── api.md                # NEW - health check API documentation

adrs/
├── 20251215-120100-chi-framework.md           # NEW - chi framework selection rationale
└── 20251215-120200-dual-server-isolation.md  # NEW - atomic startup and isolation strategy

examples/config/
├── config.development.yaml   # UPDATE - add server config examples
├── config.staging.yaml       # UPDATE - add server config examples
└── config.production.yaml    # UPDATE - add server config examples
```

**Structure Decision**: Single Go project using hexagonal architecture. HTTP servers are inbound adapters (ports), configuration loader is outbound adapter. Domain logic in `internal/domain/server/` coordinates lifecycle. Tests separated by unit/integration scope.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations. All constitution principles are satisfied.

