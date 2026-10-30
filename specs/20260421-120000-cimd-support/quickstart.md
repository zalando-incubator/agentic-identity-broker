# Quickstart: CIMD Support Implementation

**Feature**: 20260421-120000-cimd-support | **Date**: 2026-04-23

## Overview

This feature extends the existing OAuth2 authorization flow to support [Client ID Metadata Documents](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-01). No new dependencies are required — all implementation uses Go stdlib and existing project libraries.

## Key Integration Points

### 1. Domain Layer (`internal/domain/cimd/`)

New package with five files:

- **`url.go`**: `ClientIDMetadataDocumentURL` — parse and validate CIMD URLs (scheme, path, fragment, credentials, port, dot-segments). Constructor returns error on invalid input.
- **`document.go`**: `ClientIDMetadataDocument` — parse JSON, validate `client_id` match, auth method, redirect URI same-origin, keyword blocklist. Constructor takes raw JSON + fetch URL.
- **`blocklist.go`**: `SSRFBlocklist` — static RFC 6890 ranges + operator extras. `Contains(net.IP) bool`.
- **`cache.go`**: `Cache` — `sync.RWMutex`-protected `map[string]*CIMDCacheEntry`. `Get(url) (*CIMDCacheEntry, bool)`, `Put(url, entry)`. TTL clamping from HTTP headers.
- **`service.go`**: `Service` — orchestrates: URL validation → cache check → fetch via port → document validation → brand pin check → security field change detection → cache store → return document. Depends on `ports.CIMDFetcher` and `ports.AgentRepository`.

### 2. Port (`internal/ports/cimd.go`)

```go
// ClientResolver is the strategy interface for resolving a client_id to an Agent.
// Builder selects implementation based on cimd.enabled.
type ClientResolver interface {
    ResolveClient(ctx context.Context, clientID id.ClientID) (*ClientResolution, error)
}

type CIMDFetcher interface {
    Fetch(ctx context.Context, url string) (*CIMDFetchResult, error)
}
```

### 3. Adapter (`internal/adapters/cimd/fetcher.go`)

SSRF-hardened HTTP client:
- Custom `net.Dialer.Control` validates resolved IPs against `SSRFBlocklist`
- `http.Client.CheckRedirect` returns error (no redirect following)
- `io.LimitReader` on response body
- Context with `CIMDConfig.FetchTimeout` deadline

### 4. Authorization Flow Integration (`internal/domain/oauth2/service.go`)

The OAuth2 service delegates client resolution to an injected `ClientResolver` strategy:

```go
// In HandleAuthorization — replaces the direct id.ParseAgentID call:
resolution, err := s.clientResolver.ResolveClient(ctx, req.ClientID)
// resolution.Agent is the resolved agent
// resolution.CIMDMetadata is non-nil when the client was resolved via CIMD
```

**Two implementations, wired by builder based on `cimd.enabled`**:
- `OpaqueClientResolver` (`domain/oauth2/client_resolver.go`): Rejects `https://`-prefixed client IDs with `invalid_client`. Parses the rest as UUID agent IDs. This is the default when CIMD is disabled.
- `CIMDClientResolver` (`domain/cimd/client_resolver.go`): Detects `https://` prefix → validates URL → looks up agent by `client_uri` → fetches/validates CIMD document → returns resolution with metadata. Non-URL client IDs fall through to UUID parsing.

### 5. Agent Entity Extension (`internal/domain/storage/agent.go`)

Add fields: `ClientURIs []string`, `AuthMethod *string`, `JwksURI *string`. Update `Validate()`, `ValidateForCreate()`, `Copy()`.

### 6. Storage Adapters

**New repository method**: `GetByClientURI(ctx, uri string) (*Agent, error)` — exact match lookup against the `agent_client_uris` child table.

- **Memory**: secondary index `map[string]id.AgentID` built on create/update; uniqueness enforced at write time; ClientURIs hydrated from the index on every read
- **Postgres**: `SELECT agent_id FROM agent_client_uris WHERE client_uri = $1`; Create and Update run inside a single transaction — parent agents row and child agent_client_uris rows are written atomically so a UNIQUE violation rolls back the whole operation with no partial state; Get, List, GetByClientID, and GetByClientURI all JOIN or sub-SELECT against agent_client_uris to hydrate ClientURIs

### 7. Admin Handler (`internal/adapters/http/handlers/admin/agents_handler.go`)

- Extend `AgentRequest`/`AgentResponse` DTOs with `ClientURIs`, `AuthMethod`, `JwksURI`
- No new endpoints — `client_uris` is accepted on existing POST (create) and PUT (update) handlers

### 8. Consent API Extension

The authorization server redirects to the consent page with an opaque `?session_token=<jwe>` query parameter. The consent page calls `GET /api/consent/session?token=<jwe>` to retrieve structured CIMD display data (client name, verified domain, redirect URI, scopes, logo). The consent submission body includes the `session_token`; the backend decrypts it, validates `exp` and `principal`, and uses the sealed `redirect_uri`/`state`/`code_challenge` to issue the authorization code redirect. The frontend never constructs or supplies these values.

### 9. Frontend Components (`web/src/components/consent/`)

Four new components using the existing design system:
- `CIMDConsentSummary` — "The application [name] wants to access [target]" (CS-001)
- `CIMDDomainBadge` — "Verified domain: [hostname]" badge (CS-002)
- `CIMDLocalhostWarning` — prominent warning for localhost redirects (CS-003)
- `CIMDAdvancedDetails` — collapsible section with Client Name, Client ID, Redirect URI, Scopes (CS-004)

### 10. Configuration (`internal/ports/config.go`)

Add `CIMD CIMDConfig` to `OAuth2AuthServerConfig`. Register Viper defaults. Add validation in `Validate()`.

### 11. Builder Wiring (`internal/app/builder.go`)

```go
// In Build() — construct shared JWE token service (used by both OAuth2SessionService and OAuth2AuthorizationService):
jweTokenService := jwe.New(jwsKey)

// Select ClientResolver strategy based on cimd.enabled:
var clientResolver ports.ClientResolver
if config.OAuth2AuthServer.CIMD.Enabled {
    ssrfBlocklist := cimd.NewSSRFBlocklist(config.OAuth2AuthServer.CIMD.SSRF.ExtraBlockedCIDRs)
    cimdFetcher := cimdadapter.NewFetcher(config.OAuth2AuthServer.CIMD, ssrfBlocklist)
    cimdCache := cimd.NewCache()
    cimdService := cimd.NewService(cimdFetcher, cimdCache, config.OAuth2AuthServer.CIMD, logger)
    clientResolver = cimd.NewCIMDClientResolver(cimdService, agentRepo)
} else {
    clientResolver = oauth2.NewOpaqueClientResolver(agentRepo)
}
// Pass clientResolver and jweTokenService to OAuth2Service — no CIMD infrastructure instantiated when disabled
```

## Implementation Order

1. **Phase 2.7**: Agent entity extension (migration 015, storage adapters, admin handler DTOs) — structural scaffold
2. **Phase 2.5**: CIMD domain types (URL, document, blocklist) + port interface + fetcher adapter
3. **Phase 3**: CIMD service + authorization flow integration (User Story 1)
4. **Phase 4**: SSRF hardening tests (User Story 2 — mostly covered by fetcher adapter, but dedicated E2E tests)
5. **Phase 5**: Consent screen components (User Story 5)
6. **Phase 6**: Caching with HTTP semantics (User Story 3)
7. **Phase 7**: Metadata advertisement (User Story 4)

## Testing Approach

- **Unit tests**: Each domain type in `internal/domain/cimd/` gets comprehensive table-driven tests
- **Integration tests**: Fetcher adapter against `httptest.NewTLSServer` with injected resolver/dialer and dial-attempt spy, Postgres adapter with new columns
- **E2E tests**: Full authorization flow with fake public hostnames (for example `agent.example.test`) routed to an in-process HTTPS CIMD server through the fetcher resolver/dialer seam, split by user story
- **Frontend Playwright**: Consent screen rendering with CIMD metadata
