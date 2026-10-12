# Domain Layer (`internal/domain/`)

> **Use retrieval-led reasoning. Read source files before you select types, interfaces, or patterns.**

Paths in this file are relative to the repository root.

Domain logic depends on port interfaces, not concrete infrastructure.
Read `internal/AGENTS.md` for shared backend rules.

## Read Before Changes

| Area | Sources |
|---|---|
| Boundaries and domain terms | `ARCHITECTURE.md`, `.specify/memory/constitution.md` (Principles I, V, VI, VIII) |
| Entity IDs | `internal/domain/id/AGENTS.md`, `adrs/013-strongly-typed-entity-ids.md` |
| Encryption ownership and subjects | `adrs/012-encryption-layer-separation.md`, `adrs/008-encryption-context-optimization.md`, `internal/domain/encryption/subject.go` |
| OAuth2 library containment | `adrs/014-oauth2-server-mode.md` |
| CIMD key separation | `adrs/037-cimd-client-authentication-key-domain.md` |
| API-visible behavior | `api/enduser/openapi.yaml`, `api/admin/openapi.yaml`, `internal/ports/AGENTS.md` |
| Approval-sync identity fields | `specs/026-extproc-approval-sync/spec.md`, `specs/026-extproc-approval-sync/plan.md`, `specs/026-extproc-approval-sync/tasks.md` |

For other feature changes, read the relevant specification, plan, and tasks before implementation.

## Package Responsibilities

| Package | Role | Key Types |
|---|---|---|
| `internal/domain/agents/` | Agent lifecycle and canonical-ID resolution | `Service` |
| `internal/domain/approval/` | Tool approval lifecycle, rate limiting, and sync | `Service`, `ApprovalSyncBroadcaster` |
| `internal/domain/canonical/` | Canonical identifier validation | `Validate()` |
| `internal/domain/cimdclient/` | CIMD metadata, client assertions, and dedicated keys | `KeyService`, `MetadataService`, `AssertionSigner` |
| `internal/domain/config/` | Configuration domain types and validation | `LogLevel`, `LogFormat` |
| `internal/domain/consent/` | Consent management business logic | `Service` |
| `internal/domain/encryption/` | Encryption errors and branch-key subjects | `EncryptionError`, `BranchKeySubject` |
| `internal/domain/id/` | Strongly typed entity IDs | `AgentID`, `ServiceID` |
| `internal/domain/impersonation/` | Local-mode user impersonation policy | `Service` |
| `internal/domain/jwe/` | Encrypted token service | `TokenService` |
| `internal/domain/jwtauth/` | JWT authentication and claim extraction | `JWTAuthenticator`, `CELEvaluator` |
| `internal/domain/jwtclaims/` | JWT claim normalization for CEL | `FromToken()` |
| `internal/domain/keylifecycle/` | Shared key lifecycle mechanics with separate trust domains | `Engine`, `Policy` |
| `internal/domain/model/` | Shared domain entities and value objects | `Secret`, `OAuthScope`, `ThirdpartyOAuth2ProviderEntity` |
| `internal/domain/oauth2/` | OAuth2 authorization, CIMD, and JWKS publishing | `AuthorizationService` |
| `internal/domain/oauth2server/` | Broker OAuth2 authorization-server logic | `Provider`, `SigningKeyService` |
| `internal/domain/oauth2session/` | OAuth2 session lifecycle and token vault | `OAuth2SessionService` |
| `internal/domain/permissionset/` | Permission-set lifecycle and resolution | `Service` |
| `internal/domain/principal/` | Authenticated identity context | `WithPrincipal()`, `FromContext()` |
| `internal/domain/security/` | Request security context and identity normalization | `SecurityContext`, `CaptureHolder` |
| `internal/domain/server/` | Server configuration validation | `ValidateServerConfig()` |
| `internal/domain/storage/` | Domain data models and endpoint discovery | `Agent`, `UserGrant`, `UserSession` |
| `internal/domain/thirdparty/` | Third-party OAuth2 provider management | `ThirdpartyOAuth2ProviderService` |
| `internal/domain/tokenexchange/` | RFC 8693 token exchange and CEL evaluation | `TokenExchangeService` |
| `internal/domain/urivalidation/` | Redirect and resource URI validation | `MatchesRedirectURI()`, `NormalizeResourceURI()` |

## Critical Rules

### Infrastructure Boundaries

Do not import database clients, HTTP frameworks, AWS SDKs, `internal/adapters/`, or `internal/app/` into domain implementations.
Use port interfaces for infrastructure dependencies.
Existing `net/http` use is limited to `internal/domain/oauth2session/`, `internal/domain/storage/`, and `internal/domain/oauth2server/`.
Read those call sites before changes. They include upstream calls, metadata discovery, and protocol error classification.
SQL serialization uses `database/sql/driver` in domain values. This does not permit database access.
Use declared domain libraries such as `jwx`, `oauth2`, and `cel-go`.
Keep fosite types inside `internal/domain/oauth2server/`, as required by ADR 014.

### Domain Services Pattern

Use constructor injection with port interfaces. Wire application services in `internal/app/builder.go`.
Read existing constructors before changes. Do not construct concrete adapters inside domain services.

### Cross-Domain Dependencies

Keep cross-domain imports narrowly scoped. Read the importing package and its direct dependencies before you add an import.

## Domain Data Models

**Domain entities and value objects** — These are not database models. Put adapter-specific details in adapters.

### Entities

| Type | Key Invariants |
|---|---|
| `storage.Agent` | `ClientID` can be absent, but not empty when supplied. `DisplayName` and `Description` are required. |
| `storage.UserGrant` | One per `(principal, agent)` pair. It records granted permission sets and optional validity. |
| `storage.UserSession` | One per `(principal, service_id)` pair. Tokens are encrypted with a service-scoped context. |
| `ports.User` | ID, email, and timestamps. Its current declaration is in `internal/ports/storage.go`. |

### Value Objects

- `OAuthScope` — permission scope in `internal/domain/model/`
- `RequirementType` and `ServiceRequirement` — service requirements in `internal/domain/storage/` and `internal/domain/model/`
- `EncryptionContext` — service-scoped AAD in `internal/domain/storage/user_session.go`
- `BranchKeySubject` — service, signing-key, or CIMD key namespace in `internal/domain/encryption/subject.go`
- `ConnectionParameters` and `StorageBackend` — storage configuration in `internal/domain/storage/`
- `DiscoveryConfig` and `OAuth2Endpoints` — discovery configuration in `internal/domain/model/discovery_config.go`

### Validation

Keep entity validation free of I/O. Read each type for its `Validate`, create, and update variants.
Read `internal/domain/storage/agent.go` for URL rules and client-mode classification.
Keep encryption orchestration in domain services. Repositories store opaque encrypted bytes (ADR 012).

### Error Types

- `storage.StorageError` — connection, timeout, validation, not-found, conflict, or unknown error kind
- `encryption.EncryptionError` — encryption, decryption, context, integrity, or KEK error. Never include key material.
- `tokenexchange.TokenExchangeError` — RFC 8693 error. Never include token values in messages.
- Error types in `internal/domain/consent/`, `internal/domain/principal/`, and `internal/domain/config/`

### Storage Discovery

`internal/domain/storage/discovery.go` implements RFC 8414 metadata discovery.
Its scheme helper accepts HTTPS and HTTP for `localhost` or `127.0.0.1`.
The explicit `skipHTTPSValidation` argument also permits other HTTP hosts.
Read `internal/domain/storage/discovery_test.go` before changes to this behavior.

## Testing

- **TDD**: Use the parent TDD rules. Red-phase tests must compile and fail for the missing behavior.
- **Files**: Put `_test.go` beside the package. Use the local same-package or external-package convention.
- **Mocking**: Use local port doubles, with function fields or `testify/mock` according to the package convention.
- **Table-driven tests**: Use them for validation. Put security tests in `_security_test.go` files.
