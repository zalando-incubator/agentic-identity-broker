# Research: OAuth2 Authorization Server Proxy

**Feature**: 20251222-120100-oauth2-auth-server
**Date**: 2025-12-22
**Status**: Complete

## Overview

This document consolidates research findings for implementing an OAuth2 Authorization Server proxy in Go. The broker acts as a transparent proxy: it validates client_id and checks consent, then redirects (authorization) or proxies (token) requests to an upstream RFC 6749 compliant OAuth2 server.

---

## 1. OAuth2 RFC 6749 Authorization Code Flow

### Decision: Use HTTP 302 for OAuth2 redirects
**Rationale**: RFC 6749 specifies using HTTP 302 (Found) for authorization redirects. This is standard for OAuth2 flows where the user agent (browser) needs to be redirected to another server while preserving the HTTP method.

**Authorization Endpoint Parameters (Required)**:
- `response_type`: Must be "code" for authorization code flow
- `client_id`: OAuth2 client identifier (maps to Agent.client_id in our system)
- `redirect_uri`: Client's callback URL (must match registered URI)
- `scope`: Space-delimited list of requested scopes (optional but recommended)
- `state`: Opaque value for CSRF protection (recommended)

**Authorization Endpoint Parameters (Optional)**:
- `code_challenge`, `code_challenge_method`: For PKCE (RFC 7636)
- Additional parameters are allowed and must be preserved when proxying

**OAuth2 Error Codes (RFC 6749 Section 4.1.2.1)**:
- `invalid_request`: Missing required parameter, invalid parameter value
- `unauthorized_client`: Client not authorized for this grant type
- `access_denied`: Resource owner denied the request (used for consent rejection)
- `unsupported_response_type`: Authorization server doesn't support response_type
- `invalid_scope`: Invalid, unknown, or malformed scope
- `server_error`: Internal server error (HTTP 500 equivalent)
- `temporarily_unavailable`: Server temporarily overloaded (HTTP 503 equivalent)

**Broker-Specific Error Usage**:
- `invalid_client`: When client_id doesn't match registered Agent
- `access_denied`: When user hasn't granted consent (redirect to consent UI instead)
- `server_error`: When TLS validation fails, upstream unreachable
- `temporarily_unavailable`: When upstream OAuth2 server is temporarily down

**Error Response Format** (query parameters appended to redirect_uri):
```
?error=invalid_request
&error_description=Missing+required+parameter+client_id
&state=xyz123
```

**Redirect Semantics**:
- Use HTTP 302 (Found) for authorization redirects per RFC 6749
- Preserve all original query parameters when redirecting to upstream
- URL-encode all parameter values properly
- Always include `state` parameter in error redirects if present in original request

---

## 2. OAuth2 RFC 8414 Metadata Discovery

### Decision: Implement /.well-known/oauth-authorization-server endpoint
**Rationale**: RFC 8414 defines standard OAuth2 metadata discovery. This enables OAuth2 clients and libraries to automatically discover endpoints and capabilities without manual configuration.

**Metadata Endpoint Path**:
```
/.well-known/oauth-authorization-server
```

**Required Fields**:
- `issuer`: Identity broker's public base URL (e.g., "https://identity-broker.example.com")
- `authorization_endpoint`: Full URL to broker's `/oauth2/authorize` endpoint
- `token_endpoint`: Full URL to broker's `/oauth2/token` endpoint
- `response_types_supported`: Array of supported response types (e.g., `["code"]`)
- `grant_types_supported`: Array of supported grant types (e.g., `["authorization_code", "refresh_token"]`)

**Optional Fields (Recommended)**:
- `token_endpoint_auth_methods_supported`: Array of client authentication methods (e.g., `["client_secret_post", "client_secret_basic"]`)
- `code_challenge_methods_supported`: PKCE methods (e.g., `["S256"]`) if PKCE supported

**Example Metadata Response**:
```json
{
  "issuer": "https://identity-broker.example.com",
  "authorization_endpoint": "https://identity-broker.example.com/oauth2/authorize",
  "token_endpoint": "https://identity-broker.example.com/oauth2/token",
  "response_types_supported": ["code"],
  "grant_types_supported": ["authorization_code", "refresh_token"],
  "token_endpoint_auth_methods_supported": ["client_secret_post", "client_secret_basic"]
}
```

**Content-Type**: `application/json`

---

## 3. Token Endpoint Specifications (RFC 6749)

### Decision: Proxy token requests as server-to-server HTTP POST
**Rationale**: Token endpoint requests contain sensitive credentials (client_secret, authorization codes) and must not be exposed to user's browser. Server-to-server HTTP POST with proper Content-Type validation provides security and CSRF protection.

**Authorization Code Grant Parameters (Required)**:
- `grant_type`: Must be "authorization_code"
- `code`: Authorization code received from authorization endpoint
- `redirect_uri`: Must match the redirect_uri used in authorization request
- `client_id`: OAuth2 client identifier

**Client Authentication**:
- `client_secret`: Client secret (in POST body or HTTP Basic Auth header)
- Can be sent via `client_secret_post` (in body) or `client_secret_basic` (Authorization header)

**Refresh Token Grant Parameters**:
- `grant_type`: Must be "refresh_token"
- `refresh_token`: The refresh token issued by authorization server
- `scope`: Optional, requested scope (must not exceed original grant scope)

**Token Response Format (Success)**:
```json
{
  "access_token": "SlAV32hkKG",
  "token_type": "Bearer",
  "expires_in": 3600,
  "refresh_token": "8xLOxBtZp8",
  "scope": "read write"
}
```

**Token Error Response Format**:
```json
{
  "error": "invalid_grant",
  "error_description": "Authorization code is invalid or expired"
}
```

**Common Token Error Codes**:
- `invalid_request`: Missing or malformed parameter
- `invalid_client`: Client authentication failed
- `invalid_grant`: Invalid authorization code, expired code, or mismatched redirect_uri
- `unauthorized_client`: Client not authorized for this grant type
- `unsupported_grant_type`: Grant type not supported by server

**Content-Type Requirements**:
- **Request**: `application/x-www-form-urlencoded` or `application/json`
- **Response**: `application/json` with `Cache-Control: no-store` and `Pragma: no-cache`

**CSRF Protection**:
- Validate Content-Type header (reject requests without proper Content-Type)
- Token endpoint only accepts POST requests (reject GET)
- State parameter in authorization flow provides CSRF protection for authorization endpoint

---

## 4. Go net/http Client TLS Configuration

### Decision: Use http.Client with system CA pool for TLS validation
**Rationale**: Go's standard library provides robust TLS validation using the system's certificate authority pool. This rejects self-signed and expired certificates by default, providing strong security without custom crypto.

**Secure HTTP Client Configuration**:
```go
func NewSecureUpstreamClient(timeout time.Duration) (*http.Client, error) {
    // Load system CA certificates
    certPool, err := x509.SystemCertPool()
    if err != nil {
        return nil, fmt.Errorf("failed to load system cert pool: %w", err)
    }

    return &http.Client{
        Timeout: timeout,
        Transport: &http.Transport{
            TLSClientConfig: &tls.Config{
                RootCAs:    certPool,
                MinVersion: tls.VersionTLS12, // Enforce TLS 1.2+
            },
            // Connection pooling
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 10,
            IdleConnTimeout:     90 * time.Second,
            // Timeouts
            DialContext: (&net.Dialer{
                Timeout:   30 * time.Second,
                KeepAlive: 30 * time.Second,
            }).DialContext,
            TLSHandshakeTimeout:   10 * time.Second,
            ResponseHeaderTimeout: 30 * time.Second,
            ExpectContinueTimeout: 1 * time.Second,
        },
    }, nil
}
```

**Key Security Features**:
- `x509.SystemCertPool()`: Uses OS-trusted CA certificates (rejects self-signed)
- `MinVersion: tls.VersionTLS12`: Enforces modern TLS (blocks TLS 1.0/1.1)
- Timeout enforcement at multiple layers (dial, TLS handshake, response header, total request)
- Connection pooling for performance without sacrificing security

**Error Handling**:
- TLS certificate validation failures return `x509.UnknownAuthorityError` or `x509.CertificateInvalidError`
- Broker must catch these errors and return OAuth2 `server_error` to client
- Log TLS errors for operator visibility (without exposing to client)

---

## 5. Chi Router Middleware Patterns

### Decision: Implement audit logging middleware with structured logging (slog)
**Rationale**: Chi middleware integrates cleanly with existing codebase patterns. Using Go 1.21+ slog provides structured, high-performance logging for audit trails.

**Audit Logging Middleware**:
```go
func OAuth2AuditMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Generate request ID
            requestID := uuid.New().String()
            ctx := context.WithValue(r.Context(), requestIDKey, requestID)

            // Extract OAuth2 parameters
            clientID := r.URL.Query().Get("client_id")
            if clientID == "" {
                clientID = r.FormValue("client_id")
            }

            // Extract principal from context (set by RequirePrincipalMiddleware)
            // Use FromContext (not MustFromContext) as some OAuth2 endpoints may not require authentication
            principalValue, hasPrincipal := principal.FromContext(r.Context())
            if !hasPrincipal {
                principalValue = "" // No principal for public endpoints (metadata, token)
            }

            // Log request start
            logger.InfoContext(ctx, "oauth2_request_start",
                slog.String("request_id", requestID),
                slog.String("method", r.Method),
                slog.String("path", r.URL.Path),
                slog.String("client_id", clientID),
                slog.String("principal", principalValue),
            )

            // Wrap response writer to capture status code
            ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

            // Process request
            next.ServeHTTP(ww, r.WithContext(ctx))

            // Log request completion
            logger.InfoContext(ctx, "oauth2_request_complete",
                slog.String("request_id", requestID),
                slog.Int("status_code", ww.statusCode),
                slog.String("client_id", clientID),
                slog.String("principal", principalValue),
            )
        })
    }
}
```

**Context Propagation Pattern**:
```go
type contextKey string

const (
    requestIDKey contextKey = "request_id"
    clientIDKey  contextKey = "client_id"
)

func GetRequestID(ctx context.Context) string {
    if id, ok := ctx.Value(requestIDKey).(string); ok {
        return id
    }
    return ""
}
```

**Middleware Ordering**:
```
Recovery → RequestID → OAuth2Audit → RequirePrincipalMiddleware (authorize only) → OAuth2Routes
```
Note: RequirePrincipalMiddleware applies only to the /oauth2/authorize endpoint.
Token and metadata endpoints do not require principal middleware.

---

## 6. HTTP POST Proxy Implementation

### Decision: Use io.Copy for efficient body streaming, filter hop-by-hop headers
**Rationale**: Streaming with io.Copy handles large responses efficiently without loading entire response into memory. Filtering hop-by-hop headers prevents proxy-related issues.

**Proxy POST Request Function**:
```go
func ProxyPOSTRequest(client *http.Client, w http.ResponseWriter, r *http.Request, upstreamURL string) error {
    // Create upstream request
    upstreamReq, err := http.NewRequestWithContext(r.Context(), "POST", upstreamURL, r.Body)
    if err != nil {
        return fmt.Errorf("failed to create upstream request: %w", err)
    }

    // Copy headers (excluding hop-by-hop headers)
    for key, values := range r.Header {
        if !isHopByHopHeader(key) {
            for _, value := range values {
                upstreamReq.Header.Add(key, value)
            }
        }
    }

    // Execute request
    upstreamResp, err := client.Do(upstreamReq)
    if err != nil {
        return fmt.Errorf("upstream request failed: %w", err)
    }
    defer upstreamResp.Body.Close()

    // Copy response headers
    for key, values := range upstreamResp.Header {
        if !isHopByHopHeader(key) {
            for _, value := range values {
                w.Header().Add(key, value)
            }
        }
    }

    // Write status code
    w.WriteHeader(upstreamResp.StatusCode)

    // Stream response body
    _, err = io.Copy(w, upstreamResp.Body)
    return err
}

func isHopByHopHeader(header string) bool {
    hopByHop := []string{
        "Connection", "Keep-Alive", "Proxy-Authenticate",
        "Proxy-Authorization", "Te", "Trailers",
        "Transfer-Encoding", "Upgrade",
    }
    for _, h := range hopByHop {
        if strings.EqualFold(header, h) {
            return true
        }
    }
    return false
}
```

**Content-Type Handling**:
- Accept `application/x-www-form-urlencoded` and `application/json`
- Reject other Content-Types with HTTP 415 Unsupported Media Type
- Preserve Content-Type header when proxying to upstream

**Timeout Configuration**:
- Use `http.Client.Timeout` for total request timeout (30 seconds default)
- Can override per-request with `context.WithTimeout`

---

## 7. OAuth2 URL Construction

### Decision: Use url.Values for RFC-compliant parameter encoding
**Rationale**: Go's url.Values automatically handles URL encoding per RFC 3986. This prevents injection attacks and ensures proper encoding of special characters.

**Authorization URL Builder**:
```go
func BuildAuthorizeRedirectURL(baseURL string, params map[string]string) (string, error) {
    u, err := url.Parse(baseURL)
    if err != nil {
        return "", fmt.Errorf("invalid base URL: %w", err)
    }

    // Validate HTTPS (required for OAuth2)
    if u.Scheme != "https" {
        return "", errors.New("OAuth2 URLs must use HTTPS")
    }

    // Build query parameters
    q := u.Query()
    for key, value := range params {
        q.Set(key, value)
    }
    u.RawQuery = q.Encode()

    return u.String(), nil
}
```

**Parameter Preservation**:
When redirecting to upstream, preserve ALL original query parameters:
```go
func PreserveAndAddParams(originalURL *url.URL, additionalParams map[string]string) string {
    q := originalURL.Query()
    for key, value := range additionalParams {
        q.Set(key, value)
    }
    originalURL.RawQuery = q.Encode()
    return originalURL.String()
}
```

**Error Redirect Helper**:
```go
func BuildErrorRedirectURL(redirectURI, errorCode, errorDescription, state string) (string, error) {
    u, err := url.Parse(redirectURI)
    if err != nil {
        return "", fmt.Errorf("invalid redirect_uri: %w", err)
    }

    q := u.Query()
    q.Set("error", errorCode)
    if errorDescription != "" {
        q.Set("error_description", errorDescription)
    }
    if state != "" {
        q.Set("state", state)
    }
    u.RawQuery = q.Encode()

    return u.String(), nil
}
```

---

## Implementation Decisions Summary

| Decision Area | Choice | Rationale |
|--------------|--------|-----------|
| **HTTP Redirect** | HTTP 302 | RFC 6749 standard for authorization redirects |
| **TLS Validation** | System CA pool (x509.SystemCertPool) | Rejects self-signed/expired certificates by default |
| **Token Proxy** | Server-to-server HTTP POST with io.Copy | Secure (no browser exposure), efficient streaming |
| **Audit Logging** | Chi middleware + slog | Structured logging, integrates with existing patterns |
| **URL Encoding** | url.Values | RFC-compliant, prevents injection attacks |
| **Content-Type** | Validate and reject invalid types | CSRF protection per SR-008 |
| **Timeouts** | 30s default (configurable) | Balance reliability and user experience |
| **Error Handling** | RFC 6749 error codes + fail closed | Security-first, standards-compliant |

---

## Alternatives Considered

### HTTP 303 vs 302 for Redirects
- **Rejected**: HTTP 303 (See Other) changes method to GET, which breaks OAuth2 POST-redirect-GET flows
- **Chosen**: HTTP 302 (Found) preserves method and is RFC 6749 standard

### Custom TLS Certificate Pinning
- **Rejected**: Too rigid for operational environments where certificates rotate
- **Chosen**: System CA pool provides strong security with operational flexibility

### Token Endpoint Rate Limiting
- **Rejected**: Adds complexity, upstream OAuth2 server handles this
- **Chosen**: Delegate rate limiting to upstream (clarification confirmed)

### Token Endpoint Audit Logging
- **Rejected**: Risk of logging sensitive tokens, upstream has comprehensive logs
- **Chosen**: No token endpoint logging (clarification confirmed)

---

## Integration with Existing Codebase

### Reused Components
- `internal/ports/storage.go`: AgentRepository, GrantRepository (existing)
- `internal/domain/storage/agent.go`: Agent entity (existing)
- `internal/domain/storage/grant.go`: User Grant entity (existing)
- `internal/config/schema.go`: Viper-based configuration (modify to add oauth2_authorization_server block)

### New Components Required
- `internal/ports/oauth2.go`: OAuth2Service port interface
- `internal/domain/oauth2/`: Domain logic for authorization, token, metadata
- `internal/adapters/http/enduser/oauth2_*.go`: HTTP handlers for OAuth2 endpoints
- `internal/adapters/http/upstream/oauth2_client.go`: HTTP client for upstream OAuth2 server

### Testing Strategy
- **Unit Tests**: OAuth2 domain logic, URL builders, parameter validation
- **Integration Tests**: End-to-end OAuth2 flows with mock upstream server
- **TLS Tests**: Verify rejection of self-signed certificates

---

## References

- [RFC 6749: OAuth 2.0 Authorization Framework](https://datatracker.ietf.org/doc/html/rfc6749)
- [RFC 8414: OAuth 2.0 Authorization Server Metadata](https://datatracker.ietf.org/doc/html/rfc8414)
- [Go net/http documentation](https://pkg.go.dev/net/http)
- [Go crypto/tls documentation](https://pkg.go.dev/crypto/tls)
- [Chi router documentation](https://github.com/go-chi/chi)
- [Go slog documentation](https://pkg.go.dev/log/slog)

---

**Status**: Research complete. Proceeding to Phase 1 (Design & Contracts).
