# Quickstart: Protected Resource Subresources

**Feature**: 20260812-120000-protected-resource-subresources
**Purpose**: Runnable validation that the feature satisfies its spec end-to-end. References
[spec.md](./spec.md), [data-model.md](./data-model.md), and
[contracts/protected-resources.openapi.yaml](./contracts/protected-resources.openapi.yaml). No
implementation code here.

Base path: `/api/services/{service-id}/protected-resources` on the **admin** server (`:14000`, canonical
`/api/services` base per `internal/adapters/http/routing/admin.go`). Auth = the existing admin
service-management auth (FR-014).

## Prerequisites

- `just build` succeeds; admin server reachable (dev: in-memory backend; integration/prod: PostgreSQL
  with migration `027` applied).
- A third-party service exists (create via `POST /api/services`). Capture its `id` and current `ETag`
  (returned by `GET /api/services/{id}` and `GET /api/services/{id}/protected-resources`).

## Validation scenarios

Backend E2E coverage lives in `tests/e2e/service_protected_resources_test.go` (Ginkgo/Gomega, dual-server bootstrap per `tests/e2e/AGENTS.md`). Its removal scenario performs an RFC 8693 exchange before and after deletion and expects `invalid_target` after the deletion. `tests/e2e/token_exchange_test.go` contains separate token-exchange coverage; this quickstart does not treat it as subresource-operation coverage.

### US1 — Add a single resource (P1)
1. **Add unclaimed** → `PUT …/protected-resources/https%3A%2F%2Fapi.example.com%2Fv2` (percent-encoded URI segment, no body) ⇒ `201`;
   body set contains normalized URI; other service fields unchanged (re-`GET /api/services/{id}` and diff).
2. **Idempotent re-add** (same or trailing-slash variant) ⇒ `200`, set unchanged, no duplicate.
3. **Cross-service claim** (URI owned by another service) ⇒ `409`, no change.
4. **Malformed/relative/empty/whitespace URI** ⇒ `400`, no change.
5. **Add to non-existent service** ⇒ `404`.

### US2 — Remove a single resource (P1)
1. **Remove owned** → `DELETE …/protected-resources/<percent-encoded URI>` ⇒ `200` with the removed URI and resulting set; URI gone, other fields intact.
2. **Remove not-owned URI** ⇒ `404`, no change.
3. **Token-exchange reflection**: the feature E2E deletes the resource after a successful RFC 8693 exchange, then asserts a later exchange for that URI returns `invalid_target`. Already-issued downstream tokens are unaffected by this lookup change.
4. **Remove the last resource** ⇒ `200`; set becomes empty (valid).

### US3 — Rename a resource (P2)
1. **Rename to unclaimed** → `PATCH …/protected-resources/<percent-encoded A> {"to":"…B"}` ⇒ `200`; A gone,
   normalized B present, other fields unchanged.
2. **Rename to URI owned elsewhere / already on same service** ⇒ `409`, A unchanged.
3. **Rename a non-existent resource** ⇒ `404`.
4. **Rename to same URI** ⇒ `200` no-op.

### US4 — Retrieve resources (P3)
1. `GET …/protected-resources` ⇒ `200` with all N normalized URIs + `ETag`.
2. `GET` on non-existent service ⇒ `404`.

### Concurrency & convergence (spec Edge Cases; FR-007/FR-011/FR-015)
Repository and handler tests cover mutation/version behavior, but there is no dedicated live-PostgreSQL E2E suite for the following race outcomes. Treat them as explicit integration-validation scenarios rather than established coverage:
- **Concurrent claim race**: two services `PUT` the same *new* URI concurrently ⇒ exactly one `201`, the other `409`; the child table holds exactly one row (SC-005).
- **Concurrent different-resource edits**: two `PUT`s of different URIs to the same service concurrently ⇒ both `201`, both persist (SC-003, no lost update).
- **Full-PUT vs single-resource**: `GET` ETag; `PUT` a new resource (ETag advances); a full `PUT /api/services/{id}` that replaces `protected_resources` using the **stale** ETag ⇒ `412` (the add is not erased). Retried with the fresh ETag ⇒ `200`, converged set.
- **Full-PUT omitting `protected_resources`**: a secret-rotation `PUT` without the field leaves the set untouched (no `If-Match` needed); FR-012 (secret/fields isolation) holds.
- **Replace without If-Match** ⇒ `428`.

## Adapter and migration checks

- **Adapter coverage**: there is no shared repository-contract suite. The in-memory and PostgreSQL adapters have separate tests in `internal/adapters/storage/memory/thirdparty_provider_test.go` and `internal/adapters/storage/postgres/thirdparty_provider_test.go`; their behavior should not be described as a single parity suite.
- **Migration `027`**: `tests/integration/infra/service_protected_resources_migration_test.go` defines PostgreSQL integration cases for clean apply, rejection of non-canonical values and collisions, and apply→rollback→apply resource preservation. Run them with `just test-integration-infra` when PostgreSQL integration infrastructure is available.

## Commands

| Step | Command |
|---|---|
| Static checks | `just check` |
| Unit/package tests (domain, adapters, handlers) | `just test` |
| Infra integration (including migration `027` cases) | `just test-integration-infra` |
| Backend E2E (protected-resource operations, including removal/token-exchange reflection) | `ginkgo -v ./tests/e2e` |
| Full gate | `just verify` |

## Done criteria
All scenarios above pass; `just verify` green; OpenAPI delta merged into `api/admin/openapi.yaml` and
confirmed with the stakeholder (Constitution IV/X); ADR `20260813-120000` and the ARCHITECTURE.md glossary update
landed in the same PR.
