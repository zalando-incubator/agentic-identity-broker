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

### Storage (`storage/`)

| Package | Purpose |
|---|---|
| `factory.go` | Backend selection (`memory`/`postgres`) → returns `Adapter` composite with all repos |
| `memory/` | Maps + `sync.RWMutex`. All repository interfaces. Dev/testing. |
| `postgres/` | `sqlx` + `pgx v5`. All repository interfaces. Production. |

Both adapters implement the required storage contracts. The migrations are in `migrations/`. They use go-migrate names.

Ledger storage uses `business_event*.go` in both backends. Memory uses shared lifecycle/visibility gates and journaled transactions. PostgreSQL uses ambient sqlx transactions and migration-owned partition functions from migration 035. Dispatch holds deletion barriers through synchronous export and acknowledgement.

`telemetry/business_event*.go` owns the non-global synchronous ledger provider. It reuses the configured destination and leaves the ordinary slog batch provider unchanged. Read `docs/operations/business-event-ledger.md` for roles, scheduling, erasure, and policy rollout.

## Rules

- **Cross-adapter ban**: Do not import `storage/postgres/` from `encryption/aws/`, or import between other adapters.
- **Adapter boundary**: Driven adapters import ports and domain. HTTP routing can import app handlers, routing middleware, and configuration. Do not import adapters from domain.
- **Testing**: Use same-package `_test.go`, `testify/mock`, and `httptest`. Use `_integration_test.go` for real infrastructure. Test storage backends against the same contracts.
