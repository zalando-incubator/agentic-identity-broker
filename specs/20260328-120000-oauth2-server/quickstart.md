# Quickstart: OAuth2 Server Mode Implementation

**Branch**: `20260328-120000-oauth2-server` | **Date**: 2026-03-28

## Prerequisites

- Go 1.25.6
- PostgreSQL (via testcontainers for integration tests)
- Existing dependencies: `lestrrat-go/jwx/v3`, `golang.org/x/crypto`, `chi/v5`
- New dependency: `ory/fosite` (headless — handler layer only)

## Configuration

### YAML Configuration

```yaml
oauth2_authorization_server:
  mode: issue_token                      # "proxy" (default) or "issue_token"
  issuer_uri: https://broker.example.com # Required in issue_token mode
  token_ttl: 1h                          # Default: 1h
  token_claims_expression: |             # Optional CEL expression for custom token claims
    {"team": agent.display_name, "environment": "production"}
  # upstream_* fields ignored in issue_token mode (warning logged)
```

### Config Struct Changes

Add to `OAuth2AuthServerConfig` in `internal/ports/config.go`:

```go
type OAuth2AuthServerConfig struct {
    // ... existing fields ...
    Mode                  string        `mapstructure:"mode"`                    // "proxy" (default) or "issue_token"
    IssuerURI             string        `mapstructure:"issuer_uri"`              // Required when mode=issue_token
    TokenTTL              time.Duration `mapstructure:"token_ttl"`               // Default: 1h
    TokenClaimsExpression string        `mapstructure:"token_claims_expression"` // Optional CEL expression for custom claims
}
```

### Mode-Conditional Validation

`Validate()` must check mode before requiring upstream fields:

```go
func (c *OAuth2AuthServerConfig) Validate() error {
    if c.Mode == "" {
        c.Mode = "proxy"
    }

    if c.Mode == "issue_token" {
        if c.IssuerURI == "" {
            return newValidationError("oauth2_authorization_server.issuer_uri is required in issue_token mode")
        }
        if c.TokenTTL == 0 {
            c.TokenTTL = time.Hour
        }
        // token_claims_expression validated at startup via CELCompilerPort (not here — needs compiler)
        // upstream_* fields NOT required — log warning if present
        return nil
    }

    // Proxy mode: existing validation (upstream fields required)
    if c.UpstreamIssuerURI == "" {
        return newValidationError("oauth2_authorization_server.upstream_issuer_uri")
    }
    // ... rest of existing proxy validation
}
```

## Entity Implementation Pattern

Follow the existing entity pattern in `internal/domain/storage/`:

### 1. Define Entity (e.g., `broker_client_credential.go`)

```go
package storage

import (
    "time"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type BrokerClientCredential struct {
    ID             id.CredentialID   `json:"id" db:"id"`
    AgentID        id.AgentID        `json:"agent_id" db:"agent_id"`
    ClientID       id.ClientID       `json:"client_id" db:"client_id"`
    SecretHash     string            `json:"-" db:"secret_hash"` // Never serialize
    CreatedAt      time.Time         `json:"created_at" db:"created_at"`
    RotatedAt      *time.Time        `json:"rotated_at,omitempty" db:"rotated_at"`
}
```

### 2. Add Repository Port (in `internal/ports/storage.go`)

```go
type BrokerClientCredentialRepository interface {
    Create(ctx context.Context, credential *storage.BrokerClientCredential) error
    GetByAgentID(ctx context.Context, agentID id.AgentID) (*storage.BrokerClientCredential, error)
    GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.BrokerClientCredential, error)
    Delete(ctx context.Context, agentID id.AgentID) error
}
```

### 3. Implement Memory Adapter (in `internal/adapters/storage/memory/`)

```go
type BrokerClientCredentialStore struct {
    mu          sync.RWMutex
    byID        map[id.CredentialID]*storage.BrokerClientCredential
    byAgentID   map[id.AgentID]*storage.BrokerClientCredential
    byClientID  map[id.ClientID]*storage.BrokerClientCredential
}
```

### 4. Implement PostgreSQL Adapter (in `internal/adapters/storage/postgres/`)

Use `sqlx` per ADR 20251215-120300. Wrap errors in `StorageError`.

### 5. Wire in Builder (`internal/app/builder.go`)

Add repository accessor to storage adapter, wire into domain service, inject into handler.

## Domain Service Implementation

### Fosite Headless Provider (central wiring)

The fosite handlers are constructed once and wrapped behind the project's port interfaces.
Fosite types (`AuthorizeRequester`, `AccessRequester`, etc.) never leak outside `internal/domain/oauth2server/`.

```go
package oauth2server

import (
    "github.com/ory/fosite"
    fositeOAuth2 "github.com/ory/fosite/handler/oauth2"
    "github.com/ory/fosite/handler/pkce"
)

type Provider struct {
    authCodeHandler *fositeOAuth2.AuthorizeExplicitGrantHandler
    ccHandler       *fositeOAuth2.ClientCredentialsGrantHandler
    pkceHandler     *pkce.Handler
    helper          *fositeOAuth2.HandleHelper
    config          *fosite.Config
    clientAuth      *ClientAuthService
    signingKeys     *SigningKeyService
    logger          *slog.Logger
}

// NewProvider constructs fosite handlers with our custom strategies and storage adapters
func NewProvider(
    codeRepo ports.AuthorizationCodeRepository,
    credRepo ports.BrokerClientCredentialRepository,
    agentRepo ports.AgentRepository,
    signingKeyRepo ports.SigningKeyRepository,
    encryption ports.EncryptionPort,
    celCompiler ports.CELCompilerPort,
    issuerURI string,
    tokenTTL time.Duration,
    tokenClaimsExpression string,
    logger *slog.Logger,
) (*Provider, error) {
    // Compile token claims CEL expression at startup (FR-013b: fail if invalid)
    var tokenClaimsProgram ports.CELProgram
    if tokenClaimsExpression != "" {
        var err error
        tokenClaimsProgram, err = celCompiler.CompileExpression(context.Background(), tokenClaimsExpression)
        if err != nil {
            return nil, fmt.Errorf("invalid token_claims_expression: %w", err)
        }
    }

    // Our strategies — implemented with lestrrat-go/jwx and stdlib crypto
    accessStrategy := NewJWXAccessTokenStrategy(signingKeyRepo, encryption, issuerURI, tokenTTL, tokenClaimsProgram, logger)
    codeStrategy := NewRandomCodeStrategy()

    // Storage adapters wrapping our repositories
    storage := NewFositeStorage(codeRepo, agentRepo, credRepo)

    config := &fosite.Config{
        AuthorizeCodeLifespan: 60 * time.Second,
        AccessTokenLifespan:   tokenTTL,
        EnforcePKCE:          true,
        EnablePKCEPlainChallengeMethod: false,
    }

    helper := &fositeOAuth2.HandleHelper{
        AccessTokenStrategy: accessStrategy,
        AccessTokenStorage:  storage,
        Config:              config,
    }

    return &Provider{
        authCodeHandler: &fositeOAuth2.AuthorizeExplicitGrantHandler{
            AccessTokenStrategy:    accessStrategy,
            AuthorizeCodeStrategy:  codeStrategy,
            CoreStorage:            storage,
            Config:                 config,
        },
        ccHandler: &fositeOAuth2.ClientCredentialsGrantHandler{
            HandleHelper: helper,
            Config:       config,
        },
        pkceHandler: &pkce.Handler{
            AuthorizeCodeStrategy: codeStrategy,
            Storage:               storage,
            Config:                config,
        },
        // ...
    }
}
```

### Custom Access Token Strategy (using lestrrat-go/jwx)

Implements fosite's `AccessTokenStrategy` interface with our JWT library:

```go
package oauth2server

import (
    "github.com/lestrrat-go/jwx/v3/jwa"
    "github.com/lestrrat-go/jwx/v3/jwt"
    "github.com/lestrrat-go/jwx/v3/jwk"
    "github.com/ory/fosite"
    fositeOAuth2 "github.com/ory/fosite/handler/oauth2"
)

// Verify interface compliance
var _ fositeOAuth2.AccessTokenStrategy = (*JWXAccessTokenStrategy)(nil)

type JWXAccessTokenStrategy struct {
    signingKeyRepo     ports.SigningKeyRepository
    encryption         ports.EncryptionPort
    issuerURI          string
    tokenTTL           time.Duration
    tokenClaimsProgram ports.CELProgram // nil if no expression configured
    logger             *slog.Logger
}

func (s *JWXAccessTokenStrategy) GenerateAccessToken(ctx context.Context, requester fosite.Requester) (string, string, error) {
    // 1. Get current signing key from our repository
    key, err := s.signingKeyRepo.GetCurrent(ctx)
    if err != nil {
        return "", "", err
    }

    // 2. Decrypt private key material via our EncryptionPort
    privPEM, err := s.encryption.Decrypt(ctx, key.PrivateKeyEncrypted, encryptionContext(key.KID))
    if err != nil {
        return "", "", err
    }

    // 3. Parse PEM → jwk.Key
    privKey, err := jwk.ParseKey(privPEM, jwk.WithPEM(true))
    if err != nil {
        return "", "", err
    }

    // 4. Build JWT with claims from fosite requester
    jti := uuid.New().String()
    builder := jwt.NewBuilder().
        Issuer(s.issuerURI).
        Subject(requester.GetClient().GetID()).
        IssuedAt(time.Now()).
        Expiration(requester.GetSession().GetExpiresAt(fosite.AccessToken)).
        JwtID(jti).
        Claim("agent_id", requester.GetClient().GetID()).
        Claim("scope", strings.Join(requester.GetGrantedScopes(), " "))

    // 5. Evaluate CEL token_claims_expression (if configured)
    if s.tokenClaimsProgram != nil {
        customClaims, err := s.tokenClaimsProgram.Eval(ctx, s.buildCELContext(requester))
        if err != nil {
            return "", "", fmt.Errorf("token claims expression evaluation failed: %w", err)
        }
        claimsMap, ok := customClaims.(map[string]interface{})
        if !ok {
            return "", "", fmt.Errorf("token claims expression must return map, got %T", customClaims)
        }
        for k, v := range claimsMap {
            if isBaseClaim(k) {
                s.logger.Warn("CEL expression returned reserved claim, skipping", "claim", k)
                continue
            }
            builder = builder.Claim(k, v)
        }
    }

    token, err := builder.Build()
    if err != nil {
        return "", "", err
    }

    // 5. Sign with kid
    _ = privKey.Set(jwk.KeyIDKey, string(key.KID))
    signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), privKey))
    if err != nil {
        return "", "", err
    }

    // signature = SHA-256 of the token (for storage/lookup)
    return string(signed), sha256Hex(string(signed)), nil
}

func (s *JWXAccessTokenStrategy) AccessTokenSignature(_ context.Context, token string) string {
    return sha256Hex(token)
}

func (s *JWXAccessTokenStrategy) ValidateAccessToken(ctx context.Context, _ fosite.Requester, token string) error {
    // Validate via JWKS (public keys from signing key repo)
    // ...
    return nil
}
```

### Custom Authorize Code Strategy

```go
var _ fositeOAuth2.AuthorizeCodeStrategy = (*RandomCodeStrategy)(nil)

type RandomCodeStrategy struct{}

func (s *RandomCodeStrategy) GenerateAuthorizeCode(_ context.Context, _ fosite.Requester) (string, string, error) {
    code := generateSecureRandom(32) // 32 bytes → 43 char base64url
    signature := sha256Hex(code)
    return code, signature, nil
}

func (s *RandomCodeStrategy) AuthorizeCodeSignature(_ context.Context, code string) string {
    return sha256Hex(code)
}

func (s *RandomCodeStrategy) ValidateAuthorizeCode(_ context.Context, _ fosite.Requester, code string) error {
    // Validation happens in storage (expiry, single-use)
    return nil
}
```

### Fosite Storage Adapter (wrapping our repositories)

```go
type FositeStorage struct {
    codeRepo  ports.AuthorizationCodeRepository
    agentRepo ports.AgentRepository
    credRepo  ports.BrokerClientCredentialRepository
}

// Implements fositeOAuth2.AuthorizeCodeStorage
func (s *FositeStorage) CreateAuthorizeCodeSession(ctx context.Context, code string, req fosite.Requester) error {
    authCode := &storage.AuthorizationCode{
        ID:            id.NewAuthorizationCodeID(),
        CodeHash:      sha256Hex(code),
        AgentID:       extractAgentID(req.GetClient()),
        Principal:     id.NewPrincipal(req.GetSession().GetSubject()),
        RedirectURI:   req.GetRequestForm().Get("redirect_uri"),
        CodeChallenge: req.GetRequestForm().Get("code_challenge"),
        Scope:         strings.Join(req.GetRequestedScopes(), " "),
        ExpiresAt:     req.GetSession().GetExpiresAt(fosite.AuthorizeCode),
    }
    return s.codeRepo.Create(ctx, authCode)
}

// Implements fositeOAuth2.AccessTokenStorage (no-op for stateless JWT)
func (s *FositeStorage) CreateAccessTokenSession(_ context.Context, _ string, _ fosite.Requester) error {
    return nil // JWT tokens are stateless — no storage needed
}
```

### Client Authentication Service

Client authentication happens **outside** fosite (we bypass `NewAccessRequest` which does it).
We authenticate the client ourselves, then construct the fosite request with the verified client.

```go
func (s *ClientAuthService) Authenticate(ctx context.Context, clientID id.ClientID, secret string) (*BrokerClient, error) {
    cred, err := s.credentialRepo.GetByClientID(ctx, clientID)
    if err != nil {
        return nil, ErrInvalidClient
    }
    agent, err := s.agentRepo.Get(ctx, cred.AgentID)
    if err != nil {
        return nil, ErrInvalidClient
    }
    if err := s.hasher.Compare(cred.SecretHash, secret); err != nil {
        return nil, ErrInvalidClient
    }
    // Return fosite-compatible client wrapper
    return &BrokerClient{agent: agent, credential: cred}, nil
}
```

## Handler Implementation Pattern

Follow existing handler pattern (e.g., `internal/adapters/http/handlers/admin/agents_handler.go`):

```go
type ClientCredentialsHandler struct {
    credentialRepo ports.BrokerClientCredentialRepository
    agentRepo      ports.AgentRepository
    logger         *slog.Logger
}

func NewClientCredentialsHandler(
    credentialRepo ports.BrokerClientCredentialRepository,
    agentRepo ports.AgentRepository,
    logger *slog.Logger,
) *ClientCredentialsHandler { ... }

func (h *ClientCredentialsHandler) Generate(w http.ResponseWriter, r *http.Request) { ... }
func (h *ClientCredentialsHandler) Get(w http.ResponseWriter, r *http.Request) { ... }
func (h *ClientCredentialsHandler) Revoke(w http.ResponseWriter, r *http.Request) { ... }
```

## Routing Pattern

In `internal/adapters/http/routing/admin.go`, add to `SetupAdminRoutes()`:

```go
r.Route("/api/agents/{agent-id}/client-credentials", func(r chi.Router) {
    r.Post("/", handlers.AdminHandlers.ClientCredentials.Generate)
    r.Get("/", handlers.AdminHandlers.ClientCredentials.Get)
    r.Delete("/", handlers.AdminHandlers.ClientCredentials.Revoke)
})

r.Route("/api/oauth2-server/signing-keys", func(r chi.Router) {
    r.Post("/", handlers.AdminHandlers.SigningKeys.Add)
    r.Get("/", handlers.AdminHandlers.SigningKeys.List)
    r.Put("/{kid}/current", handlers.AdminHandlers.SigningKeys.SetCurrent)
    r.Delete("/{kid}", handlers.AdminHandlers.SigningKeys.Remove)
})
```

In `internal/adapters/http/routing/enduser.go`, conditionally register in `issue_token` mode:

```go
if cfg.OAuth2AuthServer.Mode == "issue_token" {
    r.Get("/oauth2/jwks.json", handlers.EnduserHandlers.JWKS.ServeJWKS)
    // Token handler already exists — extend to dispatch by grant_type
}
```

## Signing Key Auto-Generation at Startup

In `builder.go` `Build()` method, after storage is initialized:

```go
if cfg.OAuth2AuthServer.Mode == "issue_token" {
    // Check if signing keys exist
    count, err := storage.SigningKeys().CountActive(ctx)
    if count == 0 {
        // Auto-generate initial key
        key := generateES256Key()
        encryptedPEM := encryption.Encrypt(ctx, key.PEM, encryptionContext)
        storage.SigningKeys().Create(ctx, &storage.SigningKey{
            ID: id.NewSigningKeyID(),
            KID: id.NewKeyID(uuid.New().String()),
            Algorithm: "ES256",
            PrivateKeyEncrypted: encryptedPEM,
            IsCurrent: true,
        })
        logger.Info("auto-generated initial signing key", "kid", key.KID)
    }
}
```

## Testing Pattern

### Unit Tests (domain services)

```go
func TestTokenMintingService_MintAccessToken(t *testing.T) {
    tests := []struct {
        name    string
        setup   func(*mocks)
        agentID id.AgentID
        scope   string
        wantErr bool
    }{
        {
            name: "success with current key",
            // ...
        },
    }
    // table-driven
}
```

### Integration Tests (PostgreSQL adapters)

Use testcontainers pattern from existing adapter tests. Apply migrations, test CRUD.

### E2E Tests (Ginkgo/Gomega)

Follow patterns in `tests/e2e/`. Use `ServerFactory` + `StorageFactory` for fresh instances per test.

## Migration Checklist

1. [ ] Add `CredentialID`, `SigningKeyID`, `AuthorizationCodeID` to `gen_ids.go` → regenerate
2. [ ] Add `ClientID`, `KeyID` to `string_ids.go`
3. [ ] Create entity files in `internal/domain/storage/`
4. [ ] Add repository interfaces to `internal/ports/storage.go`
5. [ ] Create migration files `009–012` in `/migrations/`
6. [ ] Memory adapters in `internal/adapters/storage/memory/`
7. [ ] PostgreSQL adapters in `internal/adapters/storage/postgres/`
8. [ ] Domain services in `internal/domain/oauth2server/`
9. [ ] HTTP handlers in `internal/adapters/http/handlers/`
10. [ ] Routes in `internal/adapters/http/routing/`
11. [ ] Builder wiring in `internal/app/builder.go`
12. [ ] Config changes in `internal/ports/config.go`
13. [ ] Update `ARCHITECTURE.md` glossary
14. [ ] Create ADR `20260328-120100-oauth2-server-mode.md`
15. [ ] E2E tests in `tests/e2e/`
