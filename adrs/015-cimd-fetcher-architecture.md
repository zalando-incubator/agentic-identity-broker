# ADR 015: CIMD Fetcher Architecture — SSRF-Hardened HTTP Client with In-Process Caching

**Status**: Accepted
**Date**: 2026-04-25

---

## Context

The Client ID Metadata Document (CIMD) feature allows AI agents to identify themselves via an HTTPS URL as `client_id`. The broker must fetch and validate a JSON document at that URL before processing the authorization request.

This creates three architectural concerns:

1. **SSRF Risk**: A malicious agent could supply a URL pointing to internal network resources (169.254.x.x, 10.x.x.x, 127.x.x.x), causing the broker to act as a proxy to its own infrastructure. Standard HTTP clients that resolve hostnames at connect time are vulnerable to DNS rebinding attacks where the hostname resolves to a public IP initially but resolves to a private IP on the actual connection.

2. **Latency**: Fetching a remote document on every authorization request adds network RTT to the critical path. Repeated authorization requests for the same agent would hammer the remote server unnecessarily.

3. **Port/Interface Design**: The fetcher is an outbound infrastructure concern. Per the hexagonal architecture principle, domain logic must not depend on adapters; it must depend on a port interface.

---

## Decision

### Hexagonal Port for the Fetcher

The fetcher is defined as a port interface `CIMDFetcher` in `internal/ports/cimd.go`:

```go
type CIMDFetcher interface {
    Fetch(ctx context.Context, url string) (*CIMDFetchResult, error)
}
```

The domain package `internal/domain/oauth2server/cimd` depends only on this interface. The implementation lives in `internal/adapters/cimd/fetcher.go`.

### SSRF Hardening via Custom Dialer.Control

The fetcher adapter uses a custom `net.Dialer` with a `Control` callback that runs **after DNS resolution but before TCP connect**. This eliminates DNS rebinding vulnerabilities:

```
Hostname → DNS resolve → [Control callback: IP blocklist check] → TCP connect
```

The `SSRFBlocklist` domain value object rejects non-global addresses, RFC 6890 special-purpose ranges, IPv6 transition prefixes (NAT64, Teredo, and 6to4), and operator-configured `extra_blocked_cidrs`. It is consulted in the control callback. If the resolved IP is blocked, the connection is rejected before any byte is sent.

Additionally:
- HTTP redirects are disabled (via an anonymous `CheckRedirect` function that returns `http.ErrUseLastResponse`) — the client_id URL must serve the document directly
- `LimitedReader` caps the response body at `max_response_bytes` (default 5120)
- Request context carries the operator-configured `fetch_timeout` (default 1s)
- Only HTTPS is accepted at the URL validation layer (pre-fetch); HTTP is rejected at `ClientIDMetadataDocumentURL` parse time

### In-Process Cache

A `CIMDCache` domain type (`internal/domain/oauth2server/cimd/cache.go`) provides a `sync.RWMutex`-protected in-memory map. TTL is computed from HTTP response headers (Cache-Control `max-age`, `Expires`) and clamped to operator-configured `[min_ttl, max_ttl]` bounds (defaults: 60s min, 1h max). Entries are lazily evicted on next access after expiry.

The cache lives in the domain layer (not the adapter) because the TTL clamping logic is a business policy, not an infrastructure detail.

### Strategy Pattern for Client Resolution

The `ClientResolver` interface in `internal/ports/cimd.go` is injected into `OAuth2AuthorizationService`. The builder selects the implementation at startup:

- `CIMDClientResolver` (when `cimd.enabled = true`): routes URL-format `client_id` values through CIMD fetch/validate/cache; delegates UUID-format IDs to agent lookup
- `OpaqueClientResolver` (when `cimd.enabled = false`): rejects URL-format `client_id` values with `invalid_client`, resolves UUID-format IDs as before

This keeps the domain service unaware of CIMD mode; the build-time strategy selection is the only switch.

---

## Consequences

**Benefits**:
- TOCTOU-safe SSRF protection — DNS rebinding cannot bypass the blocklist because validation occurs on the resolved IP, not the hostname
- Predictable latency — in-process cache eliminates repeated network round-trips for the same agent
- Domain isolation — `internal/domain/oauth2server/cimd/` imports no infrastructure packages; the fetcher adapter implements the port
- Backward compatibility — when `cimd.enabled = false`, the `OpaqueClientResolver` rejects URL-format IDs and the CIMD adapter is never instantiated

**Trade-offs**:
- In-process cache is not shared across replicas — each instance maintains its own cache. Acceptable for the expected document stability and TTL semantics.
- Cache is lost on restart — first request after restart fetches fresh. Acceptable; document changes are rare and the operator TTL bounds limit stale-document exposure.
- `InsecureSkipVerify` is not used; HTTPS verification is enforced. The test suite uses `httptest.NewTLSServer` with a custom dialer that redirects the fake hostname to the test server address.
