# Phase 0 Research: Protected Resource Subresources

**Feature**: 035-protected-resource-subresources
**Spec**: [spec.md](./spec.md)
**Date**: 2026-08-12

This document records the design decisions that resolve the concurrency, identity, and API-shape
questions the spec intentionally deferred to planning (spec Assumptions §Concurrency contract,
§Identity model, and FR-007/FR-011/FR-015). Each decision lists what was chosen, why, and the
alternatives rejected. Decisions were confirmed interactively with the requester.

---

## Decision 1 — Global uniqueness + atomic single-resource ops via a normalized child table

**Decision**: Introduce a normalized child table `service_protected_resources` with the normalized
resource URI as a globally `UNIQUE` (primary) key and a `service_id` foreign key to
`thirdparty_oauth2_services`. It becomes the **single source of truth** for protected resources. The
legacy `thirdparty_oauth2_services.protected_resources TEXT[]` column and its GIN index are removed
(the requester confirmed the current table design need not be preserved).

```text
service_protected_resources
  resource_uri TEXT PRIMARY KEY            -- normalized (urivalidation.NormalizeResourceURI)
  service_id   UUID NOT NULL REFERENCES thirdparty_oauth2_services(id) ON DELETE CASCADE
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
  INDEX (service_id)                       -- list-by-service + cascade support
```

**Rationale**:
- **FR-007 (global uniqueness under concurrency)** and **SC-005** are satisfied by the database itself:
  two services concurrently claiming the same new URI both attempt `INSERT`; exactly one wins, the
  other receives Postgres `23505` → mapped to `409 Conflict`. No application-level lock protocol,
  advisory lock, or `SERIALIZABLE` retry loop is required. The previous `text[]` design has **no**
  structural uniqueness guarantee — uniqueness was enforced only by a handler-level
  `FindByProtectedResource` check followed by a whole-row write
  (`internal/adapters/http/handlers/admin/services_handler.go:389-433`,
  `internal/adapters/storage/postgres/thirdparty_provider.go:140-200`), a textbook check-then-write
  race.
- **FR-011 (atomic single-resource ops, no lost updates)**: add/remove/rename become single-row
  `INSERT`/`DELETE`/`UPDATE` keyed by `resource_uri`. Concurrent edits of *different* resources of the
  same service touch *different rows* and both commit — no lost updates by construction, with no need
  to read-modify-write a shared array.
- **FR-008 (idempotent same-service add)**: `INSERT ... ON CONFLICT (resource_uri) DO NOTHING`, then
  inspect the existing row's `service_id`: same service → idempotent success; different service → `409`.
  The conflict resolution is atomic under the constraint.
- **FR-009 / FR-010 (not-found)**: remove/rename affecting 0 child rows → `404`; a service-row
  existence/lock check yields `404` for a missing service.
- The resource's **identity is its normalized URI** (Decision 3), so `resource_uri` as the primary key
  is the natural key — no synthetic identifier is introduced.

**Alternatives rejected**:
- **Keep `TEXT[]` + transactional atomic methods (advisory lock on the normalized URI + service-row
  lock).** Localized, no data migration, but uniqueness stays application-protocol-enforced rather than
  DB-guaranteed, requires the legacy full-service PUT to be brought under the same bespoke lock
  discipline, and needs a Postgres-specific advisory-lock scheme with a hand-rolled in-memory
  equivalent. Rejected in favor of a database-guaranteed invariant (the "boring, correct" option).
- **`TEXT[]` + optimistic `updated_at` CAS only.** Compare-and-swap on the parent row does **not**
  serialize *cross-row* claims of the same new URI (two different services, two different rows), so the
  concurrent-claim race (spec Edge Cases) stays open. Rejected.
- **Postgres exclusion constraint on array overlap (`EXCLUDE USING gist (protected_resources WITH &&)`).**
  No native GiST opclass exists for `text[]` (only `intarray`), so this is not available without a
  custom opclass/extension. Rejected.

---

## Decision 2 — Full-service PUT concurrency: optional `protected_resources` field + required `If-Match` on replacement

**Decision**: On the existing full-service update (`PUT /api/services/{service-id}`):
1. `protected_resources` becomes an **optional** field, decoded as `*[]string`:
   - **omitted / `null`** → the child resource set is left **unchanged** (the PUT touches only the other
     service fields; no `If-Match` required).
   - **present (including `[]`)** → **authoritative replacement** of the child set; `If-Match` is
     **required**.
2. When the body replaces the set, the request MUST carry `If-Match: "<version>"` where the ETag is the
   service's dedicated optimistic-concurrency counter (`version BIGINT`, exposed as the strong `ETag` on
   `GET`). The replacement runs in one transaction: CAS the service row on `version`, `DELETE` the
   service's child rows, `INSERT` the payload set, increment `version`, refresh `updated_at`.
   - `If-Match` absent while replacing → `428 Precondition Required`.
   - `If-Match` present but stale → `412 Precondition Failed` (the set changed under the client; it must
     re-`GET` and retry).
3. Single-resource operations do **not** take `If-Match` (they are atomic single-row deltas, not
   read-modify-write of the whole set). Each single-resource op increments the parent `version` in the
   same transaction so that any concurrent replacing PUT's CAS detects the delta.

**Rationale** — this is the only shape that satisfies **FR-011 + FR-015** without a silent lost-update
hole:
- A routine PUT (e.g. secret rotation, display-name change) that **omits** `protected_resources` can no
  longer erase a concurrently-added resource — it does not touch the child set at all. This also fixes a
  pre-existing hazard: today omitting the array **empties** it
  (`services_handler.go:307` builds the entity from `req.ProtectedResources`, then the row write
  overwrites), forcing clients to resend the full set on every update.
- A PUT that **intends** to replace the set must prove it saw the current state (`If-Match`); if a
  single-resource add/remove landed in between, the ETag moved → `412`, so the authoritative replacement
  cannot silently drop the delta. Both paths converge on the same child-table representation (FR-015).
- `*[]string` is required to distinguish "omitted" (leave unchanged) from "present but empty" (clear all)
  — a plain `[]string` cannot tell `null`/absent from `[]` in Go's JSON decoder.

**Rejected — "optional `If-Match`" (the naive optimistic path)**: if `If-Match` were merely optional on
replacement, a legacy PUT that replaces the set *without* the header could commit after a single-resource
add and erase it → violates FR-011/FR-015. The optional path does **not** satisfy the concurrency
requirements and is explicitly not adopted. (Per the reviewer blocker on this exact hazard.)

**Rejected — pessimistic service-row lock only**: serializing full PUT and single-resource ops on
`SELECT ... FOR UPDATE` prevents interleaving corruption but not the *logical* lost update — a stale
authoritative replacement still overwrites with its stale set (last-writer-wins). Detecting the stale
write requires the optimistic token, which is why `If-Match` (not a bare lock) is the mechanism. (The
requester chose optimistic concurrency.)

**Concurrency token**: a dedicated `version BIGINT NOT NULL DEFAULT 1` column on
`thirdparty_oauth2_services` is the CAS counter, exposed as the strong `ETag`. Every mutation
(single-resource ops and full-set replacement) increments it under the service row lock, so the CAS
`WHERE id = $id AND version = $expected` is a guaranteed-monotonic check with identical semantics in the
in-memory and Postgres adapters. `updated_at` is retained for audit timestamps only, not for concurrency
control — timestamp equality is not a reliable token across DB precision and adapter parity.

---

## Decision 3 — API addressing: URI-as-identity with collection POST and member routes

**Decision**: Manage resources as individually addressable sub-resources keyed by the normalized URI carried
as a single, fully percent-encoded path segment for `PUT`, `PATCH`, and `DELETE`; also permit an additive
body-based `POST` to the collection using `resource_uri`. No query parameters are used. The retained
member-addressed `PUT` is the canonical retry-safe operation. A resource's identity is its normalized URI
(no opaque per-resource ID), consistent with the token-exchange resolver.

| Operation | Method + path | Body |
|---|---|---|
| List (US4) | `GET /api/services/{service-id}/protected-resources` | — → `{ "protected_resources": ["https://…", …] }` + `ETag` |
| Add (US1) | `POST /api/services/{service-id}/protected-resources` | `{ "resource_uri": "https://…" }`; 201 new / 200 same-service replay |
| Add (US1) | `PUT /api/services/{service-id}/protected-resources/{resource}` | — (retained idempotent member add) |
| Remove (US2) | `DELETE /api/services/{service-id}/protected-resources/{resource}` | — |
| Rename (US3) | `PATCH /api/services/{service-id}/protected-resources/{resource}` | `{ "to": "https://…" }` |

`{resource}` is the normalized URI fully RFC 3986 percent-encoded as one segment (`:`→`%3A`,
`/`→`%2F`, `?`→`%3F`, `#`→`%23`). For PATCH, it identifies the source URI. The handler extracts this
escaped final segment from `r.URL.RawPath` or equivalent router-preserved escaped path data, validates that
it is exactly one segment, then `url.PathUnescape`s it once before running the **existing** per-URI
normalize/validate (FR-005/FR-006 parity). `chi.URLParam` is not sufficient where it has lost escaping. A
segment that does not decode to a well-formed absolute URI → `400`.

**Verification requirement**: handler and E2E coverage on the supported Go 1.25.6 runtime MUST prove that
a single `{resource}` route retains a fully percent-encoded URI segment without treating `%2F` as a route
delimiter, and that one decode round-trips query and fragment values. A literal-slash URI is not a valid
member-route representation; collection POST is available when a client cannot send escaped path segments.

**Rationale**:
- Member-addressed sub-resources provide the cleanest REST surface and make PUT retry-safe; collection POST
  provides a body-based add path without weakening URI-as-identity or the member route semantics.
- URI-as-identity matches spec Assumption §Identity model and avoids a synthetic ID the token-exchange
  path would never use (it resolves by URI).
- Rename stays a distinct `PATCH` (source in the path, target in the body) so source-not-found (FR-009)
  and target-conflict (FR-007) remain separate from add's idempotent-create semantics (FR-003/FR-008) —
  no verb is overloaded to mean both create and rename.
- Every mutation returns enough state to confirm the outcome without a follow-up full fetch (FR-016):
  add/remove/rename return the affected resource and/or the updated set plus the new `ETag`.
- Residual: an upstream proxy/gateway may reject or rewrite `%2F` in the path; this is a deployment
  consideration (documented in the contract), not an app-layer limitation — the app extracts the final
  segment from `r.URL.RawPath` or equivalent router-preserved escaped data; a decoded `chi.URLParam` is not
  used when it loses the original escaping.

**Canonical route base**: `/api/services` (as registered in
`internal/adapters/http/routing/admin.go:67-73` and documented in `api/admin/openapi.yaml:430,598`).
Note a **pre-existing inconsistency**: some handler tests/comments reference
`/api/third-party/oauth2/clients` (e.g. `services_handler_test.go`). The wired, documented base is
`/api/services`; the new sub-resource routes use it. The legacy string is **not** propagated, and
reconciling it is out of scope for this feature.

**Alternatives rejected**:
- **Query addressing and body-based mutations for every operation** (original draft):
  `DELETE …/protected-resources?resource=<url-encoded>` and `PUT …/protected-resources` with a
  `{ "from", "to" }` body. They side-step URI-in-path encoding but make remove and rename less
  individually addressable. The accepted POST is limited to additive collection creation and does not
  replace the member-addressed PUT/PATCH/DELETE operations.
- **Opaque per-resource ID** (`…/protected-resources/{resource-id}`): cleanest path shape and zero
  URI-encoding surface, but adds an identifier to the domain model, storage, and responses that token
  resolution never consumes (it matches by URI), diverging from the spec's stated model. Rejected.
- **Trailing-wildcard catch-all** (`…/protected-resources/*` with literal slashes): tolerates unencoded
  URIs but concentrates proxy `//`-normalization risk; the single fully-encoded segment is preferred.

---

## Decision 4 — Migration strategy (fail-closed backfill, drop legacy array)

**Decision**: Migration `028_normalize_service_protected_resources` (next free sequence number after
main-line migration `027_add_profile_to_oauth2_codes`):
- **up**: add `version BIGINT NOT NULL DEFAULT 1` to `thirdparty_oauth2_services`; create
  `service_protected_resources` (PK `resource_uri`, FK `service_id` ON DELETE CASCADE, index on
  `service_id`); backfill by unnesting the existing `protected_resources` array; then
  `DROP INDEX idx_thirdparty_oauth2_services_protected_resources` and
  `ALTER TABLE thirdparty_oauth2_services DROP COLUMN protected_resources`.
- **down**: re-add the `TEXT[]` column + GIN index, backfill from the child table (`array_agg`), drop
  the child table, and drop the `version` column.

**Fail-closed backfill**: the migration aborts *before* any destructive DDL if the legacy data is not
provably canonical or contains collisions. A **canonicality preflight** flags any value whose path ends in
`/` (the sole shape the normalizer rewrites — including the root slash `https://x/` → `https://x`) and any
value not matching the absolute-URI shape; a **collision preflight** then flags any URI owned by more than
one service or repeated within one service's array. Detection lives in these preflights, **not** the
`UNIQUE` constraint alone (which cannot see `https://x` vs legacy `https://x/`). Because the Go
`url.Parse` normalizer cannot be faithfully reproduced as a Postgres function, the backfill inserts the
now-proven-canonical stored values **verbatim**, with the `PRIMARY KEY` as the final backstop. Operators
canonicalize any flagged rows and re-run. (Feature 035 predates production data with overlapping
resources; the checks are the safety net, not an expected path.) The exact SQL is in
[data-model.md](./data-model.md) §Migration.

**No dual source of truth**: after `up`, the array column is gone; the child table is authoritative for
reads (`Get`/`List`/`FindByProtectedResource`) and writes (`Create`/full-`Update`/single-resource ops).

---

## Decision 5 — Token-exchange resolution reads the child table

**Decision**: `FindByProtectedResource` (both adapters) resolves via the child table:
`SELECT service_id FROM service_protected_resources WHERE resource_uri = $1`, then loads the service by
id. Because `resource_uri` is `UNIQUE`, the previous "multiple services match → ambiguous" branch
(`memory/thirdparty_provider.go:158-160`, `postgres/thirdparty_provider.go` scan loop) becomes
structurally impossible; the not-found branch (`InvalidTargetError`) is preserved. The resolver call
site (`tokenexchange/service.go:216-227`) is unchanged — it still normalizes then calls
`FindByProtectedResource`, satisfying **FR-013/SC-004** (a committed add is immediately resolvable; a
committed remove is immediately unresolvable). Already-issued, downstream-cached exchanged tokens remain
governed by the existing ExtProc token-cache lifetime (spec Assumptions §Token-cache consistency) — this
feature adds no active revocation.

---

## Decision 6 — Repository / port surface (ISP)

**Decision**: Extend `ports.ThirdpartyOAuth2ProviderRepository`
(`internal/ports/thirdparty_provider.go`) with focused, single-purpose methods — the resource set is an
owned part of the provider aggregate, so it stays on the provider repository rather than a separate port:

```go
// Atomic single-resource operations on a service's protected resource set.
// Uniqueness is enforced globally by storage; conflicts return StorageError{Kind: Conflict}.
AddProtectedResource(ctx, serviceID id.ServiceID, resourceURI string) (added bool, err error)
RemoveProtectedResource(ctx, serviceID id.ServiceID, resourceURI string) error
RenameProtectedResource(ctx, serviceID id.ServiceID, fromURI, toURI string) error
ListProtectedResources(ctx, serviceID id.ServiceID) ([]string, error)
```

- `Create`, full-`Update`, `Get`, `List` continue to populate/consume `entity.ProtectedResources`; the
  adapters now read/write the child table instead of the array column. The domain entity field
  (`model.ThirdpartyOAuth2ProviderEntity.ProtectedResources []string`) is retained as the in-memory
  representation.
- Full-`Update` gains an optimistic precondition. Rather than widen every caller, the precondition is
  threaded as an explicit parameter on the provider **domain service** `Update`
  (`internal/domain/thirdparty/service.go:192`) and the repository `Update`
  (`internal/ports/thirdparty_provider.go:39`): an `expectedVersion *int64` (nil ⇒ resource set not being
  replaced ⇒ no CAS). Exact signatures are fixed in [data-model.md](./data-model.md).
- A new **domain service** (`ProtectedResourceService`, or methods on the existing
  `ThirdpartyOAuth2ProviderService`) owns validation + normalization + orchestration for the
  single-resource ops so handlers stay thin (Constitution VI: no domain logic in handlers). It reuses
  `NormalizeProtectedResources`/`ValidateProtectedResources`
  (`internal/domain/model/thirdparty_oauth2_provider.go:351-374`) applied per-URI.

**Memory adapter parity**: `InMemoryThirdpartyOAuth2ProviderRepository`
(`internal/adapters/storage/memory/thirdparty_provider.go`) adds a
`resourceOwners map[string]id.ServiceID` guarded by the existing `sync.RWMutex`. Every op mutates
`providers` and `resourceOwners` atomically under one lock, giving the same uniqueness/atomicity
guarantees as the Postgres constraint. Optimistic CAS compares the stored `version`.

---

## Decision 7 — Documentation & governance artifacts

- **New ADR `030-normalize-protected-resources.md`**: records the `text[]` → child-table normalization,
  the global-uniqueness constraint, and the optimistic-concurrency (`If-Match`) contract for the
  full-service PUT. Per Constitution II this ADR is a *proposal* within this PR (it documents, it does
  not self-justify).
- **ARCHITECTURE.md**: update the **Protected Resources** glossary entry (line ~1256, currently
  "Stored as TEXT[] column … with GIN index") to describe the `service_protected_resources` table and
  the uniqueness/optimistic-concurrency model; note the storage change in the storage section.
- **`api/admin/openapi.yaml`**: add the four sub-resource operations + request/response schemas, mark
  `protected_resources` optional on the service PUT with the `If-Match`/`ETag` semantics, and add `428`/
  `412` responses. Per Constitution IV/X these API changes require stakeholder confirmation — the
  OpenAPI diff + ADR 030 are the confirmation artifacts. Render examples in `docs/api/`.

---

## Non-impacts (verified)

- **Configuration / Helm**: no new config parameters → no `internal/config/schema.go`,
  `examples/config/`, `docs/configuration.md`, or `charts/agentic-identity-broker/` changes
  (Constitution VII N/A).
- **Frontend**: the spec describes only the admin/API-client actor; no React UI change is in scope → no
  `web/` change and no Playwright/screenshot work (Constitution XI/XIII-frontend N/A). If a UI is later
  desired it is a separate feature.
- **Encryption**: protected resources are non-secret; the service `Secret` value object, branch keys, and
  `EncryptionContext` are untouched. Single-resource ops never read, require, or alter the client secret
  (FR-012) — they operate only on the child table and the service `version` counter.
