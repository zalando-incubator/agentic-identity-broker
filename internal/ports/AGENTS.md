# Ports Layer (`internal/ports/`)

Read the relevant port declarations before changes to consumers or implementations.
Paths in this file are relative to the repository root.

Ports define interfaces, boundary DTOs, and errors.
Existing DTO helpers provide construction, validation, configuration defaults, and error classification.
Keep business workflows and infrastructure implementations outside ports.

## Read Before Changes

| Area | Sources |
|---|---|
| Boundaries | `ARCHITECTURE.md`, `.specify/memory/constitution.md` (Principles II, IV, VI), `internal/AGENTS.md` |
| Entity and encryption ownership | `internal/domain/AGENTS.md`, `adrs/012-encryption-layer-separation.md` |
| OAuth2 library containment | `adrs/014-oauth2-server-mode.md` |
| CIMD key separation | `adrs/037-cimd-client-authentication-key-domain.md` |
| API-visible DTOs | `api/enduser/openapi.yaml`, `api/admin/openapi.yaml` |

For feature changes, read the relevant specification, plan, and tasks before changing a contract.

## Port Catalog

| File | Interface(s) | Purpose |
|---|---|---|
| `internal/ports/cel.go` | `CELCompilerPort`, `CELProgram` | CEL compilation and evaluation. Context DTOs separate authorization, requests, and claim extraction. |
| `internal/ports/cimd.go` | `ClientResolver`, `CIMDFetcher` | Client resolution, metadata DTOs, fetch results, and typed boundary errors |
| `internal/ports/cimd_client.go` | `CIMDClientKeyService`, narrow capability interfaces | Dedicated CIMD keys, client assertions, metadata-safe service reads, and public metadata |
| `internal/ports/config.go` | `ConfigPort` | Configuration access, nested DTOs, defaults, validation, and mode resolution |
| `internal/ports/encryption.go` | `EncryptionPort`, `BranchKeyRepository`, `BranchKeyIdProvider` | Encryption and typed branch-key subjects |
| `internal/ports/jwks.go` | `JWKSPort`, `JWKSHealthPort` | JWKS retrieval with embedded health reporting and the health-only interface |
| `internal/ports/jwks_publisher.go` | `JWKSPublisherPort`, `JWKSPublisherHealthPort` | Aggregated JWKS publication and health |
| `internal/ports/oauth2.go` | `OAuth2Service`, token and session interfaces | Authorization decisions, metadata, token issuance, and authorization sessions |
| `internal/ports/oauth2_mode_config.go` | `OAuth2ModeConfig` | Resolved proxy, local, and hybrid configuration |
| `internal/ports/oauth2_token_proxy.go` | `OAuth2TokenProxy`, `TokenProxyResponse`, `TokenOutcomeService` | Transport ownership and token-response policy |
| `internal/ports/oauth2server.go` | `SigningKeyManager`, `SigningKeyBootstrapCoordinator`, credential interfaces | Signing-key lifecycle, coordinated bootstrap, and broker credentials |
| `internal/ports/server.go` | `HealthState`, `ComponentHealth` | Server lifecycle and component health values |
| `internal/ports/storage.go` | Repository and transaction interfaces | Entity persistence, approval sync, session refresh, and not-found classification |
| `internal/ports/thirdparty_provider.go` | Provider and canonical-ID repositories | Provider storage and protected-resource mutation results |

## Rules

- **Interface scope**: Expose only operations that consumers need. Justify each new operation.
- **Business logic**: Keep workflows in domain services. Read existing DTO helpers before adding boundary validation or constructors.
- **New boundaries**: Record architectural decisions in an ADR, as required by Constitution Principle II.
- **DTOs**: Reuse the owning domain type unless the boundary needs a separate representation.
- **Library containment**: Do not expose fosite types in ports (ADR 014).
- **Encryption context**: Supply exactly one subject key: `service_id` for services or `kid` for token-signing and CIMD keys.
- **Context isolation**: Do not include secrets or both subject keys. Read `internal/domain/encryption/subject.go` for typed constructors.
- **Error conventions**: Read each method contract for its errors and absence behavior. Do not replace documented nil results with errors.
- **Not-found errors**: Use `IsNotFoundErr` for both `ports.ErrNotFound` and `storage.StorageError` with `ErrorKindNotFound`.
- **Encryption errors**: Use `encryption.EncryptionError` without key material or plaintext tokens.
- **Response ownership**: Close token-proxy response bodies through `TokenProxyResponse.CloseBody()` after use.
