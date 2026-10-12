# Adapters — `internal/adapters/`

All repository paths in this file are relative to the repository root.
Read `internal/AGENTS.md` and the relevant interfaces in `internal/ports/` before changes.
Use `ARCHITECTURE.md` and `.specify/memory/constitution.md` for architecture and security requirements.

## Boundaries

- Keep domain behavior behind ports. Do not expose SQL models or AWS SDK types through ports.
- Do not couple independent adapters, such as encryption and storage.
- Keep backend selection in `internal/adapters/storage/factory.go`.
- Keep shared encryption helpers in `internal/adapters/encryption/branchkey/`.
- Keep HTTP route registration separate from service construction.
- Pass pre-wired handlers from `internal/app/handlers.go` through the routing functions.
- Construct their services in `internal/app/builder.go`.

Factories and helpers within one adapter subsystem already import their implementation packages.
HTTP routing also imports app handler structs, HTTP middleware, and read-only configuration.
These composition paths do not permit dependencies between unrelated adapters.

## Encryption

| Path | Purpose |
|---|---|
| `internal/adapters/encryption/aws/` | AWS Encryption SDK implementation for KMS and base64 AES-256 key modes |
| `internal/adapters/encryption/branchkey/` | Typed branch-key identifiers |
| `internal/adapters/encryption/noop/` | No-op branch-key manager, not plaintext encryption |

Read `.agents/skills/aws-crypto-go/SKILL.md` before encryption changes.
Apply the repository-specific context rules instead of the skill's generic context example.

- Use `RequireEncryptRequireDecrypt`.
- Keep exactly one non-secret context subject: `service_id` for service assets or `kid` for signing-key material.
- Reject missing or multiple subjects. Do not add secrets, purpose fields, or `key_domain` to the context.
- Keep encryption in domain services. Storage adapters store opaque ciphertext.
- Do not add a plaintext fallback.

Sources: `adrs/008-encryption-context-optimization.md` (including its amendment),
`adrs/009-envelope-encryption-design.md`, `adrs/012-encryption-layer-separation.md`,
and `adrs/037-cimd-client-authentication-key-domain.md`.
Use `docs/ENCRYPTION_INTEGRATION_GUIDE.md` for integration details.

## HTTP

`adrs/004-dual-server-isolation.md` defines separate end-user (`:8000`) and admin (`:14000`) servers with atomic startup.

| Path | Purpose |
|---|---|
| `internal/adapters/http/routing/admin.go` | Admin route registration |
| `internal/adapters/http/routing/enduser.go` | End-user, approval, OAuth2, and SPA routes |
| `internal/adapters/http/handlers/admin/` | Agents, services, protected resources, permission sets, credentials, and key management |
| `internal/adapters/http/handlers/approval/` | Approval creation, lifecycle, synchronization, and lists |
| `internal/adapters/http/handlers/consent/` | User info, agent delegation, details, and grants |
| `internal/adapters/http/enduser/` | OAuth2 authorization, token, and metadata handlers |
| `internal/adapters/http/handlers/enduser/` | Broker JWKS handler |
| `internal/adapters/http/oauth2_sessions/` | Session initiation, callback, and termination |
| `internal/adapters/http/middleware/` | Authentication, CORS, CSRF, and audit controls |
| `internal/adapters/http/upstream/` | Upstream OAuth2 proxy |

Parse requests, call domain services, and map results to HTTP responses.
Read `api/admin/openapi.yaml` or `api/enduser/openapi.yaml` before endpoint changes.
Preserve the endpoint-specific approval authentication in `adrs/018-approval-endpoint-auth-boundaries.md`:

| Endpoint | Authentication |
|---|---|
| `POST /api/approvals` | Subject token plus client assertion |
| `GET /api/approvals` | Client assertion |
| `POST /api/approvals/{id}/consume` | Subject token |
| Browser detail, lists, approve, deny, and revoke | Acting-user principal |

Keep approval synchronization as full snapshots, not deltas.
Read `adrs/014-long-poll-listen-notify.md` for ETags and PostgreSQL wake-up behavior.
For approval-sync changes, read `specs/026-extproc-approval-sync/spec.md`, `specs/026-extproc-approval-sync/plan.md`, and `specs/026-extproc-approval-sync/tasks.md`.
Use `specs/026-extproc-approval-sync/contracts/broker-approval-client.md` and `docs/api/approval-endpoints.md` for the machine-client contract.

## Other Adapters

| Path | Purpose and source |
|---|---|
| `internal/adapters/jwks/` | Upstream refresh through `httprc`. `internal/adapters/jwks/published_adapter.go` serves the broker JWKS to in-process consumers |
| `internal/adapters/jwtauth/` | Configurable end-user JWT authenticator |
| `internal/adapters/cimd/` | CIMD HTTP fetcher. Read `adrs/015-cimd-fetcher-architecture.md` for SSRF and redirect controls |
| `internal/adapters/telemetry/` | OpenTelemetry providers and context-aware logging |
| `internal/adapters/storage/factory.go` | Selects memory or PostgreSQL and returns the repository composite |
| `internal/adapters/storage/memory/` | In-memory repositories for development and tests |
| `internal/adapters/storage/postgres/` | PostgreSQL repositories with `sqlx` and `pgx` |

Keep token-signing and CIMD client-authentication keys on separate public JWKS surfaces.
Read `adrs/037-cimd-client-authentication-key-domain.md` before key publication or persistence changes.
Do not generalize the unsigned-subject exception to end-user authentication.
Its complete scope is in `adrs/031-unverified-subject-unsigned-jwt.md` and Constitution Principle I.

For storage changes, read `adrs/004-storage-layer-architecture.md`, `internal/ports/storage.go`, and `docs/STORAGE_EXTENSION_GUIDE.md`.
Implement the same required contracts in both storage backends.
Put schema changes in `migrations/` with go-migrate names.
Use `docs/STORAGE_TROUBLESHOOTING.md` for operational faults.

## Tests

Use nearby `_test.go` files for the local package convention.
Both same-package and external-package tests exist.
Use `testify/mock` where the package uses it, and `httptest` for HTTP behavior.
Keep infrastructure-dependent coverage in `_integration_test.go` files.
Cover shared storage contracts in both backends and PostgreSQL behavior with real migrations.
