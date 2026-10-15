# Research: Third-Party OAuth2 Session Management

**Feature Branch**: `008-thirdparty-oauth2-sessions`  
**Date**: 2025-12-23  
**Purpose**: Resolve technical unknowns and establish best practices for implementation

## Table of Contents

1. [OAuth2 Authorization Code Flow with PKCE](#oauth2-authorization-code-flow-with-pkce)
2. [JWE State Token Implementation](#jwe-state-token-implementation)
3. [Domain Service Design Pattern](#domain-service-design-pattern)
4. [Token Encryption Strategy](#token-encryption-strategy)
5. [Database Schema Design](#database-schema-design)
6. [Error Handling Patterns](#error-handling-patterns)
7. [Retry Logic for Token Exchange](#retry-logic-for-token-exchange)

---

## OAuth2 Authorization Code Flow with PKCE

### Decision
Use Go's standard `golang.org/x/oauth2` library for OAuth2 flows, with manual PKCE generation.

### Rationale
- **Battle-tested library**: `golang.org/x/oauth2` is the de facto standard for OAuth2 in Go
- **Clean abstraction**: Provides `oauth2.Config` for managing client credentials and endpoints
- **Token exchange built-in**: `Exchange()` method handles authorization code → token exchange
- **Context support**: All operations accept `context.Context` for timeout/cancellation
- **PKCE extension**: Library supports PKCE via `oauth2.SetAuthURLParam()` for code_challenge

### Implementation Approach

```go
import (
    "golang.org/x/oauth2"
    "crypto/sha256"
    "encoding/base64"
    "crypto/rand"
)

// OAuth2Config creates oauth2.Config from ThirdpartyOAuth2Service
func (s *OAuth2SessionService) createOAuth2Config(service *storage.ThirdpartyOAuth2Service) *oauth2.Config {
    return &oauth2.Config{
        ClientID:     service.ClientID,
        ClientSecret: service.ClientSecret,
        Endpoint: oauth2.Endpoint{
            AuthURL:  service.Endpoints.AuthorizeEndpoint,
            TokenURL: service.Endpoints.TokenEndpoint,
        },
        Scopes:      extractScopeStrings(service.Scopes),
        RedirectURL: s.callbackURL,
    }
}

// PKCE code verifier generation (RFC 7636)
func generatePKCE(length int) (verifier, challenge string, err error) {
    // verifier: 32-128 bytes of cryptographically random data, base64url encoded
    bytes := make([]byte, length)
    if _, err := rand.Read(bytes); err != nil {
        return "", "", err
    }
    verifier = base64.RawURLEncoding.EncodeToString(bytes)
    
    // challenge: SHA256 hash of verifier, base64url encoded
    hash := sha256.Sum256([]byte(verifier))
    challenge = base64.RawURLEncoding.EncodeToString(hash[:])
    
    return verifier, challenge, nil
}
```

### Authorization URL Construction

```go
// Add PKCE parameters to authorization URL
authURL := config.AuthCodeURL(
    stateToken,
    oauth2.SetAuthURLParam("code_challenge", challenge),
    oauth2.SetAuthURLParam("code_challenge_method", "S256"),
)
```

### Token Exchange with PKCE

```go
// Exchange authorization code for tokens with PKCE verifier
token, err := config.Exchange(
    ctx,
    code,
    oauth2.SetAuthURLParam("code_verifier", pkceVerifier),
)
```

### Alternatives Considered
1. **Custom HTTP client**: Rejected - reinvents wheel, error-prone
2. **go-oidc library**: Rejected - focuses on OIDC, adds unnecessary complexity for pure OAuth2
3. **Manual token exchange**: Rejected - `golang.org/x/oauth2` handles this correctly

---

## JWE State Token Implementation

### Decision
Use `github.com/lestrrat-go/jwx/v3` for JWE (JSON Web Encryption) state tokens.

### Rationale
- **Complete JWx suite**: Implements JWA, JWE, JWK, JWS, JWT (RFC 7516, 7517, 7518, 7519)
- **Authenticated encryption**: JWE provides both confidentiality and integrity
- **Symmetric key support**: Can use a single key for encrypt/decrypt (simpler than asymmetric)
- **Well-maintained**: Active development, 2.3k stars, used by 5.2k projects
- **Clean API**: `jwe.Encrypt()` / `jwe.Decrypt()` with clear option patterns

### State Token Claims Structure

```go
type OAuth2StateTokenClaims struct {
    Principal   string    `json:"principal"`      // Current authenticated user
    PKCEVerifier string   `json:"pkce_verifier"` // PKCE code verifier for token exchange
    ServiceID   string    `json:"service_id"`     // Third-party service identifier
    RedirectURI string    `json:"redirect_uri"`   // Original redirect after flow completes
    IssuedAt    time.Time `json:"iat"`            // Token creation timestamp
    ExpiresAt   time.Time `json:"exp"`            // Token expiration (TTL: 10 min default)
}
```

### JWE Implementation

```go
import (
    "github.com/lestrrat-go/jwx/v3/jwa"
    "github.com/lestrrat-go/jwx/v3/jwe"
    "github.com/lestrrat-go/jwx/v3/jwk"
)

// CreateStateToken creates a JWE-encrypted state token
func (s *OAuth2SessionService) CreateStateToken(claims *OAuth2StateTokenClaims) (string, error) {
    // Serialize claims to JSON
    payload, err := json.Marshal(claims)
    if err != nil {
        return "", fmt.Errorf("failed to serialize claims: %w", err)
    }
    
    // Encrypt with symmetric key using A256GCM (authenticated encryption)
    encrypted, err := jwe.Encrypt(
        payload,
        jwe.WithKey(jwa.A256GCMKW(), s.jweKey),
        jwe.WithContentEncryption(jwa.A256GCM()),
    )
    if err != nil {
        return "", fmt.Errorf("failed to encrypt state token: %w", err)
    }
    
    return string(encrypted), nil
}

// ValidateStateToken decrypts and validates a state token
func (s *OAuth2SessionService) ValidateStateToken(token string, currentPrincipal string) (*OAuth2StateTokenClaims, error) {
    // Decrypt token
    decrypted, err := jwe.Decrypt(
        []byte(token),
        jwe.WithKey(jwa.A256GCMKW(), s.jweKey),
    )
    if err != nil {
        return nil, fmt.Errorf("failed to decrypt state token: %w", err)
    }
    
    // Deserialize claims
    var claims OAuth2StateTokenClaims
    if err := json.Unmarshal(decrypted, &claims); err != nil {
        return nil, fmt.Errorf("failed to parse claims: %w", err)
    }
    
    // Validate expiration
    if time.Now().After(claims.ExpiresAt) {
        return nil, ErrStateTokenExpired
    }
    
    // Validate principal matches current user (CSRF protection)
    if claims.Principal != currentPrincipal {
        return nil, ErrPrincipalMismatch
    }
    
    return &claims, nil
}
```

### Key Management

```go
// Load JWE key from configuration (environment variable)
func loadJWEKey(keyString string) (jwk.Key, error) {
    // Key must be 32 bytes for A256GCMKW (256-bit AES key wrapping)
    keyBytes, err := base64.StdEncoding.DecodeString(keyString)
    if err != nil {
        return nil, fmt.Errorf("invalid base64 key: %w", err)
    }
    if len(keyBytes) != 32 {
        return nil, fmt.Errorf("key must be 32 bytes for A256GCMKW, got %d", len(keyBytes))
    }
    
    key, err := jwk.Import(keyBytes)
    if err != nil {
        return nil, fmt.Errorf("failed to import key: %w", err)
    }
    
    return key, nil
}
```

### Alternatives Considered
1. **JWT (signed only)**: Rejected - state token contains sensitive PKCE verifier, needs encryption
2. **Custom encryption**: Rejected - violates Constitution Principle III (Library-First Security)
3. **jose-go**: Rejected - less active development than lestrrat-go/jwx

---

## Domain Service Design Pattern

### Decision
Create `OAuth2SessionService` as a domain service in `internal/domain/oauth2session/`.

### Rationale
- **Complex orchestration**: OAuth2 flow spans multiple entities (service config, user session, state tokens)
- **Domain logic ownership**: PKCE generation, state validation, token storage are domain concerns
- **Hexagonal alignment**: Service depends on ports (repositories, encryption), not adapters
- **Testability**: Pure domain logic can be unit tested without HTTP/database dependencies

### Service Structure

```go
// OAuth2SessionService orchestrates OAuth2 authorization flows and session management.
// This is a domain service per DDD patterns - handles complex operations spanning multiple entities.
type OAuth2SessionService struct {
    serviceRepo   ports.ThirdpartyOAuth2ServiceRepository
    sessionRepo   ports.UserSessionRepository
    encryption    ports.EncryptionPort
    jweKey        jwk.Key
    config        OAuth2SessionConfig
    logger        *slog.Logger
}

// OAuth2SessionConfig holds configuration for OAuth2 session management
type OAuth2SessionConfig struct {
    CallbackBaseURL    string        // Base URL for OAuth2 callbacks
    StateTokenTTL      time.Duration // TTL for state tokens (default: 10 min)
    PKCEVerifierLength int           // PKCE verifier length in bytes (default: 32)
    MaxRetries         int           // Max retries for token exchange (default: 3)
}

// NewOAuth2SessionService creates a new OAuth2 session service
func NewOAuth2SessionService(
    serviceRepo ports.ThirdpartyOAuth2ServiceRepository,
    sessionRepo ports.UserSessionRepository,
    encryption ports.EncryptionPort,
    jweKey jwk.Key,
    config OAuth2SessionConfig,
    logger *slog.Logger,
) *OAuth2SessionService {
    return &OAuth2SessionService{
        serviceRepo:   serviceRepo,
        sessionRepo:   sessionRepo,
        encryption:    encryption,
        jweKey:        jweKey,
        config:        config,
        logger:        logger,
    }
}
```

### Service Methods

| Method | Purpose | Returns |
|--------|---------|---------|
| `InitiateOAuth2Flow` | Create PKCE + state token, return auth URL | Authorization URL string |
| `HandleCallback` | Validate state, exchange code, store tokens | UserSession |
| `ListUserSessions` | Get all sessions for a principal | []UserSessionSummary |
| `TerminateSession` | Delete session and tokens for a principal+service | void |
| `GetAffectedAgents` | Count agents depending on a session | int |

### Why Domain Service (not Entity Method)

The OAuth2 flow orchestration doesn't belong to any single entity:
- `ThirdpartyOAuth2Service` - contains OAuth2 config but doesn't own the flow
- `UserSession` - result of the flow but doesn't orchestrate it
- State tokens are ephemeral value objects, not aggregates

A domain service is the correct DDD pattern for operations that:
1. Are stateless (input → output transformation)
2. Span multiple entities
3. Don't naturally belong to any single aggregate

---

## Token Encryption Strategy

### Decision
Use existing `EncryptionPort` with encryption context binding tokens to user and session.

### Rationale
- **Existing infrastructure**: EncryptionPort already implemented (likely AES-256-GCM)
- **Context binding**: Encryption context provides additional authenticated data (AAD)
- **Separation of concerns**: Token encryption is different from JWE state tokens
- **Audit trail**: Encryption context provides metadata for key rotation/audit

### Encryption Context Design

```go
// Token encryption context binds ciphertext to user/session metadata
func createEncryptionContext(principal, serviceID, sessionID string) map[string]string {
    return map[string]string{
        "principal":  principal,
        "service_id": serviceID,
        "session_id": sessionID,
        "purpose":    "oauth2_token",
    }
}

// Encrypt access token before storage
encryptedAccessToken, err := encryption.Encrypt(
    ctx,
    []byte(token.AccessToken),
    createEncryptionContext(principal, serviceID, sessionID),
)
```

### Token Storage Fields

| Field | Type | Purpose |
|-------|------|---------|
| `encrypted_access_token` | bytea | AES-GCM encrypted access token |
| `encrypted_refresh_token` | bytea (nullable) | Stored refresh credential, including rejected credentials |
| `token_type` | string | Token type (usually "Bearer") |
| `access_token_expires_at` | timestamp (nullable) | Recorded access expiry. NULL means unknown, not permanent validity. |
| `refresh_token_rejected` | boolean, NOT NULL DEFAULT FALSE | Observed terminal provider rejection. False does not prove validity. |
| `encryption_context` | jsonb | AAD context for decryption validation |

### Third-Party Credential Validity

The provider token-set expiry describes the access token only. The broker does not predict expiry for opaque third-party refresh credentials.
HTTP 4xx `invalid_grant`, excluding 429, records refresh rejection. The broker retains ciphertext, access expiry, and other metadata.
Recorded rejection immediately stops future token issuance, even with an unexpired access token. Transient refresh errors do not record rejection.
`requires_reauthentication` is true after recorded rejection or access expiry without refresh credentials. `has_refresh_token` reports ciphertext presence, including rejected credentials.
The card shows "Reauthentication required", "Access Token Expired", or "Active", in that order. No refresh-expiry date or permanent-validity claim appears.
Login starts the real authorize/callback flow. Successful authorization upsert clears rejection. Termination remains optional, and normal refresh cannot recover rejected credentials.
Locked callback errors do not persist changes. The service commits rejection through `(true, nil)` before it returns the captured provider error.

---

## Database Schema Design

### Decision
Keep the unique constraint on (principal, service_id). Preserve migrations 004 and 037 unchanged. Use online phases 038 prepare, 039 expand, and 040 contract. Preparation/new brokers use a stable SQL view and invoker write functions at each phase. Drain every unprepared replica before 39 activates durable rejection. The temporary legacy column serves storage compatibility only. Historical dates never indicate provider validity or rejection. See the [online rollout runbook](../../docs/operations/session-sweep.md#online-session-validity-rollout-migrations-038040) for prerequisites, fail-fast lock handling, and guarded rollback.

The release sequence separates these phases into deployable PRs. This cleanup release includes migrations 038, 039, and 040 and the prepared application code. PR1 #224 supplies preparation. PR2 #225 activates durable rejection on already-running PR1 brokers without restart. Migration 040 removes only the unused legacy column. PostgreSQL at 38 supports ordinary traffic but rejects true writes as `persistence_failed`. The memory adapter already supports recorded rejection. This release defaults and limits Helm targets to 40. Fresh installs use direct UP-all through 40, while upgrades use the guarded wrapper. Before merging or deploying PR2, complete PR1 across every replica. Drain every unprepared replica and in-flight operation first. Before merging or deploying PR3, complete PR2 across every replica. End all legacy database dependencies and upgrade the SPA and all external session-API consumers first. API compatibility defaulted true in preparation and activation. Cleanup defaults false and omits the legacy wire field. Explicit true remains a deliberate override for legacy consumers, adding only `is_expired=requires_reauthentication`, never refresh dates. It does not waive the consumer-upgrade gate.

### Current Schema (After Migration 040)

```sql
CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    principal VARCHAR(200) NOT NULL,
    service_id UUID NOT NULL REFERENCES thirdparty_oauth2_services(id) ON DELETE RESTRICT,
    encrypted_access_token BYTEA NOT NULL,
    encrypted_refresh_token BYTEA,
    token_type VARCHAR(50) NOT NULL DEFAULT 'Bearer',
    access_token_expires_at TIMESTAMPTZ,
    refresh_token_rejected BOOLEAN NOT NULL DEFAULT FALSE,
    scope TEXT[] NOT NULL DEFAULT '{}',
    encryption_context JSONB NOT NULL DEFAULT '{}',
    initiated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    CONSTRAINT user_sessions_principal_service_unique UNIQUE (principal, service_id)
);

CREATE INDEX idx_user_sessions_principal ON user_sessions(principal);
CREATE INDEX idx_user_sessions_service_id ON user_sessions(service_id);
```

### Rationale for Unique Constraint
- **One session per user per service**: Prevents token duplication and race conditions
- **Upsert semantics**: Successful reauthorization replaces credentials and clears recorded rejection while it preserves the row identity and initiation time.
- **Concurrent callbacks**: The unique constraint permits one row. Each successful callback uses the same authorization upsert.

### ON DELETE RESTRICT Rationale
- **Prevents orphaned sessions**: A service cannot be deleted while retained session rows reference it, including rejected sessions.
- **Admin workflow**: Admin must terminate all sessions before deleting service
- **Error handling**: Repository catches constraint violation, returns user-friendly error

---

## Error Handling Patterns

### Decision
Define domain-specific errors for OAuth2 session operations.

### Error Types

```go
package oauth2session

import "errors"

var (
    // State token errors
    ErrStateTokenExpired     = errors.New("state token has expired")
    ErrStateTokenInvalid     = errors.New("state token is invalid or tampered")
    ErrPrincipalMismatch     = errors.New("principal does not match state token")
    ErrServiceIDMismatch     = errors.New("service ID does not match state token")
    
    // OAuth2 flow errors
    ErrOAuth2AuthError       = errors.New("authorization server returned error")
    ErrTokenExchangeFailed   = errors.New("failed to exchange authorization code for tokens")
    ErrServiceNotFound       = errors.New("third-party service not found")
    
    // Session errors
    ErrSessionNotFound       = errors.New("session not found")
    ErrSessionAlreadyExists  = errors.New("session already exists for this service")
    
    // Validation errors
    ErrInvalidRedirectURI    = errors.New("redirect URI is not allowed")
    ErrMissingAuthCode       = errors.New("authorization code is missing")
)

// OAuth2Error wraps OAuth2 error responses from authorization servers
type OAuth2Error struct {
    Code        string // OAuth2 error code (e.g., "access_denied")
    Description string // Human-readable description
    URI         string // Optional URI with more info
}

func (e *OAuth2Error) Error() string {
    if e.Description != "" {
        return fmt.Sprintf("%s: %s", e.Code, e.Description)
    }
    return e.Code
}
```

### HTTP Error Mapping

| Domain Error | HTTP Status | Response |
|--------------|-------------|----------|
| ErrStateTokenExpired | 400 Bad Request | Retry flow |
| ErrStateTokenInvalid | 400 Bad Request | Security event logged |
| ErrPrincipalMismatch | 403 Forbidden | Security event logged |
| ErrOAuth2AuthError | Redirect | Error params passed to UI |
| ErrServiceNotFound | 404 Not Found | Standard error response |
| ErrSessionNotFound | 404 Not Found | Standard error response |
| ErrRefreshRejected (manual refresh) | 409 Conflict | `refresh_rejected`: `refresh token rejected by provider; reauthentication required` |
| ErrRefreshNotAvailable (missing refresh credentials) | 409 Conflict | `refresh_unavailable` |
| ErrRefreshFailed (transient upstream error) | 502 Bad Gateway | `refresh_failed` |

---

## Retry Logic for Token Exchange

### Decision
Implement exponential backoff retry for token exchange (1s, 2s, 4s delays).

### Rationale
- **Network resilience**: Token exchange may fail due to transient network issues
- **Third-party reliability**: OAuth2 providers may have temporary outages
- **User experience**: Automatic retry prevents unnecessary manual retries

### Implementation

```go
// retryWithBackoff attempts an operation with exponential backoff
func (s *OAuth2SessionService) retryWithBackoff(
    ctx context.Context,
    operation func() (*oauth2.Token, error),
) (*oauth2.Token, error) {
    delays := []time.Duration{
        1 * time.Second,
        2 * time.Second,
        4 * time.Second,
    }
    
    var lastErr error
    for attempt, delay := range delays {
        token, err := operation()
        if err == nil {
            return token, nil
        }
        
        lastErr = err
        s.logger.Warn("token exchange attempt failed",
            "attempt", attempt+1,
            "max_attempts", len(delays),
            "error", err,
            "next_retry_in", delay)
        
        // Check if context cancelled before sleeping
        select {
        case <-ctx.Done():
            return nil, ctx.Err()
        case <-time.After(delay):
            // Continue to next attempt
        }
    }
    
    // All retries exhausted
    return nil, fmt.Errorf("token exchange failed after %d attempts: %w", len(delays), lastErr)
}
```

### Final Attempt (no delay after)

```go
// After 3 delays, make one final attempt
token, err := operation()
if err == nil {
    return token, nil
}
return nil, fmt.Errorf("token exchange failed after %d attempts: %w", len(delays)+1, lastErr)
```

---

## Summary of Technology Choices

| Component | Technology | Version | Purpose |
|-----------|------------|---------|---------|
| OAuth2 Client | golang.org/x/oauth2 | latest | Authorization code flow with PKCE |
| JWE Encryption | github.com/lestrrat-go/jwx/v3 | v3.x | State token encryption/decryption |
| Token Encryption | Existing EncryptionPort | - | Access/refresh token encryption |
| Database | PostgreSQL + sqlx | 15+ | Session persistence |
| HTTP Router | Chi | v5.x | HTTP handlers |
| Configuration | Viper | 1.19+ | JWE key, TTL settings |

---

## Open Questions Resolved

| Question | Resolution |
|----------|------------|
| How to handle PKCE? | Manual generation using crypto/rand + SHA256 |
| Which JWE algorithm? | A256GCMKW (key wrap) + A256GCM (content encryption) |
| Where does OAuth2 orchestration live? | Domain service (OAuth2SessionService) |
| How to handle race conditions? | Database unique constraint (principal, service_id) |
| Retry strategy for token exchange? | 3 retries with exponential backoff (1s, 2s, 4s) |
| How to bind tokens to user? | Encryption context with principal, service_id, session_id |
