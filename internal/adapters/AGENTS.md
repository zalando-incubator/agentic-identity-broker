# Adapters Layer (`internal/adapters/`)

> **Use retrieval-led reasoning. Read the port interfaces in `internal/ports/*.go` before you implement adapters.**

Driven adapters import `ports/` and `domain/`. HTTP routing receives pre-wired handlers from `app/`. It can use routing middleware and configuration. Do not import one adapter from another adapter.

## Adapter Map

### Encryption (`encryption/`)

`aws/` implements `ports.EncryptionPort` for AWS KMS and base64 AES-256 key modes.

| Package | Purpose |
|---|---|
| `aws/` | `EncryptionPort` implementation for AWS KMS or base64 AES-256 keys |
| `branchkey/` | `BranchKeyIdProvider` implementation |
| `noop/` | No-op `BranchKeyManager` when no branch-key store is configured |

Encryption rules: See `.claude/skills/aws-crypto-go/SKILL.md`.

### HTTP (`http/`)

[ADR 004: Dual-Server Isolation](../../adrs/004-dual-server-isolation.md) defines the
end-user `:8000` server and the admin `:14000` server.

| Package | Purpose |
|---|---|
| `routing/{admin,enduser}.go` | Route registration only. Receives pre-wired handlers |
| `handlers/admin/` | Agent, service, protected-resource, permission-set, credential, and signing-key handlers |
| `handlers/approval/` | Tool-approval create, lifecycle, sync, and list handlers |
| `handlers/consent/` | User info, agent delegation, detail, and grant handlers |
| `enduser/` | `OAuth2AuthorizeHandler`, `OAuth2TokenHandler`, `OAuth2MetadataHandler` |
| `handlers/enduser/` | Broker JWKS handler |
| `oauth2_sessions/` | Session initiation, callback, and termination |
| `middleware/` | Auth (`RequirePrincipal`), CORS, CSRF, audit |
| `upstream/` | Upstream OAuth2 proxy |

**Handler rules**: Parse the request. Call a domain service. Format the response. Map errors to HTTP status codes. Then update `app/handlers.go`, `builder.go`, and `routing/`.

### JWKS (`jwks/`)

`httprc` resources refresh upstream data in the background. The adapter implements `ports.JWKSPort`.
`published_adapter.go` provides the broker aggregated JWKS to in-process consumers.

Trust anchors can use a local key set instead of a remote `jwks_uri` ([ADR 040](../../adrs/040-trust-anchor-local-key-sets.md)). Every adapter below implements `ports.JWKSPort`, and the domain validates each local document with `trustanchor.ParseLocalKeySet`.

| Type | Purpose |
|---|---|
| `StaticAdapter` | Serves one validated inline (`jwks`) set for the process lifetime. No network, no refresh. `HealthState()` is always `healthy`. |
| `FileAdapter` | Reads a `jwks_file` at construction (a failure fails startup) and re-reads it in the background every `jwks_min_refresh`. A valid re-read replaces the set. A failed re-read keeps the last valid set and reports `degraded`. After `jwks_max_refresh` since the last success, `GetKeySet` fails closed. `Shutdown` stops the loop. |
| `TrustDirectory` | Wraps `os.Root` over `security.trust_material_dir`. Opens a file by its path relative to the directory, so `..` and symbolic links that leave it are refused. Absolute symbolic links are refused even when the target is inside. The caller checks the regular file on the opened descriptor. |

The builder creates these adapters and injects them. `adapters/jwtauth/` receives a `ports.JWKSPort` through its config and never imports `adapters/jwks/`. Errors and logs never include document content or key material.

### Storage (`storage/`)

| Package | Purpose |
|---|---|
| `factory.go` | Backend selection (`memory`/`postgres`) → returns `Adapter` composite with all repos |
| `memory/` | Maps + `sync.RWMutex`. All repository interfaces. Dev/testing. |
| `postgres/` | `sqlx` + `pgx v5`. All repository interfaces. Production. |

Both adapters implement the required storage contracts. The migrations are in `migrations/`. They use go-migrate names.

List SQL result columns explicitly. Do not use `SELECT *` or `RETURNING *` in storage adapters. Additive schema changes must not break record scans.

## Rules

- **Cross-adapter ban**: Do not import `storage/postgres/` from `encryption/aws/`, or import between other adapters.
- **Adapter boundary**: Driven adapters import ports and domain. HTTP routing can import app handlers, routing middleware, and configuration. Do not import adapters from domain.
- **Testing**: Use same-package `_test.go`, `testify/mock`, and `httptest`. Use `_integration_test.go` for real infrastructure. Test storage backends against the same contracts.
