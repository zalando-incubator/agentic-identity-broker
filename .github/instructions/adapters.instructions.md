---
applyTo: "internal/adapters/**"
---

# Adapters Layer — Port Implementations

Full reference: `internal/adapters/AGENTS.md`

## Core Rule

Adapters implement port interfaces. Each adapter imports only from `ports/` and `domain/`. **Cross-adapter imports are forbidden** (`encryption/aws/` must never import `storage/postgres/`).

## Adapter Map

### Encryption (`encryption/`)
| Package | Purpose |
|---|---|
| `aws/` | Production — AWS KMS Hierarchical Keyring (three-layer envelope: KEK → Branch Key → DEK) |
| `memory/` | Development/testing — in-memory encryption |
| `branchkey/` | Shared branch key ID logic (`service_{id}_branch_key`) |

**Encryption rules** (constitution + ADR 20260116-120200):
- Commitment policy: Always `RequireEncryptRequireDecrypt`
- Encryption context: Only `{"service_id": "<id>"}` (ADR 20260116-120000). Never include secrets.
- Hierarchical keyring: Go SDK has no Caching CMM — use Hierarchical Keyring with DynamoDB
- Explicit wrapping keys: Always specify KMS key ARN. No discovery mode.

### HTTP (`http/`)
Dual-server architecture (ADR 20251215-120200): end-user `:8000` + admin `:14000`.

| Package | Purpose |
|---|---|
| `routing/admin.go` | `SetupAdminRoutes(r, h)` — CRUD for agents and services |
| `routing/enduser.go` | `SetupEnduserRoutes(r, h, cfg)` — consent, sessions, OAuth2, SPA |
| `handlers/admin/` | `AgentsHandler`, `ServicesHandler` |
| `handlers/consent/` | `UserInfoHandler`, `AgentsHandler`, `AgentDetailHandler`, `GrantsHandler` |
| `enduser/` | `OAuth2AuthorizeHandler`, `OAuth2TokenHandler`, `OAuth2MetadataHandler` |
| `oauth2_sessions/` | Session initiate/callback/terminate |
| `middleware/` | Auth, CORS, CSRF, audit middleware |
| `upstream/` | Upstream OAuth2 proxy integration |

**Routing rules**: Routing functions receive pre-wired handler structs — never instantiate services. Route registration only, no business logic.

**Handler rules**: HTTP-to-domain translation only. Parse request → call service → format response. Domain errors → HTTP status codes.

### Storage (`storage/`)
| Package | Purpose |
|---|---|
| `factory.go` | Backend selection (`memory` or `postgres`) from config |
| `memory/` | In-memory (maps + `sync.RWMutex`). All 5 repository interfaces. |
| `postgres/` | PostgreSQL (`sqlx` + `pgx v5`). All 5 repository interfaces. |

Both adapters must implement identical interface contracts — same error semantics, same concurrency behavior.

Migrations in `/migrations/` using go-migrate naming (`NNN_description.{up,down}.sql`).

### JWKS (`jwks/`)
`lestrrat-go/jwx/v3` `jwk.Cache` for background JWKS refresh. Implements `ports.JWKSPort`.

## Adapter ↔ Domain Boundary

- Adapters may import `ports/` and `domain/`
- Adapters must **not** be imported by `domain/`
- Adapter-specific types (SQL models, AWS SDK types) must not leak into port interfaces

## Testing

- `_test.go` co-located, same package (white-box)
- `testify/mock` with `.On()` / `.AssertExpectations()`. Mock helpers in `mocks_test.go`.
- HTTP tests: `httptest.NewRecorder()` + `httptest.NewRequest()` with chi router
- `_integration_test.go` suffix for tests requiring real infrastructure
- Storage: Both memory and postgres tested against same interface contracts
