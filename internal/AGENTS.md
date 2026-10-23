# Backend — Hexagonal Architecture (`internal/`)

> **Use retrieval-led reasoning.** Read source files and the nearest child `AGENTS.md` before you select types, interfaces, or patterns.

## Child AGENTS.md Index

`internal/domain/AGENTS.md` | Domain layer rules, data models, error types, zero-infra imports
`internal/domain/id/AGENTS.md` | Strongly typed entity IDs, code generation, type catalogue
`internal/ports/AGENTS.md` | Port interface catalogue (read `internal/ports/*.go` directly for contract)
`internal/adapters/AGENTS.md` | Adapter map, cross-adapter ban, storage/encryption/HTTP
`internal/extproc/AGENTS.md` | Standalone ExtProc service (separate binary, no broker imports)

## Hexagonal Architecture Boundaries

Dependency arrows flow **inward only**: `app/` → `adapters/` → `ports/` ← `domain/`

- **Domain** (`domain/`) — Pure business logic. ZERO infrastructure imports. Never imports `adapters/` or `app/`.
- **Ports** (`ports/`) — Interfaces + minimal DTOs only. The hexagonal boundary definitions.
- **Driven adapters** (`adapters/`) — Infrastructure port implementations. Import `ports/` and `domain/`, never other adapters.
- **HTTP routing** (`adapters/http/routing/`) — Receives handlers from `app/`.
  It can use routing middleware and configuration. Do not create services or adapters there.

Inward-dependency violations break the architecture. If the dependency is unclear, read `ports/*.go` for available interfaces before you add cross-layer dependencies.

## Package Map

```
domain/
  agents/          Agent lifecycle and canonical-ID resolution
  approval/        Tool approval lifecycle, rate limiting, and sync
  canonical/       Canonical identifier validation
  config/          Configuration domain types and validation
  consent/         Agent delegation and grant management
  encryption/      Encryption domain errors (not implementations)
  id/              Strongly typed entity IDs
  jwe/             Encrypted token service
  jwtauth/         JWT authentication and claim extraction
  model/           Shared domain entities and value objects
  oauth2/          OAuth2 authorization, CIMD, and JWKS publishing
  oauth2server/    Broker OAuth2 authorization-server logic
  oauth2session/   OAuth2 session lifecycle and token vault
  permissionset/   Permission-set lifecycle and resolution
  principal/       Principal context extraction
  server/          Server lifecycle configuration
  storage/         Domain data models and endpoint discovery
  thirdparty/      Third-party OAuth2 provider management
  tokenexchange/   RFC 8693 token exchange and CEL evaluation
  trustanchor/     Trust-anchor key-source resolution and local key-set validation
  urivalidation/   Redirect and resource URI validation

ports/             Port contracts; read the `.go` files before editing
  cel.go            CELCompilerPort
  cimd.go           CIMD fetch and client-resolution contracts
  config.go         ConfigPort and Config
  encryption.go     EncryptionPort and branch-key contracts
  jwks.go           JWKSPort and JWKSHealthPort
  jwks_publisher.go JWKSPublisherPort and publisher health contracts
  oauth2.go         OAuth2Service
  oauth2_mode_config.go  Resolved proxy, local, and hybrid OAuth2 config
  oauth2server.go   Signing-key and credential contracts
  server.go         HealthState
  storage.go        Repository and transaction contracts
  thirdparty_provider.go  ThirdpartyOAuth2ProviderRepository

adapters/
  cimd/            CIMD HTTP fetcher
  encryption/
    aws/           AWS KMS hierarchical keyring or base64 AES-256 mode
    branchkey/     BranchKeyIdProvider implementation
    noop/          Branch-key manager for the memory encryption backend
  http/
    enduser/       OAuth2 authorize, token, and metadata handlers
    handlers/      SPA plus admin, approval, consent, and JWKS handlers
    middleware/    Authentication, CORS, CSRF, and audit middleware
    oauth2_sessions/  OAuth2 session management handlers
    routing/       Route registration only
    upstream/      Upstream OAuth2 proxy integration
  jwks/            JWKS fetcher, published-JWKS, static (inline), and file key-set adapters
  jwtauth/         JWT authenticator
  storage/
    factory.go     Backend selection from configuration
    memory/        In-memory storage
    postgres/      PostgreSQL storage with sqlx and pgx v5
  telemetry/       OpenTelemetry provider and slog handler

app/
  builder.go       Builder pattern — all DI wiring
  handlers.go      AdminHandlers + EnduserHandlers struct definitions
```

## Builder Pattern (DI Wiring)

`NewBuilder().With*().Build()` in `app/builder.go` creates dependencies in three phases:

1. **Encryption** — Create the required encryption adapter and branch-key manager.
2. **Domain services** — Create services from port contracts.
3. **Handlers** — Wire `AdminHandlers` and `EnduserHandlers` with the services.

**Rules**:

- Give routing pre-wired handler structs. Do not create services in routing.
- Add new handlers to `app/handlers.go`. Create them in `builder.go`. Then register them in `routing/`.
- Make driven adapters depend on port interfaces, not concrete infrastructure. Give HTTP routing pre-wired handlers.

### Handler Structs

**AdminHandlers**: `Agents`, `Services`, `ProtectedResources`, `PermissionSets`, `ClientCredentials`, `SigningKeys`.

**EnduserHandlers**: consent, session, OAuth2, JWKS, approval, and SPA handlers.

### Routing Signatures

```go
func SetupAdminRoutes(r chi.Router, h *app.AdminHandlers, cfg AdminRouteConfig)
func SetupEnduserRoutes(r chi.Router, h *app.EnduserHandlers, cfg EnduserRouteConfig)
```

## Testing Conventions

- **TDD**: Write tests first. Make them compile and fail for the expected behavior before you implement it.
- **Files**: Put `_test.go` beside the package. Most tests use the package name, not a `_test` suffix.
- **Assertions**: Use `testify`. Use `require` for fatal preconditions. Use `assert` for soft checks.
- **Subtests**: Use `t.Run("description", ...)`. Use table-driven tests for validation and parameter sets.
- **Mocking**:
  - **Adapters**: Use `testify/mock` with `.On()` and `.AssertExpectations()`. Keep helpers in `mocks_test.go`.
  - **Domain**: Use hand-written port mocks with configurable function fields.
- **HTTP**: Use `httptest.NewRecorder()` and `httptest.NewRequest()`. Use chi for URL parameters.

## Key Storage Types (domain/storage/)

These are domain data models, not database models. Adapter records stay in adapter packages.

| Type | Location | Key Fields |
|---|---|---|
| `Agent` | `domain/storage/` | ID, ClientID, DisplayName, Description, ServiceRequirements |
| `ThirdpartyOAuth2ProviderEntity` | `domain/model/` | ID, DisplayName, ClientID, Secret (value object), Scopes, ProtectedResources |
| `UserGrant` | `domain/storage/` | ID, principal, agent ID, permission sets, validity |
| `UserSession` | `domain/storage/` | ID, principal, service ID, encrypted tokens, expiry |

## Port Interfaces Quick Reference

| Port | Interface | File |
|---|---|---|
| Encryption | `EncryptionPort` | `ports/encryption.go` |
| Branch Key | `BranchKeyManager` (= `BranchKeyRepository`) | `ports/encryption.go` |
| Config | `ConfigPort` | `ports/config.go` |
| Storage | Repository and transaction interfaces | `ports/storage.go` |
| Provider Storage | `ThirdpartyOAuth2ProviderRepository` | `ports/thirdparty_provider.go` |
| JWKS | `JWKSPort`, `JWKSHealthPort`, `JWKSPublisherPort` | `ports/jwks*.go` |
| OAuth2 | `OAuth2Service`, `OAuth2ModeConfig` | `ports/oauth2*.go` |
| OAuth2 Server | Signing-key and credential contracts | `ports/oauth2server.go` |
| CEL | `CELCompilerPort` | `ports/cel.go` |
