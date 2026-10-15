# Quickstart Guide: Third-Party OAuth2 Session Management

**Feature Branch**: `008-thirdparty-oauth2-sessions`  
**Date**: 2025-12-23  
**Audience**: Developers implementing this feature  
**Purpose**: Step-by-step implementation guide

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Phase 1: Database Migration](#phase-1-database-migration)
3. [Phase 2: Domain Types](#phase-2-domain-types)
4. [Phase 3: Repository Interface](#phase-3-repository-interface)
5. [Phase 4: Storage Adapters](#phase-4-storage-adapters)
6. [Phase 5: Domain Service](#phase-5-domain-service)
7. [Phase 6: HTTP Handlers](#phase-6-http-handlers)
8. [Phase 7: Configuration](#phase-7-configuration)
9. [Phase 8: Frontend Components](#phase-8-frontend-components)
10. [Testing Strategy](#testing-strategy)

---

## Prerequisites

Before implementing, ensure you have:

1. **Dependencies installed**:
   ```bash
   go get golang.org/x/oauth2
   go get github.com/lestrrat-go/jwx/v3
   ```

2. **Existing infrastructure**:
   - `EncryptionPort` implemented (from previous features)
   - `ThirdpartyOAuth2ServiceRepository` implemented
   - `UserGrantRepository` implemented
   - Principal middleware working

3. **Read these documents**:
   - [research.md](research.md) - Technology decisions
   - [data-model.md](data-model.md) - Domain model
   - [contracts/oauth2-sessions.yaml](contracts/oauth2-sessions.yaml) - API contract

---

## Phase 1: Database Migration

Migration 004 is immutable history. Do not change its file to match the current model.
This cleanup release includes migrations 038, 039, and 040. Migration 040 removes only the unused legacy column.
Contributor model samples use access expiry and observed refresh rejection only. At 39 and 40, prepared brokers persist rejection through the stable boundary.
PostgreSQL at 38 exposes false through the stable view and rejects true writes.

### Step 1.1: Current Schema Reference (After Migration 040)

**Historical migration**: `migrations/004_create_user_sessions.up.sql` remains unchanged. This sample describes this release's phase-40 table without the legacy column.

```sql
-- Current schema reference, not a replacement for migration 004.
-- Feature: 008-thirdparty-oauth2-sessions

CREATE TABLE user_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    principal VARCHAR(200) NOT NULL,
    service_id UUID NOT NULL,
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
    
    CONSTRAINT fk_user_sessions_service 
        FOREIGN KEY (service_id) 
        REFERENCES thirdparty_oauth2_services(id) 
        ON DELETE RESTRICT,
    
    CONSTRAINT user_sessions_principal_service_unique 
        UNIQUE (principal, service_id)
);

CREATE INDEX idx_user_sessions_principal ON user_sessions(principal);
CREATE INDEX idx_user_sessions_service_id ON user_sessions(service_id);

-- Update trigger
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
```

### Step 1.2: Original Down Migration (History)

**File**: `migrations/004_create_user_sessions.down.sql`

```sql
DROP TRIGGER IF EXISTS trigger_user_sessions_updated_at ON user_sessions;
DROP FUNCTION IF EXISTS update_user_sessions_updated_at();
DROP TABLE IF EXISTS user_sessions;
```

### Sequential Validity Releases: 038, Then 039, Then 040

Migration 038 preserves the original table shape for legacy strict readers and writes. It adds `user_session_state` and stable invoker write functions. The view reports rejection=false. True rejection writes report `persistence_failed`, not durable rejection. Ordinary acquisition, refresh, and authorization remain supported. The memory adapter already supports rejection.

1. With existing replicas serving, deploy PR1 #224 through a normal Helm upgrade at its default target 38.
2. Complete the prepared broker and embedded SPA rollout across every replica.
3. Before merging or deploying PR2 #225, verify that PR1 completed and all unprepared replicas and in-flight operations drained.
4. Deploy PR2 normally at its default target 39 to activate durable rejection on already-running PR1 brokers.
5. Before merging or deploying PR3, complete PR2 across every replica and upgrade the SPA and every external session-API consumer.
6. After all legacy database dependencies end, deploy PR3 at its default target 40 and compatibility false.

This release defaults and limits `migration.targetVersion` to 40. Fresh installs run direct UP-all through 40. Normal upgrades use the guarded forward wrapper. Explicit target 0 remains refused on PostgreSQL upgrades. Compatibility defaulted true in preparation and activation. Cleanup defaults false and omits the legacy wire field. Explicit true remains a deliberate override for legacy consumers, adding only `is_expired=requires_reauthentication`, without refresh dates. It does not waive the consumer-upgrade gate.

Unprepared binaries do not enforce the new rejection policy and cannot coexist at 39 or 40. The legacy column at 38 and 39 serves storage compatibility only, never provider-validity evidence. Migration 040 removes it. Historical dates never imply rejection. All phases preserve rows, ciphertext, access expiry, and other token metadata. Preserve migration 037 and original migration history.

Online rollout requires at least two healthy replicas, `maxUnavailable=0`, ready surge capacity, a disruption budget, and sufficient traffic-drain and termination time. Fail-fast locks abort schema changes on contention while compatible replicas continue to serve. There is no absolute latency guarantee.

This release supplies DOWN 038, guarded DOWN 039, and DOWN 040. DOWN 040 restores only a nullable legacy column and retains rejection. DOWN 039 refuses while rejected rows remain. Successful reauthentication or explicit termination must resolve those rows. Drain preparation/new replicas before DOWN 038 removes their SQL boundary. Inspect actual schema and dirty metadata before deliberate repair. Never automate `Force`, clear rejection, or delete rows automatically. See the [online rollout runbook](../../docs/operations/session-sweep.md#online-session-validity-rollout-migrations-038040) for phase compatibility and cleanup values.

### Step 1.3: Test Migration

Use a disposable database for these commands. DOWN 039 requires the rejection guard to pass. Live rollout must select explicit phase targets.

```bash
# Apply migration
just migrate-up

# Verify table created
psql $DATABASE_URL -c "\d user_sessions"

# Rollback
just migrate-down

# Re-apply
just migrate-up
```

---

## Phase 2: Domain Types

### Step 2.1: Create UserSession Entity

**File**: `internal/domain/storage/user_session.go`

```go
package storage

import (
    "database/sql/driver"
    "encoding/json"
    "errors"
    "time"
)

// UserSession represents an authenticated OAuth2 session between a user and a third-party service.
type UserSession struct {
    ID                     string            `json:"id" db:"id"`
    Principal              string            `json:"principal" db:"principal"`
    ServiceID              string            `json:"service_id" db:"service_id"`
    EncryptedAccessToken   []byte            `json:"-" db:"encrypted_access_token"`
    EncryptedRefreshToken  []byte            `json:"-" db:"encrypted_refresh_token"`
    TokenType              string            `json:"token_type" db:"token_type"`
    AccessTokenExpiresAt   *time.Time        `json:"access_token_expires_at,omitempty" db:"access_token_expires_at"`
    RefreshTokenRejected   bool              `json:"-" db:"refresh_token_rejected"`
    Scope                  StringSlice       `json:"scope" db:"scope"`
    EncryptionContext      EncryptionContext `json:"encryption_context" db:"encryption_context"`
    InitiatedAt            time.Time         `json:"initiated_at" db:"initiated_at"`
    CreatedAt              time.Time         `json:"created_at" db:"created_at"`
    UpdatedAt              time.Time         `json:"updated_at" db:"updated_at"`
}

// EncryptionContext holds metadata for token encryption/decryption.
type EncryptionContext struct {
    Principal string `json:"principal"`
    ServiceID string `json:"service_id"`
    SessionID string `json:"session_id"`
    Purpose   string `json:"purpose"`
}

// Value implements driver.Valuer for JSONB serialization.
func (ec EncryptionContext) Value() (driver.Value, error) {
    return json.Marshal(ec)
}

// Scan implements sql.Scanner for JSONB deserialization.
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

// Validate performs domain validation.
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
    return nil
}

// RequiresReauthentication reports recorded rejection or exhausted access-only credentials.
func (s *UserSession) RequiresReauthentication() bool {
    return s.RefreshTokenRejected || (!s.HasValidAccessToken() && !s.CanRefresh())
}

// CanRefresh reports stored credentials with no recorded provider rejection.
func (s *UserSession) CanRefresh() bool {
    return !s.RefreshTokenRejected && len(s.EncryptedRefreshToken) > 0
}

// HasValidAccessToken checks recorded access expiry only, not provider acceptance.
func (s *UserSession) HasValidAccessToken() bool {
    if s.AccessTokenExpiresAt == nil {
        return true // Expiry is unknown, not permanent validity.
    }
    return time.Now().Before(*s.AccessTokenExpiresAt)
}
```

`RefreshTokenRejected=false` means no observed terminal rejection, not proven provider validity.
HTTP 4xx `invalid_grant`, excluding 429, records rejection and immediately stops future broker token issuance.
The row and its ciphertext, access expiry, and other token metadata remain. Transient errors do not record rejection.
Locked callback errors never persist changes. The service commits rejection through `(true, nil)` before it returns the captured provider error.

### Step 2.2: Add StringSlice Type (if not exists)

**File**: `internal/domain/storage/types.go`

```go
// StringSlice is a []string that implements sql.Scanner and driver.Valuer for PostgreSQL text[].
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
    if s == nil {
        return nil, nil
    }
    return pq.Array(s).Value()
}

func (s *StringSlice) Scan(value interface{}) error {
    if value == nil {
        *s = nil
        return nil
    }
    arr := pq.StringArray{}
    if err := arr.Scan(value); err != nil {
        return err
    }
    *s = StringSlice(arr)
    return nil
}
```

---

## Phase 3: Repository Interface

### Step 3.1: Add UserSessionRepository to Ports

**File**: `internal/ports/storage.go` (add to existing file)

```go
// UserSessionRepository defines storage operations for user OAuth2 sessions.
type UserSessionRepository interface {
    // Create creates a new user session (upsert semantics on principal+service_id).
    Create(ctx context.Context, session *storage.UserSession) error
    
    // Get retrieves a session by ID.
    Get(ctx context.Context, id string) (*storage.UserSession, error)
    
    // FindByPrincipalAndService retrieves the session for a principal and service.
    FindByPrincipalAndService(ctx context.Context, principal, serviceID string) (*storage.UserSession, error)
    
    // ListByPrincipal retrieves all sessions for a principal.
    ListByPrincipal(ctx context.Context, principal string) ([]*storage.UserSession, error)
    
    // Delete deletes a session by ID.
    Delete(ctx context.Context, id string) error
    
    // DeleteByPrincipalAndService deletes the session for a principal and service.
    DeleteByPrincipalAndService(ctx context.Context, principal, serviceID string) error
    
    // CountByService counts sessions referencing a service.
    CountByService(ctx context.Context, serviceID string) (int, error)
}
```

---

## Phase 4: Storage Adapters

### Step 4.1: In-Memory Adapter

**File**: `internal/adapters/storage/memory/user_session.go`

```go
package memory

import (
    "context"
    "sync"
    
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type userSessionStore struct {
    mu       sync.RWMutex
    sessions map[string]*storage.UserSession // key: id
    byKey    map[string]string               // key: principal:service_id -> id
}

func newUserSessionStore() *userSessionStore {
    return &userSessionStore{
        sessions: make(map[string]*storage.UserSession),
        byKey:    make(map[string]string),
    }
}

func (s *userSessionStore) Create(ctx context.Context, session *storage.UserSession) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    key := session.Principal + ":" + session.ServiceID
    
    // Successful authorization supplies RefreshTokenRejected=false.
    // Preserve the existing identity and lifecycle timestamps on upsert.
    updated := session.Copy()
    if oldID, exists := s.byKey[key]; exists {
        existing := s.sessions[oldID]
        updated.ID = existing.ID
        updated.InitiatedAt = existing.InitiatedAt
        updated.CreatedAt = existing.CreatedAt
    }

    s.sessions[updated.ID] = updated
    s.byKey[key] = updated.ID
    
    return nil
}

func (s *userSessionStore) FindByPrincipalAndService(ctx context.Context, principal, serviceID string) (*storage.UserSession, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    key := principal + ":" + serviceID
    id, exists := s.byKey[key]
    if !exists {
        return nil, nil // Not found is not an error
    }
    
    session := s.sessions[id]
    if session == nil {
        return nil, nil
    }
    
    return session.Copy(), nil
}

func (s *userSessionStore) ListByPrincipal(ctx context.Context, principal string) ([]*storage.UserSession, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    var result []*storage.UserSession
    for _, session := range s.sessions {
        if session.Principal == principal {
            result = append(result, session.Copy())
        }
    }
    
    return result, nil
}

// ... implement remaining methods following pattern
```

### Step 4.2: PostgreSQL Adapter

**File**: `internal/adapters/storage/postgres/user_session.go`

```go
package postgres

import (
    "context"
    "database/sql"
    "errors"
    
    "github.com/jmoiron/sqlx"
    
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type userSessionRepository struct {
    db *sqlx.DB
}

func newUserSessionRepository(db *sqlx.DB) *userSessionRepository {
    return &userSessionRepository{db: db}
}

const createSessionSQL = `
INSERT INTO user_sessions (
    id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
    token_type, access_token_expires_at, refresh_token_rejected, scope,
    encryption_context, initiated_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
ON CONFLICT (principal, service_id) DO UPDATE SET
    encrypted_access_token = EXCLUDED.encrypted_access_token,
    encrypted_refresh_token = EXCLUDED.encrypted_refresh_token,
    token_type = EXCLUDED.token_type,
    access_token_expires_at = EXCLUDED.access_token_expires_at,
    refresh_token_rejected = EXCLUDED.refresh_token_rejected,
    scope = EXCLUDED.scope,
    encryption_context = EXCLUDED.encryption_context,
    updated_at = NOW()
`

func (r *userSessionRepository) Create(ctx context.Context, session *storage.UserSession) error {
    _, err := r.db.ExecContext(ctx, createSessionSQL,
        session.ID,
        session.Principal,
        session.ServiceID,
        session.EncryptedAccessToken,
        session.EncryptedRefreshToken,
        session.TokenType,
        session.AccessTokenExpiresAt,
        session.RefreshTokenRejected,
        session.Scope,
        session.EncryptionContext,
        session.InitiatedAt,
    )
    if err != nil {
        return storage.NewStorageError("Create", storage.ErrorKindInternal, err, "failed to create session")
    }
    return nil
}

const findByPrincipalAndServiceSQL = `
SELECT id, principal, service_id, encrypted_access_token, encrypted_refresh_token,
       token_type, access_token_expires_at, refresh_token_rejected, scope,
       encryption_context, initiated_at, created_at, updated_at
FROM user_sessions
WHERE principal = $1 AND service_id = $2
`

func (r *userSessionRepository) FindByPrincipalAndService(ctx context.Context, principal, serviceID string) (*storage.UserSession, error) {
    var session storage.UserSession
    err := r.db.GetContext(ctx, &session, findByPrincipalAndServiceSQL, principal, serviceID)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, nil // Not found is not an error
    }
    if err != nil {
        return nil, storage.NewStorageError("FindByPrincipalAndService", storage.ErrorKindInternal, err, "query failed")
    }
    return &session, nil
}

// ... implement remaining methods following pattern
```

Successful authorization calls this upsert with `RefreshTokenRejected=false`. It replaces rejected credentials while it retains the row identity and initiation time.
Normal refresh cannot clear rejection. Rejected rows cannot enter the provider refresh path.

---

## Phase 5: Domain Service

### Step 5.1: Create OAuth2SessionService

**File**: `internal/domain/oauth2session/service.go`

```go
package oauth2session

import (
    "context"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/url"
    "time"
    
    "github.com/google/uuid"
    "github.com/lestrrat-go/jwx/v3/jwa"
    "github.com/lestrrat-go/jwx/v3/jwe"
    "github.com/lestrrat-go/jwx/v3/jwk"
    "golang.org/x/oauth2"
    
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Service orchestrates OAuth2 authorization flows and session management.
type Service struct {
    serviceRepo ports.ThirdpartyOAuth2ServiceRepository
    sessionRepo ports.UserSessionRepository
    grantRepo   ports.UserGrantRepository
    encryption  ports.EncryptionPort
    jweKey      jwk.Key
    config      Config
    logger      *slog.Logger
}

// Config holds configuration for the OAuth2 session service.
type Config struct {
    CallbackBaseURL    string
    StateTokenTTL      time.Duration
    PKCEVerifierLength int
    MaxRetries         int
    RetryBaseDelay     time.Duration
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

// NewService creates a new OAuth2 session service.
func NewService(
    serviceRepo ports.ThirdpartyOAuth2ServiceRepository,
    sessionRepo ports.UserSessionRepository,
    grantRepo ports.UserGrantRepository,
    encryption ports.EncryptionPort,
    jweKey jwk.Key,
    config Config,
    logger *slog.Logger,
) *Service {
    return &Service{
        serviceRepo: serviceRepo,
        sessionRepo: sessionRepo,
        grantRepo:   grantRepo,
        encryption:  encryption,
        jweKey:      jweKey,
        config:      config,
        logger:      logger,
    }
}

// InitiateFlowResult contains the authorization URL to redirect the user to.
type InitiateFlowResult struct {
    AuthorizationURL string
}

// InitiateOAuth2Flow starts the OAuth2 authorization code flow.
func (s *Service) InitiateOAuth2Flow(
    ctx context.Context,
    principal string,
    serviceID string,
    redirectURI string,
) (*InitiateFlowResult, error) {
    // 1. Fetch service configuration
    service, err := s.serviceRepo.Get(ctx, serviceID)
    if err != nil {
        return nil, fmt.Errorf("failed to get service: %w", err)
    }
    if service == nil {
        return nil, ErrServiceNotFound
    }
    
    // 2. Generate PKCE
    verifier, challenge, err := generatePKCE(s.config.PKCEVerifierLength)
    if err != nil {
        return nil, fmt.Errorf("failed to generate PKCE: %w", err)
    }
    
    // 3. Create state token claims
    now := time.Now()
    claims := &StateTokenClaims{
        Principal:    principal,
        PKCEVerifier: verifier,
        ServiceID:    serviceID,
        RedirectURI:  redirectURI,
        IssuedAt:     now,
        ExpiresAt:    now.Add(s.config.StateTokenTTL),
    }
    
    // 4. Encrypt state token
    stateToken, err := s.createStateToken(claims)
    if err != nil {
        return nil, fmt.Errorf("failed to create state token: %w", err)
    }
    
    // 5. Build OAuth2 config
    oauth2Config := s.buildOAuth2Config(service)
    
    // 6. Build authorization URL with PKCE
    authURL := oauth2Config.AuthCodeURL(
        stateToken,
        oauth2.SetAuthURLParam("code_challenge", challenge),
        oauth2.SetAuthURLParam("code_challenge_method", "S256"),
    )
    
    s.logger.Info("OAuth2 flow initiated",
        "principal", principal,
        "service_id", serviceID,
        "redirect_uri", redirectURI)
    
    return &InitiateFlowResult{AuthorizationURL: authURL}, nil
}

// HandleCallbackRequest contains parameters from the OAuth2 callback.
type HandleCallbackRequest struct {
    ServiceID string
    Code      string
    State     string
    Error     string
    ErrorDesc string
}

// HandleCallback processes the OAuth2 callback.
func (s *Service) HandleCallback(
    ctx context.Context,
    principal string,
    req *HandleCallbackRequest,
) (*storage.UserSession, error) {
    // 1. Check for OAuth2 error from provider
    if req.Error != "" {
        return nil, &OAuth2Error{
            Code:        req.Error,
            Description: req.ErrorDesc,
        }
    }
    
    // 2. Validate state token
    claims, err := s.validateStateToken(req.State, principal)
    if err != nil {
        s.logger.Warn("State token validation failed",
            "principal", principal,
            "service_id", req.ServiceID,
            "error", err)
        return nil, err
    }
    
    // 3. Verify service ID matches
    if claims.ServiceID != req.ServiceID {
        s.logger.Warn("Service ID mismatch",
            "expected", claims.ServiceID,
            "got", req.ServiceID)
        return nil, ErrServiceIDMismatch
    }
    
    // 4. Fetch service configuration
    service, err := s.serviceRepo.Get(ctx, req.ServiceID)
    if err != nil || service == nil {
        return nil, ErrServiceNotFound
    }
    
    // 5. Exchange code for tokens (with retry)
    oauth2Config := s.buildOAuth2Config(service)
    token, err := s.exchangeCodeWithRetry(ctx, oauth2Config, req.Code, claims.PKCEVerifier)
    if err != nil {
        return nil, fmt.Errorf("token exchange failed: %w", err)
    }
    
    // 6. Create session with encrypted tokens
    session, err := s.createSession(ctx, principal, req.ServiceID, token)
    if err != nil {
        return nil, fmt.Errorf("failed to create session: %w", err)
    }
    
    s.logger.Info("OAuth2 session established",
        "principal", principal,
        "service_id", req.ServiceID,
        "session_id", session.ID)
    
    return session, nil
}

// Helper functions

func generatePKCE(length int) (verifier, challenge string, err error) {
    bytes := make([]byte, length)
    if _, err := rand.Read(bytes); err != nil {
        return "", "", err
    }
    verifier = base64.RawURLEncoding.EncodeToString(bytes)
    hash := sha256.Sum256([]byte(verifier))
    challenge = base64.RawURLEncoding.EncodeToString(hash[:])
    return verifier, challenge, nil
}

func (s *Service) buildOAuth2Config(service *storage.ThirdpartyOAuth2Service) *oauth2.Config {
    scopes := make([]string, len(service.Scopes))
    for i, scope := range service.Scopes {
        scopes[i] = scope.ScopeValue
    }
    
    return &oauth2.Config{
        ClientID:     service.ClientID,
        ClientSecret: service.ClientSecret,
        Endpoint: oauth2.Endpoint{
            AuthURL:  service.Endpoints.AuthorizeEndpoint,
            TokenURL: service.Endpoints.TokenEndpoint,
        },
        Scopes:      scopes,
        RedirectURL: s.config.CallbackBaseURL + "/api/third-party/" + service.ID + "/oauth2/callback",
    }
}

func (s *Service) createStateToken(claims *StateTokenClaims) (string, error) {
    payload, err := json.Marshal(claims)
    if err != nil {
        return "", err
    }
    
    encrypted, err := jwe.Encrypt(
        payload,
        jwe.WithKey(jwa.A256GCMKW(), s.jweKey),
        jwe.WithContentEncryption(jwa.A256GCM()),
    )
    if err != nil {
        return "", err
    }
    
    return string(encrypted), nil
}

func (s *Service) validateStateToken(token string, currentPrincipal string) (*StateTokenClaims, error) {
    decrypted, err := jwe.Decrypt(
        []byte(token),
        jwe.WithKey(jwa.A256GCMKW(), s.jweKey),
    )
    if err != nil {
        return nil, ErrStateTokenInvalid
    }
    
    var claims StateTokenClaims
    if err := json.Unmarshal(decrypted, &claims); err != nil {
        return nil, ErrStateTokenInvalid
    }
    
    if time.Now().After(claims.ExpiresAt) {
        return nil, ErrStateTokenExpired
    }
    
    if claims.Principal != currentPrincipal {
        return nil, ErrPrincipalMismatch
    }
    
    return &claims, nil
}

func (s *Service) exchangeCodeWithRetry(
    ctx context.Context,
    config *oauth2.Config,
    code string,
    pkceVerifier string,
) (*oauth2.Token, error) {
    var lastErr error
    delays := []time.Duration{
        s.config.RetryBaseDelay,
        s.config.RetryBaseDelay * 2,
        s.config.RetryBaseDelay * 4,
    }
    
    for attempt := 0; attempt <= len(delays); attempt++ {
        token, err := config.Exchange(
            ctx,
            code,
            oauth2.SetAuthURLParam("code_verifier", pkceVerifier),
        )
        if err == nil {
            return token, nil
        }
        
        lastErr = err
        
        if attempt < len(delays) {
            s.logger.Warn("Token exchange attempt failed",
                "attempt", attempt+1,
                "error", err)
            
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-time.After(delays[attempt]):
                continue
            }
        }
    }
    
    return nil, fmt.Errorf("%w: %v", ErrTokenExchangeFailed, lastErr)
}

// createSession is the authorization callback path, not a normal refresh path.
func (s *Service) createSession(
    ctx context.Context,
    principal string,
    serviceID string,
    token *oauth2.Token,
) (*storage.UserSession, error) {
    sessionID := uuid.New().String()
    now := time.Now()
    
    // Create encryption context
    encCtx := map[string]string{
        "principal":  principal,
        "service_id": serviceID,
        "session_id": sessionID,
        "purpose":    "oauth2_token",
    }
    
    // Encrypt access token
    encryptedAccess, err := s.encryption.Encrypt(ctx, []byte(token.AccessToken), encCtx)
    if err != nil {
        return nil, fmt.Errorf("failed to encrypt access token: %w", err)
    }
    
    // Encrypt refresh token (if present)
    var encryptedRefresh []byte
    if token.RefreshToken != "" {
        encryptedRefresh, err = s.encryption.Encrypt(ctx, []byte(token.RefreshToken), encCtx)
        if err != nil {
            return nil, fmt.Errorf("failed to encrypt refresh token: %w", err)
        }
    }
    
    // Record the provider's access-token expiry only.
    var accessExpires *time.Time
    if !token.Expiry.IsZero() {
        accessExpires = &token.Expiry
    }
    
    session := &storage.UserSession{
        ID:                    sessionID,
        Principal:             principal,
        ServiceID:             serviceID,
        EncryptedAccessToken:  encryptedAccess,
        EncryptedRefreshToken: encryptedRefresh,
        TokenType:             token.TokenType,
        AccessTokenExpiresAt:  accessExpires,
        RefreshTokenRejected:  false, // Successful authorization clears recorded rejection.
        Scope:                 extractScopes(token),
        EncryptionContext: storage.EncryptionContext{
            Principal: principal,
            ServiceID: serviceID,
            SessionID: sessionID,
            Purpose:   "oauth2_token",
        },
        InitiatedAt: now,
    }
    
    if err := s.sessionRepo.Create(ctx, session); err != nil {
        return nil, err
    }
    
    return session, nil
}

func extractScopes(token *oauth2.Token) []string {
    // OAuth2 tokens may have scope in Extra
    if scope, ok := token.Extra("scope").(string); ok {
        return strings.Split(scope, " ")
    }
    return nil
}
```

### Step 5.2: Create State Token Types

**File**: `internal/domain/oauth2session/state_token.go`

```go
package oauth2session

import "time"

// StateTokenClaims contains the claims embedded in a JWE state token.
type StateTokenClaims struct {
    Principal    string    `json:"principal"`
    PKCEVerifier string    `json:"pkce_verifier"`
    ServiceID    string    `json:"service_id"`
    RedirectURI  string    `json:"redirect_uri"`
    IssuedAt     time.Time `json:"iat"`
    ExpiresAt    time.Time `json:"exp"`
}
```

### Step 5.3: Create Errors

**File**: `internal/domain/oauth2session/errors.go`

```go
package oauth2session

import (
    "errors"
    "fmt"
)

var (
    ErrStateTokenExpired   = errors.New("state token has expired")
    ErrStateTokenInvalid   = errors.New("state token is invalid or tampered")
    ErrPrincipalMismatch   = errors.New("principal does not match state token")
    ErrServiceIDMismatch   = errors.New("service ID does not match state token")
    ErrServiceNotFound     = errors.New("third-party service not found")
    ErrSessionNotFound     = errors.New("session not found")
    ErrTokenExchangeFailed = errors.New("failed to exchange authorization code")
    ErrInvalidRedirectURI  = errors.New("redirect URI is not allowed")
)

// OAuth2Error wraps OAuth2 error responses from authorization servers.
type OAuth2Error struct {
    Code        string
    Description string
}

func (e *OAuth2Error) Error() string {
    if e.Description != "" {
        return fmt.Sprintf("%s: %s", e.Code, e.Description)
    }
    return e.Code
}
```

---

## Phase 6: HTTP Handlers

### Step 6.1: Create Session Handlers

**File**: `internal/adapters/http/oauth2_sessions/handler.go`

```go
package oauth2_sessions

import (
    "encoding/json"
    "net/http"
    "net/url"
    
    "github.com/go-chi/chi/v5"
    
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

type Handler struct {
    service *oauth2session.Service
    logger  *slog.Logger
}

func NewHandler(service *oauth2session.Service, logger *slog.Logger) *Handler {
    return &Handler{service: service, logger: logger}
}

// RegisterRoutes registers OAuth2 session routes.
func (h *Handler) RegisterRoutes(r chi.Router) {
    r.Get("/api/third-party/sessions", h.ListSessions)
    r.Get("/api/third-party/{serviceId}/oauth2/authorize", h.InitiateFlow)
    r.Get("/api/third-party/{serviceId}/oauth2/callback", h.HandleCallback)
    r.Get("/api/third-party/{serviceId}/session", h.GetSessionDetails)
    r.Delete("/api/third-party/{serviceId}/session", h.TerminateSession)
}

func (h *Handler) ListSessions(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    userPrincipal := principal.MustFromContext(ctx)
    
    sessions, err := h.service.ListUserSessions(ctx, userPrincipal)
    if err != nil {
        h.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list sessions")
        return
    }
    
    h.writeJSON(w, http.StatusOK, map[string]interface{}{"data": sessions})
}

func (h *Handler) InitiateFlow(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    userPrincipal := principal.MustFromContext(ctx)
    serviceID := chi.URLParam(r, "serviceId")
    redirectURI := r.URL.Query().Get("redirect_uri")
    
    if redirectURI == "" {
        h.writeError(w, http.StatusBadRequest, "missing_parameter", "redirect_uri is required")
        return
    }
    
    // Validate redirect_uri is same-origin
    if !h.validateRedirectURI(r, redirectURI) {
        h.writeError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uri must be same-origin")
        return
    }
    
    result, err := h.service.InitiateOAuth2Flow(ctx, userPrincipal, serviceID, redirectURI)
    if err != nil {
        if errors.Is(err, oauth2session.ErrServiceNotFound) {
            h.writeError(w, http.StatusNotFound, "service_not_found", "Service not found")
            return
        }
        h.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to initiate flow")
        return
    }
    
    http.Redirect(w, r, result.AuthorizationURL, http.StatusFound)
}

func (h *Handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    userPrincipal := principal.MustFromContext(ctx)
    
    req := &oauth2session.HandleCallbackRequest{
        ServiceID: chi.URLParam(r, "serviceId"),
        Code:      r.URL.Query().Get("code"),
        State:     r.URL.Query().Get("state"),
        Error:     r.URL.Query().Get("error"),
        ErrorDesc: r.URL.Query().Get("error_description"),
    }
    
    session, err := h.service.HandleCallback(ctx, userPrincipal, req)
    
    // Get redirect URI from state token (before validation, we need to extract it)
    redirectURI := h.extractRedirectURI(req.State)
    if redirectURI == "" {
        redirectURI = "/consent/sessions" // Fallback
    }
    
    if err != nil {
        h.redirectWithError(w, r, redirectURI, err)
        return
    }
    
    // Success redirect
    successURL, _ := url.Parse(redirectURI)
    q := successURL.Query()
    q.Set("session_established", "true")
    q.Set("service_id", session.ServiceID)
    successURL.RawQuery = q.Encode()
    
    http.Redirect(w, r, successURL.String(), http.StatusFound)
}

func (h *Handler) TerminateSession(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    userPrincipal := principal.MustFromContext(ctx)
    serviceID := chi.URLParam(r, "serviceId")
    
    err := h.service.TerminateSession(ctx, userPrincipal, serviceID)
    if err != nil {
        if errors.Is(err, oauth2session.ErrSessionNotFound) {
            h.writeError(w, http.StatusNotFound, "session_not_found", "Session not found")
            return
        }
        h.writeError(w, http.StatusInternalServerError, "internal_error", "Failed to terminate session")
        return
    }
    
    w.WriteHeader(http.StatusNoContent)
}

// Helper methods...
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, code, message string) {
    h.writeJSON(w, status, map[string]string{"error": code, "message": message})
}

func (h *Handler) validateRedirectURI(r *http.Request, redirectURI string) bool {
    parsed, err := url.Parse(redirectURI)
    if err != nil {
        return false
    }
    return parsed.Host == r.Host
}
```

---

## Phase 7: Configuration

### Step 7.1: Add Configuration Schema

**File**: Update `internal/config/schema.go`

```go
// ThirdPartyOAuth2Config holds OAuth2 session configuration.
type ThirdPartyOAuth2Config struct {
    JWESigningKey      string `mapstructure:"jwe_signing_key"`
    StateTokenTTL      int    `mapstructure:"state_token_ttl"` // seconds
    PKCEVerifierLength int    `mapstructure:"pkce_verifier_length"`
}
```

### Step 7.2: Create Example Configuration

**File**: `examples/config/third-party-oauth2.yaml`

```yaml
# Third-party OAuth2 session configuration
# This file shows all available options for OAuth2 session management

third_party_oauth2:
  # JWE signing key for state tokens (REQUIRED)
  # Must be 32 bytes, base64 encoded
  # Load from environment variable (never commit actual keys!)
  jwe_signing_key: ${IDENTITY_BROKER_JWE_SIGNING_KEY}
  
  # State token time-to-live in seconds (default: 600 = 10 minutes)
  # Maximum allowed: 900 (15 minutes)
  state_token_ttl: 600
  
  # PKCE verifier length in bytes (default: 32)
  # Valid range: 32-128 per RFC 7636
  pkce_verifier_length: 32
```

---

## Phase 8: Frontend Components

### Step 8.1: Session API Client

**File**: `web/src/services/api/sessions.ts`

```typescript
import { apiClient } from './client';

export interface SessionSummary {
  id: string;
  service_id: string;
  service_display_name: string;
  token_type: string;
  scope: string[];
  initiated_at: string;
  requires_reauthentication: boolean;
  access_token_expired: boolean;
  has_refresh_token: boolean;
  dependent_agent_count: number;
  is_encrypted: boolean;
}

export interface SessionDetails {
  session: SessionSummary;
  dependent_agents: { id: string; display_name: string }[];
}

export const sessionsApi = {
  list: async (): Promise<SessionSummary[]> => {
    const response = await apiClient.get('/api/third-party/sessions');
    return response.data.data;
  },
  
  getDetails: async (serviceId: string): Promise<SessionDetails> => {
    const response = await apiClient.get(`/api/third-party/${serviceId}/session`);
    return response.data.data;
  },
  
  terminate: async (serviceId: string): Promise<void> => {
    await apiClient.delete(`/api/third-party/${serviceId}/session`);
  },
};
```

`has_refresh_token` reports stored ciphertext, including provider-rejected credentials. It does not prove validity.
The card shows "Reauthentication required" first, then "Access Token Expired", then "Active".
Show Login when reauthentication is required. Show Refresh only with stored refresh credentials and no required reauthentication.
Keep Terminate available for every retained session. Do not show refresh-expiry dates, seven-day lifetime warnings, or permanent-validity claims.
After a manual refresh error, refetch the list before showing the saved error message. Rejection changes persisted state despite HTTP 409.

### Step 8.2: Sessions Page

**File**: `web/src/pages/ThirdPartySessionsPage.tsx`

```tsx
import React, { useEffect, useState } from 'react';
import { SessionCard } from '../components/sessions/SessionCard';
import { TerminationDialog } from '../components/sessions/TerminationDialog';
import { sessionsApi, SessionSummary } from '../services/api/sessions';
import { EmptyState } from '../components/ui/EmptyState';
import { Skeleton } from '../components/ui/Skeleton';

export const ThirdPartySessionsPage: React.FC = () => {
  const [sessions, setSessions] = useState<SessionSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [terminatingService, setTerminatingService] = useState<string | null>(null);

  useEffect(() => {
    loadSessions();
  }, []);

  const loadSessions = async () => {
    try {
      setLoading(true);
      const data = await sessionsApi.list();
      setSessions(data);
    } catch (err) {
      setError('Failed to load sessions');
    } finally {
      setLoading(false);
    }
  };

  const handleLogin = (serviceId: string) => {
    const redirectURI = new URL('/sessions', window.location.origin).toString();
    window.location.href = `/api/third-party/${serviceId}/oauth2/authorize?redirect_uri=${encodeURIComponent(redirectURI)}`;
  };

  const handleTerminate = async () => {
    if (!terminatingService) return;
    try {
      await sessionsApi.terminate(terminatingService);
      await loadSessions();
    } finally {
      setTerminatingService(null);
    }
  };

  if (loading) {
    return <Skeleton count={3} />;
  }

  if (sessions.length === 0) {
    return (
      <EmptyState
        title="No third-party sessions"
        description="Connect to third-party services to allow agents to access them on your behalf."
      />
    );
  }

  return (
    <div className="space-y-4">
      <h1 className="text-2xl font-semibold">Third-Party Sessions</h1>
      <p className="text-neutral-600">
        Manage your connections to third-party services.
      </p>
      
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3">
        {sessions.map((session) => (
          <SessionCard
            key={session.id}
            session={session}
            onLogin={handleLogin}
            onTerminate={() => setTerminatingService(session.service_id)}
          />
        ))}
      </div>
      
      <TerminationDialog
        open={terminatingService !== null}
        onClose={() => setTerminatingService(null)}
        onConfirm={handleTerminate}
        serviceId={terminatingService}
      />
    </div>
  );
};
```

Login completes the real authorize/callback flow for the same principal and service. Successful callback upsert clears rejection and restores the "Active" card.
Manual refresh returns 409 `refresh_rejected` for terminal rejection, 409 `refresh_unavailable` for missing credentials, or 502 `refresh_failed` for transient errors.

---

## Testing Strategy

### Unit Tests

```go
// internal/domain/oauth2session/service_test.go
func TestGeneratePKCE(t *testing.T) {
    verifier, challenge, err := generatePKCE(32)
    require.NoError(t, err)
    assert.Len(t, verifier, 43) // 32 bytes base64url = 43 chars
    assert.Len(t, challenge, 43)
    
    // Verify challenge is SHA256 of verifier
    hash := sha256.Sum256([]byte(verifier))
    expectedChallenge := base64.RawURLEncoding.EncodeToString(hash[:])
    assert.Equal(t, expectedChallenge, challenge)
}

func TestValidateStateToken_ExpiredToken(t *testing.T) {
    // Create token with past expiry
    claims := &StateTokenClaims{
        Principal:    "alice@example.com",
        PKCEVerifier: "test-verifier",
        ServiceID:    "github",
        RedirectURI:  "https://example.com/callback",
        IssuedAt:     time.Now().Add(-2 * time.Hour),
        ExpiresAt:    time.Now().Add(-1 * time.Hour), // Expired
    }
    
    // Create and validate token
    service := setupTestService(t)
    token, _ := service.createStateToken(claims)
    
    _, err := service.validateStateToken(token, "alice@example.com")
    assert.ErrorIs(t, err, ErrStateTokenExpired)
}

func TestValidateStateToken_PrincipalMismatch(t *testing.T) {
    claims := &StateTokenClaims{
        Principal:    "alice@example.com",
        PKCEVerifier: "test-verifier",
        ServiceID:    "github",
        RedirectURI:  "https://example.com/callback",
        IssuedAt:     time.Now(),
        ExpiresAt:    time.Now().Add(10 * time.Minute),
    }
    
    service := setupTestService(t)
    token, _ := service.createStateToken(claims)
    
    // Validate with different principal
    _, err := service.validateStateToken(token, "bob@example.com")
    assert.ErrorIs(t, err, ErrPrincipalMismatch)
}
```

### Integration Tests

```go
// tests/integration/user_sessions_test.go
func TestUserSessionRepository_PostgreSQL(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    
    ctx := context.Background()
    
    // Setup testcontainer
    pg := setupPostgres(t)
    defer pg.Terminate(ctx)
    
    // Run migrations
    runMigrations(t, pg.ConnectionString())
    
    // Create repository
    db := sqlx.MustConnect("postgres", pg.ConnectionString())
    repo := postgres.NewUserSessionRepository(db)
    
    t.Run("Create and Find", func(t *testing.T) {
        session := &storage.UserSession{
            ID:                   uuid.New().String(),
            Principal:            "alice@example.com",
            ServiceID:            testServiceID,
            EncryptedAccessToken: []byte("encrypted-token"),
            TokenType:            "Bearer",
            Scope:                []string{"repo", "user"},
            InitiatedAt:          time.Now(),
        }
        
        err := repo.Create(ctx, session)
        require.NoError(t, err)
        
        found, err := repo.FindByPrincipalAndService(ctx, "alice@example.com", testServiceID)
        require.NoError(t, err)
        assert.Equal(t, session.ID, found.ID)
    })
    
    t.Run("Upsert replaces tokens", func(t *testing.T) {
        // Create first session
        session1 := &storage.UserSession{
            ID:                   uuid.New().String(),
            Principal:            "bob@example.com",
            ServiceID:            testServiceID,
            EncryptedAccessToken: []byte("old-token"),
            TokenType:            "Bearer",
            InitiatedAt:          time.Now(),
        }
        require.NoError(t, repo.Create(ctx, session1))
        
        // Create second session (same principal+service)
        session2 := &storage.UserSession{
            ID:                   uuid.New().String(),
            Principal:            "bob@example.com",
            ServiceID:            testServiceID,
            EncryptedAccessToken: []byte("new-token"),
            TokenType:            "Bearer",
            InitiatedAt:          time.Now(),
        }
        require.NoError(t, repo.Create(ctx, session2))
        
        // Should have new token
        found, err := repo.FindByPrincipalAndService(ctx, "bob@example.com", testServiceID)
        require.NoError(t, err)
        assert.Equal(t, []byte("new-token"), found.EncryptedAccessToken)
    })
}
```

---

## Checklist

Before considering this feature complete:

- [ ] Database migrations created and tested (up + down)
- [ ] UserSession entity implemented with validation
- [ ] UserSessionRepository interface defined in ports
- [ ] Memory adapter implemented with tests
- [ ] PostgreSQL adapter implemented with tests
- [ ] OAuth2SessionService implemented
- [ ] State token JWE encryption/decryption working
- [ ] PKCE generation correct per RFC 7636
- [ ] HTTP handlers implemented
- [ ] Redirect URI validation working
- [ ] Token exchange retry logic working
- [ ] Token encryption using EncryptionPort
- [ ] Configuration schema updated
- [ ] Example configuration added
- [ ] Frontend API client created
- [ ] Sessions page component created
- [ ] Unit tests passing
- [ ] Integration tests passing
- [ ] OpenAPI spec matches implementation
- [ ] ARCHITECTURE.md glossary updated
