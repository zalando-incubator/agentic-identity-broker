# Quickstart: OAuth2 Authorization Server Proxy Implementation

**Feature**: 20251222-120100-oauth2-auth-server
**Prerequisites**: Features 20251217-120000 (Domain Model APIs) and 007 (Consent Frontend) must be implemented

## Overview

This guide walks through implementing the OAuth2 Authorization Server proxy. The broker acts as an RFC 6749/8414 compliant OAuth2 proxy that validates agents and consent before proxying requests to an upstream OAuth2 server.

**Architecture**: Hexagonal (ports and adapters)
- **Domain Layer**: OAuth2 business logic (authorization decisions, consent validation)
- **Ports**: OAuth2Service interface
- **Adapters**: HTTP handlers (chi router), upstream HTTP client

---

## Step 1: Configuration Schema

Add `oauth2_authorization_server` configuration block to `internal/config/schema.go`:

```go
type Config struct {
    // ... existing fields ...

    OAuth2AuthServer OAuth2AuthServerConfig `mapstructure:"oauth2_authorization_server"`
}

type OAuth2AuthServerConfig struct {
    UpstreamIssuerURI         string   `mapstructure:"upstream_issuer_uri"`
    UpstreamAuthorizeEndpoint string   `mapstructure:"upstream_authorize_endpoint"`
    UpstreamTokenEndpoint     string   `mapstructure:"upstream_token_endpoint"`
    SupportedResponseTypes    []string `mapstructure:"supported_response_types"`
    SupportedGrantTypes       []string `mapstructure:"supported_grant_types"`
    UpstreamTimeout           time.Duration `mapstructure:"upstream_timeout"`
}

// Validate validates OAuth2 configuration
func (c *OAuth2AuthServerConfig) Validate() error {
    if c.UpstreamIssuerURI == "" {
        return errors.New("upstream_issuer_uri is required")
    }
    if c.UpstreamAuthorizeEndpoint == "" {
        return errors.New("upstream_authorize_endpoint is required")
    }
    if c.UpstreamTokenEndpoint == "" {
        return errors.New("upstream_token_endpoint is required")
    }

    // Validate HTTPS
    for _, url := range []string{c.UpstreamIssuerURI, c.UpstreamAuthorizeEndpoint, c.UpstreamTokenEndpoint} {
        if !strings.HasPrefix(url, "https://") {
            return fmt.Errorf("OAuth2 URLs must use HTTPS: %s", url)
        }
    }

    // Set defaults
    if len(c.SupportedResponseTypes) == 0 {
        c.SupportedResponseTypes = []string{"code"}
    }
    if len(c.SupportedGrantTypes) == 0 {
        c.SupportedGrantTypes = []string{"authorization_code", "refresh_token"}
    }
    if c.UpstreamTimeout == 0 {
        c.UpstreamTimeout = 30 * time.Second
    }

    return nil
}
```

**Configuration Example** (`examples/config/oauth2-authorization-server.yaml`):
```yaml
oauth2_authorization_server:
  # Upstream OAuth2 server configuration
  upstream_issuer_uri: "https://upstream-oauth2.example.com"
  upstream_authorize_endpoint: "https://upstream-oauth2.example.com/oauth2/authorize"
  upstream_token_endpoint: "https://upstream-oauth2.example.com/oauth2/token"

  # Identity broker public URL (for metadata and redirects) is configured
  # under server.enduser.public_url, not in this section.

  # Supported OAuth2 flows (optional, defaults shown)
  supported_response_types:
    - "code"
  supported_grant_types:
    - "authorization_code"
    - "refresh_token"

  # Upstream request timeout (optional)
  upstream_timeout: 30s
```

---

## Step 2: Domain Layer

### 2.1 OAuth2 Service Port (`internal/ports/oauth2.go`)

```go
package ports

import (
    "context"
    "net/url"
)

// OAuth2Service coordinates OAuth2 operations
type OAuth2Service interface {
    // HandleAuthorization validates client_id and checks consent
    HandleAuthorization(ctx context.Context, req *AuthorizationRequest, principal string) (*AuthorizationDecision, error)

    // GenerateMetadata returns OAuth2 metadata for discovery
    GenerateMetadata(ctx context.Context) (*MetadataResponse, error)
}

type AuthorizationRequest struct {
    ClientID             string
    RedirectURI          string
    Scope                string
    State                string
    ResponseType         string
    CodeChallenge        string
    CodeChallengeMethod  string
    OriginalURL          *url.URL // Full original request URL
}

type AuthorizationDecision struct {
    Action      string // "redirect_to_upstream", "redirect_to_consent", "error"
    RedirectURL string
    ErrorCode   string
    ErrorDesc   string
}

type MetadataResponse struct {
    Issuer                              string   `json:"issuer"`
    AuthorizationEndpoint               string   `json:"authorization_endpoint"`
    TokenEndpoint                       string   `json:"token_endpoint"`
    ResponseTypesSupported              []string `json:"response_types_supported"`
    GrantTypesSupported                 []string `json:"grant_types_supported"`
    TokenEndpointAuthMethodsSupported   []string `json:"token_endpoint_auth_methods_supported,omitempty"`
}
```

### 2.2 OAuth2 Service Implementation (`internal/domain/oauth2/service.go`)

```go
package oauth2

import (
    "context"
    "fmt"
    "net/url"

    "github.com/your-org/agentic-identity-broker/internal/ports"
)

type service struct {
    agentRepo ports.AgentRepository
    grantRepo ports.GrantRepository
    config    *OAuth2Config
}

type OAuth2Config struct {
    UpstreamAuthorizeEndpoint string
    PublicBaseURL             string
    SupportedResponseTypes    []string
    SupportedGrantTypes       []string
}

func NewService(agentRepo ports.AgentRepository, grantRepo ports.GrantRepository, cfg *OAuth2Config) ports.OAuth2Service {
    return &service{
        agentRepo: agentRepo,
        grantRepo: grantRepo,
        config:    cfg,
    }
}

func (s *service) HandleAuthorization(ctx context.Context, req *ports.AuthorizationRequest, principal string) (*ports.AuthorizationDecision, error) {
    // Step 1: Validate client_id against Agent registry
    agent, err := s.agentRepo.FindByClientID(ctx, req.ClientID)
    if err != nil || agent == nil {
        // Return OAuth2 error: invalid_client
        errorURL, _ := buildErrorRedirectURL(req.RedirectURI, "invalid_client", "Unknown client_id", req.State)
        return &ports.AuthorizationDecision{
            Action:      "error",
            RedirectURL: errorURL,
            ErrorCode:   "invalid_client",
            ErrorDesc:   "Unknown client_id",
        }, nil
    }

    // Step 2: Check if active grant exists
    grant, err := s.grantRepo.FindByPrincipalAndAgent(ctx, principal, agent.ID)
    if err != nil || grant == nil || grant.IsExpired() {
        // No active grant → redirect to consent UI
        consentURL := fmt.Sprintf("%s/consent/agent/%s?redirect_uri=%s",
            s.config.PublicBaseURL,
            agent.ID,
            url.QueryEscape(req.OriginalURL.String()),
        )
        return &ports.AuthorizationDecision{
            Action:      "redirect_to_consent",
            RedirectURL: consentURL,
        }, nil
    }

    // Step 3: Active grant exists → redirect to upstream OAuth2 server
    upstreamURL := buildUpstreamAuthorizeURL(s.config.UpstreamAuthorizeEndpoint, req)
    return &ports.AuthorizationDecision{
        Action:      "redirect_to_upstream",
        RedirectURL: upstreamURL,
    }, nil
}

func (s *service) GenerateMetadata(ctx context.Context) (*ports.MetadataResponse, error) {
    return &ports.MetadataResponse{
        Issuer:                            s.config.PublicBaseURL,
        AuthorizationEndpoint:             s.config.PublicBaseURL + "/oauth2/authorize",
        TokenEndpoint:                     s.config.PublicBaseURL + "/oauth2/token",
        ResponseTypesSupported:            s.config.SupportedResponseTypes,
        GrantTypesSupported:               s.config.SupportedGrantTypes,
        TokenEndpointAuthMethodsSupported: []string{"client_secret_post", "client_secret_basic"},
    }, nil
}

func buildUpstreamAuthorizeURL(baseURL string, req *ports.AuthorizationRequest) string {
    u, _ := url.Parse(baseURL)
    q := u.Query()
    q.Set("client_id", req.ClientID)
    q.Set("redirect_uri", req.RedirectURI)
    q.Set("response_type", req.ResponseType)
    if req.Scope != "" {
        q.Set("scope", req.Scope)
    }
    if req.State != "" {
        q.Set("state", req.State)
    }
    if req.CodeChallenge != "" {
        q.Set("code_challenge", req.CodeChallenge)
        q.Set("code_challenge_method", req.CodeChallengeMethod)
    }
    u.RawQuery = q.Encode()
    return u.String()
}

func buildErrorRedirectURL(redirectURI, errorCode, errorDesc, state string) (string, error) {
    u, err := url.Parse(redirectURI)
    if err != nil {
        return "", err
    }
    q := u.Query()
    q.Set("error", errorCode)
    q.Set("error_description", errorDesc)
    if state != "" {
        q.Set("state", state)
    }
    u.RawQuery = q.Encode()
    return u.String(), nil
}
```

---

## Step 3: HTTP Adapters

### 3.1 Authorization Endpoint Handler (`internal/adapters/http/enduser/oauth2_authorize.go`)

```go
package enduser

import (
    "log/slog"
    "net/http"

    "github.com/your-org/agentic-identity-broker/internal/domain/principal"
    "github.com/your-org/agentic-identity-broker/internal/ports"
)

type OAuth2AuthorizeHandler struct {
    oauth2Svc ports.OAuth2Service
    logger    *slog.Logger
}

func NewOAuth2AuthorizeHandler(svc ports.OAuth2Service, logger *slog.Logger) *OAuth2AuthorizeHandler {
    return &OAuth2AuthorizeHandler{oauth2Svc: svc, logger: logger}
}

func (h *OAuth2AuthorizeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Extract principal from context (set by RequirePrincipalMiddleware)
    // This handler must be protected by RequirePrincipalMiddleware, so MustFromContext is safe
    principalValue := principal.MustFromContext(ctx)

    // Parse authorization request
    authReq := &ports.AuthorizationRequest{
        ClientID:            r.URL.Query().Get("client_id"),
        RedirectURI:         r.URL.Query().Get("redirect_uri"),
        Scope:               r.URL.Query().Get("scope"),
        State:               r.URL.Query().Get("state"),
        ResponseType:        r.URL.Query().Get("response_type"),
        CodeChallenge:       r.URL.Query().Get("code_challenge"),
        CodeChallengeMethod: r.URL.Query().Get("code_challenge_method"),
        OriginalURL:         r.URL,
    }

    // Validate required parameters
    if authReq.ClientID == "" || authReq.RedirectURI == "" || authReq.ResponseType == "" {
        http.Error(w, "Missing required parameter", http.StatusBadRequest)
        return
    }

    // Handle authorization (checks client_id and consent)
    decision, err := h.oauth2Svc.HandleAuthorization(ctx, authReq, principalValue)
    if err != nil {
        h.logger.ErrorContext(ctx, "authorization failed", slog.String("error", err.Error()))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Log authorization decision
    h.logger.InfoContext(ctx, "oauth2_authorization",
        slog.String("client_id", authReq.ClientID),
        slog.String("principal", principalValue),
        slog.String("action", decision.Action),
    )

    // Execute redirect
    http.Redirect(w, r, decision.RedirectURL, http.StatusFound) // 302
}
```

### 3.2 Token Endpoint Handler (`internal/adapters/http/enduser/oauth2_token.go`)

```go
package enduser

import (
    "io"
    "log/slog"
    "mime"
    "net/http"
    "time"
)

type OAuth2TokenHandler struct {
    upstreamClient *http.Client
    upstreamTokenURL string
    logger         *slog.Logger
}

func NewOAuth2TokenHandler(client *http.Client, upstreamURL string, logger *slog.Logger) *OAuth2TokenHandler {
    return &OAuth2TokenHandler{
        upstreamClient:   client,
        upstreamTokenURL: upstreamURL,
        logger:           logger,
    }
}

func (h *OAuth2TokenHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Validate Content-Type (CSRF protection)
    contentType := r.Header.Get("Content-Type")
    mediaType, _, err := mime.ParseMediaType(contentType)
    if err != nil || mediaType != "application/x-www-form-urlencoded" {
        http.Error(w, "Bad Request", http.StatusBadRequest)
        return
    }

    // Create upstream request
    upstreamReq, err := http.NewRequestWithContext(ctx, "POST", h.upstreamTokenURL, r.Body)
    if err != nil {
        h.logger.ErrorContext(ctx, "failed to create upstream request", slog.String("error", err.Error()))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Forward only the validated form Content-Type request header. Client credentials
    // supplied as form fields remain in the request body; credential headers, cookies,
    // and proxy identity headers must not cross the upstream boundary.
    upstreamReq.Header.Set("Content-Type", contentType)

    // Execute request
    upstreamResp, err := h.upstreamClient.Do(upstreamReq)
    if err != nil {
        h.logger.ErrorContext(ctx, "upstream request failed", slog.String("error", err.Error()))
        http.Error(w, "Bad Gateway", http.StatusBadGateway)
        return
    }
    defer upstreamResp.Body.Close()

    // Relay only OAuth2 response metadata. Upstream cookies and arbitrary
    // response headers must not reach the caller.
    for _, header := range []string{"Content-Type", "Cache-Control", "Pragma", "WWW-Authenticate"} {
        for _, value := range upstreamResp.Header.Values(header) {
            w.Header().Add(header, value)
        }
    }


    // Write status code and stream body
    w.WriteHeader(upstreamResp.StatusCode)
    io.Copy(w, upstreamResp.Body)
}

```

### 3.3 Metadata Endpoint Handler (`internal/adapters/http/enduser/oauth2_metadata.go`)

```go
package enduser

import (
    "encoding/json"
    "log/slog"
    "net/http"

    "github.com/your-org/agentic-identity-broker/internal/ports"
)

type OAuth2MetadataHandler struct {
    oauth2Svc ports.OAuth2Service
    logger    *slog.Logger
}

func NewOAuth2MetadataHandler(svc ports.OAuth2Service, logger *slog.Logger) *OAuth2MetadataHandler {
    return &OAuth2MetadataHandler{oauth2Svc: svc, logger: logger}
}

func (h *OAuth2MetadataHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Generate metadata
    metadata, err := h.oauth2Svc.GenerateMetadata(ctx)
    if err != nil {
        h.logger.ErrorContext(ctx, "failed to generate metadata", slog.String("error", err.Error()))
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    // Return JSON response
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(metadata)
}
```

---

## Step 4: Router Integration

Add OAuth2 endpoints to the enduser router (`internal/adapters/http/server.go` or similar):

```go
// OAuth2 endpoints
oauth2Service := oauth2.NewService(agentRepo, grantRepo, &oauth2.OAuth2Config{
    UpstreamAuthorizeEndpoint: cfg.OAuth2AuthServer.UpstreamAuthorizeEndpoint,
    PublicBaseURL:             cfg.OAuth2AuthServer.PublicBaseURL,
    SupportedResponseTypes:    cfg.OAuth2AuthServer.SupportedResponseTypes,
    SupportedGrantTypes:       cfg.OAuth2AuthServer.SupportedGrantTypes,
})

// Create secure upstream HTTP client
upstreamClient, err := NewSecureUpstreamClient(cfg.OAuth2AuthServer.UpstreamTimeout)
if err != nil {
    return nil, fmt.Errorf("failed to create upstream client: %w", err)
}

oauth2AuthorizeHandler := enduser.NewOAuth2AuthorizeHandler(oauth2Service, logger)
oauth2TokenHandler := enduser.NewOAuth2TokenHandler(upstreamClient, cfg.OAuth2AuthServer.UpstreamTokenEndpoint, logger)
oauth2MetadataHandler := enduser.NewOAuth2MetadataHandler(oauth2Service, logger)

// Authorization endpoint requires authenticated principal (protected by RequirePrincipalMiddleware)
// Token endpoint does not require middleware (client authentication via client_secret)
// Metadata endpoint is public (no authentication required)
enduserRouter.Get("/oauth2/authorize", oauth2AuthorizeHandler.ServeHTTP)
enduserRouter.Post("/oauth2/token", oauth2TokenHandler.ServeHTTP)
enduserRouter.Get("/.well-known/oauth-authorization-server", oauth2MetadataHandler.ServeHTTP)
```

---

## Step 5: Testing

### Unit Tests

Test OAuth2 service logic:
```go
func TestHandleAuthorization_NoActiveGrant_RedirectsToConsent(t *testing.T) {
    mockAgentRepo := &MockAgentRepository{}
    mockGrantRepo := &MockGrantRepository{}

    mockAgentRepo.On("FindByClientID", mock.Anything, "test-client").Return(&storage.Agent{ID: "agent-123", ClientID: "test-client"}, nil)
    mockGrantRepo.On("FindByPrincipalAndAgent", mock.Anything, "user@example.com", "agent-123").Return(nil, nil) // No grant

    svc := oauth2.NewService(mockAgentRepo, mockGrantRepo, &oauth2.OAuth2Config{
        PublicBaseURL: "https://broker.example.com",
    })

    req := &ports.AuthorizationRequest{
        ClientID:    "test-client",
        RedirectURI: "https://client.example.com/callback",
        OriginalURL: mustParseURL("https://broker.example.com/oauth2/authorize?client_id=test-client&redirect_uri=https://client.example.com/callback"),
    }

    decision, err := svc.HandleAuthorization(context.Background(), req, "user@example.com")

    assert.NoError(t, err)
    assert.Equal(t, "redirect_to_consent", decision.Action)
    assert.Contains(t, decision.RedirectURL, "/consent/agent/agent-123")
}
```

### Integration Tests

Test end-to-end OAuth2 flow with mock upstream server:
```go
func TestOAuth2Flow_E2E(t *testing.T) {
    // Start mock upstream OAuth2 server
    mockUpstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/oauth2/authorize" {
            // Return authorization code
            redirectURI := r.URL.Query().Get("redirect_uri")
            http.Redirect(w, r, redirectURI+"?code=TEST_CODE&state="+r.URL.Query().Get("state"), http.StatusFound)
        } else if r.URL.Path == "/oauth2/token" {
            // Return access token
            json.NewEncoder(w).Encode(map[string]interface{}{
                "access_token": "TEST_ACCESS_TOKEN",
                "token_type":   "Bearer",
                "expires_in":   3600,
            })
        }
    }))
    defer mockUpstream.Close()

    // Configure broker with mock upstream
    // Create agent and grant
    // Test authorization flow
    // Test token exchange
}
```

---

## Common Pitfalls

1. **Logging Sensitive Data**: Never log access_token, refresh_token, or authorization codes
2. **TLS Validation**: Ensure http.Client uses system CA pool (no InsecureSkipVerify)
3. **Content-Type Validation**: Token endpoint must validate Content-Type to prevent CSRF
4. **Parameter Preservation**: Preserve ALL original query parameters when redirecting to upstream
5. **HTTP Status Codes**: Use 302 (Found) for OAuth2 redirects, not 303 or 307
6. **Error Handling**: Return RFC 6749 compliant error codes (invalid_client, server_error, etc.)
7. **Header Allowlists**: Forward only the validated form Content-Type request header and relay only safe response metadata
8. **Grant Expiration**: Check grant.ValidUntil timestamp (treat NULL as indefinite)

---

## Next Steps

After implementation:
1. Add integration tests with real upstream OAuth2 server (dev environment)
2. Update ARCHITECTURE.md with OAuth2 proxy flow diagram
3. Add end-user documentation to docs/configuration.md
4. Merge OpenAPI spec into /api/enduser/openapi.yaml
5. Run `/speckit.tasks` to generate implementation tasks
