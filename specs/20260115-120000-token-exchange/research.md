# Research: RFC 8693 OAuth 2.0 Token Exchange

## Overview

This document captures research findings for implementing RFC 8693 Token Exchange in the Agentic Identity Broker. Key areas: JWKS handling with caching, CEL expression evaluation, and RFC 8693 compliance.

---

## JWKS Handling with lestrrat-go/jwx/v3

### Decision
Use `github.com/lestrrat-go/jwx/v3` for JWKS fetching with built-in `jwk.Cache` for automatic caching and background refresh.

### Rationale
- **Already in codebase**: v3.0.12 used for JWE state tokens (20251222-120000-thirdparty-oauth2-sessions)
- **Built-in caching**: `jwk.Cache` provides auto-refresh without custom implementation
- **Async refresh**: Background goroutine refreshes keys before expiry (configurable)
- **HTTP abstraction**: `jwk.Fetch` handles HTTP with configurable client

### Implementation Pattern

```go
// internal/adapters/jwks/adapter.go
package jwks

import (
    "context"
    "time"
    
    "github.com/lestrrat-go/jwx/v3/jwk"
)

// JWKSAdapter fetches and caches JWKS from remote endpoints.
type JWKSAdapter struct {
    cache      *jwk.Cache
    httpClient *http.Client
}

// NewJWKSAdapter creates a new JWKS adapter with caching.
func NewJWKSAdapter(ctx context.Context, jwksURI string, opts ...Option) (*JWKSAdapter, error) {
    cache := jwk.NewCache(ctx)
    
    // Register JWKS URI with cache settings
    err := cache.Register(jwksURI,
        jwk.WithMinRefreshInterval(15*time.Minute),  // Min time between refreshes
        jwk.WithRefreshInterval(1*time.Hour),        // How often to check for refresh
    )
    if err != nil {
        return nil, fmt.Errorf("failed to register JWKS URI: %w", err)
    }
    
    // Force initial fetch to fail fast on invalid URI
    _, err = cache.Refresh(ctx, jwksURI)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch initial JWKS: %w", err)
    }
    
    return &JWKSAdapter{cache: cache}, nil
}

// GetKey retrieves a specific key by kid from the cached JWKS.
func (a *JWKSAdapter) GetKey(ctx context.Context, kid string) (jwk.Key, error) {
    keyset, err := a.cache.Get(ctx, jwksURI)
    if err != nil {
        return nil, fmt.Errorf("failed to get JWKS: %w", err)
    }
    
    key, ok := keyset.LookupKeyID(kid)
    if !ok {
        return nil, fmt.Errorf("key not found: %s", kid)
    }
    
    return key, nil
}
```

### Cache Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| `WithMinRefreshInterval` | 15 min | Minimum time between refresh attempts |
| `WithRefreshInterval` | 1 hour | Background check interval |
| `WithRefreshWindow` | - | Time before expiry to trigger refresh |
| `WithHTTPClient` | http.DefaultClient | Custom HTTP client |

### Alternatives Considered
1. **Manual caching**: Rejected - would reinvent jwk.Cache functionality
2. **No caching**: Rejected - JWKS fetch on every request too slow
3. **go-jose/v4**: Rejected - less feature-complete, no built-in cache

---

## CEL Expression Evaluation

### Decision
Use `github.com/google/cel-go` for Common Expression Language evaluation.

### Rationale
- **Google-backed**: Actively maintained, production-ready
- **Sandboxed**: Cannot access system resources by design
- **Fast**: Compiled expressions cached, evaluation <100ms
- **Well-documented**: Official spec and Go documentation available
- **Used by K8s**: Kubernetes uses CEL for policy evaluation

### Implementation Pattern

```go
// internal/domain/tokenexchange/cel_evaluator.go
package tokenexchange

import (
    "github.com/google/cel-go/cel"
    "github.com/google/cel-go/checker/decls"
)

// CELEvaluator evaluates CEL expressions for authorization and claim extraction.
type CELEvaluator struct {
    authzProgram        cel.Program  // Compiled authorization expression
    principalProgram    cel.Program  // Compiled principal extraction expression
    agentClientIDProgram cel.Program // Compiled agent_client_id extraction expression
}

// NewCELEvaluator compiles CEL expressions at startup (fail-fast on syntax errors).
func NewCELEvaluator(config CELConfig) (*CELEvaluator, error) {
    // Define environment with available variables
    env, err := cel.NewEnv(
        cel.Declarations(
            // client_assertion claims available to authorization expression
            decls.NewVar("client_assertion", decls.NewMapType(decls.String, decls.Dyn)),
            // subject_token claims available to claim extraction expressions
            decls.NewVar("subject_token", decls.NewMapType(decls.String, decls.Dyn)),
            // Request context
            decls.NewVar("request", decls.NewMapType(decls.String, decls.Dyn)),
        ),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create CEL environment: %w", err)
    }
    
    // Compile authorization expression
    authzAst, issues := env.Compile(config.AuthorizationExpression)
    if issues != nil && issues.Err() != nil {
        return nil, fmt.Errorf("invalid authorization CEL expression: %w", issues.Err())
    }
    authzProgram, err := env.Program(authzAst)
    if err != nil {
        return nil, fmt.Errorf("failed to create authorization program: %w", err)
    }
    
    // Compile claim extraction expressions...
    
    return &CELEvaluator{
        authzProgram: authzProgram,
        // ...
    }, nil
}

// EvaluateAuthorization evaluates the authorization expression.
// Returns true if request is authorized, false otherwise.
func (e *CELEvaluator) EvaluateAuthorization(ctx context.Context, input AuthzInput) (bool, error) {
    // Build activation with input variables
    activation := map[string]interface{}{
        "client_assertion": input.ClientAssertionClaims,
        "request": map[string]interface{}{
            "resource":   input.Resource,
            "grant_type": input.GrantType,
            "scope":      input.Scope,
        },
    }
    
    // Evaluate with timeout
    ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
    defer cancel()
    
    result, _, err := e.authzProgram.ContextEval(ctx, activation)
    if err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            return false, fmt.Errorf("CEL evaluation timeout")
        }
        return false, fmt.Errorf("CEL evaluation error: %w", err)
    }
    
    // Result must be boolean
    allowed, ok := result.Value().(bool)
    if !ok {
        return false, fmt.Errorf("CEL expression must return bool, got %T", result.Value())
    }
    
    return allowed, nil
}
```

### CEL Environment Variables

| Variable | Type | Description |
|----------|------|-------------|
| `client_assertion.iss` | string | JWT issuer |
| `client_assertion.sub` | string | Privileged client identifier (e.g., API gateway, reverse proxy) |
| `client_assertion.aud` | string/list | Audience(s) |
| `client_assertion.exp` | int | Expiration timestamp |
| `client_assertion.iat` | int | Issued at timestamp |
| `client_assertion.scope` | string | Space-delimited scopes (if present) |
| `subject_token.sub` | string | User principal |
| `subject_token.azp` | string | Agent client_id (default extraction) |
| `request.resource` | string | Target resource URI |
| `request.grant_type` | string | Always token-exchange |
| `request.scope` | string | Requested scopes |

### Example CEL Expressions

**Authorization**:
```cel
# Only allow privileged clients from trusted issuer with token-exchange scope
client_assertion.iss == "https://upstream.example.com" && 
"token-exchange" in client_assertion.scope.split(" ")

# Allow any valid privileged client (default)
true
```

**Principal Extraction**:
```cel
# Extract from sub claim (default)
subject_token.sub

# Extract from custom claim
subject_token.preferred_username

# Fallback pattern
has(subject_token.email) ? subject_token.email : subject_token.sub
```

### Alternatives Considered
1. **Rego/OPA**: Rejected for initial release - CEL simpler, OPA reserved for future
2. **Go templates**: Rejected - no sandbox, security concerns
3. **Hardcoded rules**: Rejected - not configurable, violates spec requirements

---

## RFC 8693 Token Exchange Compliance

### Request Format (Section 2.1)

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&subject_token=<JWT>
&subject_token_type=urn:ietf:params:oauth:token-type:access_token
&resource=https://api.github.com
&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer
&client_assertion=<JWT>
```

### Required Parameters

| Parameter | Value | Description |
|-----------|-------|-------------|
| `grant_type` | `urn:ietf:params:oauth:grant-type:token-exchange` | Identifies token exchange |
| `subject_token` | JWT | Token to exchange (contains principal + agent_client_id) |
| `subject_token_type` | `urn:ietf:params:oauth:token-type:access_token` | Type of subject_token |
| `resource` | URI | Target service (matches protected_resources) |
| `client_assertion_type` | `urn:ietf:params:oauth:client-assertion-type:jwt-bearer` | Privileged client auth method |
| `client_assertion` | JWT | Privileged client authentication |

### Response Format (Section 2.2)

```json
{
    "access_token": "<third-party-access-token>",
    "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
    "token_type": "Bearer",
    "expires_in": 3600
}
```

### Error Responses (Section 5.2)

| Error Code | HTTP Status | Condition |
|------------|-------------|-----------|
| `invalid_request` | 400 | Missing/malformed parameters, invalid subject_token |
| `invalid_client` | 401 | Invalid/missing client_assertion |
| `invalid_grant` | 400 | No session, tokens expired (both access + refresh) |
| `invalid_target` | 400 | No service matches resource, ambiguous resource |
| `access_denied` | 403 | No user grant, grant revoked/expired, CEL denied privileged client |

---

## Resource URI Normalization

### Decision
Normalize URIs by removing trailing slashes before storage and comparison.

### Implementation

```go
// internal/domain/tokenexchange/resource_uri.go
package tokenexchange

import (
    "net/url"
    "strings"
)

// NormalizeResourceURI normalizes a resource URI for consistent storage/lookup.
func NormalizeResourceURI(uri string) (string, error) {
    // Parse to validate
    parsed, err := url.Parse(uri)
    if err != nil {
        return "", fmt.Errorf("invalid URI: %w", err)
    }
    
    // Must have scheme
    if parsed.Scheme == "" {
        return "", fmt.Errorf("URI must have scheme")
    }
    
    // Remove trailing slash from path
    parsed.Path = strings.TrimSuffix(parsed.Path, "/")
    
    return parsed.String(), nil
}
```

### Examples

| Input | Normalized |
|-------|------------|
| `https://api.github.com/` | `https://api.github.com` |
| `https://api.github.com` | `https://api.github.com` |
| `https://api.github.com/v1/` | `https://api.github.com/v1` |

---

## JWT Validation Flow

### Decision
Use lestrrat-go/jwx/v3 for JWT validation in domain layer (pure computation, no HTTP).

### Validation Steps

1. **Parse JWT**: Decode without verification to extract header
2. **Get Key**: Use JWKS adapter to fetch signing key by `kid`
3. **Verify Signature**: Validate signature using public key
4. **Validate Claims**: Check `iss`, `aud`, `exp`, `iat`
5. **Extract Claims**: Return claims map for CEL evaluation

### Implementation Pattern

```go
// internal/domain/tokenexchange/jwt_validator.go
package tokenexchange

import (
    "github.com/lestrrat-go/jwx/v3/jwt"
    "github.com/lestrrat-go/jwx/v3/jwk"
)

// JWTValidator validates JWTs using JWKS.
type JWTValidator struct {
    jwksAdapter ports.JWKSPort
    issuer      string
    audience    string
}

// ValidateToken validates a JWT and returns its claims.
func (v *JWTValidator) ValidateToken(ctx context.Context, tokenString string) (map[string]interface{}, error) {
    // Get JWKS from adapter
    keyset, err := v.jwksAdapter.GetKeySet(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to get JWKS: %w", err)
    }
    
    // Parse and verify JWT
    token, err := jwt.Parse(
        []byte(tokenString),
        jwt.WithKeySet(keyset),
        jwt.WithIssuer(v.issuer),
        jwt.WithAudience(v.audience),
        jwt.WithValidate(true),
    )
    if err != nil {
        return nil, fmt.Errorf("JWT validation failed: %w", err)
    }
    
    // Extract claims as map
    claims := token.PrivateClaims()
    claims["sub"] = token.Subject()
    claims["iss"] = token.Issuer()
    claims["aud"] = token.Audience()
    claims["exp"] = token.Expiration().Unix()
    claims["iat"] = token.IssuedAt().Unix()
    
    return claims, nil
}
```

---

## Phased Implementation Approach

To ensure E2E tests compile from the start (per user requirement), structures and interfaces will be added early with stub implementations:

### Phase 1: Structures & Interfaces (E2E Tests Compile)

1. Create `internal/ports/jwks.go` with `JWKSPort` interface
2. Create `internal/domain/tokenexchange/` package with:
   - `request.go`, `response.go` (value objects, no logic)
   - `service.go` (interface + struct definition, empty methods)
   - `errors.go` (error types)
3. Create `internal/adapters/jwks/adapter.go` (stub returning error)
4. Wire into `app.Builder` (nil-safe initialization)

### Phase 2: E2E Tests (Red Phase)

1. Write all E2E tests against structures from Phase 1
2. Tests call real endpoints but fail (not implemented)
3. Verify tests compile and fail correctly

### Phase 3: Implementation (Green Phase)

1. Implement JWKS adapter with caching
2. Implement JWT validation
3. Implement CEL evaluator
4. Implement TokenExchangeService
5. Wire handler to routing
6. E2E tests turn green

---

## Configuration Example

```yaml
# examples/config/token-exchange.yaml

# Reuse existing upstream OAuth2 config for JWKS
upstream_oauth2:
  issuer: "https://auth.example.com"
  jwks_uri: "https://auth.example.com/.well-known/jwks.json"
  audience: "agentic-identity-broker"

# Token exchange specific configuration
token_exchange:
  claim_extraction:
    # CEL expression to extract user principal from subject_token
    # Default: subject_token.sub
    principal_expression: "subject_token.sub"
    
    # CEL expression to extract agent identifier from subject_token  
    # Default: subject_token.azp
    agent_id_expression: "subject_token.azp"
  
  authorization:
    type: cel  # "cel" or "opa" (opa reserved for future)
    cel:
      # CEL expression for privileged client authorization
      # Default: true (allow all valid privileged clients)
      expression: |
        client_assertion.iss == "https://auth.example.com" &&
        "token-exchange" in client_assertion.scope.split(" ")
  
  refresh:
    # Whether to auto-refresh expired access tokens
    enabled: true
```

---

## Dependencies to Add

```go
// go.mod additions
require (
    github.com/google/cel-go v0.20.1  // CEL expression evaluation
)
```

Note: `lestrrat-go/jwx/v3` already in go.mod (v3.0.12).

---

## Summary of Decisions

| Topic | Decision | Rationale |
|-------|----------|-----------|
| JWKS Library | lestrrat-go/jwx/v3 | Already in codebase, built-in caching |
| JWKS Location | Adapter layer | User requirement, enables future caching improvements |
| JWT Validation | Domain layer | Pure computation using library |
| CEL Library | google/cel-go | Google-backed, sandboxed, fast |
| CEL Location | Domain layer | Pure computation, no external deps |
| URI Normalization | Remove trailing slash | Consistent matching |
| Error Codes | RFC 8693 Section 5.2 | Standards compliance |
| Terminology | Privileged client | More accurate than "gateway" for entities like API gateways, reverse proxies |
