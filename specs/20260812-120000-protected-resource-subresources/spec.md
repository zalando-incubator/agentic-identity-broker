# Feature Specification: Protected Resource Subresources on Third-Party Services API

**Feature Branch**: `20260812-120000-protected-resource-subresources`

**Created**: 2026-08-12

**Status**: Draft

**Input**: User description: "On the API for third-party services, I want to expose adding / modifying / deleting protected resources on a third-party service directly via sub-resources on the API. This should avoid a client having to load a service, modify the internal list and then updating the full service."

## User Scenarios & Testing *(mandatory)*

A third-party service (an external OAuth2 provider configuration) carries a set of **protected resources** — normalized resource URIs used to resolve which service can mint exchanged tokens for a given target (RFC 8693 token exchange). Today these resources exist only as an embedded list inside the service record. The only way to change one is to fetch the entire service, edit the list locally, and resubmit the whole service configuration.

The actor throughout is an **administrator / API client** managing third-party service configuration. Each user story is an independent, independently-testable slice.

### User Story 1 - Add a single protected resource (Priority: P1)

An administrator adds one resource to an existing service through either the idempotent member-addressed `PUT` operation or the body-based collection `POST` operation, without fetching and resubmitting the full service configuration.

**Why this priority**: This is the core motivation. The current load-modify-update flow forces the client to resend every field (including credentials) and risks silently overwriting concurrent changes. Adding one resource in one operation removes both problems and delivers immediate value on its own.

**Independent Test**: Create a service, add one new resource through both supported single-resource operations, and confirm the service's resource set contains the normalized URI while every other field is unchanged.

**Acceptance Scenarios**:

1. **Given** a service with an existing set of protected resources, **When** the administrator adds a new valid, unclaimed resource URI, **Then** the normalized URI is added to that service's set and no other service field is modified.
2. **Given** a service that already owns a URI, **When** the administrator adds the same URI (or one that normalizes to it) again, **Then** the operation succeeds and the set is unchanged (no duplicate is created).
3. **Given** a resource URI already owned by a *different* service, **When** the administrator tries to add it, **Then** the operation is rejected as a conflict and nothing changes.
4. **Given** a malformed, relative, empty, or whitespace-only URI, **When** the administrator tries to add it, **Then** the operation is rejected with a validation error and nothing changes.
5. **Given** a non-existent service, **When** the administrator tries to add a resource to it, **Then** a not-found result is returned.

---

### User Story 2 - Remove a single protected resource (Priority: P1)

An administrator removes one protected resource from a service through a dedicated single-resource operation, without resubmitting the full service.

**Why this priority**: Symmetric to adding; required to deconfigure a resource safely. Without it, removal still forces the risky full-service update, so it belongs in the same MVP.

**Independent Test**: Add a resource, remove it through the single-resource operation, and confirm it is gone while the remaining resources and all other fields are intact.

**Acceptance Scenarios**:

1. **Given** a service that owns a resource URI, **When** the administrator removes that resource, **Then** it is no longer in the service's set and every other field is unchanged.
2. **Given** a service that does not own a given URI, **When** the administrator tries to remove it, **Then** a not-found result is returned and nothing changes.
3. **Given** a resource that was just removed, **When** token exchange attempts to resolve it, **Then** it is no longer resolvable to that service.
4. **Given** a service with exactly one protected resource, **When** the administrator removes it, **Then** the service ends with an empty resource set (a valid state).

---

### User Story 3 - Modify (rename) a protected resource (Priority: P2)

An administrator changes the URI value of an existing protected resource in place, without a separate delete-then-add round trip.

**Why this priority**: Explicitly requested and convenient for correcting a URI, but it is expressible as remove + add, so it ranks below the two core operations.

**Independent Test**: Change a resource's URI from A to a new unclaimed URI B and confirm A is gone, B (normalized) is present, and uniqueness is enforced.

**Acceptance Scenarios**:

1. **Given** a service owning URI A, **When** the administrator changes it to a valid, unclaimed URI B, **Then** A is removed, normalized B is present, and no other field changes.
2. **Given** a target URI B already owned by another service (or already present on the same service), **When** the administrator changes A to B, **Then** the operation is rejected as a conflict and A remains unchanged.
3. **Given** a resource that does not exist on the service, **When** the administrator tries to modify it, **Then** a not-found result is returned.
4. **Given** a resource with URI A, **When** the administrator "changes" it to the same URI A, **Then** the operation succeeds as a no-op.

---

### User Story 4 - Retrieve a service's protected resources (Priority: P3)

An administrator retrieves just the protected resources of a service, without loading the entire service record.

**Why this priority**: Supports confirmation and UI display after single-resource edits, but the existing full-service read already exposes the list, so this is a convenience rather than a gap.

**Independent Test**: Retrieve the protected resources for a service and compare the returned set to the known configured set.

**Acceptance Scenarios**:

1. **Given** a service with N protected resources, **When** the administrator retrieves the service's protected resources, **Then** all N normalized URIs are returned.
2. **Given** a non-existent service, **When** the administrator retrieves its resources, **Then** a not-found result is returned.

---

### Edge Cases

- **Normalization collisions**: A URI that normalizes (e.g. by removing a trailing slash) to one already present is treated as the same resource — idempotent add, no duplicate.
- **Concurrent add of the same new URI**: Two clients adding the same new URI to the same service converge on a single entry without producing a duplicate or corrupting the set.
- **Concurrent edits of different resources**: Two clients editing *different* resources of the same service at the same time must both persist — neither change is silently lost. Because a service's protected resources are managed as a single set, this convergence is a property the design must guarantee, not an automatic outcome.
- **Concurrent claim race**: Two services attempting to claim the *same* new URI at the same time must not both succeed — global uniqueness must hold under concurrency, so the race must be handled explicitly rather than relying on request ordering.
- **Interaction with the full-service update**: A `PUT /api/services/{id}` that omits `protected_resources` leaves the set untouched. A `PUT` that supplies the field, including an explicit empty set, replaces the set only with the current strong `If-Match` ETag; an absent precondition is rejected with `428`, and a stale precondition with `412`, without modifying the set.
- **Credential and field isolation**: A single-resource operation must never require, accept, or alter the service's client secret or any unrelated field.
- **Removal vs. already-issued tokens**: Removing a protected resource stops *new* token-exchange resolutions for that URI immediately, but an exchanged token already issued and cached downstream remains usable until it expires under the existing token-cache lifetime; this feature does not add active revocation of previously issued tokens.

## Clarifications

### Session 2026-08-13

- Q: How should protected resources be added and individually addressed on the API? → A: Add through body-based `POST …/protected-resources` with `resource_uri` or the retained idempotent member-addressed `PUT …/protected-resources/{resource}`. Remove uses `DELETE …/{resource}` and rename uses `PATCH …/{resource}` with a `to` target body; for PATCH, `{resource}` identifies the source URI. Member URIs are a single, fully percent-encoded path segment; no query parameters are used. API-surface only — accepted values, normalization, and validation are unchanged (FR-005/FR-006 parity preserved); rename stays distinct from add (FR-003/FR-007/FR-009).
- **Escaped resource-path handling**: The server MUST obtain `{resource}` from the escaped request path, validate that it occupies exactly one fully percent-encoded path segment, and percent-decode it exactly once before existing URI validation and normalization. For PATCH, `{resource}` is the source URI. It MUST NOT use a generically decoded path value when that can turn `%2F` into a route delimiter.
- **POST add operation**: `POST …/protected-resources` accepts `{ "resource_uri": "<absolute URI>" }` in the JSON body. It is an alternate collection-oriented add operation; the existing idempotent member-addressed `PUT …/protected-resources/{resource}` remains supported. Both operations have identical validation, normalization, ownership, conflict, audit, and response semantics: `201` for a newly added URI, `200` when the same service already owns its normalized form, `409` when another service owns it, and the mutation result plus current strong `ETag` on success.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST allow adding a single protected resource to an existing service without requiring the client to submit the full service configuration through both `POST /api/services/{service-id}/protected-resources` with a `resource_uri` body field and the retained idempotent `PUT /api/services/{service-id}/protected-resources/{resource}` member operation.
- **FR-002**: The system MUST allow removing a single protected resource from an existing service without requiring the full service configuration.
- **FR-003**: The system MUST allow changing the URI of an existing protected resource on a service without requiring the full service configuration.
- **FR-004**: The system MUST allow retrieving the current set of protected resources for a service independently of the full service record.
- **FR-005**: The system MUST normalize submitted resource URIs identically to the existing full-service path (trailing slashes removed) before storing or matching them.
- **FR-006**: The system MUST reject a submitted resource URI that is not a well-formed absolute URI (scheme and host present), including empty or whitespace-only values, with a clear validation error and no state change.
- **FR-007**: The system MUST enforce global uniqueness of protected resource URIs across all services: assigning (via add or modify) a URI already owned by a different service MUST be rejected as a conflict with no state change. This uniqueness MUST hold under concurrent requests.
- **FR-008**: Adding a resource URI already present on the same service through either add operation MUST be idempotent in outcome — the operation succeeds with `200` and the resource set is unchanged (no duplicate entry).
- **FR-009**: Removing or modifying a protected resource that does not exist on the target service MUST return a not-found result with no state change.
- **FR-010**: Any single-resource operation targeting a non-existent service MUST return a not-found result.
- **FR-011**: Each single-resource operation MUST be atomic and MUST NOT drop or overwrite concurrent changes to other protected resources — or to any other field — of the same service (no lost updates).
- **FR-012**: Single-resource operations MUST NOT require or accept the service's client secret and MUST leave the stored secret and all other service fields unchanged.
- **FR-013**: A change made through a single-resource operation MUST be reflected in the authoritative token-exchange resource resolution for all *new* exchange attempts as soon as the operation succeeds — an added resource becomes resolvable and a removed resource is no longer resolvable at the point of resolution. (Already-issued, downstream-cached exchanged tokens for a removed resource are governed by the existing token-cache lifetime; see Assumptions.)
- **FR-014**: Single-resource operations MUST enforce the same authentication and authorization as the existing third-party service management operations.
- **FR-015**: The existing full-service create and update operations MUST remain functional and continue to accept the complete protected-resources set. For full-service `PUT`, omitting `protected_resources` MUST leave the current set unchanged; supplying it, including an explicit empty set, MUST replace the set only when a current strong `If-Match` ETag is supplied. A missing required precondition MUST return `428`, and a stale ETag MUST return `412`, with no state change. The set produced by full-service updates and by single-resource operations MUST converge on the same underlying representation.
- **FR-016**: Each successful single-resource operation MUST return enough of the resulting state for the client to confirm the outcome (the affected resource and/or the updated resource set) without a follow-up full-service fetch. For a successful removal, the submitted normalized resource URI together with `204 No Content` and the returned current strong `ETag` is sufficient confirmation.
- **FR-017**: Successful or rejected add, remove, and rename operations MUST emit a structured audit event identifying the authenticated administrator, action, service ID, outcome, HTTP status, and request ID. Add and remove events use `resource_uri`; rename events use `source_resource_uri` and `target_resource_uri`. Each URI field MUST be populated only when its corresponding submitted URI validates and normalizes; it MUST otherwise be null. The event MUST NOT contain raw submitted URI input, the service client secret, or other credential material.

### Key Entities *(include if feature involves data)*

- **Third-Party Service**: An external OAuth2 provider configuration. It owns an unordered set of unique protected resources. Its credentials, endpoints, scopes, and other attributes are unaffected by single-resource operations.
- **Protected Resource**: A normalized absolute URI naming a target the owning service can issue exchanged tokens for (RFC 8693). It is globally unique across services; its identity is its normalized URI value (there is no separate opaque identifier today).

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Adding, changing, or removing a single protected resource takes exactly one API operation, down from the current minimum of two (fetch, then full update).
- **SC-002**: Editing protected resources never requires transmitting or re-supplying the service's client secret or any other configuration field.
- **SC-003**: When two administrators change *different* protected resources of the same service concurrently, 100% of the non-conflicting changes persist (no lost update).
- **SC-004**: A newly added protected resource is resolvable by a new token-exchange attempt as soon as the add succeeds; a removed resource is rejected by any new token-exchange attempt as soon as the removal succeeds. Reuse of an already-issued cached token is bounded by the existing token-cache lifetime, not by this operation.
- **SC-005**: 100% of attempts to assign a URI already owned by another service are rejected as conflicts, including under concurrent requests.
- **SC-006**: 100% of malformed, relative, empty, or whitespace-only resource URIs are rejected before any state change.
- **SC-007**: 100% of successful and rejected protected-resource mutations produce a structured audit event with the required non-secret metadata.

## Assumptions

- **Identity model**: Protected resources continue to be identified by their normalized URI value; no new opaque per-resource identifier is introduced. This matches the existing model (a resource is a URI string; token exchange resolves by URI). If stakeholders prefer stable per-resource identifiers, this is the primary decision to revisit during planning.
- **Modify semantics**: "Modify" a protected resource means changing its URI value (a rename); a protected resource has no attributes other than its URI.
- **Single-resource scope**: Only single-resource add/modify/remove operations are in scope. Bulk operations (adding or removing many resources in one call, or operations spanning multiple services) remain the responsibility of the existing full-service update and are out of scope here.
- **Conflict and not-found semantics** (FR-007/FR-008/FR-009/FR-010): The choices to treat same-service duplicate adds as idempotent successes, cross-service duplicates as conflicts, and missing target/service as not-found are design decisions to be confirmed against existing service-handler behavior during planning; they are not yet verified against current handler responses.
- **Full-service concurrency contract**: The full-service update preserves the existing endpoint but distinguishes an omitted `protected_resources` field from an explicit set. Omitted leaves the set unchanged; an explicit replacement, including empty, requires the current strong `If-Match` ETag and fails with `428` when absent or `412` when stale. This mechanism preserves FR-011 and FR-015 while avoiding silent loss of a concurrent single-resource mutation.
- **Token-cache consistency**: Downstream token exchange caches exchanged tokens for a bounded lifetime (existing behavior). Consequently, "immediately reflected" (FR-013, SC-004) applies to authoritative resolution of new exchange attempts; a previously issued token for a removed resource may remain usable until it expires. This feature preserves that existing consistency model and does not introduce active token revocation.
- **Validation and normalization parity**: Normalization and validation rules for resource URIs are identical to those already applied when protected resources are set through full-service create/update.
- **Audit URI fields**: Audit events retain normalized URIs only. Add and remove events populate `resource_uri` after successful validation. Rename events independently populate `source_resource_uri` and `target_resource_uri` after successful validation. Each URI field is null when its corresponding input cannot validate/normalize; raw submitted URI input is never logged, preventing malformed values (which may contain credential-like data) from entering audit records.
- **Authorization**: These operations reuse the authentication and authorization of the existing third-party service management (admin) API.
- **API resource addressing**: Single-resource `PUT`, `DELETE`, and `PATCH` operations address a protected resource by its normalized URI in one fully percent-encoded `{resource}` path segment under the owning service’s `protected-resources` sub-collection; no query parameters. For PATCH, `{resource}` identifies the source URI and the `to` body identifies the target, preserving distinct source-not-found (FR-009) and target-conflict (FR-007) outcomes. `POST /protected-resources` is also supported for add with a `resource_uri` JSON field; it returns `201` for a new URI and `200` for a same-service normalized replay. This API-surface decision preserves URI normalization and validation parity (FR-005/FR-006). Full-service create remains unchanged; full-service update retains its existing fields but applies the documented `protected_resources` and optimistic-concurrency delta (FR-015). Handlers must extract the final member segment from `URL.RawPath` or equivalent router-preserved escaped data and decode once; decoded `chi.URLParam` is not sufficient when escaping is lost.

### Dependencies

- Existing third-party services administrative API and its protected-resources model.
- RFC 8693 token-exchange resource resolution, which must observe changes made through single-resource operations.

### Out of Scope

- Introducing per-resource metadata beyond the URI (labels, descriptions, timestamps).
- Bulk or cross-service resource operations.
- Any change to token-exchange resolution semantics beyond reflecting the updated resource set.
