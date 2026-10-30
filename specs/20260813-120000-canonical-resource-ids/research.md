# Research: Canonical Resource IDs

## Decision: Keep typed entity IDs UUID-backed; resolve canonical IDs at administrative boundaries

**Rationale**: `id.AgentID`, `id.ServiceID`, and `id.PermissionSetID` are generated `uuid.UUID` aliases. Their `[16]byte` layout is required by `Scan`, `Value`, text/JSON serialization, UUID database columns, JSONB references, foreign keys, and typed-ID map keys. Replacing their representation with a UUID-or-string union would violate ADR 20260313-120000, require a superseding accepted ADR, and risk persisting canonical text into UUID fields. The minimal safe interpretation of the requested reuse is to repurpose administrative wire identifier fields (`id`, `service_id`, and `permission_set_id`) as UUID-or-canonical input and resolved output. The resolver converts either form to the existing UUID typed ID before domain services or persistence.

**Alternatives considered**:
- Change each typed ID to a UUID-or-canonical union: rejected; high churn, breaks UUID-only storage/serialization and canonical/UUID map-key equivalence, and conflicts with ADR 20260313-120000.
- Add a separate canonical identity type to persisted relationships: rejected; references already require a stable UUID relationship and resolving before persistence avoids a second identity representation.

## Decision: Model canonical ID as optional entity metadata with per-table uniqueness

**Rationale**: Add nullable `canonical_id VARCHAR(255)` to `agents`, `thirdparty_oauth2_services`, and `permission_sets`. Each table gets a partial unique index on non-null values. This implements the clarified requirement: the same canonical ID may exist in different resource types, while a duplicate within one type is rejected. PostgreSQL is the concurrency authority; the in-memory repositories maintain matching per-type canonical indexes under their existing write mutexes.

**Alternatives considered**:
- Global registry: rejected; user clarified uniqueness is per table/type.
- Application-only duplicate checks: rejected; concurrent PostgreSQL writers require a database uniqueness constraint.
- Non-null empty-string sentinel: rejected; null cleanly represents an absent optional ID and allows multiple resources without one.

## Decision: Store references as resolved UUIDs

**Rationale**: Agent service requirements and permission-set declarations remain UUID JSONB values; permission-set service scopes remain UUID foreign keys. A canonical ID is resolved to the target UUID before validation and persistence. Removing or changing a target canonical ID therefore never invalidates an already persisted relationship.

**Alternatives considered**:
- Persist canonical references: rejected; renaming/removing canonical IDs would break relationships and duplicate identity logic.

## Decision: Keep nested reference schemas shared; reads stay UUID, writes accept either

**Rationale**: Top-level reads keep `id` as the stable UUID and add a nullable, read-only `canonical_id`; no separate `uuid` field is introduced. Nested reference identifiers (`service_requirements[].service_id`, `permission_sets[].permission_set_id`, `service_scopes[].service_id`) remain the target UUID on read. The shared `ServiceScope` and `AgentPermissionSetEntry` schemas stay shared; their identifier field relaxes from `format: uuid` to `type: string` so a canonical value is accepted on write while responses still emit UUIDs. Read and write therefore use one schema each and a GET response round-trips through PUT verbatim, which is the dominant admin edit workflow. The already-split `ServiceRequirement`/`ServiceRequirementRequest` pair keeps `format: uuid` on the response and relaxes only the request. This preserves backwards compatibility—existing `format: uuid`-only clients keep sending UUIDs—and never makes an identifier value polymorphic.

**Alternatives considered**:
- Resolve read identifiers to canonical when present (top-level `id` and nested references): rejected; it makes a resource's primary identifier and nested references mutable and polymorphic, breaks the published `format: uuid` response semantics, and breaks consumers that join nested `service_id` as a UUID.
- Add read-only `uuid`/`service_uuid` companion fields and split every nested schema: rejected; the companions duplicate data already reachable via the target's own read, and splitting the currently shared schemas desymmetrizes read and write, forcing read-modify-write clients to strip read-only fields for no write-side benefit.
- A `resolve_canonical_ids` query parameter: rejected; OpenAPI cannot model a field whose type depends on a query parameter, it reintroduces identifier polymorphism keyed on a request flag, and canonical rendering is a client presentation concern with no current server-side consumer.

## Decision: Opt-in canonical presentation via an RFC 7240 Prefer header (default UUID)

**Rationale**: A read request MAY send `Prefer: reference-id=canonical` (RFC 7240) to render nested reference identifiers as canonical where the target has one; absent or unrecognized preferences return UUIDs, so the default representation is byte-for-byte the pre-feature behavior. The server acknowledges an applied preference with `Preference-Applied: reference-id=canonical` and sends `Vary: Prefer` so caches never cross-serve. Because the nested reference field is already `type: string`, the canonical representation needs no body-schema change, and every rendered canonical value is a valid write input, so a preference-canonical read still round-trips through PUT. The extra per-target canonical lookup runs only when the preference is present, keeping the default read path free of added loads. The top-level `id` is deliberately out of scope: it stays UUID in all representations and its canonical is always available via `canonical_id`.

**Alternatives considered**:
- Flip the default to canonical-when-present (UUID via a `reference-id=uuid` opt-out): rejected; a header-less existing client would silently receive canonical identifiers once any referenced resource gains a canonical ID—an opt-out breaking change that violates FR-010 and the backwards-compatibility assumption—and it inverts the Prefer idiom by making the opt-in a no-op.
- Server default canonical behind a new API version: deferred; a breaking representation change belongs to an explicit version bump, not this additive contract, and no consumer requires it today.
- Query parameter, or an always-on `service_canonical_id` companion: rejected earlier for URL forking / schema-by-parameter and for read/write desymmetrization, respectively.

## Decision: Reject invalid canonical IDs before mutation; resolve references atomically

**Rationale**: A supplied canonical ID must be non-empty and fail UUID parsing. Creation/update validates before repository mutation; duplicate index violations map to conflict. Resolver lookups return not found for paths and validation errors for write references. Reference existence is checked in the same PostgreSQL transaction, including a lock-backed service check for JSONB agent service requirements, preventing a target deletion between resolution and persistence.

**Alternatives considered**:
- Resolve then persist without transactional revalidation: rejected; concurrent deletion can produce a dangling JSONB reference.

## Decision: Treat the user request as administrative API-only

**Rationale**: There is no admin React client. The consent SPA does not call the admin resource endpoints. Backend OpenAPI, handlers, repositories, migrations, and Ginkgo HTTP E2E tests are in scope; frontend Playwright work is not.

**Alternatives considered**:
- Add consent UI support: rejected; no end-user-facing canonical-ID requirement exists.