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
| `trustanchor/` | Trust-anchor key-source resolution and local key-set validation | `ResolveKeySetSource()`, `ParseLocalKeySet()` |
| `urivalidation/` | Redirect and resource URI validation | `MatchesRedirectURI()`, `NormalizeResourceURI()` |

## Critical Rules

### Zero Infrastructure Imports

Do not import `database/sql`, `pgx`, `sqlx`, `chi`, AWS SDKs, `internal/adapters/`, or `internal/app/`. Only `oauth2session`, `storage`, and `oauth2server` import `net/http`.
Use the Go standard library, `internal/ports/`, other `internal/domain/` packages, and declared domain libraries such as `jwx`, `oauth2`, and `cel-go`.

### Domain Services Pattern

Use constructor injection with port interfaces. Create services in `app/builder.go`. Do not self-instantiate services. Read existing services for patterns.

### Cross-Domain Dependencies

Keep cross-domain imports narrowly scoped. Read the importing package and its direct dependencies before you add an import.

## Trust Anchor Key Sets (`trustanchor/`)

A bounded context with no I/O. It depends only on the standard library and `jwx`. See [ADR 040](../../adrs/040-trust-anchor-local-key-sets.md).

- `ResolveKeySetSource()` selects one Key Set Source per trust anchor (`remote`, `inline`, or `file`) and enforces the structural rules: mutually exclusive sources, no refresh bounds with `jwks`, no key source with `verification: none`, and an absolute `jwks_file` inside the Trust Directory.
- `ParseLocalKeySet()` validates an inline or file document: at most 1 MiB, a JWK Set with at least one key, no symmetric or private keys, approved `alg` values only, and unique `kid` values.
- `IsApprovedAlgorithm()` is the single approved asymmetric algorithm set. `impersonation.IsApprovedAlgorithm` delegates to it.
- `DiffKeyIDs()` supports the key-set re-read log.
- Errors use fixed categories. Never include document content, key material, or jwx error text.

Consumers: `internal/config/validator.go` (load-time rules), `internal/app/builder.go` (startup resolution), `internal/adapters/jwks/` (every read of a local key set), and `impersonation/`. `tokenexchange/` and the jwtauth adapter receive the resulting key set through `ports.JWKSPort`, not through this package.

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
