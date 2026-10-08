# Domain Layer (`internal/domain/`)

> **Use retrieval-led reasoning. Read source files before you select types, interfaces, or patterns.**

This is the innermost hexagonal ring. It has zero infrastructure dependencies. Do not import `adapters/`, `app/`, or external I/O libraries.
Domain packages can depend on `ports/` interfaces. They can use other domain packages
and declared domain libraries.

## Package Responsibilities

| Package | Role | Key Types |
|---|---|---|
| `agents/` | Agent lifecycle and canonical-ID resolution | `Service` |
| `approval/` | Tool approval lifecycle, rate limiting, and sync | `Service`, `ApprovalSyncBroadcaster` |
| `canonical/` | Canonical identifier validation | `Validate()` |
| `config/` | Configuration domain types and validation | `LogLevel`, `LogFormat` |
| `consent/` | Consent management business logic | `Service` |
| `encryption/` | Encryption domain errors | `EncryptionError` |
| `id/` | Strongly typed entity IDs | `AgentID`, `ServiceID` |
| `jwe/` | Encrypted token service | `TokenService` |
| `jwtauth/` | JWT authentication and claim extraction | `JWTAuthenticator`, `CELEvaluator` |
| `ledger/` | Closed-schema recording, trusted context, investigation, and lifecycle invariants | `Service`, `Registry`, `NormalizeRetention` |
| `model/` | Shared domain entities and value objects | `Secret`, `OAuthScope` |
| `oauth2/` | OAuth2 authorization, CIMD, and JWKS publishing | `OAuth2AuthorizationService` |
| `oauth2server/` | Broker OAuth2 authorization-server logic | `Provider`, `SigningKeyService` |
| `oauth2session/` | OAuth2 session lifecycle and token vault | `OAuth2SessionService` |
| `permissionset/` | Permission-set lifecycle and resolution | `Service` |
| `principal/` | Authenticated identity context | `WithPrincipal()`, `FromContext()` |
| `server/` | Server lifecycle configuration | `Config` |
| `storage/` | Domain data models and endpoint discovery | `Agent`, `UserGrant`, `UserSession` |
| `thirdparty/` | Third-party OAuth2 provider management | `ThirdpartyOAuth2ProviderService` |
| `tokenexchange/` | RFC 8693 token exchange and CEL evaluation | `TokenExchangeService` |
| `urivalidation/` | Redirect and resource URI validation | `MatchesRedirectURI()`, `NormalizeResourceURI()` |

Ledger recipes determine credential-free facts from domain-established outcomes.

Before you change a producer, read accepted ADR 039 and `specs/048-business-event-ledger/contracts/producers.md`.
Record mutations and facts in one owning transaction.
Keep the initiating caller, affected subject, and represented principal distinct.

## Critical Rules

### Zero Infrastructure Imports

Do not import `database/sql`, `pgx`, `sqlx`, `chi`, AWS SDKs, `internal/adapters/`, or `internal/app/`. Only `oauth2session`, `storage`, and `oauth2server` import `net/http`.
Use the Go standard library, `internal/ports/`, other `internal/domain/` packages, and declared domain libraries such as `jwx`, `oauth2`, and `cel-go`.

### Domain Services Pattern

Use constructor injection with port interfaces. Create services in `app/builder.go`. Do not self-instantiate services. Read existing services for patterns.

### Cross-Domain Dependencies

Keep cross-domain imports narrowly scoped. Read the importing package and its direct dependencies before you add an import.

## Domain Data Models (`storage/`)

**Domain entities and value objects** — These are not database models. Put adapter-specific details in adapters.

### Entities

| Type | Key Invariants |
|---|---|
| `Agent` | `ClientID` can be empty. `DisplayName` is required. URLs require HTTP(S). |
| `UserGrant` | One per `(principal, agent)` pair. It records granted permission sets and optional validity. |
| `UserSession` | One per `(principal, service_id)`. Tokens are encrypted. The context uses one approved subject key. |
| `User` | ID + email, timestamps |

### Value Objects

- `OAuthScope` — permission scope in `model/`
- `RequirementType` and `ServiceRequirement` — service requirements in `storage/`
- `EncryptionContext` — service-scoped AAD in `storage/`
- `BranchKeySubject` — service or signing-key encryption namespace in `encryption/`
- `ConnectionParameters`, `StorageBackend`, `DiscoveryConfig`, and `OAuth2Endpoints` in `storage/`

### Validation

Entities use `Validate() error`. Some use create or update variants. Validation does not do I/O.

### Error Types

- `storage.StorageError` — connection, timeout, validation, not-found, conflict, or unknown error kind
- `encryption.EncryptionError` — encryption, decryption, context, integrity, or KEK error. Never include key material.
- `tokenexchange.TokenExchangeError` — RFC 8693 error. Never include token values in messages.
- Error types in `consent/`, `principal/`, and `config/`

### Storage Discovery

`storage/discovery.go` — `DiscoverOAuth2Endpoints()` gets RFC 8414 metadata. HTTPS is required in prod. HTTP is permitted for localhost in dev.

## Testing

- **TDD**: Write tests first. Make them fail before you implement the behavior.
- **Files**: Put `_test.go` beside the package. Use the same package (white-box).
- **Mocking**: Use hand-rolled mocks (structs with function fields). Do not use `testify/mock` in domain.
- **Table-driven tests**: Use them for validation. Put security tests in `_security_test.go` files.
