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

The token cache is implemented as a `map[tokenCacheKey]*cachedToken` protected by `sync.RWMutex`, where:

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

A background goroutine runs a periodic eviction sweep at an interval of `default_ttl / 2` (minimum 30 seconds). The sweep removes entries whose `expiresAt` is in the past. This bounds memory growth for long-running deployments with many distinct `(subject_token, resource_uri)` pairs.

A configurable `cache.max_ttl` cap (defaulting to the value of `expires_in` from the token exchange response, subject to operator-defined ceiling) prevents very large `expires_in` values from defeating token revocation. The same bound applies to any token-bound metadata cached alongside the exchanged access token.

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

### 5. Periodic eviction sweep bounds memory growth

Without active eviction, expired entries for tokens that are never re-requested remain in the map indefinitely. In a long-running deployment with many distinct `(token, resource)` pairs — for example, many users each accessing many distinct resource URIs — the map would grow without bound. A background ticker running at `default_ttl / 2` scans and removes expired entries. The sweep interval is proportional to the TTL so that the average time an expired entry occupies memory is bounded.

### 6. `cache.max_ttl` cap prevents defeating token revocation

If the identity broker issues tokens with very large `expires_in` values (hours or days), caching them at face value means a revoked token would remain valid in the cache until expiry. A configurable `cache.max_ttl` allows operators to cap the effective cache TTL below the token's stated lifetime, limiting the revocation window. Any token-bound metadata cached with that token inherits the same bound.

---

## Alternatives Considered

### A. Redis or external cache

Rejected. Introduces an operational dependency (Redis deployment, network hop, connection pooling) that is over-engineered for a single-binary MVP. The spec explicitly requires "simple in memory cache".

### B. `sync.Map`

Rejected. `sync.Map` is optimised for key-stable workloads (keys written once, read many times, rarely deleted), which matches this use case. However, `sync.Map` does not expose direct iteration with deletion, making the periodic eviction sweep more complex to implement correctly. `sync.RWMutex` with a plain `map` gives full control over eviction and produces clearer, more auditable code.

### C. LRU cache (e.g., `github.com/hashicorp/golang-lru`)

Rejected. An LRU eviction policy would evict the _least recently used_ token regardless of whether it has expired. This could evict a valid, frequently-used token simply because another key was accessed more recently. TTL-based eviction correctly keeps all valid tokens in cache until they expire. The periodic sweep achieves the same memory bounding property as an LRU size cap with less complexity.

### D. SHA256 hash as cache key

Rejected. A `sha256(subjectToken + "|" + resourceURI)` string key was considered in the research phase (see `specs/015-extproc-token-exchange/research.md`, R-006). It was superseded by the struct key in the final design. The separator-injection collision risk, while unlikely with well-formed JWTs, is an anti-pattern for security-sensitive data. A struct key is more correct, simpler, and has no performance downside. The hash-based approach also obscures the cache key semantics; a struct makes the composite nature of the key explicit in the type system.

---

## Consequences

### Positive

- **Low latency on cache hit**: Map lookup under read lock is a nanosecond-level operation. Sub-10ms header processing time is achievable on cache hit.
- **No external dependency**: No Redis, Memcached, or database required for the cache layer.
- **Thundering herd protection**: Singleflight ensures one exchange call per key regardless of concurrent request volume.
- **Bounded memory**: Periodic eviction prevents unbounded map growth in long-running deployments.
- **Type-safe key**: Struct key makes the composite cache key explicit and collision-free.

### Negative

- **Cold cache on restart**: The in-memory cache is not durable. After a restart, every `(subject_token, resource)` pair requires a fresh token exchange.
- **Single-instance only**: Each binary instance maintains its own independent cache. In a horizontally scaled deployment, cache hits are not shared across instances. This is accepted for MVP.
- **Bounded revocation lag for token-bound metadata**: Any metadata cached with the exchanged token (for example `granted_permission_sets`) remains a snapshot until token expiry or `cache.max_ttl`, not an independent live authority.

### Future extension

If horizontal scaling is required, the cache layer must be extracted to an external store (e.g., Redis with TTL-keyed entries). The `TokenExchanger` interface in `internal/extproc/server/exchanger.go` provides the seam at which a distributed cache adapter can be substituted without changes to the gRPC server logic.

---

## References

- Feature Specification: `specs/015-extproc-token-exchange/`
- Data Model: `specs/015-extproc-token-exchange/data-model.md`
- Research (cache key discussion): `specs/015-extproc-token-exchange/research.md` (R-006)
- Implementation: `internal/extproc/server/exchanger.go`
- Go standard library: `sync.RWMutex`, `golang.org/x/sync/singleflight`
