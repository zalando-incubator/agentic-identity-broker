# ADR 012: In-Memory Token Cache for ExtProc Token Exchange

**Status**: Accepted
**Date**: 2026-02-23
**Feature**: 015-extproc-token-exchange

---

## Context

The ExtProc token exchange service (`cmd/extproc-token-exchange`) caches exchanged OAuth2 tokens to reduce latency and load on the identity broker. Every request that passes through agentgateway triggers the ExtProc gRPC `Process` RPC. Without caching, each request would require a synchronous HTTP call to the identity broker's `POST /oauth2/token` endpoint, adding round-trip latency and proportionally scaling load on the broker.

A caching strategy must be chosen that:

1. Is simple to implement and reason about (MVP scope).
2. Is safe under concurrent access (multiple goroutines serve ExtProc requests simultaneously).
3. Avoids the thundering herd problem when a cached token expires and many concurrent requests arrive simultaneously.
4. Bounds memory growth for long-running deployments.
5. Does not introduce an external infrastructure dependency.
6. Uses a cache key that is free from separator-injection collisions.

---

## Decision

The accepted decision selects an in-memory map, a struct key, `sync.RWMutex`, singleflight refresh, periodic cleanup, and a maximum cache TTL.
The [implementation clarification](#implementation-clarification) records current behavior separately. It does not approve changes to the accepted policy.

The original decision described the following cache shape. The current [cache types](../internal/extproc/server/exchanger.go#L33-L79) also hold metadata, re-authentication errors, and a stale deadline.

```go
type tokenCacheKey struct {
    subjectToken string
    resourceURI  string
}

type cachedToken struct {
    accessToken string
    tokenType   string
    expiresAt   time.Time
}
```

Concurrent refresh deduplication is handled by `golang.org/x/sync/singleflight`. When multiple goroutines detect a cache miss or expiry for the same key simultaneously, `singleflight` ensures only one token exchange call is made to the identity broker; all waiting goroutines receive the same result.

The original decision called for an expiry sweep every `default_ttl / 2`, with a 30-second minimum.
Current [sweep code](../internal/extproc/server/exchanger.go#L559-L596) uses a one-second minimum and retains success entries through their stale deadline.
These differences remain explicit implementation discrepancies, not amendments to the accepted decision.

The maximum TTL limits cached token reuse and token-bound metadata.
The [TTL calculation](../internal/extproc/server/exchanger.go#L470-L483) caps positive `expires_in` values, or `cache.default_ttl`, at `cache.max_ttl`.
The [configuration defaults](../internal/extproc/config/loader.go#L354-L355) are five minutes and one hour, respectively.

---

## Implementation clarification

### Fresh reads and refresh

A fresh entry returns its token and metadata without a broker call, provided the client assertion remains usable.
A fresh re-authentication entry returns its cached error instead.
An absent or expired assertion bypasses both fresh-entry paths and prevents stale fallback.
Sources: [fresh reads](../internal/extproc/server/exchanger.go#L212-L256), [assertion check](../internal/extproc/server/exchanger.go#L410-L424), and [stale fallback](../internal/extproc/server/exchanger.go#L302-L321).

An expired entry remains in the map during refresh. A miss or expiry enters singleflight, which checks the cache again.
Each caller can cancel its wait without canceling the shared exchange.
Sources: [refresh path](../internal/extproc/server/exchanger.go#L242-L300) and [caller cancellation](../internal/extproc/server/exchanger.go#L346-L402).

### Expiry and stale reuse

A successful exchange sets `expiresAt` to insertion time plus the effective TTL.
Positive `expires_in` values supply that TTL. Missing, zero, or negative values use `cache.default_ttl`.
The calculation caps either value at `cache.max_ttl`.
Sources: [population](../internal/extproc/server/exchanger.go#L282-L299) and [TTL calculation](../internal/extproc/server/exchanger.go#L470-L483).

The same insertion sets `staleUntil` to the earlier of `expiresAt + 5s` and insertion time plus `cache.max_ttl`.
Transient failures do not extend this deadline.
If the effective TTL equals `cache.max_ttl`, there is no stale-reuse window after expiry.
Sources: [five-second constant](../internal/extproc/server/exchanger.go#L46-L51), [population](../internal/extproc/server/exchanger.go#L282-L299), and [fallback](../internal/extproc/server/exchanger.go#L302-L321).

After an exchange error, fallback requires a nonempty cached token, a usable assertion, and a current time strictly before `staleUntil`.
The classifier permits broker 5xx errors and HTTP 429 without `error_uri`.
It rejects assertion expiry and other broker errors, including HTTP 429 with `error_uri`.
Other errors also qualify, including network, circuit-open, and malformed-success-response errors. The classifier does not restrict fallback to transport errors.
Sources: [fallback and circuit breaker](../internal/extproc/server/exchanger.go#L302-L343), [classifier](../internal/extproc/server/exchanger.go#L366-L388), and [response errors](../internal/extproc/server/exchanger.go#L435-L467).

A non-transient broker error with `error_uri` replaces the entry with a five-second re-authentication error.
Other exchange errors do not delete the previous entry.
An authoritative rejection returns an error for that call, but does not generally erase the old token.
Sources: [error population](../internal/extproc/server/exchanger.go#L267-L299) and [error propagation](../internal/extproc/server/exchanger.go#L302-L343).

### Retention, capacity, and cleanup

The sweep runs on a ticker with interval `max(cache.default_ttl / 2, 1s)`.
It deletes an expired entry only when `staleUntil` is zero or strictly earlier than the sweep time.
Thus, success entries survive ordinary expiry through their stale window. Re-authentication entries have no stale window.
Sources: [error entries](../internal/extproc/server/exchanger.go#L267-L277) and [sweep](../internal/extproc/server/exchanger.go#L559-L596).

Neither a fresh read nor a failed refresh guarantees deletion.
An entry can remain in memory after reuse is forbidden, until a later sweep or replacement.
The eviction loop also performs assertion refresh, so its ticker interval is not an exact deletion deadline.
`Shutdown` signals the loop to stop. It does not clear the map.
Sources: [reads and replacement](../internal/extproc/server/exchanger.go#L225-L321), [shutdown](../internal/extproc/server/exchanger.go#L405-L408), and [background loop](../internal/extproc/server/exchanger.go#L559-L618).

There is no capacity eviction, maximum entry count, or byte limit.
TTL cleanup limits retention during normal operation, not the number of distinct keys that can arrive before cleanup.
Sources: [cache configuration](../internal/extproc/config/config.go#L48-L52), [insertion](../internal/extproc/server/exchanger.go#L267-L299), and [eviction](../internal/extproc/server/exchanger.go#L587-L596).

The server uses successful results, including stale results, to replace the Authorization header.
OPA requests also receive the returned token-bound metadata.
Sources: [standard caller](../internal/extproc/server/server.go#L453-L460) and [OPA caller](../internal/extproc/server/server.go#L527-L537).

### Unresolved policy conflict

[Feature 015, FR-015](../specs/015-extproc-token-exchange/spec.md#L120-L126) requires eviction before reuse of expired entries.
The original decision also describes expiry-based removal. Current stale retention and reuse do not satisfy that literal requirement.
Accepted [036-extproc-approval-cache-and-gate.md](036-extproc-approval-cache-and-gate.md#separate-from-adr-012) acknowledges bounded stale-token reuse but also describes invalidation at token expiry.
These statements do not establish one consistent expiry policy.

[Constitution II](../.specify/memory/constitution.md#ii-architecture-documentation--decision-records) requires a superseding ADR for deviations from accepted decisions.
This clarification preserves the accepted status and records the conflict for review.
It neither approves stale-token reuse as a new security exception nor changes runtime behavior.

---

## Rationale

### 1. In-memory is sufficient for single-binary MVP

The spec requirement is a "simple in memory cache". The ExtProc service is a single-binary, single-instance deployment for MVP. No distributed cache is needed.

### 2. Struct key eliminates separator-injection collisions

A composite struct key `{subjectToken, resourceURI string}` is used in place of a hashed string key such as `sha256(subjectToken + "|" + resourceURI)`. A struct key eliminates the separator-injection collision risk: an attacker who controls the `subjectToken` or `resourceURI` values cannot manufacture a key collision by embedding the separator character in their input. For a cache that maps security tokens, this is the correct design even though conforming JWT inputs make the collision unlikely in practice.

Using a struct as a Go map key is supported natively by the language for any struct containing only comparable fields, and has no runtime overhead beyond the underlying map lookup.

### 3. `sync.RWMutex` + `map` is idiomatic for a read-heavy workload

Token exchange produces tokens with lifetimes measured in minutes or hours. Once cached, a token is read on every request for that `(subject_token, resource)` pair until it expires. Writes (cache insertions and evictions) are rare relative to reads. `sync.RWMutex` allows multiple concurrent readers to proceed without blocking each other, while writers acquire an exclusive lock only for the brief duration of a map insert or delete.

### 4. Singleflight prevents thundering herd on cache expiry

When a cached token expires, all in-flight requests for the same `(subject_token, resource)` key detect a cache miss simultaneously. Without deduplication, each goroutine would independently issue a token exchange call to the identity broker — N calls instead of one. `golang.org/x/sync/singleflight` deduplicates these: the first goroutine to detect the miss initiates the exchange; all others block and receive the result when the exchange completes.

### 5. Periodic cleanup limits retention, not capacity

The [sweep](../internal/extproc/server/exchanger.go#L587-L596) removes expired entries after their stale deadline.
The [cache configuration](../internal/extproc/config/config.go#L48-L52) contains TTLs but no capacity limit.
Periodic cleanup is not a fixed memory bound.

### 6. `cache.max_ttl` limits cached reuse

The [population and fallback paths](../internal/extproc/server/exchanger.go#L282-L321) apply the maximum TTL to token reuse, including stale fallback.
Token-bound metadata shares those deadlines. This bound does not provide immediate revocation or deletion at expiry.

---

## Alternatives Considered

### A. Redis or external cache

Rejected. Introduces an operational dependency (Redis deployment, network hop, connection pooling) that is over-engineered for a single-binary MVP. The spec explicitly requires "simple in memory cache".

### B. `sync.Map`

Rejected. `sync.Map` is optimised for key-stable workloads (keys written once, read many times, rarely deleted), which matches this use case. However, `sync.Map` does not expose direct iteration with deletion, making the periodic eviction sweep more complex to implement correctly. `sync.RWMutex` with a plain `map` gives full control over eviction and produces clearer, more auditable code.

### C. LRU cache (e.g., `github.com/hashicorp/golang-lru`)

Rejected in the original decision. The current implementation has no LRU or other capacity eviction.
Its [TTL sweep](../internal/extproc/server/exchanger.go#L587-L596) does not provide the same memory bound as a size cap.

### D. SHA256 hash as cache key

Rejected. A `sha256(subjectToken + "|" + resourceURI)` string key was considered in the research phase (see `specs/015-extproc-token-exchange/research.md`, R-006). It was superseded by the struct key in the final design. The separator-injection collision risk, while unlikely with well-formed JWTs, is an anti-pattern for security-sensitive data. A struct key is more correct, simpler, and has no performance downside. The hash-based approach also obscures the cache key semantics; a struct makes the composite nature of the key explicit in the type system.

---

## Consequences

### Positive

- **Low latency on cache hit**: Map lookup under read lock is a nanosecond-level operation. Sub-10ms header processing time is achievable on cache hit.
- **No external dependency**: No Redis, Memcached, or database required for the cache layer.
- **Thundering herd protection**: Singleflight ensures one exchange call per key regardless of concurrent request volume.
- **Periodic cleanup**: The [sweep](../internal/extproc/server/exchanger.go#L587-L596) removes entries after expiry and their stale deadline. It does not enforce a capacity bound.
- **Type-safe key**: Struct key makes the composite cache key explicit and collision-free.

### Negative

- **Cold cache on restart**: The in-memory cache is not durable. After a restart, every `(subject_token, resource)` pair requires a fresh token exchange.
- **Single-instance only**: Each binary instance maintains its own independent cache. In a horizontally scaled deployment, cache hits are not shared across instances. This is accepted for MVP.
- **Token-bound metadata can be stale**: The [fallback](../internal/extproc/server/exchanger.go#L282-L321) returns the same metadata snapshot through `staleUntil`, capped by `cache.max_ttl`.

### Future extension

If horizontal scaling is required, the cache layer must be extracted to an external store (e.g., Redis with TTL-keyed entries). The `TokenExchanger` interface in `internal/extproc/server/exchanger.go` provides the seam at which a distributed cache adapter can be substituted without changes to the gRPC server logic.

---

## References

- Feature Specification: `specs/015-extproc-token-exchange/`
- Data Model: `specs/015-extproc-token-exchange/data-model.md`
- Research (cache key discussion): `specs/015-extproc-token-exchange/research.md` (R-006)
- Implementation: `internal/extproc/server/exchanger.go`
- Go standard library: `sync.RWMutex`, `golang.org/x/sync/singleflight`
