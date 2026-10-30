# Research: CIMD Support

**Feature**: 20260421-120000-cimd-support | **Date**: 2026-04-23

## R-001: CIMD URL Detection in Authorization Flow — Client Resolver Strategy

**Decision**: Extract client resolution into a strategy interface (`ClientResolver`) injected into the OAuth2 domain service. Two implementations: `OpaqueClientResolver` (existing UUID-based lookup) and `CIMDClientResolver` (URL detection → CIMD fetch/validate → fallback to UUID). The builder selects the implementation based on `cimd.enabled`.

**Rationale**: The codebase uses strategy interfaces injected via the builder for all behavioral branching (see `AuthorizationProceedStrategy`, `TokenGrantStrategy`, `TokenMintingStrategy`). The domain service is mode-agnostic — it delegates to strategies. CIMD support follows this same pattern: the domain service calls `clientResolver.ResolveClient(ctx, clientID)` which returns the resolved Agent and optional CIMD metadata. When `cimd.enabled` is `false`, the `OpaqueClientResolver` is wired and URL-format client IDs are rejected immediately with `invalid_client` — the CIMD code path is never reachable because the strategy implementation that handles URLs does not exist in the call graph. When `cimd.enabled` is `true`, the `CIMDClientResolver` checks for `https://` prefix, routes to CIMD resolution, and falls back to UUID for non-URL client IDs.

This makes the enabled/disabled gate structural rather than conditional: the builder composes different behavior, not a runtime `if cimd.enabled` check. The domain service has no awareness of CIMD enablement.

**Alternatives considered**:
- Inline `if strings.HasPrefix(clientID, "https://")` in `HandleAuthorization` — rejected because it introduces a runtime conditional in the domain layer, violates the established strategy pattern, and makes the CIMD-disabled guarantee harder to reason about
- Attempt UUID parse first, fall back to CIMD on error — rejected because it obscures intent; a URL is never a valid UUID
- Create a separate authorization endpoint for CIMD clients — rejected because the spec says CIMD is transparent to the client (API-002)

## R-002: SSRF-Hardened HTTP Client Architecture

**Decision**: Custom `net.Dialer.Control` function that validates resolved IP addresses against the blocklist before TCP connect. Use Go stdlib `net/http` with `CheckRedirect` returning error (FR-005), `io.LimitReader` for body size (FR-007), and context timeout for total request (FR-008).

**Rationale**: Go's `net.Dialer` supports a `Control` callback invoked after DNS resolution but before TCP connect — the exact TOCTOU-safe interception point specified in SR-002. No external library needed; stdlib `net` provides `net.IP.IsLoopback()`, `net.IP.IsPrivate()`, `net.IP.IsLinkLocalUnicast()`, and CIDR matching via `net.IPNet.Contains()`. The RFC 6890 Special-Purpose Address Registry ranges can be encoded as a static `[]net.IPNet` slice.

**Alternatives considered**:
- Use a third-party SSRF protection library — rejected per user input ("dependency updates or new dependencies should not be required")
- Validate DNS at URL parse time instead of at connect time — rejected because SR-002 explicitly requires TOCTOU protection (resolve → validate → connect to validated address)

## R-003: In-Process Cache Design

**Decision**: `sync.RWMutex`-protected `map[string]*CIMDCacheEntry` keyed by URL string. TTL computed from HTTP `Cache-Control` / `Expires` headers, clamped to operator `min_ttl` / `max_ttl`. No singleflight (per spec clarification session 2026-04-23).

**Rationale**: The spec explicitly states "no singleflight coordination required" and "cache is in-process memory only". A simple mutex-protected map is sufficient for the expected load pattern (one CIMD URL per agent, fetched at most once per `min_ttl`). Cache entries store the parsed document, ETag, and expiry timestamp. Expired entries are lazily evicted on next access.

**Alternatives considered**:
- `sync.Map` — rejected because it optimizes for high-read/low-write with stable keys, but CIMD cache has mixed read/write patterns and needs TTL eviction logic that sync.Map doesn't support
- LRU cache with max size — considered but deferred; the spec says "cache eviction under memory pressure" is treated as expiry, so a simple map with lazy eviction is sufficient for now

## R-004: Agent Entity Extension Strategy

**Decision**: Add two nullable columns to `agents` table (`auth_method`, `jwks_uri`) and a normalized `agent_client_uris` child table (`agent_id UUID REFERENCES agents(id) ON DELETE CASCADE`, `client_uri TEXT NOT NULL`, `UNIQUE(client_uri)`).

**Rationale**: The `UNIQUE(client_uri)` constraint on the child table is the only mechanism that can guarantee global uniqueness of Client ID Metadata Document URLs under concurrent writes. An application-level transactional check (e.g. SELECT-then-INSERT) is subject to TOCTOU races; a GIN index on a TEXT[] array column is not a uniqueness constraint. A normalized table with a database-level unique constraint eliminates the race without requiring serializable isolation and makes `GetByClientURI` a simple primary-key-style lookup (`SELECT agent_id FROM agent_client_uris WHERE client_uri = $1`). The `auth_method` and `jwks_uri` snapshot fields remain on the agents table because they are scalar, per-agent values with no uniqueness requirement.

**Alternatives considered**:
- TEXT[] column + GIN index + application-level transactional check — rejected because the GIN index is not a uniqueness constraint; concurrent writes can still produce duplicates, breaking deterministic resolution and the documented 409 Conflict behavior
- Separate `cimd_snapshots` table — rejected because the spec says "the stored Agent state IS the baseline" (Clarifications session 2026-04-22)

## R-005: Admin API for Client URI Management

**Decision**: Add `client_uris` field to the existing `AgentRequest` DTO used by both `POST /api/agents` (create) and `PUT /api/agents/{agent-id}` (update). No separate PATCH endpoint.

**Rationale**: The existing admin API uses POST for creation and PUT for full replacement updates. API-004 (clarified 2026-04-23) specifies that `client_uris` is managed through these existing endpoints. Adding `client_uris` as an optional field (defaults to `[]`) to the existing `AgentRequest` DTO is consistent with the established API style and requires no new endpoint or handler pattern. Each URI is validated at write time as a well-formed HTTPS URL.

**Alternatives considered**:
- Dedicated PATCH endpoint — rejected because POST/PUT already exist and the codebase has no PATCH pattern; adding one for a single array field introduces unnecessary API surface

## R-006: Consent Screen CIMD Data Flow

**Decision**: Extend the consent API response (`GET /api/consent/agent/:agent-id`) with CIMD-specific fields when the authorization request originates from a CIMD-based client. Add `cimdMetadata` object to the response containing `clientName`, `clientID` (URL), `redirectURI`, `verifiedDomain`, `isLocalhostRedirect`, `requestedScopes`.

**Rationale**: The consent page currently receives agent data via `consent.AgentDetail`. For CIMD clients, additional metadata from the fetched document must reach the UI. Rather than changing the agent detail endpoint, the CIMD metadata is attached to the authorization/consent session context and exposed through the existing consent API. The frontend conditionally renders CIMD-specific components (CS-001–CS-004) when `cimdMetadata` is present.

**Alternatives considered**:
- Separate CIMD metadata endpoint — rejected because the consent page already loads agent detail in one call; adding another round-trip degrades UX
- Store CIMD metadata on the Agent entity — rejected because CIMD metadata is per-authorization-request (fetched fresh or from cache), not a persisted agent property

## R-007: Brand Pin and Keyword Blocklist Implementation

**Decision**: Brand pin comparison is case-sensitive string equality between `Agent.DisplayName` and CIMD `client_name` (FR-023a). Keyword blocklist uses case-insensitive exact string match (FR-023b/c). Default blocklist includes: "admin", "system", "operator", "administrator", "root", and the display names of configured identity providers.

**Rationale**: The spec says brand mismatch is logged but not blocking (FR-023a), while keyword blocklist match causes rejection (FR-023b). Case-insensitive for blocklist prevents trivial evasion ("Admin" vs "admin"). The built-in default set covers the most obvious impersonation targets; operators can extend via `client_name_blocklist` config.

## R-008: Redirect URI Same-Origin Validation

**Decision**: For CIMD clients, each `redirect_uri` in the document must be same-origin with the `client_id` URL (same scheme, host, port), with an explicit exception for `localhost` and `127.0.0.1` on any port (FR-004a). This is a CIMD-specific validation layered on top of the existing redirect URI check (FR-004).

**Rationale**: The spec is explicit about same-origin with localhost exception. This prevents cross-origin redirect attacks while supporting locally-running agent tooling. The validation runs during CIMD document validation, before the document is cached.

## R-009: Package Placement — `domain/cimd/` vs `domain/oauth2/`

**Decision**: New `domain/cimd/` package for CIMD domain types and service logic. Fetcher port in `ports/cimd.go`. Adapter in `adapters/cimd/`.

**Rationale**: The codebase organizes domain packages by bounded context (`oauth2/`, `oauth2session/`, `consent/`, `thirdparty/`, `tokenexchange/`). CIMD is a distinct sub-concern: document parsing, URL validation, SSRF blocklist management, and in-process caching. Merging into `domain/oauth2/` would roughly double that package's surface area and mix two responsibilities (authorization flow orchestration vs. external document fetching/validation). The `oauth2session/` package is a direct precedent — it's a sub-concern of OAuth2 that lives in its own package because it has its own domain types and lifecycle.

The `ports/cimd.go` file follows the one-file-per-concern pattern established by `ports/jwks.go` and `ports/cel.go`. The `CIMDFetcher` interface is structurally analogous to `JWKSPort` — both abstract an external HTTP fetch with caching behind a hexagonal boundary. The adapter in `adapters/cimd/` mirrors `adapters/jwks/` and reuses the existing `upstream.NewSecureUpstreamClient()` for TLS-hardened HTTP transport.

**Alternatives considered**:
- Merge into `domain/oauth2/` — rejected because `oauth2/` currently contains only authorization orchestration (service + errors + multi-agent verifier, ~4 files). Adding 5 CIMD files with distinct concerns (URL parsing, document validation, SSRF blocklist, caching, CIMD service) would dilute the package's focus
- Place fetcher port in `ports/oauth2.go` — rejected because `ports/oauth2.go` defines the `OAuth2Service` consumer interface and DTOs; the `CIMDFetcher` is a driven (outbound) port for infrastructure, not a consumer interface. Mixing driven and driving ports in one file obscures the architectural boundary

## R-010: E2E Strategy for Non-Localhost CIMD URLs

**Decision**: E2E tests use fake public hostnames such as `agent.example.test` in the `client_id` URL and inject a resolver/dialer mapping in the CIMD fetcher adapter so those hostnames connect to an in-process `httptest.NewTLSServer`.

**Rationale**: Happy-path CIMD tests must exercise production-like URLs, not localhost client IDs, because the feature is defined around externally hosted HTTPS metadata documents. `httptest.NewTLSServer` provides a real HTTPS endpoint, but its native localhost URL cannot be used directly as the `client_id`. The resolver/dialer seam lets tests preserve a public-looking URL (`https://agent.example.test/client`) while routing the actual TCP connection to the in-process test server. This also preserves port-443 validation semantics because the logical request URL remains unchanged.

The same seam provides a clean test boundary for SSRF enforcement. Adapter tests can inject blocked resolutions and use a dial-attempt spy to prove `no dial attempted` for blocked IP categories. E2E tests then focus on observable behavior: authorization rejection/success, consent rendering, and request counts against the CIMD server.

**Alternatives considered**:
- Use localhost URLs in E2E — rejected because it does not match the production client identity model and conflicts with the spec's port/scheme/host expectations
- Bind a real TLS server to `127.0.0.1:443` — rejected because it is fragile, platform-dependent, and inappropriate for normal test execution
- Assert `no outbound connection` directly in E2E without a seam — rejected because it is not reliably observable from the black-box system boundary

## R-011: Structural CIMD Gate — Enabled/Disabled via Strategy Selection

**Decision**: The `cimd.enabled` configuration controls which `ClientResolver` implementation the builder wires into the OAuth2 service. When `false`, the `OpaqueClientResolver` is wired — it rejects any `client_id` starting with `https://` with `invalid_client` immediately. When `true`, the `CIMDClientResolver` is wired — it handles URL-format client IDs via the CIMD fetch/validate/cache path and delegates non-URL client IDs to opaque UUID resolution.

**Rationale**: This makes the CIMD gate structural, not conditional. When CIMD is disabled:
- No CIMD-related code is in the call graph (the `CIMDClientResolver`, `CIMDService`, `CIMDFetcher`, and cache are never instantiated)
- URL-format client IDs are rejected at the strategy boundary with a clear error, not silently parsed and failed deep in the stack
- The guarantee is trivially testable: E2E tests with `cimd.enabled: false` send a URL-format `client_id` and assert `invalid_client` rejection

This follows the established pattern: `builder.go` selects strategy implementations based on config, handlers/services are unaware of enablement state.

**Alternatives considered**:
- Runtime `if cimd.enabled` guard in `HandleAuthorization` — rejected because it breaks the strategy pattern, pollutes the domain service with feature flag logic, and requires the CIMD infrastructure to always be instantiated (even when disabled)
