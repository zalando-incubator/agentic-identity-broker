# Data Model: RFC 8693 Token Exchange

This document defines the domain model entities, value objects, and their relationships for the Token Exchange feature.

---

## Entity Relationship Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Token Exchange Flow                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Privileged Client                                                           │
│     │                                                                        │
│     │ POST /oauth2/token                                                     │
│     │ (client_assertion, subject_token, resource)                           │
│     ▼                                                                        │
│  ┌──────────────────────┐                                                   │
│  │ TokenExchangeRequest │                                                   │
│  │  (Value Object)      │                                                   │
│  └──────────┬───────────┘                                                   │
│             │                                                                │
│             ▼                                                                │
│  ┌──────────────────────┐      ┌────────────────────┐                       │
│  │ ClientAssertion      │◄─────│ JWKS Adapter       │                       │
│  │  (Value Object)      │      │ (validates JWT)    │                       │
│  └──────────────────────┘      └────────────────────┘                       │
│             │                                                                │
│             ▼                                                                │
│  ┌──────────────────────┐      ┌────────────────────┐                       │
│  │ SubjectToken         │◄─────│ CEL Evaluator      │                       │
│  │  (Value Object)      │      │ (extracts claims)  │                       │
│  │  - principal         │      └────────────────────┘                       │
│  │  - agent_client_id   │                                                   │
│  └──────────────────────┘                                                   │
│             │                                                                │
│             ▼                                                                │
│  ┌──────────────────────┐      ┌────────────────────────────────┐          │
│  │ ResourceURI          │─────►│ ThirdpartyOAuth2Service        │          │
│  │  (Value Object)      │      │  (Entity - EXTENDED)           │          │
│  └──────────────────────┘      │  + protected_resources []string│          │
│                                └────────────────────────────────┘          │
│             │                               │                               │
│             ▼                               │                               │
│  ┌──────────────────────┐                  │                               │
│  │ UserGrant            │◄─────────────────┘                               │
│  │  (Existing Entity)   │  (verify principal+agent+service)               │
│  │  - principal         │                                                   │
│  │  - agent_id          │                                                   │
│  │  - delegated_tokens  │                                                   │
│  └──────────────────────┘                                                   │
│             │                                                                │
│             ▼                                                                │
│  ┌──────────────────────┐                                                   │
│  │ UserSession          │                                                   │
│  │  (Existing Entity)   │  (retrieve stored tokens)                        │
│  │  - access_token      │                                                   │
│  │  - refresh_token     │                                                   │
│  │  - expires_at        │                                                   │
│  └──────────────────────┘                                                   │
│             │                                                                │
│             ▼                                                                │
│  ┌──────────────────────┐                                                   │
│  │ TokenExchangeResponse│                                                   │
│  │  (Value Object)      │                                                   │
│  │  - access_token      │                                                   │
│  │  - token_type        │                                                   │
│  │  - issued_token_type │                                                   │
│  │  - expires_in        │                                                   │
│  └──────────────────────┘                                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Value Objects

### TokenExchangeRequest

Represents an incoming RFC 8693 token exchange request. Immutable after parsing.

```go
// internal/domain/tokenexchange/request.go
package tokenexchange

// TokenExchangeRequest represents a parsed RFC 8693 token exchange request.
// Immutable after creation via ParseTokenExchangeRequest.
type TokenExchangeRequest struct {
    // RFC 8693 required parameters
    GrantType           string // Must be "urn:ietf:params:oauth:grant-type:token-exchange"
    SubjectToken        string // JWT from upstream OAuth2 server
    SubjectTokenType    string // Must be "urn:ietf:params:oauth:token-type:access_token"
    Resource            string // Target resource URI (may have multiple)
    
    // Client authentication (RFC 7523 JWT Bearer)
    ClientAssertionType string // Must be "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
    ClientAssertion     string // Privileged client's JWT credential
    
    // Optional parameters
    Scope               string // Space-delimited requested scopes
    Audience            string // Intended audience for new token
}

// Validate validates the request structure (not JWT contents).
func (r *TokenExchangeRequest) Validate() error {
    if r.GrantType != TokenExchangeGrantType {
        return NewInvalidRequestError("grant_type must be token-exchange")
    }
    if r.SubjectToken == "" {
        return NewInvalidRequestError("subject_token is required")
    }
    if r.SubjectTokenType != AccessTokenType {
        return NewInvalidRequestError("subject_token_type must be access_token")
    }
    if r.Resource == "" {
        return NewInvalidRequestError("resource parameter is required")
    }
    if r.ClientAssertion == "" {
        return NewInvalidClientError("client_assertion is required")
    }
    if r.ClientAssertionType != JWTBearerType {
        return NewInvalidClientError("client_assertion_type must be jwt-bearer")
    }
    return nil
}

// Constants for RFC 8693 token types
const (
    TokenExchangeGrantType = "urn:ietf:params:oauth:grant-type:token-exchange"
    AccessTokenType        = "urn:ietf:params:oauth:token-type:access_token"
    JWTBearerType          = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)
```

### TokenExchangeResponse

Represents the RFC 8693 compliant response. Immutable.

```go
// internal/domain/tokenexchange/response.go
package tokenexchange

// TokenExchangeResponse represents an RFC 8693 token exchange response.
type TokenExchangeResponse struct {
    // REQUIRED: The security token issued by the authorization server
    AccessToken string `json:"access_token"`
    
    // REQUIRED: Token type (e.g., "Bearer") - passed through from stored token
    TokenType string `json:"token_type"`
    
    // REQUIRED: URI indicating the type of issued token
    IssuedTokenType string `json:"issued_token_type"`
    
    // OPTIONAL: Lifetime in seconds of the access token
    ExpiresIn *int64 `json:"expires_in,omitempty"`
    
    // OPTIONAL: Scopes of the access token
    Scope string `json:"scope,omitempty"`
    
    // OPTIONAL: Refresh token (typically not returned in token exchange)
    RefreshToken string `json:"refresh_token,omitempty"`
}

// NewTokenExchangeResponse creates a response from a UserSession.
func NewTokenExchangeResponse(accessToken, tokenType string, expiresIn *int64) *TokenExchangeResponse {
    return &TokenExchangeResponse{
        AccessToken:     accessToken,
        TokenType:       tokenType,
        IssuedTokenType: AccessTokenType,
        ExpiresIn:       expiresIn,
    }
}
```

### ClientAssertion

Validated client_assertion JWT with extracted claims. Represents the **privileged client** identity (e.g., API gateway, reverse proxy).

```go
// internal/domain/tokenexchange/client_assertion.go
package tokenexchange

import "time"

// ClientAssertion represents a validated client_assertion JWT.
// The client_assertion identifies the PRIVILEGED CLIENT (e.g., API gateway, reverse proxy) making the token exchange request.
// Immutable after validation.
type ClientAssertion struct {
    // Standard JWT claims
    Issuer    string    // iss - must match upstream OAuth2 issuer
    Subject   string    // sub - privileged client identifier (used for audit logging)
    Audience  []string  // aud - must include broker's identifier
    ExpiresAt time.Time // exp
    IssuedAt  time.Time // iat
    
    // Custom claims (accessible via Claims map)
    Claims map[string]interface{}
}

// PrivilegedClientID returns the privileged client identifier for audit logging.
func (c *ClientAssertion) PrivilegedClientID() string {
    return c.Subject
}

// HasScope checks if the client_assertion includes a specific scope.
// Scopes are expected in the "scope" claim as space-delimited string.
func (c *ClientAssertion) HasScope(scope string) bool {
    scopeClaim, ok := c.Claims["scope"].(string)
    if !ok {
        return false
    }
    for _, s := range strings.Split(scopeClaim, " ") {
        if s == scope {
            return true
        }
    }
    return false
}
```

### SubjectToken

Validated subject_token JWT containing user principal and agent identifier.

```go
// internal/domain/tokenexchange/subject_token.go
package tokenexchange

import "time"

// SubjectToken represents a validated subject_token JWT.
// Contains BOTH the user principal and agent identifier.
// - Principal: extracted via configurable CEL expression (default: sub claim)
// - AgentClientID: extracted via configurable CEL expression (default: azp claim)
// Immutable after validation.
type SubjectToken struct {
    // Standard JWT claims
    Issuer    string    // iss - must match upstream OAuth2 issuer
    Subject   string    // sub - typically the user principal
    Audience  []string  // aud
    ExpiresAt time.Time // exp
    IssuedAt  time.Time // iat
    
    // Extracted values (via CEL expressions)
    Principal     string // User principal (from principal_expression)
    AgentClientID string // Agent identifier (from agent_id_expression)
    
    // All claims for CEL evaluation
    Claims map[string]interface{}
}
```

### ResourceURI

Normalized URI identifying the target resource/service.

```go
// internal/domain/tokenexchange/resource_uri.go
package tokenexchange

import (
    "net/url"
    "strings"
)

// ResourceURI represents a normalized target resource URI.
// Trailing slashes are removed for consistent matching.
type ResourceURI struct {
    value string // Normalized URI string
}

// NewResourceURI creates a ResourceURI after validation and normalization.
func NewResourceURI(rawURI string) (ResourceURI, error) {
    parsed, err := url.Parse(rawURI)
    if err != nil {
        return ResourceURI{}, NewInvalidRequestError("invalid resource URI: " + err.Error())
    }
    
    if parsed.Scheme == "" {
        return ResourceURI{}, NewInvalidRequestError("resource URI must have scheme")
    }
    
    if parsed.Scheme != "https" && parsed.Scheme != "http" {
        return ResourceURI{}, NewInvalidRequestError("resource URI must use http or https scheme")
    }
    
    // Normalize: remove trailing slash
    parsed.Path = strings.TrimSuffix(parsed.Path, "/")
    
    return ResourceURI{value: parsed.String()}, nil
}

// String returns the normalized URI string.
func (r ResourceURI) String() string {
    return r.value
}
```

---

## Extended Entities

### ThirdpartyOAuth2Service (EXTENDED)

Add `ProtectedResources` field to existing entity.

```go
// internal/domain/storage/thirdparty_service.go
// EXTEND existing struct

type ThirdpartyOAuth2Service struct {
    // ... existing fields ...
    
    // NEW: URIs that map to this service for token exchange
    // Stored normalized (trailing slashes removed)
    // Example: ["https://api.github.com", "https://github.com/api/v3"]
    ProtectedResources []string `json:"protected_resources,omitempty" db:"protected_resources"`
}

// ValidateProtectedResources validates the protected_resources field.
func (s *ThirdpartyOAuth2Service) ValidateProtectedResources() error {
    seen := make(map[string]bool)
    for i, uri := range s.ProtectedResources {
        // Validate URI format
        normalized, err := normalizeResourceURI(uri)
        if err != nil {
            return fmt.Errorf("protected_resources[%d]: %w", i, err)
        }
        
        // Check for duplicates within this service
        if seen[normalized] {
            return fmt.Errorf("protected_resources[%d]: duplicate URI %s", i, normalized)
        }
        seen[normalized] = true
        
        // Store normalized version
        s.ProtectedResources[i] = normalized
    }
    return nil
}
```

---

## Domain Events

### TokenExchangeSucceeded

Fired when token exchange completes successfully.

```go
// internal/domain/tokenexchange/events.go
package tokenexchange

import "time"

// TokenExchangeSucceeded is emitted when a token exchange succeeds.
type TokenExchangeSucceeded struct {
    Timestamp           time.Time
    Principal           string // User principal (full, for audit)
    ServiceID           string // Target service ID
    AgentClientID       string // Agent requesting access (from subject_token)
    PrivilegedClientID  string // Privileged client ID (from client_assertion sub)
    Resource            string // Requested resource URI
}
```

### TokenExchangeFailed

Fired when token exchange fails.

```go
// TokenExchangeFailed is emitted when a token exchange fails.
type TokenExchangeFailed struct {
    Timestamp          time.Time
    Principal          string // User principal if available
    AgentClientID      string // Agent ID if available
    PrivilegedClientID string // Privileged client ID if available
    Resource           string // Requested resource
    ErrorCode          string // RFC 8693 error code
    ErrorDescription   string
}
```

### TokenRefreshed

Fired when automatic token refresh occurs.

```go
// TokenRefreshed is emitted when a token is automatically refreshed.
type TokenRefreshed struct {
    Timestamp   time.Time
    Principal   string
    ServiceID   string
}
```

---

## Port Interfaces

### JWKSPort

Port for JWKS operations (implemented by JWKS adapter).

```go
// internal/ports/jwks.go
package ports

import (
    "context"
    
    "github.com/lestrrat-go/jwx/v3/jwk"
)

// JWKSPort defines the interface for JWKS operations.
// Implementation is in internal/adapters/jwks/ which handles HTTP fetching and caching.
type JWKSPort interface {
    // GetKeySet returns the cached JWKS key set.
    // The adapter handles caching and background refresh.
    GetKeySet(ctx context.Context) (jwk.Set, error)
    
    // GetKey retrieves a specific key by key ID (kid).
    // Returns error if key not found.
    GetKey(ctx context.Context, kid string) (jwk.Key, error)
}
```

### ThirdpartyOAuth2ServiceRepository (EXTENDED)

Add method to find service by protected resource.

```go
// internal/ports/storage.go
// EXTEND existing interface

type ThirdpartyOAuth2ServiceRepository interface {
    // ... existing methods ...
    
    // FindByProtectedResource finds services where protected_resources contains the URI.
    // Returns empty slice if no services match.
    // Used by token exchange to map resource parameter to service.
    FindByProtectedResource(ctx context.Context, resourceURI string) ([]*storage.ThirdpartyOAuth2Service, error)
}
```

---

## Configuration Types

### TokenExchangeConfig

```go
// internal/ports/config.go
// ADD to Config struct

type Config struct {
    // ... existing fields ...
    
    // TokenExchange holds token exchange specific configuration
    TokenExchange TokenExchangeConfig `mapstructure:"token_exchange"`
}

// TokenExchangeConfig holds configuration for RFC 8693 token exchange.
type TokenExchangeConfig struct {
    // ClaimExtraction configures how claims are extracted from subject_token
    ClaimExtraction ClaimExtractionConfig `mapstructure:"claim_extraction"`
    
    // Authorization configures privileged client authorization rules
    Authorization AuthorizationConfig `mapstructure:"authorization"`
    
    // Refresh configures automatic token refresh behavior
    Refresh RefreshConfig `mapstructure:"refresh"`
}

// ClaimExtractionConfig configures CEL expressions for claim extraction.
type ClaimExtractionConfig struct {
    // PrincipalExpression is a CEL expression to extract user principal from subject_token.
    // Default: "subject_token.sub"
    PrincipalExpression string `mapstructure:"principal_expression"`
    
    // AgentIDExpression is a CEL expression to extract agent ID from subject_token.
    // Default: "subject_token.azp"
    AgentIDExpression string `mapstructure:"agent_id_expression"`
}

// AuthorizationConfig configures privileged client authorization.
type AuthorizationConfig struct {
    // Type is the authorization method: "cel" or "opa" (opa reserved for future)
    Type string `mapstructure:"type"`
    
    // CEL holds CEL-specific authorization configuration
    CEL CELAuthorizationConfig `mapstructure:"cel"`
}

// CELAuthorizationConfig holds CEL authorization configuration.
type CELAuthorizationConfig struct {
    // Expression is the CEL expression evaluated for authorization.
    // Must return boolean.
    // Default: "true" (allow all valid privileged clients)
    Expression string `mapstructure:"expression"`
}

// RefreshConfig configures automatic token refresh.
type RefreshConfig struct {
    // Enabled determines if expired access tokens should be auto-refreshed.
    // Default: true
    Enabled bool `mapstructure:"enabled"`
}

// DefaultTokenExchangeConfig returns default configuration.
func DefaultTokenExchangeConfig() TokenExchangeConfig {
    return TokenExchangeConfig{
        ClaimExtraction: ClaimExtractionConfig{
            PrincipalExpression:      "subject_token.sub",
            AgentIDExpression:  "subject_token.azp",
        },
        Authorization: AuthorizationConfig{
            Type: "cel",
            CEL: CELAuthorizationConfig{
                Expression: "true",
            },
        },
        Refresh: RefreshConfig{
            Enabled: true,
        },
    }
}
```

---

## Domain Service

### TokenExchangeService

```go
// internal/domain/tokenexchange/service.go
package tokenexchange

import (
    "context"
    "log/slog"
)

// TokenExchangeService orchestrates RFC 8693 token exchange operations.
type TokenExchangeService struct {
    jwksAdapter   ports.JWKSPort
    serviceRepo   ports.ThirdpartyOAuth2ServiceRepository
    sessionRepo   ports.UserSessionRepository
    grantRepo     ports.UserGrantRepository
    agentRepo     ports.AgentRepository
    celEvaluator  *CELEvaluator
    config        Config
    logger        *slog.Logger
}

// Exchange performs a token exchange operation.
// Returns RFC 8693 compliant response or error.
func (s *TokenExchangeService) Exchange(ctx context.Context, req *TokenExchangeRequest) (*TokenExchangeResponse, error) {
    // 1. Validate request structure
    if err := req.Validate(); err != nil {
        return nil, err
    }
    
    // 2. Validate and extract client_assertion (privileged client identity)
    clientAssertion, err := s.validateClientAssertion(ctx, req.ClientAssertion)
    if err != nil {
        return nil, err // invalid_client
    }
    
    // 3. Evaluate CEL authorization (privileged client authorization)
    if err := s.evaluateAuthorization(ctx, clientAssertion, req); err != nil {
        return nil, err // access_denied
    }
    
    // 4. Validate and extract subject_token (principal + agent_client_id)
    subjectToken, err := s.validateSubjectToken(ctx, req.SubjectToken)
    if err != nil {
        return nil, err // invalid_request
    }
    
    // 5. Look up service by resource URI
    service, err := s.findServiceByResource(ctx, req.Resource)
    if err != nil {
        return nil, err // invalid_target
    }
    
    // 6. Verify user grant exists for agent+service
    if err := s.verifyUserGrant(ctx, subjectToken.Principal, subjectToken.AgentClientID, service.ID); err != nil {
        return nil, err // access_denied
    }
    
    // 7. Retrieve stored tokens (auto-refresh if needed)
    accessToken, tokenType, expiresIn, err := s.retrieveTokens(ctx, subjectToken.Principal, service.ID)
    if err != nil {
        return nil, err // invalid_grant
    }
    
    // 8. Log success event
    s.logSuccess(ctx, subjectToken.Principal, subjectToken.AgentClientID, clientAssertion.PrivilegedClientID(), service.ID, req.Resource)
    
    // 9. Return RFC 8693 response
    return NewTokenExchangeResponse(accessToken, tokenType, expiresIn), nil
}
```

---

## Database Migration

### 005_add_service_protected_resources.up.sql

```sql
-- Add protected_resources column to thirdparty_services table
ALTER TABLE thirdparty_services
ADD COLUMN protected_resources TEXT[] DEFAULT '{}';

-- Create GIN index for efficient array lookups
CREATE INDEX idx_thirdparty_services_protected_resources 
ON thirdparty_services USING GIN (protected_resources);

-- Add comment for documentation
COMMENT ON COLUMN thirdparty_services.protected_resources IS 
'Array of normalized resource URIs that map to this service for RFC 8693 token exchange. URIs are normalized with trailing slashes removed.';
```

### 005_add_service_protected_resources.down.sql

```sql
-- Remove index first
DROP INDEX IF EXISTS idx_thirdparty_services_protected_resources;

-- Remove column
ALTER TABLE thirdparty_services
DROP COLUMN IF EXISTS protected_resources;
```

---

## Domain Invariants

These invariants MUST be maintained at all times and are enforced through validation, database constraints, or business logic:

### JWT Validation Invariants

1. **JWT Validation Mandatory**: Every token exchange request MUST validate both client_assertion and subject_token JWTs against the upstream OAuth2 server's JWKS. There is NO bypass configuration or mode where JWT validation can be disabled.

2. **Signature Verification**: JWT signatures MUST be verified using lestrrat-go/jwx/v3 library with no custom cryptographic implementations. Validation failure results in immediate request denial.

3. **Issuer Verification**: Both client_assertion and subject_token issuers (iss claim) MUST match the configured upstream_oauth2.issuer value. Mismatch results in immediate rejection.

4. **Expiration Check**: Token expiration (exp claim) MUST be checked before processing. Expired tokens are rejected with invalid_request error. Clock skew tolerance is configurable but defaults to 60 seconds.

5. **Audience Validation**: client_assertion audience (aud claim) MUST include the broker's configured identifier. JWKS verification prevents audience spoofing.

### Authorization Invariants

6. **UserGrant Verification Required**: Token exchange MUST verify that the user has an active, non-revoked, non-expired grant for the (principal, agent_client_id, service_id) combination. Missing grants return 403 access_denied with specific error description.

7. **Grant Status Checks**: A grant is only valid if: (a) status is "active" (not "revoked"), (b) valid_until is null OR valid_until > NOW(), and (c) the grant contains a delegated_token for the target service.

8. **CEL Authorization Enforced**: If authorization.type is "cel", the configured CEL expression MUST be evaluated and return true for authorization to proceed. False return results in 403 access_denied.

### Resource Discovery Invariants

9. **Resource URI Normalization**: All resource URIs MUST be normalized (trailing slashes removed) consistently for storage and lookup. Example: "https://api.github.com/" becomes "https://api.github.com" before storage and comparison.

10. **Exact Resource Match**: Resource lookup uses exact string matching on normalized URIs. Both ambiguous matches (multiple services) and no-match cases return 400 invalid_target error with descriptive error_description.

11. **Database Query Correctness**: PostgreSQL FindByProtectedResource MUST use the GIN index with @> operator for performant array containment queries. In-memory storage uses iterative matching of resource arrays.

### Session & Token Invariants

12. **Token Refresh Logic**: If access_token is expired but refresh_token is valid, system MUST attempt automatic refresh using the stored refresh_token. If refresh fails or refresh is disabled (refresh.enabled=false), return 400 invalid_grant error.

13. **No Token Duplication**: Only one (principal, service_id) session can exist at any time (enforced by database unique constraint). Tokens are stored encrypted in token vault.

14. **Session Lookup Order**: When tokens don't exist, grant verification MUST occur before session lookup to distinguish between "no grant" (403 access_denied) and "no session" (400 invalid_grant) error codes.

### Configuration Invariants

15. **CEL Expression Validation at Startup**: All CEL expressions (principal_expression, agent_id_expression, authorization.cel.expression) MUST be validated at application startup. Syntax errors cause application startup failure with clear error messages.

16. **Configuration Immutability**: TokenExchangeConfig is loaded once at startup and never changed during runtime. Configuration changes require application restart.

---

## Glossary Additions (for ARCHITECTURE.md)

| Term | Definition |
|------|------------|
| **TokenExchangeRequest** | RFC 8693 token exchange request containing grant_type, subject_token, client_assertion, and resource parameters. Parsed from form-urlencoded POST body. |
| **TokenExchangeResponse** | RFC 8693 compliant response containing access_token, token_type, issued_token_type, and optional expires_in. |
| **ClientAssertion** | JWT authenticating the privileged client (e.g., API gateway, reverse proxy) making the token exchange request. Contains privileged client identifier in 'sub' claim. Validated against upstream OAuth2 server's JWKS. |
| **SubjectToken** | JWT containing both user principal and agent identifier. Principal extracted via configurable CEL (default: sub). Agent extracted via configurable CEL (default: azp). |
| **ResourceURI** | URI identifying the target resource/service for token exchange. Normalized (trailing slashes removed) before storage and comparison. Matched against service protected_resources. |
| **Privileged Client** | Entity (e.g., API gateway, reverse proxy) that initiates token exchange on behalf of agents. Authenticates using client_assertion JWT. |
| **CEL Authorization** | Common Expression Language policy evaluation for privileged client authorization. Expression evaluated against client_assertion claims and request context. |
| **Protected Resources** | Array of normalized resource URIs on ThirdpartyOAuth2Service that identify which resources map to that service for token exchange. |
