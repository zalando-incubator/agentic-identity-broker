# Ports Layer (`internal/ports/`)

> **Use retrieval-led reasoning. Read the port `.go` files directly before you implement. They are the authoritative contract.**

Ports contain **interfaces only** and minimal DTOs. Do not put business logic or implementations in ports. Domain services depend on these interfaces. Adapters implement them.

## Port Catalog

| File | Interface(s) | Purpose |
|---|---|---|
| `cel.go` | `CELCompilerPort`, `CELProgram` | CEL expression compilation and evaluation. DTOs: `CELAuthorizationContext`, `CELRequestContext`, `CELClaimExtractionContext` |
| `cimd.go` | See `cimd.go` for authoritative interface/error/DTO names | CIMD client-resolution and fetch ports. Read the file for the exact contract surface. |
| `config.go` | `ConfigPort` | Configuration loading and access. DTOs: `Config` and all nested config types (largest port file) |
| `encryption.go` | Encryption and branch-key contracts | `EncryptionPort`, `BranchKeyRepository`, `BranchKeyIdProvider` |
| `jwks.go` | `JWKSPort`, `JWKSHealthPort` | JWKS retrieval with embedded health reporting and the health-only facet |
| `jwks_publisher.go` | `JWKSPublisherPort`, `JWKSPublisherHealthPort` | Aggregated JWKS publishing and health |
| `oauth2.go` | `OAuth2Service` | OAuth2 authorization and RFC 8414 metadata. DTOs: `AuthorizationRequest`, `AuthorizationDecision`, `MetadataResponse` |
| `oauth2_mode_config.go` | `OAuth2ModeConfig` | Resolved proxy, local, and hybrid OAuth2 configuration |
| `oauth2server.go` | OAuth2 server key and credential contracts | Read the file for exact interfaces |
| `server.go` | `HealthState` (enum) | Server lifecycle: `Starting`, `Healthy`, `ShuttingDown`, `Unhealthy` |
| `storage.go` | Repository and transaction interfaces | Storage contracts. Sentinel: `ErrNotFound` |
| `thirdparty_provider.go` | Provider configuration storage | `ThirdpartyOAuth2ProviderRepository` |

## Rules

- **ISP**: Expose only operations that consumers need. Justify new operations.
- **No business logic**: Do not add validation, orchestration, conditionals, or error handling beyond sentinels.
- **New ports**: New architectural boundaries require an ADR. New methods require consumer justification.
- **Minimal DTOs**: Prefer the domain type from its owning package. Do not create port DTOs that do not add a boundary.
- **Encryption context**: `Encrypt` and `Decrypt` take exactly one approved subject key: `service_id`, `kid`, or `refresh_session_id`. Never combine keys or include secrets.
- **Error conventions**: Storage returns `storage.StorageError`. Encryption returns `encryption.EncryptionError`. Use `ports.ErrNotFound` for simple identity checks.
