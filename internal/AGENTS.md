# Backend — Hexagonal Architecture (`internal/`)

> Read the source and nearest child `AGENTS.md` before you select types, interfaces, or patterns.

Paths are relative to the repository root unless a section states otherwise.

## Child AGENTS.md Index

`internal/domain/AGENTS.md` | Domain layer rules, data models, error types, zero-infra imports
`internal/domain/id/AGENTS.md` | Strongly typed entity IDs, code generation, type catalogue
`internal/ports/AGENTS.md` | Port interface catalogue (read `internal/ports/*.go` directly for contract)
`internal/adapters/AGENTS.md` | Adapter map, cross-adapter ban, storage/encryption/HTTP
`internal/extproc/AGENTS.md` | Standalone ExtProc service and its narrow shared-package exceptions

## Local Source Routes

Use the root `AGENTS.md` for the constitution, ADR index, API contracts, and feature selection.

| Path | Purpose |
|---|---|
| `internal/app/builder.go` | Broker construction order, shared services, and shutdown |
| `internal/app/handlers.go` | Handler groups passed to routing |
| `internal/config/loader.go` | Broker defaults, files, environment, and CLI flags |
| `internal/config/validator.go` | Broker startup validation |
| `docs/configuration.md` | Operator configuration contract |
| `examples/config/README.md` | Feature configuration examples |
| `charts/agentic-identity-broker/` | Broker workload configuration |
| `docs/reference/token-exchange.md` | Credential-free diagnostic attributes and classification |
| `tests/AGENTS.md` | Integration and acceptance test routes |

## Hexagonal Architecture Boundaries

Domain services depend on ports. Adapters implement ports. The app layer supplies their dependencies.

- **Domain** (`internal/domain/`) — Business logic without infrastructure imports. Do not import adapters or app.
- **Ports** (`internal/ports/`) — Interfaces, boundary data types, and configuration contracts.
- **Driven adapters** (`internal/adapters/`) — Port implementations. Do not couple independent adapter subsystems.
- **HTTP routing** (`internal/adapters/http/routing/`) — Receives handlers from app.
  It can use routing middleware and configuration. Do not create services or adapters there.

Read `internal/adapters/AGENTS.md` for existing subsystem factories and shared helpers.

Before you add a cross-layer dependency, read the port contracts and applicable ADR.

## Package Map

Paths in this map are relative to `internal/`.

```
domain/
  agents/          Agent lifecycle and canonical-ID resolution
  approval/        Tool approval lifecycle, rate limiting, and sync
  canonical/       Canonical identifier validation
  cimdclient/      Outbound CIMD metadata, assertion signing, and key ownership
  config/          Configuration domain types and validation
  consent/         Agent delegation and grant management
  encryption/      Encryption errors and branch-key subject types
  id/              Strongly typed entity IDs
  impersonation/   Local-mode delegated token issuance
  jwe/             Encrypted token service
  jwtauth/         JWT authentication and claim extraction
  model/           Shared domain entities and value objects
  jwtclaims/       JWT claim normalization for CEL
  keylifecycle/    Shared key lifecycle with separate trust domains
  oauth2/          OAuth2 authorization, CIMD, and JWKS publishing
  oauth2server/    Broker OAuth2 authorization-server logic
  oauth2session/   OAuth2 session lifecycle and token vault
  permissionset/   Permission-set lifecycle and resolution
  principal/       Principal context extraction
  security/        Request security context and audit identity
  server/          Server lifecycle configuration
  storage/         Domain data models and endpoint discovery
  thirdparty/      Third-party OAuth2 provider management
  tokenexchange/   RFC 8693 token exchange and CEL evaluation
  urivalidation/   Redirect and resource URI validation

ports/             Port contracts; read the `.go` files before editing
  cel.go            CELCompilerPort
  cimd.go           CIMD fetch and client-resolution contracts
  cimd_client.go    Outbound CIMD key, metadata, and assertion contracts
  config.go         ConfigPort and Config
  encryption.go     EncryptionPort and branch-key contracts
  jwks.go           JWKSPort and JWKSHealthPort
  jwks_publisher.go JWKSPublisherPort and publisher health contracts
  oauth2.go         OAuth2Service
  oauth2_mode_config.go  Resolved proxy, local, and hybrid OAuth2 config
  oauth2_token_proxy.go  Upstream token request and response contracts
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
  jwks/            JWKS fetcher and published-JWKS adapter
  jwtauth/         JWT authenticator
  storage/
    factory.go     Backend selection from configuration
    memory/        In-memory storage
    postgres/      PostgreSQL storage with sqlx and pgx v5
  telemetry/       OpenTelemetry provider and slog handler

app/
  builder.go       Builder pattern — all DI wiring
  handlers.go      AdminHandlers + EnduserHandlers struct definitions

config/            Broker configuration adapter
telemetryhttp/     Credential-free outbound HTTP instrumentation
testutil/          Adapter-backed test helpers, not for domain tests
ptr/               Pointer helper
```

## Builder Pattern (DI Wiring)

`NewBuilder().With*().Build()` in `internal/app/builder.go` validates inputs and initializes telemetry before these phases:

1. **Encryption** — Create the required encryption adapter and branch-key manager.
2. **Domain services** — Create services from port contracts.
3. **Handlers** — Wire `AdminHandlers` and `EnduserHandlers` with the services.

**Rules**:

- Give routing pre-wired handler structs. Do not create services in routing.
- Add handlers to `internal/app/handlers.go`. Create them in `internal/app/builder.go`. Register their routes in `internal/adapters/http/routing/`.
- Make driven adapters depend on port interfaces, not concrete infrastructure. Give HTTP routing pre-wired handlers.

### Handler Structs

**AdminHandlers**: `Agents`, `Services`, `ProtectedResources`, `PermissionSets`, `ClientCredentials`, `SigningKeys`, `CIMDClientKeys`.

**EnduserHandlers**: consent, session, OAuth2, JWKS, CIMD metadata, approval, and SPA handlers.

### Routing Signatures

The contracts are in `internal/adapters/http/routing/admin.go` and `internal/adapters/http/routing/enduser.go`.

```go
func SetupAdminRoutes(r chi.Router, h *app.AdminHandlers, cfg AdminRouteConfig)
func SetupEnduserRoutes(r chi.Router, h *app.EnduserHandlers, cfg EnduserRouteConfig)
```

## Testing Conventions

- Use the root TDD requirements. Keep domain tests independent of infrastructure adapters.
- Put `_test.go` files beside the package. Use the existing package convention.
- **Assertions**: Use `testify`. Use `require` for fatal preconditions. Use `assert` for soft checks.
- **Subtests**: Use `t.Run("description", ...)`. Use table-driven tests for validation and parameter sets.
- For adapter mocks, use the existing `testify/mock` helpers and expectation checks.
- For domain mocks, use the local port doubles. These use function fields or `testify/mock`, depending on the package.
- **HTTP**: Use `httptest.NewRecorder()` and `httptest.NewRequest()`. Use chi for URL parameters.

## Key Storage Types

These are domain models, not database records. Paths in these reference tables are relative to `internal/`.
Keep adapter records in adapter packages.

| Type | Location | Key Fields |
|---|---|---|
| `Agent` | `domain/storage/` | ID, ClientID, DisplayName, Description, ServiceRequirements |
| `ThirdpartyOAuth2ProviderEntity` | `domain/model/` | ID, DisplayName, ClientID, Secret, Scopes, ProtectedResources |
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
| Outbound CIMD | Key lifecycle, readiness, metadata, and assertion interfaces | `ports/cimd_client.go` |
| Upstream Token Proxy | `OAuth2TokenProxy` | `ports/oauth2_token_proxy.go` |
