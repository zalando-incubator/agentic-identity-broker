# Research: Broker-Hosted Aggregated JWKS

**Branch**: `20260604-120000-aggregated-jwks` | **Date**: 2026-06-04

## Research Questions

### RQ-1: How to reuse the existing JWKS adapter for upstream key aggregation?

**Decision**: Reuse `internal/adapters/jwks/adapter.go` (`*jwks.Adapter`) unchanged.

**Rationale**: The existing adapter already implements `JWKSPort` with `GetKeySet(ctx) (jwk.Set, error)` which returns the complete upstream JWKS as a `jwk.Set`. This is exactly what the aggregated publisher needs. The adapter handles caching, background refresh, error propagation, and OTel tracing. No modifications needed — a second instance is created at startup for the JWKS publisher path (separate from any token exchange adapter instance) using the same upstream JWKS URI.

**Alternatives Considered**:
- Creating a dedicated "upstream key source" adapter → rejected: duplicates existing functionality
- Sharing the same adapter instance between token exchange and JWKS publisher → acceptable and preferred when both use the same upstream URI (saves memory and HTTP connections)

### RQ-2: Where to place the aggregation domain logic?

**Decision**: New domain package `internal/domain/jwkspublisher/` with a `Service` struct.

**Rationale**: The aggregation logic (mode-based key selection, kid conflict detection, fail-closed error handling) is pure domain logic that depends on port interfaces. It doesn't belong in:
- The handler (violates "handlers do HTTP translation only")
- The existing JWKS adapter (violates ISP — adapter fetches upstream keys, doesn't aggregate)
- The OAuth2 authorization service (unrelated concern)

The new service composes:
- `ports.SigningKeyManager` (for local keys via `BuildJWKS()`)
- `ports.JWKSPort` (for upstream keys via `GetKeySet()`) — nullable for local-only mode
- Mode enum to select behavior

**Alternatives Considered**:
- Inlining logic in the handler → rejected: domain logic in adapter layer violates hexagonal architecture
- Adding a method to `SigningKeyService` → rejected: conflates signing key management with key aggregation/publishing

### RQ-3: How to define the new port interface?

**Decision**: New `JWKSPublisherPort` interface in `internal/ports/jwks_publisher.go`:

```go
type JWKSPublisherPort interface {
    PublishJWKS(ctx context.Context) (jwk.Set, error)
}
```

**Rationale**: Single-method interface (ISP). The JWKS handler delegates to this port. The domain service implements it. Error semantics: returns a domain error when upstream is unavailable (handler maps to 503) vs internal error (handler maps to 500).

**Alternatives Considered**:
- Expanding existing `JWKSPort` → rejected: `JWKSPort` is for *fetching* upstream keys; `JWKSPublisherPort` is for *publishing* the aggregated set. Different responsibilities.
- Using `SigningKeyManager.BuildJWKS()` directly with a wrapper → rejected: doesn't support mode-dependent behavior cleanly

### RQ-4: How to detect upstream JWKS staleness at runtime?

**Decision**: Use `jwk.Cache.Ready()` check + explicit `Refresh()` with error propagation.

**Rationale**: The existing `jwks.Adapter.GetKeySet()` already:
1. Checks `cache.Ready()` — if not ready, forces `Refresh()`
2. Returns an error if refresh fails
3. Returns cached set if available

For the fail-closed requirement, the publisher service calls `GetKeySet()`. If it errors, the publisher returns a specific `ErrUpstreamUnavailable` error. The handler maps this to HTTP 503.

For the "cache expired" scenario: `jwk.Cache` in `lestrrat-go/jwx/v3` returns the last successful result from `Lookup()` even after MaxInterval passes — it's a stale-while-revalidate pattern. To enforce strict expiry (spec FR-010), the publisher service needs to track the last successful fetch timestamp and compare against a max staleness threshold (e.g., 2× the cache max-age = 600s).

**Implementation detail**: The JWKS adapter will be extended with a `IsStale(maxAge time.Duration) bool` method, or the publisher service will wrap the adapter with a freshness gate. The latter is preferred (keeps adapter generic).

**Alternatives Considered**:
- Trusting jwk.Cache's internal TTL → rejected: jwk.Cache serves stale data on refresh failure (stale-while-revalidate), which violates FR-010
- Adding TTL tracking inside the adapter → acceptable but couples staleness policy to adapter; publisher-level tracking is cleaner

### RQ-5: How to handle upstream JWKS validation at startup?

**Decision**: Eager fetch during `builder.Build()` — call `GetKeySet()` on the upstream adapter before completing server startup.

**Rationale**: Spec FR-009 requires the broker to fail startup if upstream JWKS cannot be established. The builder already performs eager validation (e.g., checking signing key availability). Adding an eager `GetKeySet()` call with a timeout context ensures the upstream JWKS is reachable and valid before the server starts accepting requests.

**Implementation**:
```go
// In builder.go, after creating upstream JWKS adapter:
startupCtx, cancel := context.WithTimeout(context.Background(), ov.upstreamTimeout)
defer cancel()
if _, err := upstreamJWKSAdapter.GetKeySet(startupCtx); err != nil {
    return nil, fmt.Errorf("failed to establish upstream JWKS at startup: %w", err)
}
```

**Alternatives Considered**:
- Deferring validation to first request → rejected: violates FR-009 (fail at startup)
- Using `WithWaitReady(true)` in cache registration → acceptable but less explicit; the eager fetch approach gives clear error messages

### RQ-6: How to handle duplicate kid detection efficiently?

**Decision**: Check at aggregation time in the publisher service. Build a `map[string]struct{}` of local kids, then iterate upstream keys checking for collisions.

**Rationale**: The check runs on every `PublishJWKS()` call but is cheap:
- Local key set typically has 1-3 keys
- Upstream key set typically has 2-5 keys
- Building a map and iterating is O(n) where n ≤ 10

The publisher returns a specific `ErrKidConflict` error with the conflicting kid value. The handler maps this to HTTP 503.

At startup, the same check runs on the eagerly-fetched upstream keys against the local keys. Conflict at startup → startup failure with clear error message.

**Alternatives Considered**:
- Caching the conflict state → rejected: keys can rotate at any refresh; must check every time
- Checking only at refresh time → rejected: introduces state management; per-request check is cheap enough

### RQ-7: How to modify the discovery metadata to include jwks_uri in all modes?

**Decision**: Modify `AuthorizationService.GenerateMetadata()` to include `jwks_uri` unconditionally (all modes).

**Rationale**: Currently, `GenerateMetadata()` only sets `metadata.JWKSURI` when mode is local or hybrid (line 504 of service.go). The change is a one-line modification: remove the mode conditional around JWKS URI assignment.

```go
// Before:
if mode := s.config.ModeStrategy.Mode(); mode == servermode.Local || mode == servermode.Hybrid {
    metadata.JWKSURI = fmt.Sprintf("%s/oauth2/jwks.json", issuer)
    ...
}

// After:
metadata.JWKSURI = fmt.Sprintf("%s/oauth2/jwks.json", issuer)
if mode := s.config.ModeStrategy.Mode(); mode == servermode.Local || mode == servermode.Hybrid {
    metadata.CodeChallengeMethodsSupported = []string{"S256"}
    ...
}
```

**Alternatives Considered**:
- Adding a separate discovery endpoint per mode → rejected: spec requires single discovery endpoint with mode-independent jwks_uri

### RQ-8: How to wire the JWKS handler unconditionally across all modes?

**Decision**: Always create the JWKS handler in `builder.go`, passing the `JWKSPublisherPort` implementation instead of `SigningKeyManager`. The handler is always non-nil, so routing always registers `/oauth2/jwks.json`.

**Current state**: `jwksHandler` is only set inside `buildLocalProvider()` (called for local/hybrid), leaving it nil for proxy mode. Routing conditionally registers: `if h.JWKS != nil { ... }`.

**New approach**:
1. Create `JWKSPublisherService` outside the mode switch (after mode-specific setup resolves upstream adapter)
2. Pass upstream adapter (nil in local mode) + signing key service (nil in proxy mode) + mode
3. Always assign to `jwksHandler` field
4. Routing registers unconditionally

**Alternatives Considered**:
- Keeping conditional routing and adding a separate proxy-mode JWKS handler → rejected: unnecessary complexity; one handler with mode-dependent publisher is simpler
