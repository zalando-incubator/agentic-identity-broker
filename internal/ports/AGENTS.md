# Ports Layer (`internal/ports/`)

> **Use retrieval-led reasoning. Read the port `.go` files directly before you implement. They are the authoritative contract.**

Ports contain **interfaces only** and minimal DTOs. Do not put business logic or implementations in ports. Domain services depend on these interfaces. Adapters implement them.

## Port Catalog

| File | Interface(s) | Purpose |
|---|---|---|
| `cel.go` | `CELCompilerPort`, `CELProgram` | CEL expression compilation and evaluation. DTOs: `CELAuthorizationContext`, `CELRequestContext`, `CELClaimExtractionContext` |
| `cimd.go` | `ClientResolver`, `CIMDFetcher` | CIMD client resolution and fetch contracts. DTOs include `CIMDMetadataDTO`, `CIMDFetchResult`, and `ClientResolution`. Errors include `ClientIDError`, `SSRFBlockedError`, and transient `CIMDUnavailableError`. |
| `config.go` | `ConfigPort` | Configuration loading and access. DTOs: `Config` and all nested config types (largest port file) |
| `encryption.go` | Encryption and branch-key contracts | `EncryptionPort`, `BranchKeyRepository`, `BranchKeyIdProvider` |
| `jwks.go` | `JWKSPort`, `JWKSHealthPort` | JWKS retrieval with embedded health reporting and the health-only facet |
| `jwks_publisher.go` | `JWKSPublisherPort`, `JWKSPublisherHealthPort` | Aggregated JWKS publishing and health |
| `oauth2.go` | `OAuth2Service` | OAuth2 authorization and RFC 8414 metadata. DTOs: `AuthorizationRequest`, `AuthorizationDecision`, `MetadataResponse`, `ClientCredentials` |
| `oauth2_mode_config.go` | `OAuth2ModeConfig` | Resolved proxy, local, and hybrid OAuth2 configuration |
| `oauth2server.go` | OAuth2 server key and credential contracts | Read the file for exact interfaces |
| `server.go` | `HealthState` (enum) | Server lifecycle: `Starting`, `Healthy`, `ShuttingDown`, `Unhealthy` |
| `storage.go` | Repository and transaction interfaces, including `ClientAssertionReplayRepository` | Storage contracts. The replay repository atomically consumes `(client_id, jti)` and removes expired records. Sentinel: `ErrNotFound` |
| `thirdparty_provider.go` | Provider configuration storage | `ThirdpartyOAuth2ProviderRepository` |

## Rules

- **ISP**: Expose only operations that consumers need. Justify new operations.
- **No business logic**: Do not add validation, orchestration, conditionals, or error handling beyond sentinels.
- **New ports**: New architectural boundaries require an ADR. New methods require consumer justification.
- **Minimal DTOs**: Prefer the domain type from its owning package. Do not create port DTOs that do not add a boundary.
- **Encryption context**: `Encrypt` and `Decrypt` take one subject key. Use `service_id` for services or `kid` for signing keys. Do not include secrets or both keys.
- **Error conventions**: Storage returns `storage.StorageError`. Encryption returns `encryption.EncryptionError`. Use `ports.ErrNotFound` for simple identity checks.
