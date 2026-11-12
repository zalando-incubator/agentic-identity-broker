# Data Model: CIMD Support

**Feature**: 028-cimd-support | **Date**: 2026-04-23

## Entity Extensions

### Agent (existing entity — extended)

| Field | Type | New? | Description |
|-------|------|------|-------------|
| `ID` | `id.AgentID` (UUID) | No | Primary key |
| `ClientID` | `id.ClientID` (string) | No | Opaque client identifier |
| `DisplayName` | `string` | No | Operator-set display name; brand pin baseline |
| `ClientURIs` | `[]string` | **Yes** | Pre-registered Client ID Metadata Document URLs |
| *(all existing fields preserved)* | | | |

**Validation rules**:
- `ClientURIs`: each entry must be a valid HTTPS URL (validated via `ClientIDMetadataDocumentURL.Parse()`)
- `ClientURIs`: must be globally unique across all agents (no two agents share a Client URI)

**Database migration** (015):
```sql
-- UP
CREATE TABLE agent_client_uris (
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    client_uri TEXT NOT NULL,
    UNIQUE(client_uri)
);

-- DOWN
DROP TABLE agent_client_uris;
```

No `authorization_sessions` table is required; session state is carried in the stateless JWE `session_token` (see `AuthorizationSessionClaims` below).

**Uniqueness**: `UNIQUE(client_uri)` on the child table is the sole enforcement mechanism. No two agents may share a Client ID Metadata Document URL; the constraint violation maps to a 409 Conflict response on admin writes.

## New Domain Types

### ClientIDMetadataDocument (value object)

Represents a parsed and validated CIMD JSON document. Immutable after construction.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ClientID` | `string` | Yes | Must exactly match the fetch URL (FR-003, SR-008) |
| `ClientName` | `string` | No | Display name for consent screen (FR-023) |
| `LogoURI` | `string` | No | Logo URL (displayed as-is, not prefetched) |
| `RedirectURIs` | `[]string` | Yes | Allowed redirect URIs (FR-004); empty = rejection |
| `AuthMethod` | `string` | No | `token_endpoint_auth_method`; defaults to `"none"` |
| `GrantTypes` | `[]string` | No | Supported grant types |
| `ResponseTypes` | `[]string` | No | Supported response types |
| `JwksURI` | `string` | No | JWKS endpoint for client authentication |
| `PolicyURI` | `string` | No | Privacy policy URL |
| `TosURI` | `string` | No | Terms of service URL |

**Validation rules** (applied at parse or resolution time as indicated):
- `ClientID` must exactly match the URL the document was fetched from (byte-for-byte, SR-008)
- `RedirectURIs` must not be empty (edge case: "omits redirect_uris" → rejection)
- `AuthMethod` must not be `client_secret_post`, `client_secret_basic`, or `client_secret_jwt` (FR-022)
- `ClientName` must not match any entry in the keyword blocklist (FR-023b, case-insensitive)
- Each `RedirectURI` must be same-origin with the `ClientID` URL, use a `localhost`/`127.0.0.1`/`::1` host, or be a valid private-use scheme URI that the resolved Agent's `redirect_uris` lists byte for byte (FR-004a; checked on every resolution by `ValidatePrivateUseRedirectURIs`)

**Package**: `internal/domain/cimd/document.go`

### ClientIDMetadataDocumentURL (value object)

Validated HTTPS URL used as a `client_id`. Validated at parse time — invalid URLs cannot be constructed.

| Validation | Rule | Requirement |
|------------|------|-------------|
| Scheme | Must be `https` | FR-014 |
| Path | Must have a non-empty path component | FR-014a |
| Path | No `.` or `..` segments | FR-015 |
| Fragment | Must not contain `#` | FR-016 |
| Userinfo | Must not contain credentials | FR-017 |
| Port | Must be 443 or absent | FR-018 |

**Package**: `internal/domain/cimd/url.go`

### SSRFBlocklist (value object)

Immutable set of CIDR ranges blocked for CIMD fetches. Built at startup from defaults + operator config.

| Range | Category | Requirement |
|-------|----------|-------------|
| `127.0.0.0/8` | Loopback (IPv4) | FR-010 |
| `::1/128` | Loopback (IPv6) | FR-010 |
| `10.0.0.0/8` | Private (RFC 1918) | FR-011 |
| `172.16.0.0/12` | Private (RFC 1918) | FR-011 |
| `192.168.0.0/16` | Private (RFC 1918) | FR-011 |
| `169.254.0.0/16` | Link-local (IPv4) | FR-012 |
| `fe80::/10` | Link-local (IPv6) | FR-012 |
| `0.0.0.0/8` | "This" network | FR-013 |
| `100.64.0.0/10` | Shared address (CGN) | FR-013 |
| `192.0.0.0/24` | IETF protocol assignments | FR-013 |
| `192.0.2.0/24` | Documentation (TEST-NET-1) | FR-013 |
| `198.18.0.0/15` | Benchmarking | FR-013 |
| `198.51.100.0/24` | Documentation (TEST-NET-2) | FR-013 |
| `203.0.113.0/24` | Documentation (TEST-NET-3) | FR-013 |
| `240.0.0.0/4` | Reserved | FR-013 |
| `255.255.255.255/32` | Limited broadcast | FR-013 |
| `fc00::/7` | Unique local (IPv6) | FR-013 |
| `2001:db8::/32` | Documentation (IPv6) | FR-013 |
| Operator `extra_blocked_cidrs` | Custom | FR-024 |

**Package**: `internal/domain/cimd/blocklist.go`

### CIMDCacheEntry (in-process, not persisted)

| Field | Type | Description |
|-------|------|-------------|
| `URL` | `string` | Cache key (the Client ID Metadata Document URL) |
| `Document` | `*ClientIDMetadataDocument` | Parsed and validated document |
| `FetchedAt` | `time.Time` | When the document was last fetched |
| `ExpiresAt` | `time.Time` | Computed from HTTP cache headers + operator TTL bounds |
| `ETag` | `string` | HTTP ETag for conditional requests (future optimization) |

**Package**: `internal/domain/cimd/cache.go`

### AuthorizationSessionClaims (value object — sealed in JWE)

Claims sealed into the JWE `session_token` at authorization initiation. Stateless — no database storage. Decrypted by the broker on demand.

| Field | Type | Description |
|-------|------|-------------|
| `AgentID` | `id.AgentID` | Resolved agent (validated at consent-submission time) |
| `Principal` | `id.Principal` | Authenticated user; must match at submission |
| `ClientID` | `string` | The `client_id` value from the authorization request |
| `OriginalURL` | `string` | Full `/authorize` URL; redirect target after consent |
| `RedirectURI` | `string` | Trusted redirect URI (from CIMD document) |
| `Scope` | `string` | Requested OAuth2 scopes |
| `State` | `string` | Agent-supplied opaque state value |
| `CodeChallenge` | `string` | PKCE code challenge |
| `CodeChallengeMethod` | `string` | PKCE method (`S256`) |
| `CIMDMetadata` | `*storage.CIMDMetadataSnapshot` | Resolved CIMD display fields |
| `IssuedAt` | `time.Time` | Token mint time |
| `ExpiresAt` | `time.Time` | 10-minute TTL; `exp` claim |

**Package**: `internal/domain/oauth2/authorization_session_token.go`

**Encryption**: A256GCMKW key wrapping + A256GCM content encryption via `internal/domain/jwe/TokenService`. Key: `IDENTITY_BROKER_JWE_SIGNING_KEY` (same key used for `OAuth2StateToken`).

## New Port Interfaces

### ClientResolver (strategy interface)

```go
// ClientResolver resolves a client_id from an authorization request to an Agent
// and optional CIMD metadata. The builder selects the implementation based on
// cimd.enabled configuration.
//
// When CIMD is disabled: OpaqueClientResolver is wired — rejects URL-format
// client_id values with invalid_client immediately.
//
// When CIMD is enabled: CIMDClientResolver is wired — handles URL-format
// client_id via CIMD fetch/validate/cache, delegates non-URL client_id to
// opaque UUID resolution.
type ClientResolver interface {
    ResolveClient(ctx context.Context, clientID id.ClientID) (*ClientResolution, error)
}

type ClientResolution struct {
    Agent        *storage.Agent
    CIMDMetadata *cimd.ClientIDMetadataDocument // nil for opaque client_id
}
```

**Implementations**:
- `OpaqueClientResolver` in `internal/domain/oauth2/client_resolver.go` — existing UUID-based agent lookup. Rejects any `client_id` starting with `https://` with `invalid_client` error
- `CIMDClientResolver` in `internal/domain/cimd/client_resolver.go` — URL detection → CIMD fetch/validate/cache → agent lookup by client URI. Falls back to opaque UUID lookup for non-URL client IDs

**Package**: `internal/ports/cimd.go`

### CIMDFetcher (port)

```go
// CIMDFetcher fetches Client ID Metadata Documents from remote HTTPS endpoints
// with SSRF protection, timeout, and size limits.
type CIMDFetcher interface {
    Fetch(ctx context.Context, url string) (*CIMDFetchResult, error)
}

type CIMDFetchResult struct {
    Document    *cimd.ClientIDMetadataDocument
    CacheControl string   // raw Cache-Control header
    ETag         string   // ETag header value
    Expires      string   // Expires header value
}
```

**Package**: `internal/ports/cimd.go`

## Configuration Types

### CIMDConfig

```go
type CIMDConfig struct {
    Enabled          bool          `mapstructure:"enabled"`
    FetchTimeout     time.Duration `mapstructure:"fetch_timeout"`
    MaxResponseBytes int           `mapstructure:"max_response_bytes"`
    Cache            CIMDCacheConfig `mapstructure:"cache"`
    SSRF             CIMDSSRFConfig  `mapstructure:"ssrf"`
    ClientNameBlocklist []string    `mapstructure:"client_name_blocklist"`
}

type CIMDCacheConfig struct {
    MaxTTL time.Duration `mapstructure:"max_ttl"`
    MinTTL time.Duration `mapstructure:"min_ttl"`
}

type CIMDSSRFConfig struct {
    ExtraBlockedCIDRs []string `mapstructure:"extra_blocked_cidrs"`
}
```

**Defaults**: `enabled: false`, `fetch_timeout: 1s`, `max_response_bytes: 5120`, `cache.max_ttl: 1h`, `cache.min_ttl: 60s`, `ssrf.extra_blocked_cidrs: []`, `client_name_blocklist: []`

**Location**: Added to `OAuth2AuthServerConfig` in `internal/ports/config.go`

## Domain Events (structured audit logs)

| Event | Fields | Trigger |
|-------|--------|---------|
| `CIMDDocumentFetched` | URL, cache TTL, client_name | Successful fetch + validation |
| `CIMDFetchBlocked` | URL, reason (blocked IP, invalid scheme, etc.) | SSRF guard rejection |
| `CIMDFetchFailed` | URL, reason (non-200, timeout, oversized) | Fetch error |
| `BrandPinMismatchDetected` | AgentID, Agent.DisplayName, CIMD client_name | client_name differs from DisplayName |

Events are emitted via structured logging (existing `slog` logger), not a separate event bus.

## Relationships

```
Agent 1──* ClientIDMetadataDocumentURL (pre-registered, stored in agent_client_uris child table)
CIMDCacheEntry *──1 ClientIDMetadataDocument (in-process only)
```

## State Transitions

**CIMD Authorization Flow State Machine**:

```
[URL received] → validate URL format → [valid URL]
    → check pre-registration (GetByClientURI) → [agent found]
    → check cache → [cache hit: use cached doc] OR [cache miss: fetch]
    → SSRF validate resolved IPs → [IPs safe]
    → HTTP GET (timeout + size limit) → [200 OK, valid JSON]
    → validate document (client_id match, auth method, redirect URIs, blocklist)
    → compare brand pin (client_name vs Agent.DisplayName) → emit audit event on mismatch
    → mint JWE session_token (AuthorizationSessionClaims + 10-min exp)
    → redirect to consent page with ?session_token=<jwe> only
    → consent page calls GET /api/consent/session?token=<jwe> → broker decrypts + validates exp
    → user submits consent with session_token → broker decrypts, validates exp + principal
    → issue authorization code using trusted redirect_uri/state/code_challenge from token
```

Any failure at any step → reject authorization request (fail closed).
