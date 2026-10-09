# ADR 030: Normalize Protected Resources

**Status**: Accepted
**Date**: 2026-08-13
**Feature**: 035-protected-resource-subresources

---

## Context

A third-party OAuth2 provider owns a set of protected resource URIs used to resolve the target service for RFC 8693 token exchange. The current design stores that set in a `TEXT[]` column on the provider. That representation cannot make a normalized URI globally unique across providers with a database constraint, and a whole-set update can overwrite a concurrent single-resource change.

Feature 035 introduces first-class protected-resource subresource operations while retaining a full-service update path. The storage model must ensure that concurrent claims of the same normalized URI cannot succeed for different services, that independent single-resource changes do not require clients to round-trip a complete service record, and that a stale full-set replacement cannot erase a concurrent delta.

A protected resource has no attributes beyond its URI. Its identity is therefore its normalized URI, not a synthetic identifier. Normalization trims trailing path slashes before storage and matching; validation continues to require a well-formed absolute URI with a scheme and host.

## Decision

Protected resources move from `thirdparty_oauth2_services.protected_resources` to a `service_protected_resources` child table:

- `resource_uri TEXT PRIMARY KEY` is the normalized URI and enforces global uniqueness across all services.
- `service_id UUID NOT NULL REFERENCES thirdparty_oauth2_services(id) ON DELETE CASCADE` identifies the owning provider, with an index on `service_id` for service-scoped listing.
- The legacy `TEXT[]` column and its GIN index are removed after a fail-closed migration verifies canonical values and absence of normalized collisions.
- The provider aggregate continues to expose the resource set in memory for whole-set reads and writes, while storage adapters materialize that set from the child table. Single-resource operations mutate child rows atomically rather than read, modify, and rewrite the complete set.

The parent service has a monotonic `version BIGINT` optimistic-concurrency counter, initially `1`. Changes to active service configuration or its owned `protected_resources` increment this counter in the same transaction. Read responses expose the active configuration and owned resource set as one strong service `ETag`. A failed discovery attempt updates only a separate logical status resource. This failure-only status has no ETag and does not increment the service version.

A full-service `PUT /api/services/{service-id}` that omits or supplies `null` for `protected_resources` preserves the resource set and does not require `If-Match` for that reason. A request that supplies `protected_resources`, including `[]`, authoritatively replaces the set and must include the current strong `If-Match` ETag. The replacement compare-and-swaps the parent version, replaces child rows in the same transaction, and increments the version. A missing precondition returns `428 Precondition Required`; a stale precondition returns `412 Precondition Failed` without modifying the set.

Single-resource add, remove, and rename operations do not require `If-Match`: they are atomic deltas and still advance the parent version so that a concurrent stale full-set replacement is rejected.

## Consequences

### Positive

- The database, rather than request ordering or application-level checks, guarantees that a normalized protected resource belongs to at most one service.
- A resource can be added, removed, or renamed without fetching, resending, or changing unrelated service fields, including credentials.
- Independent single-resource changes can persist without whole-set lost updates.
- A full-set replacement cannot silently discard a concurrent resource mutation: the changed version causes a stale `If-Match` to fail, and the client must retrieve the current state before retrying.
- The resource URI remains the visible and storage identity, avoiding a synthetic identifier with no domain meaning.

### Negative

- The migration must backfill child rows, reject legacy non-canonical or colliding data before destructive changes, and support rollback.
- Full-service clients that intentionally replace `protected_resources` must first obtain and send a current ETag; clients that only update other fields must omit the field.
- The persistence implementation gains transactional child-row writes, version maintenance, and unique-constraint error mapping.

### Risks

- A client that retries a `412` using an old desired set without reconciling the refreshed set can still intentionally replace recent changes; the contract prevents silent stale replacement but cannot choose a merge policy for the client.
- Proxies or gateways that reject or rewrite encoded slashes can prevent member-addressed resource URIs from reaching the API intact. Deployments using those routes must pass the fully percent-encoded segment through unchanged.
