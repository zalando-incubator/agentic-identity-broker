# Quickstart: Broker-Hosted Aggregated JWKS

**Branch**: `20260604-120000-aggregated-jwks` | **Date**: 2026-06-04

## Implementation Order

### Phase 2: Design Preconditions

1. **Update OpenAPI spec** (`api/enduser/openapi.yaml`)
   - Update `/oauth2/jwks.json` description to reflect mode-dependent content
   - Add 503 response documentation
   - Update `/.well-known/oauth-authorization-server` to show `jwks_uri` is always present
   - Reference: `specs/20260604-120000-aggregated-jwks/contracts/jwks-endpoint.yaml`

2. **Write E2E tests** (`tests/e2e/aggregated_jwks_test.go`)
   - 20 `It()` blocks mapping 1:1 to spec scenarios
   - Use `helpers.NewMockUpstreamJWKS()` for upstream simulation
   - All tests must compile and fail semantically (red phase)

3. **Update ARCHITECTURE.md glossary**
   - Add: `AggregatedKeySet`, `KeySource`, `JWKSPublisher`

### Phase 2.5: New Port + Domain Service Skeleton

4. **Create port** (`internal/ports/jwks_publisher.go`)
   ```go
   type JWKSPublisherPort interface {
       PublishJWKS(ctx context.Context) (jwk.Set, error)
   }
   ```

5. **Create domain service** (`internal/domain/jwkspublisher/service.go`)
   - Constructor: `NewService(mode, localKeys, upstreamKeys, logger)`
   - Method: `PublishJWKS(ctx) (jwk.Set, error)`
   - Sentinel errors: `ErrUpstreamUnavailable`, `ErrKidConflict`

6. **Write unit tests first** (`internal/domain/jwkspublisher/service_test.go`)
   - Table-driven tests for all modes and error cases
   - Hand-rolled mocks for `SigningKeyManager` and `JWKSPort`

### Phase 3: User Stories 1 + 2 (Core Aggregation + Discovery)

7. **Implement `JWKSPublisherService`** — mode-based logic:
   - `local`: call `localKeys.BuildJWKS(ctx)`, return result
   - `proxy`: call `upstreamKeys.GetKeySet(ctx)`, return result
   - `hybrid`: call both, merge into single `jwk.Set`

8. **Modify JWKS handler** (`internal/adapters/http/handlers/enduser/jwks_handler.go`)
   - Change dependency from `ports.SigningKeyManager` to `ports.JWKSPublisherPort`
   - Map `ErrUpstreamUnavailable` / `ErrKidConflict` → HTTP 503

9. **Modify discovery metadata** (`internal/domain/oauth2/service.go`)
   - Set `metadata.JWKSURI` unconditionally (remove mode guard)
   - Keep `CodeChallengeMethodsSupported` conditional on local/hybrid

10. **Wire in builder** (`internal/app/builder.go`)
    - Create upstream JWKS adapter in proxy/hybrid modes (reuse existing pattern)
    - Create `JWKSPublisherService` with mode-appropriate sources
    - Always set `jwksHandler` (no longer nil in proxy mode)

11. **Update routing** (`internal/adapters/http/routing/enduser.go`)
    - Remove conditional: always register `/oauth2/jwks.json`
    - Update comment to reflect "all modes"

### Phase 4: User Story 3 (Fail-Closed Availability)

12. **Add startup validation in builder**
    - After creating upstream adapter, eagerly fetch `GetKeySet(ctx)` with timeout
    - Startup fails with clear error if unreachable

13. **Add freshness tracking to publisher service**
    - Track last successful upstream fetch timestamp
    - If staleness exceeds threshold (2× cache max-age = 600s), return `ErrUpstreamUnavailable`
    - Alternative: rely on adapter error propagation if cache truly fails

### Phase 5: User Story 4 (Duplicate Kid Detection)

14. **Implement kid conflict check in publisher** (hybrid mode only)
    - Build `map[string]struct{}` from local kid values
    - Iterate upstream keys; if kid exists in map → return `ErrKidConflict` with kid in log
    - Log conflict with specific kid value for operator visibility

15. **Add startup kid conflict check in builder**
    - After eager upstream fetch + local key availability check
    - Compare kid values; conflict → startup failure with clear error

### Phase N: Constitution Compliance

16. **Verify all E2E tests pass** — 20 scenarios green
17. **Run `just check`** — static analysis clean
18. **Run `just verify`** — full verification gate
19. **Update existing E2E tests** that assert proxy-mode JWKS returns 404
    - `tests/e2e/oauth2_discovery_e2e_test.go` "discovery 404 in proxy mode" → update expectations

## Key Integration Points

| Component | File | Change Type |
|-----------|------|-------------|
| JWKSPublisherPort | `internal/ports/jwks_publisher.go` | NEW |
| JWKSPublisherService | `internal/domain/jwkspublisher/service.go` | NEW |
| JWKSHandler | `internal/adapters/http/handlers/enduser/jwks_handler.go` | MODIFIED |
| OAuth2 Metadata | `internal/domain/oauth2/service.go` | MODIFIED |
| Builder | `internal/app/builder.go` | MODIFIED |
| Routing | `internal/adapters/http/routing/enduser.go` | MODIFIED |
| E2E Tests | `tests/e2e/aggregated_jwks_test.go` | NEW |
| E2E Discovery Test | `tests/e2e/oauth2_discovery_e2e_test.go` | MODIFIED |
| OpenAPI | `api/enduser/openapi.yaml` | MODIFIED |
| Architecture | `ARCHITECTURE.md` | MODIFIED (glossary) |

## Common Pitfalls

1. **Don't share signing key service nil-check**: In proxy mode, `localKeys` is nil. The publisher must handle this gracefully (never call `BuildJWKS` in proxy mode).
2. **Don't modify upstream keys**: FR-006 requires upstream keys republished without modification. Use `jwk.Set` directly — no key transformation.
3. **Don't forget existing test updates**: The test "discovery 404 in proxy mode" expects JWKS to return 404 in proxy mode. This must be updated to expect 200 with upstream keys.
4. **Don't block on upstream refresh**: The handler must not hang waiting for upstream refresh. Use context timeout from the HTTP request.
5. **Don't expose the staleness logic to the handler**: The publisher service owns the "is stale?" decision. The handler just sees success or `ErrUpstreamUnavailable`.
