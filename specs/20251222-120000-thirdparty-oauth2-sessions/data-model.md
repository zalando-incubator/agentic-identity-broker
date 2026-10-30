# Data Model: Third-Party OAuth2 Session Management

**Feature Branch**: `20251222-120000-thirdparty-oauth2-sessions`  
**Date**: 2025-12-23  
**Purpose**: Document domain entities, aggregates, value objects, and relationships

## Table of Contents

1. [Domain Overview](#domain-overview)
2. [Entities](#entities)
3. [Value Objects](#value-objects)
4. [Domain Service](#domain-service)
5. [Domain Events](#domain-events)
6. [Repository Interfaces](#repository-interfaces)
7. [State Transitions](#state-transitions)
8. [Database Schema](#database-schema)

---

## Domain Overview

The OAuth2 Session domain enables users to authenticate with third-party OAuth2 services through the identity broker. The broker orchestrates the OAuth2 Authorization Code flow with PKCE, securely stores encrypted tokens, and provides session lifecycle management.

### Aggregate Boundaries

```
┌─────────────────────────────────────────────────────────────┐
│                    UserSession (Aggregate Root)             │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ • Owns encrypted tokens                              │   │
│  │ • Manages session lifecycle                          │   │
│  │ • Tracks expiration status                           │   │
│  └─────────────────────────────────────────────────────┘   │
│                           │                                  │
│                           ▼                                  │
│  ┌─────────────────────────────────────────────────────┐   │
│  │         EncryptedToken (Value Object)                │   │
│  │ • Ciphertext + encryption context                    │   │
│  │ • Immutable after creation                           │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│              OAuth2StateToken (Value Object)                │
│  ┌─────────────────────────────────────────────────────┐   │
│  │ • JWE-encrypted ephemeral flow state                 │   │
│  │ • Short-lived (10 min TTL)                           │   │
│  │ • Binds callback to initiating request               │   │
│  └─────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### Entity Relationships

```
ThirdpartyOAuth2Service (existing)        UserGrant (existing)
        │                                        │
        │ 1:N                                    │ references
        ▼                                        ▼
    UserSession ◄───────────────────────────────┘
        │
        │ contains
        ▼
  EncryptedToken (access + refresh)
```

---

## Entities

### UserSession (Aggregate Root)

Represents an authenticated OAuth2 session between a user (principal) and a third-party service. Owns the encrypted tokens and manages session lifecycle.

**Location**: `internal/domain/storage/user_session.go`

```go
package storage

import (
    "database/sql/driver"
    "encoding/json"
    "errors"
    "time"
)

// UserSession represents an authenticated OAuth2 session between a user and a third-party service.
// This is an aggregate root - it owns the encrypted tokens and manages session lifecycle.
// One session per (principal, service_id) pair, enforced by database unique constraint.
type UserSession struct {
    ID                     string             `json:"id" db:"id"`
    Principal              string             `json:"principal" db:"principal"`
    ServiceID              string             `json:"service_id" db:"service_id"`
    EncryptedAccessToken   []byte             `json:"-" db:"encrypted_access_token"`
    EncryptedRefreshToken  []byte             `json:"-" db:"encrypted_refresh_token"`
    TokenType              string             `json:"token_type" db:"token_type"`
    AccessTokenExpiresAt   *time.Time         `json:"access_token_expires_at,omitempty" db:"access_token_expires_at"`
    RefreshTokenExpiresAt  *time.Time         `json:"refresh_token_expires_at,omitempty" db:"refresh_token_expires_at"`
    Scope                  []string           `json:"scope" db:"scope"`
    EncryptionContext      EncryptionContext  `json:"encryption_context" db:"encryption_context"`
    InitiatedAt            time.Time          `json:"initiated_at" db:"initiated_at"`
    CreatedAt              time.Time          `json:"created_at" db:"created_at"`
    UpdatedAt              time.Time          `json:"updated_at" db:"updated_at"`
}

// EncryptionContext holds metadata for token encryption/decryption.
// This is stored in JSONB and included as AAD (Additional Authenticated Data).
type EncryptionContext struct {
    Principal  string `json:"principal"`
    ServiceID  string `json:"service_id"`
    SessionID  string `json:"session_id"`
    Purpose    string `json:"purpose"` // Always "oauth2_token"
}

// Value implements driver.Valuer for EncryptionContext (JSONB serialization).
func (ec EncryptionContext) Value() (driver.Value, error) {
    return json.Marshal(ec)
}

// Scan implements sql.Scanner for EncryptionContext (JSONB deserialization).
func (ec *EncryptionContext) Scan(value interface{}) error {
    if value == nil {
        return nil
    }
    bytes, ok := value.([]byte)
    if !ok {
        return errors.New("invalid type for EncryptionContext")
    }
    return json.Unmarshal(bytes, ec)
}
```

**Invariants**:
- `Principal` must be non-empty, max 200 characters
- `ServiceID` must reference existing ThirdpartyOAuth2Service
- `EncryptedAccessToken` must be non-empty (access token required)
- `TokenType` defaults to "Bearer"
- `InitiatedAt` set on creation, immutable

**Business Methods**:

```go
// Validate performs domain validation on UserSession.
func (s *UserSession) Validate() error {
    if s.ID == "" {
        return errors.New("session ID cannot be empty")
    }
    if s.Principal == "" {
        return errors.New("principal is required")
    }
    if len(s.Principal) > 200 {
        return errors.New("principal exceeds 200 characters")
    }
    if s.ServiceID == "" {
        return errors.New("service_id is required")
    }
    if len(s.EncryptedAccessToken) == 0 {
        return errors.New("encrypted access token is required")
    }
    if s.TokenType == "" {
        return errors.New("token_type is required")
    }
    return nil
}

// IsExpired returns true if the session's refresh token has expired.
// Access token expiration does not mark session as expired (future auto-refresh).
// Session without refresh token expiration is considered non-expiring.
func (s *UserSession) IsExpired() bool {
    if s.RefreshTokenExpiresAt == nil {
        return false // No expiration set
    }
    return time.Now().After(*s.RefreshTokenExpiresAt)
}

// HasValidAccessToken returns true if access token has not expired.
func (s *UserSession) HasValidAccessToken() bool {
    if s.AccessTokenExpiresAt == nil {
        return true // No expiration set
    }
    return time.Now().Before(*s.AccessTokenExpiresAt)
}

// CanRefresh returns true if session has a refresh token that hasn't expired.
func (s *UserSession) CanRefresh() bool {
    if len(s.EncryptedRefreshToken) == 0 {
        return false // No refresh token
    }
    if s.RefreshTokenExpiresAt == nil {
        return true // No expiration set
    }
    return time.Now().Before(*s.RefreshTokenExpiresAt)
}
```

---

### UserSessionSummary (Read Model)

A projection of UserSession for API responses, including dependent agent count.

```go
// UserSessionSummary is a read model for displaying session information.
// Includes computed fields like dependent agent count.
type UserSessionSummary struct {
    ID                    string              `json:"id"`
    ServiceID             string              `json:"service_id"`
    ServiceDisplayName    string              `json:"service_display_name"`
    TokenType             string              `json:"token_type"`
    Scope                 []string            `json:"scope"`
    InitiatedAt           time.Time           `json:"initiated_at"`
    IsExpired             bool                `json:"is_expired"`
    AccessTokenExpired    bool                `json:"access_token_expired"`
    RefreshTokenExpiresAt *time.Time          `json:"refresh_token_expires_at,omitempty"`
    DependentAgentCount   int                 `json:"dependent_agent_count"`
    IsEncrypted           bool                `json:"is_encrypted"` // Always true
}

// NewUserSessionSummary creates a summary from a UserSession.
func NewUserSessionSummary(session *UserSession, serviceDisplayName string, agentCount int) *UserSessionSummary {
    return &UserSessionSummary{
        ID:                    session.ID,
        ServiceID:             session.ServiceID,
        ServiceDisplayName:    serviceDisplayName,
        TokenType:             session.TokenType,
        Scope:                 session.Scope,
        InitiatedAt:           session.InitiatedAt,
        IsExpired:             session.IsExpired(),
        AccessTokenExpired:    !session.HasValidAccessToken(),
        RefreshTokenExpiresAt: session.RefreshTokenExpiresAt,
        DependentAgentCount:   agentCount,
        IsEncrypted:           true,
    }
}
```

---

## Value Objects

### OAuth2StateToken

Ephemeral JWE-encrypted token that binds an OAuth2 callback to the initiating request. Contains PKCE verifier and user identity.

**Location**: `internal/domain/oauth2session/state_token.go`

```go
package oauth2session

import (
    "encoding/json"
    "time"
)

// OAuth2StateTokenClaims contains the claims embedded in a JWE state token.
// These claims bind the OAuth2 callback to the initiating request.
type OAuth2StateTokenClaims struct {
    // Principal is the authenticated user who initiated the OAuth2 flow.
    // Must match the principal at callback time (CSRF protection).
    Principal string `json:"principal"`
    
    // PKCEVerifier is the PKCE code verifier for token exchange.
    // Base64url-encoded, 32-128 bytes per RFC 7636.
    PKCEVerifier string `json:"pkce_verifier"`
    
    // ServiceID is the third-party service being authorized.
    // Must match the serviceId path parameter at callback.
    ServiceID string `json:"service_id"`
    
    // RedirectURI is where to redirect after flow completes.
    // Must be same-origin with the authorize request.
    RedirectURI string `json:"redirect_uri"`
    
    // IssuedAt is when the token was created.
    IssuedAt time.Time `json:"iat"`
    
    // ExpiresAt is when the token expires (TTL: 10 min default).
    ExpiresAt time.Time `json:"exp"`
}

// Validate checks that all required claims are present.
func (c *OAuth2StateTokenClaims) Validate() error {
    if c.Principal == "" {
        return errors.New("principal is required")
    }
    if c.PKCEVerifier == "" {
        return errors.New("pkce_verifier is required")
    }
    if c.ServiceID == "" {
        return errors.New("service_id is required")
    }
    if c.RedirectURI == "" {
        return errors.New("redirect_uri is required")
    }
    if c.IssuedAt.IsZero() {
        return errors.New("iat is required")
    }
    if c.ExpiresAt.IsZero() {
        return errors.New("exp is required")
    }
    return nil
}

// IsExpired returns true if the token has expired.
func (c *OAuth2StateTokenClaims) IsExpired() bool {
    return time.Now().After(c.ExpiresAt)
}
```

**Characteristics**:
- **Immutable**: Created once, never modified
- **Short-lived**: TTL of 10 minutes (configurable, max 15 minutes)
- **Encrypted**: JWE with A256GCMKW + A256GCM
- **Self-validating**: Contains expiration and all data needed for validation

---

### PKCE

Uses types from `golang.org/x/oauth2` but provides helper functions.

```go
package oauth2session

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
)

// GeneratePKCE creates a PKCE code verifier and challenge per RFC 7636.
// verifierLength should be 32-128 bytes (recommended: 32 = 256 bits).
func GeneratePKCE(verifierLength int) (verifier, challenge string, err error) {
    if verifierLength < 32 || verifierLength > 128 {
        return "", "", errors.New("verifier length must be 32-128 bytes")
    }
    
    // Generate cryptographically random bytes
    randomBytes := make([]byte, verifierLength)
    if _, err := rand.Read(randomBytes); err != nil {
        return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
    }
    
    // Base64url encode without padding
    verifier = base64.RawURLEncoding.EncodeToString(randomBytes)
    
    // SHA256 hash of verifier, base64url encoded
    hash := sha256.Sum256([]byte(verifier))
    challenge = base64.RawURLEncoding.EncodeToString(hash[:])
    
    return verifier, challenge, nil
}
```

---

## Domain Service

### OAuth2SessionService

Orchestrates OAuth2 authorization flows and session management. This is a domain service per DDD patterns - handles complex operations spanning multiple entities.

**Location**: `internal/domain/oauth2session/service.go`

```go
package oauth2session

import (
    "context"
    "log/slog"
    "time"
    
    "github.com/lestrrat-go/jwx/v3/jwk"
    "golang.org/x/oauth2"
    
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// OAuth2SessionService orchestrates OAuth2 authorization flows and session management.
type OAuth2SessionService struct {
    serviceRepo   ports.ThirdpartyOAuth2ServiceRepository
    sessionRepo   ports.UserSessionRepository
    grantRepo     ports.UserGrantRepository  // For counting dependent agents
    encryption    ports.EncryptionPort
    jweKey        jwk.Key
    config        Config
    logger        *slog.Logger
}

// Config holds configuration for the OAuth2 session service.
type Config struct {
    CallbackBaseURL    string        // e.g., "https://broker.example.com"
    StateTokenTTL      time.Duration // Default: 10 minutes
    PKCEVerifierLength int           // Default: 32 bytes
    MaxRetries         int           // Default: 3
    RetryBaseDelay     time.Duration // Default: 1 second
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() Config {
    return Config{
        StateTokenTTL:      10 * time.Minute,
        PKCEVerifierLength: 32,
        MaxRetries:         3,
        RetryBaseDelay:     time.Second,
    }
}
```

**Service Methods**:

```go
// InitiateFlowResult contains the data needed to redirect user to authorization.
type InitiateFlowResult struct {
    AuthorizationURL string // Full URL to redirect user to
}

// InitiateOAuth2Flow starts the OAuth2 authorization code flow.
// Creates PKCE verifier/challenge, generates JWE state token, returns auth URL.
func (s *OAuth2SessionService) InitiateOAuth2Flow(
    ctx context.Context,
    principal string,
    serviceID string,
    redirectURI string,
) (*InitiateFlowResult, error)

// HandleCallbackRequest contains parameters from the OAuth2 callback.
type HandleCallbackRequest struct {
    ServiceID   string // From URL path
    Code        string // Authorization code from query
    State       string // JWE state token from query
    Error       string // OAuth2 error code (optional)
    ErrorDesc   string // OAuth2 error description (optional)
}

// HandleCallback processes the OAuth2 callback, exchanges code for tokens, stores session.
func (s *OAuth2SessionService) HandleCallback(
    ctx context.Context,
    principal string,
    req *HandleCallbackRequest,
) (*storage.UserSession, error)

// ListUserSessions returns all sessions for a principal with summary info.
func (s *OAuth2SessionService) ListUserSessions(
    ctx context.Context,
    principal string,
) ([]*UserSessionSummary, error)

// TerminateSession deletes a session and its encrypted tokens.
func (s *OAuth2SessionService) TerminateSession(
    ctx context.Context,
    principal string,
    serviceID string,
) error

// GetSessionWithAgents returns session details including list of dependent agents.
type SessionWithAgents struct {
    Session        *storage.UserSession
    DependentAgents []string // Agent IDs that use this session
}

func (s *OAuth2SessionService) GetSessionWithAgents(
    ctx context.Context,
    principal string,
    serviceID string,
) (*SessionWithAgents, error)
```

---

## Domain Events

Domain events capture significant state changes for audit logging and potential future event-driven features.

```go
package oauth2session

import "time"

// SessionEstablishedEvent is raised when a user successfully completes OAuth2 flow.
type SessionEstablishedEvent struct {
    SessionID   string    `json:"session_id"`
    Principal   string    `json:"principal"`
    ServiceID   string    `json:"service_id"`
    Scope       []string  `json:"scope"`
    EstablishedAt time.Time `json:"established_at"`
}

// SessionTerminatedEvent is raised when a user explicitly terminates a session.
type SessionTerminatedEvent struct {
    SessionID     string    `json:"session_id"`
    Principal     string    `json:"principal"`
    ServiceID     string    `json:"service_id"`
    TerminatedAt  time.Time `json:"terminated_at"`
    AffectedAgents []string `json:"affected_agents"`
}

// StateValidationFailedEvent is raised for security audit when state validation fails.
type StateValidationFailedEvent struct {
    Principal       string    `json:"principal"`
    ServiceID       string    `json:"service_id"`
    FailureReason   string    `json:"failure_reason"`
    RemoteAddr      string    `json:"remote_addr"`
    OccurredAt      time.Time `json:"occurred_at"`
}
```

---

## Repository Interfaces

### UserSessionRepository

**Location**: `internal/ports/storage.go` (extend existing file)

```go
// UserSessionRepository defines storage operations for user OAuth2 sessions.
// One session per (principal, service_id) pair.
type UserSessionRepository interface {
    // Create creates a new user session.
    // Uses upsert semantics: if session exists for (principal, service_id), replaces tokens.
    // Returns error if:
    // - Service ID doesn't exist (StorageError with Kind=NotFound via FK constraint)
    // - Storage connection fails (StorageError with Kind=Connection)
    // - Operation timeout (StorageError with Kind=Timeout)
    Create(ctx context.Context, session *storage.UserSession) error
    
    // Get retrieves a session by ID.
    // Returns StorageError with Kind=NotFound if session not found.
    Get(ctx context.Context, id string) (*storage.UserSession, error)
    
    // FindByPrincipalAndService retrieves the session for a principal and service.
    // Returns nil if no session exists (not an error).
    FindByPrincipalAndService(ctx context.Context, principal, serviceID string) (*storage.UserSession, error)
    
    // ListByPrincipal retrieves all sessions for a principal.
    // Returns empty slice if no sessions exist (not an error).
    ListByPrincipal(ctx context.Context, principal string) ([]*storage.UserSession, error)
    
    // Delete deletes a session by ID.
    // Returns error if storage operation fails.
    // Idempotent: safe to delete non-existent session.
    Delete(ctx context.Context, id string) error
    
    // DeleteByPrincipalAndService deletes the session for a principal and service.
    // Returns error if storage operation fails.
    // Idempotent: safe to delete non-existent session.
    DeleteByPrincipalAndService(ctx context.Context, principal, serviceID string) error
    
    // CountByService counts sessions referencing a service.
    // Used to enforce deletion protection (cannot delete service with active sessions).
    CountByService(ctx context.Context, serviceID string) (int, error)
}
```

---

## State Transitions

### Session Lifecycle

```
┌─────────────┐                              ┌─────────────┐
│   No        │  InitiateOAuth2Flow()        │   Flow      │
│   Session   │ ───────────────────────────► │   Started   │
└─────────────┘                              └──────┬──────┘
                                                    │
                                         User approves at OAuth2 provider
                                                    │
                                                    ▼
                                             ┌──────────────┐
                         HandleCallback()   │   Active     │
                         ◄──────────────────│   Session    │
                                             └──────┬──────┘
                                                    │
                    ┌───────────────────────────────┼───────────────────────────────┐
                    │                               │                               │
                    ▼                               ▼                               ▼
            ┌──────────────┐              ┌──────────────┐              ┌──────────────┐
            │   Expired    │              │  Terminated  │              │   Active     │
            │   Session    │              │   Session    │              │   Session    │
            │ (refresh     │              │   (deleted)  │              │ (re-auth)    │
            │  token exp)  │              └──────────────┘              └──────────────┘
            └──────┬───────┘                                                    │
                   │                                                            │
                   ▼                                                            │
         User must terminate                                                    │
         and re-authenticate                                                    │
                   │                                                            │
                   └────────────────────────────────────────────────────────────┘
```

### OAuth2 Flow State Machine

```
┌────────────────────────────────────────────────────────────────────────────┐
│                        OAuth2 Authorization Flow                           │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│  [Start] ─► Generate PKCE ─► Create State Token ─► Build Auth URL         │
│                                                           │                │
│                                                           ▼                │
│                                              Redirect to OAuth2 Provider   │
│                                                           │                │
│                                                           ▼                │
│                                              User approves/denies          │
│                                              at provider                   │
│                                                           │                │
│                              ┌────────────────────────────┴───────┐        │
│                              │                                    │        │
│                              ▼                                    ▼        │
│                     [Callback: code]                    [Callback: error]  │
│                              │                                    │        │
│                              ▼                                    ▼        │
│                     Validate State Token               Parse OAuth2 Error  │
│                     (decrypt, check principal,                    │        │
│                      check service_id, check exp)                 │        │
│                              │                                    │        │
│                    ┌─────────┴─────────┐                         │        │
│                    │                   │                         │        │
│                    ▼                   ▼                         ▼        │
│              [Valid]             [Invalid]              [Redirect to UI   │
│                    │                   │                with error params] │
│                    │                   │                                   │
│                    ▼                   ▼                                   │
│         Exchange Code        Log Security Event                            │
│         for Tokens          Reject with 400/403                           │
│         (with PKCE)                                                        │
│              │                                                             │
│              ▼                                                             │
│    ┌─────────┴─────────┐                                                   │
│    │                   │                                                   │
│    ▼                   ▼                                                   │
│ [Success]        [Failure]                                                 │
│    │                   │                                                   │
│    ▼                   ▼                                                   │
│ Encrypt tokens    Retry up to 3x                                          │
│ Store session     with backoff                                            │
│ Redirect to UI    (1s, 2s, 4s)                                            │
│ with success           │                                                   │
│                        ▼                                                   │
│              [All retries failed]                                          │
│                        │                                                   │
│                        ▼                                                   │
│              Redirect to UI with error                                     │
│                                                                            │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Database Schema

### Migration: 004_create_user_sessions.up.sql

```sql
-- Migration: 004_create_user_sessions.up.sql
-- Purpose: Create user_sessions table for storing OAuth2 session data
-- Feature: 20251222-120000-thirdparty-oauth2-sessions

CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    principal VARCHAR(200) NOT NULL,
    service_id UUID NOT NULL,
    encrypted_access_token BYTEA NOT NULL,
    encrypted_refresh_token BYTEA,
    token_type VARCHAR(50) NOT NULL DEFAULT 'Bearer',
    access_token_expires_at TIMESTAMPTZ,
    refresh_token_expires_at TIMESTAMPTZ,
    scope TEXT[] NOT NULL DEFAULT '{}',
    encryption_context JSONB NOT NULL DEFAULT '{}',
    initiated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Foreign key to thirdparty_oauth2_services (ON DELETE RESTRICT)
    -- Cannot delete service with active sessions
    CONSTRAINT fk_user_sessions_service 
        FOREIGN KEY (service_id) 
        REFERENCES thirdparty_oauth2_services(id) 
        ON DELETE RESTRICT,
    
    -- Unique constraint: one session per user per service
    -- Prevents race conditions; first callback wins
    CONSTRAINT user_sessions_principal_service_unique 
        UNIQUE (principal, service_id)
);

-- Index for fast lookups by principal (list user's sessions)
CREATE INDEX idx_user_sessions_principal ON user_sessions(principal);

-- Index for counting sessions by service (deletion protection)
CREATE INDEX idx_user_sessions_service_id ON user_sessions(service_id);

-- Trigger to update updated_at on row modification
CREATE OR REPLACE FUNCTION update_user_sessions_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_user_sessions_updated_at
    BEFORE UPDATE ON user_sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_user_sessions_updated_at();

COMMENT ON TABLE user_sessions IS 'Stores OAuth2 sessions between users and third-party services';
COMMENT ON COLUMN user_sessions.principal IS 'Authenticated user identifier (email, username, etc.)';
COMMENT ON COLUMN user_sessions.service_id IS 'Reference to thirdparty_oauth2_services';
COMMENT ON COLUMN user_sessions.encrypted_access_token IS 'AES-GCM encrypted OAuth2 access token';
COMMENT ON COLUMN user_sessions.encrypted_refresh_token IS 'AES-GCM encrypted OAuth2 refresh token (nullable)';
COMMENT ON COLUMN user_sessions.encryption_context IS 'AAD context for token encryption/decryption';
COMMENT ON COLUMN user_sessions.initiated_at IS 'When the OAuth2 session was first established';
```

### Migration: 004_create_user_sessions.down.sql

```sql
-- Migration: 004_create_user_sessions.down.sql
-- Purpose: Rollback user_sessions table creation
-- Feature: 20251222-120000-thirdparty-oauth2-sessions

DROP TRIGGER IF EXISTS trigger_user_sessions_updated_at ON user_sessions;
DROP FUNCTION IF EXISTS update_user_sessions_updated_at();
DROP TABLE IF EXISTS user_sessions;
```

---

## Glossary Additions

The following terms should be added to `ARCHITECTURE.md` Glossary section:

| Term | Definition |
|------|------------|
| **UserSession** | An authenticated OAuth2 session between a user (principal) and a third-party service. Contains encrypted access/refresh tokens, scope, and expiration metadata. One session per (principal, service_id) pair. |
| **OAuth2StateToken** | A JWE-encrypted ephemeral token that binds an OAuth2 callback to the initiating request. Contains principal, PKCE verifier, service_id, and redirect_uri. Short-lived (10 min TTL) to limit exposure. |
| **PKCE** | Proof Key for Code Exchange (RFC 7636). Security extension for OAuth2 that prevents authorization code interception attacks. Uses code_verifier (random secret) and code_challenge (SHA256 hash of verifier). |
| **Token Vault** | Secure storage for encrypted OAuth2 tokens. Tokens are encrypted using AES-GCM with encryption context binding them to principal, service, and session. |
| **Session Termination** | User-initiated action to delete their OAuth2 session with a third-party service. Removes encrypted tokens from storage and displays warning about affected agents. |
| **OAuth2SessionService** | Domain service that orchestrates OAuth2 authorization flows. Handles PKCE generation, state token management, token exchange, and session lifecycle. |
| **Encryption Context** | Additional authenticated data (AAD) included in token encryption. Binds ciphertext to principal, service_id, and session_id. Used for auditing and prevents cross-context token usage. |
