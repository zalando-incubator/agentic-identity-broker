# Data Model: Envoy ExtProc Token Exchange Service

**Branch**: `015-extproc-token-exchange`  
**Date**: 2026-02-23
> **Implementation note (superseded raw input):**
> [Feature 043](../043-extproc-metadata-input/contracts/extproc-metadata-input.md) and [ADR 036](../../adrs/036-extproc-metadata-token-exchange-input.md) replace raw `Authorization` and pseudo-header input, no-Bearer pass-through, and raw resource validation.
> This document retains the standalone process, configuration, cache, circuit-breaker, and exchange mechanics that remain applicable.


## Overview

This feature introduces a **standalone application** that does not extend the identity broker's domain model. Instead, it defines its own internal types for the ExtProc gRPC service. No new database entities or migrations are required.

## Configuration Schema

The ExtProc service uses the same configuration infrastructure (Viper/Cobra) but with a completely separate schema under env prefix `EXTPROC_`.

### Config (Root)

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `grpc` | GRPCConfig | Yes | - | gRPC server settings |
| `oauth2` | OAuth2Config | Yes | - | OAuth2/token exchange settings |
| `cache` | CacheConfig | Yes | - | Token cache settings |
| `log` | LogConfig | No | level: info, format: text | Logging settings |

### GRPCConfig

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `bind` | string | No | `0.0.0.0` | gRPC listen address |
| `port` | int | No | `50051` | gRPC listen port |
| `max_concurrent_streams` | int | No | `100` | Maximum concurrent gRPC streams |

### OAuth2Config

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `token_endpoint` | string (URL) | Yes | - | Identity broker token exchange endpoint (`/oauth2/token`) |
| `issuer` | string (URL) | Yes | - | OAuth2 authorization server issuer |
| `client_id` | string | Yes | - | Client ID for obtaining client assertion |
| `client_secret` | string | Yes | - | Client secret for obtaining client assertion |
| `client_credentials_endpoint` | string (URL) | No | derived from `issuer` | Explicit token endpoint for client_credentials grant (defaults to `{issuer}/oauth/token` for Auth0-compatible providers; other providers must set this explicitly) |
| `client_assertion_type` | string (enum) | No | `id_token` | Token type to use as client assertion: `id_token` (ID token) or `access_token` (access token from client_credentials response). Choose based on your upstream OAuth2 server's capabilities. |
| `exchange_timeout` | duration | No | `5s` | Timeout for outbound token exchange HTTP calls |

### TLSConfig

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `insecure_skip_verify` | bool | No | `false` | Skip TLS certificate verification (DEV ONLY) |
| `ca_bundle_path` | string | No | `""` | Path to custom CA bundle file |
| `allow_http` | bool | No | `false` | Allow HTTP endpoints (DEV ONLY; disables SR-002 enforcement) |

### CacheConfig

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `default_ttl` | duration | No | `5m` | Fallback TTL when exchanged token lacks `expires_in` |
| `max_ttl` | duration | No | `1h` | Maximum cache TTL cap, regardless of `expires_in` |

### LogConfig

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `level` | string | No | `info` | Log level (debug/info/warn/error) |
| `format` | string | No | `text` | Log format (text/json) |

## Port Interface

The `Exchanger` interface (defined in `internal/extproc/server/`) is the hexagonal architecture port between the gRPC server and the token exchange logic. This enables unit testing of the server without a live OAuth2 server.

```go
// Exchanger performs RFC 8693 token exchange with caching.
// Implementations must be safe for concurrent use.
type Exchanger interface {
    Exchange(ctx context.Context, subjectToken, resourceURI string) (string, error)
    Shutdown()
}
```

The `Server` struct holds an `Exchanger` interface. The concrete `TokenExchanger` struct (in `exchanger.go`) implements this interface. Tests inject a stub implementation.

## Internal Value Objects

These types are internal to the ExtProc service and not shared with the identity broker.

### CachedToken

In-memory cached token entry.

| Field | Type | Description |
|-------|------|-------------|
| `access_token` | string | The exchanged access token |
| `token_type` | string | Token type (typically "Bearer") |
| `expires_at` | time.Time | When this cached token expires |

**Cache Key**: `tokenCacheKey{subjectToken, resourceURI}` — a Go struct used directly as a map key. This eliminates separator-injection collisions that affect hash-based approaches (e.g. `sha256(a + "|" + b)` can be collided if `a` or `b` contains `|`).

### ClientAssertionCache

Cached client assertion (ID token) obtained from the OAuth2 server.

| Field | Type | Description |
|-------|------|-------------|
| `id_token` | string | The ID token to use as client assertion |
| `expires_at` | time.Time | When this ID token expires |

### TokenExchangeResult

Parsed response from the identity broker's token exchange endpoint.

| Field | Type | Description |
|-------|------|-------------|
| `access_token` | string | Exchanged access token |
| `token_type` | string | Token type |
| `issued_token_type` | string | RFC 8693 issued token type |
| `expires_in` | `*int` | Seconds until token expires; `nil` when absent from response; `0` or negative treated as absent |

## State Transitions

### Token Exchange Flow (per request)

```
      ┌─────────────────────┐
      │  Request Received   │
      └─────────┬───────────┘
                │
                ▼
      ┌─────────────────────┐     No Auth header
      │ Extract Bearer Token│────────────────────► Pass Through (no modification)
      └─────────┬───────────┘
                │ Has Bearer
                ▼
      ┌─────────────────────┐     Empty/invalid URI
      │ Extract Request URI │────────────────────► Return 503 (ImmediateResponse)
      └─────────┬───────────┘
                │ Valid URI
                ▼
      ┌─────────────────────┐     Cache hit
      │  Check Token Cache  │────────────────────► Replace Authorization header
      └─────────┬───────────┘
                │ Cache miss/expired
                ▼
      ┌─────────────────────┐
      │ Read Cached Client  │ (always available;
      │ Assertion           │  refreshed in bg)
      └─────────┬───────────┘
                │
                ▼
      ┌─────────────────────┐     Success
      │  Token Exchange     │────────────────────► Cache token, Replace header
      │  (singleflight)     │
      └─────────┬───────────┘
                │ Failure
                ▼
      ┌─────────────────────┐
      │ Return 500          │
      │ (ImmediateResponse) │
      └─────────────────────┘
```

### Cache Entry Lifecycle

```
  [Empty] ──create──► [Valid] ──expiry──► [Expired] ──refresh──► [Valid]
                                              │
                                              └─── (concurrent) ── singleflight wait ──► [Valid]
```

### Cache Memory Management

A background goroutine runs at `cache.default_ttl / 2` interval and removes all entries where `time.Now().After(expiresAt)`. This bounds memory growth for long-running deployments where many distinct (token, resource) pairs are processed.

A `cache.max_ttl` cap prevents servers returning very large `expires_in` values from defeating token revocation. TTLs are always capped to `min(expires_in, cache.max_ttl)`.

## Relationships (Activity Diagram)

Numbered flow showing the end-to-end token exchange activity through all components:

```
┌──────────────┐
│ Sample Agent │
└──────┬───────┘
       │
       │ 1. MCP tool call with Bearer token
       ▼
┌──────────────────────┐
│   agentgateway       │
│   (port 4000)        │
│                      │
│  2. ExtProc policy   │
│     intercepts       │
└──────┬───────────────┘
       │
       │ 3. gRPC Process(RequestHeaders) with Authorization + :path
       ▼
┌──────────────────────────┐
│  ExtProc Token Exchange  │
│  (port 50051)            │
│                          │
│  4. Check token cache    │─── cache hit ──► 8. Return header mutation
│     (subject_token +     │                     (replace Authorization)
│      resource_uri)       │
│                          │
│  5. Read cached client   │
│     assertion            │
│     (obtained at startup,│
│      refreshed in bg)    │
│                          │
│  6. POST /oauth2/token   │
│     (RFC 8693 exchange)  │───────────────────────────────┐
│                          │                               │
│  7. Cache exchanged      │                               │
│     token                │                               │
│                          │                               │
│  8. Return header        │                               │
│     mutation             │                               │
└──────┬───────────────────┘                               │
       │                                                   ▼
       │ 9. agentgateway forwards               ┌──────────────────────┐
       │    request with exchanged token         │  Identity Broker     │
       ▼                                         │  /oauth2/token       │
┌──────────────────────┐                         │  (RFC 8693)          │
│  MCP Server Mock     │                         └──────────────────────┘
│  (port 9003)         │
│                      │           Startup flow (not per-request):
│  10. Receives request│           ┌──────────────────────────┐
│      with exchanged  │           │  ExtProc startup:        │
│      Bearer token    │           │  S1. client_credentials  │
│                      │           │      grant ──────────────┼──► Upstream OAuth2
│  11. show_claims     │           │  S2. Cache assertion     │    Server
│      tool decodes    │           │  S3. Background refresh  │    (issuer)
│      JWT claims      │           │      at 80% TTL          │
└──────────────────────┘           └──────────────────────────┘
```

## Validation Rules

### Configuration Validation (at startup)

| Field | Rule | Error |
|-------|------|-------|
| `grpc.port` | 1-65535 | "gRPC port must be between 1 and 65535" |
| `grpc.bind` | Non-empty string | "gRPC bind address must not be empty" |
| `oauth2.token_endpoint` | Valid URL, starts with http:// or https:// | "token_endpoint must be a valid URL" |
| `oauth2.issuer` | Valid URL | "issuer must be a valid URL" |
| `oauth2.client_id` | Non-empty | "client_id must not be empty" |
| `oauth2.client_secret` | Non-empty | "client_secret must not be empty" |
| `cache.default_ttl` | Positive duration | "default_ttl must be positive" |

### Runtime Validation

| Input | Rule | Action |
|-------|------|--------|
| Authorization header | Must be "Bearer <token>" format | Pass through if absent/non-Bearer |
| Request URI (`:path` pseudo-header) | Must be a non-empty absolute URI with `http` or `https` scheme | Return 503 ImmediateResponse |
| Token exchange response | HTTP 200 with valid JSON | Return 500 ImmediateResponse on failure |
