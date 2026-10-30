# Quickstart: Implementing Token Exchange (RFC 8693)

This guide provides step-by-step instructions for implementing the token exchange feature following the phased approach that ensures E2E tests compile at each stage.

## Prerequisites

- Go 1.24.0+
- Existing codebase with hexagonal architecture
- Understanding of [RFC 8693 Token Exchange](https://datatracker.ietf.org/doc/html/rfc8693)

## Phase Overview

| Phase | Focus | Tests Status |
|-------|-------|--------------|
| 1 | Interfaces & Types | Compiles, 25 failing |
| 2 | E2E Test Skeletons | Compiles, 25 skipped |
| 3 | JWKS Adapter | Compiles, 20+ failing |
| 4 | CEL Authorization | Compiles, 15+ failing |
| 5 | Token Exchange Logic | Compiles, 5+ failing |
| 6 | Integration | Compiles, 0 failing |

---

## Phase 1: Define Interfaces & Types

### Step 1.1: Add Domain Value Objects

Create `internal/domain/tokenexchange/types.go`:

```go
package tokenexchange

import (
    "net/url"
    "time"
)

// ResourceURI represents a normalized protected resource URI
type ResourceURI string

// NewResourceURI creates a normalized ResourceURI (trailing slash removed)
func NewResourceURI(uri string) (ResourceURI, error) {
    parsed, err := url.Parse(uri)
    if err != nil {
        return "", err
    }
    // Normalize: remove trailing slash
    normalized := parsed.String()
    if len(normalized) > 1 && normalized[len(normalized)-1] == '/' {
        normalized = normalized[:len(normalized)-1]
    }
    return ResourceURI(normalized), nil
}

// TokenExchangeRequest represents an RFC 8693 token exchange request
type TokenExchangeRequest struct {
    GrantType           string     `json:"grant_type"`
    SubjectToken        string     `json:"subject_token"`
    SubjectTokenType    string     `json:"subject_token_type"`
    Resource            string     `json:"resource,omitempty"`
    Scope               string     `json:"scope,omitempty"`
    ClientAssertion     string     `json:"client_assertion,omitempty"`
    ClientAssertionType string     `json:"client_assertion_type,omitempty"`
}

// TokenExchangeResponse represents a successful token exchange response
type TokenExchangeResponse struct {
    AccessToken     string `json:"access_token"`
    IssuedTokenType string `json:"issued_token_type"`
    TokenType       string `json:"token_type"`
    ExpiresIn       int64  `json:"expires_in,omitempty"`
    Scope           string `json:"scope,omitempty"`
    RefreshToken    string `json:"refresh_token,omitempty"`
}

// SubjectToken represents validated claims from the subject token
type SubjectToken struct {
    Subject   string
    Issuer    string
    Audience  []string
    Claims    map[string]interface{}
    ExpiresAt time.Time
}

// ClientAssertion represents validated claims from client_assertion JWT
type ClientAssertion struct {
    PrivilegedClientID string                 // Privileged client ID (e.g., API gateway, reverse proxy) from 'sub' claim
    Claims             map[string]interface{} // All claims for CEL evaluation
}
```

### Step 1.2: Add JWKS Port Interface

Create `internal/ports/jwks.go`:

```go
package ports

import (
    "context"

    "github.com/lestrrat-go/jwx/v3/jwk"
)

// JWKSPort abstracts JWKS fetching with caching support
type JWKSPort interface {
    // FetchJWKS retrieves the JWKS for the given issuer URI.
    // Implementations should handle caching and automatic refresh.
    FetchJWKS(ctx context.Context, issuerURI string) (jwk.Set, error)
}
```

### Step 1.3: Add Token Exchange Service Port

Create `internal/ports/tokenexchange.go`:

```go
package ports

import (
    "context"

    "github.com/agentic-identity-broker/internal/domain/tokenexchange"
)

// TokenExchangeService handles RFC 8693 token exchange
type TokenExchangeService interface {
    // Exchange performs the token exchange operation
    Exchange(ctx context.Context, req tokenexchange.TokenExchangeRequest) (*tokenexchange.TokenExchangeResponse, error)
}
```

### Step 1.4: Extend Configuration Types

Add to `internal/ports/config.go`:

```go
// TokenExchangeConfig holds token exchange settings
type TokenExchangeConfig struct {
    Enabled              bool     `yaml:"enabled"`
    TrustedIssuers       []string `yaml:"trusted_issuers"`
    RequireResourceParam bool     `yaml:"require_resource_param"`
    MaxClockSkew         int      `yaml:"max_clock_skew_seconds"` // Default: 60
    CEL                  CELConfig `yaml:"cel"`
}

// CELConfig holds CEL authorization settings
type CELConfig struct {
    AuthorizationExpression string `yaml:"authorization_expression"`
    Timeout                 int    `yaml:"timeout_ms"` // Default: 100
}
```

### Step 1.5: Extend Storage Interface

Add to `internal/ports/storage.go`:

```go
// In ThirdpartyOAuth2ServiceRepository interface, add:

// FindByProtectedResource finds a service that protects the given resource URI.
// Returns nil if no service found.
FindByProtectedResource(ctx context.Context, resourceURI string) (*storage.ThirdpartyOAuth2Service, error)
```

### Step 1.6: Extend Domain Entity

Add to `internal/domain/storage/thirdparty_service.go`:

```go
// In ThirdpartyOAuth2Service struct, add:

// ProtectedResources lists URIs that map to this service for token exchange
ProtectedResources []string `json:"protected_resources" db:"protected_resources"`
```

---

## Phase 2: E2E Test Skeletons

### Step 2.1: Create Test File

Create `tests/e2e/token_exchange_test.go`:

```go
package e2e_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

var _ = Describe("Token Exchange (RFC 8693)", Ordered, func() {
    
    // Test fixtures
    var (
        validSubjectToken     string
        validClientAssertion  string
        expiredSubjectToken   string
        invalidSignatureToken string
    )
    
    BeforeAll(func() {
        // TODO: Generate test JWTs signed with test keys
        Skip("Test fixtures not yet implemented")
    })
    
    Describe("US-001: Exchange Token for Third-Party Token", func() {
        When("valid subject_token and client_assertion provided", func() {
            It("returns third-party access_token with correct grant scope", func() {
                Skip("Implementation pending")
            })
        })
        
        When("user has no grant for the service", func() {
            It("returns error with code 'invalid_target'", func() {
                Skip("Implementation pending")
            })
        })
    })
    
    Describe("US-002: Client Assertion Validation", func() {
        When("client_assertion signature is invalid", func() {
            It("returns error with code 'invalid_client'", func() {
                Skip("Implementation pending")
            })
        })
        
        When("privileged client (agent) not registered", func() {
            It("returns error with code 'invalid_client'", func() {
                Skip("Implementation pending")
            })
        })
        
        When("client_assertion is expired", func() {
            It("returns error with code 'invalid_client'", func() {
                Skip("Implementation pending")
            })
        })
    })
    
    Describe("US-003: Subject Token Validation", func() {
        When("subject_token issuer not in trusted list", func() {
            It("returns error with code 'invalid_request'", func() {
                Skip("Implementation pending")
            })
        })
        
        When("subject_token is expired", func() {
            It("returns error with code 'invalid_request'", func() {
                Skip("Implementation pending")
            })
        })
        
        When("subject_token signature verification fails", func() {
            It("returns error with code 'invalid_request'", func() {
                Skip("Implementation pending")
            })
        })
    })
    
    Describe("US-004: Resource-Based Service Lookup", func() {
        When("resource parameter maps to registered service", func() {
            It("exchanges for that service's tokens", func() {
                Skip("Implementation pending")
            })
        })
        
        When("resource parameter doesn't match any service", func() {
            It("returns error with code 'invalid_target'", func() {
                Skip("Implementation pending")
            })
        })
        
        When("resource omitted but service discoverable", func() {
            It("uses default service lookup", func() {
                Skip("Implementation pending")
            })
        })
    })
    
    Describe("US-005: Scope Filtering", func() {
        When("requested scope is subset of granted scope", func() {
            It("returns token with requested scope only", func() {
                Skip("Implementation pending")
            })
        })
        
        When("requested scope exceeds granted scope", func() {
            It("returns error with code 'invalid_scope'", func() {
                Skip("Implementation pending")
            })
        })
        
        When("no scope requested", func() {
            It("returns token with full granted scope", func() {
                Skip("Implementation pending")
            })
        })
    })
    
    Describe("US-006: CEL Authorization", func() {
        When("CEL expression evaluates to true", func() {
            It("allows the token exchange", func() {
                Skip("Implementation pending")
            })
        })
        
        When("CEL expression evaluates to false", func() {
            It("returns error with code 'access_denied'", func() {
                Skip("Implementation pending")
            })
        })
        
        When("CEL expression times out", func() {
            It("returns error with code 'server_error'", func() {
                Skip("Implementation pending")
            })
        })
    })
    
    Describe("Error Scenarios", func() {
        When("grant_type is not urn:ietf:params:oauth:grant-type:token-exchange", func() {
            It("returns unsupported_grant_type error", func() {
                Skip("Implementation pending")
            })
        })
        
        When("subject_token_type is missing", func() {
            It("returns invalid_request error", func() {
                Skip("Implementation pending")
            })
        })
        
        When("stored third-party token is expired with no refresh_token", func() {
            It("returns invalid_grant error suggesting re-authorization", func() {
                Skip("Implementation pending")
            })
        })
    })
})
```

### Step 2.2: Verify Tests Compile

```bash
cd tests/e2e
go test -c -o /dev/null ./...
```

---

## Phase 3: Implement JWKS Adapter

### Step 3.1: Create JWKS Adapter

Create `internal/adapters/jwks/adapter.go`:

```go
package jwks

import (
    "context"
    "fmt"
    "sync"
    "time"

    "github.com/lestrrat-go/jwx/v3/jwk"
)

// Adapter implements ports.JWKSPort with caching
type Adapter struct {
    cache     *jwk.Cache
    cacheOnce sync.Once
}

// NewAdapter creates a new JWKS adapter
func NewAdapter() *Adapter {
    return &Adapter{}
}

// FetchJWKS retrieves JWKS for the issuer with caching
func (a *Adapter) FetchJWKS(ctx context.Context, issuerURI string) (jwk.Set, error) {
    a.cacheOnce.Do(func() {
        a.cache = jwk.NewCache(ctx)
    })
    
    jwksURI := fmt.Sprintf("%s/.well-known/jwks.json", issuerURI)
    
    // Register URI if not already cached
    // jwk.Cache handles refresh automatically
    if _, err := a.cache.Get(ctx, jwksURI); err != nil {
        // First fetch - register with refresh interval
        err := a.cache.Register(jwksURI, jwk.WithMinRefreshInterval(15*time.Minute))
        if err != nil {
            return nil, fmt.Errorf("registering JWKS URI: %w", err)
        }
    }
    
    return a.cache.Get(ctx, jwksURI)
}
```

### Step 3.2: Wire Adapter in Builder

Add to `internal/app/builder.go`:

```go
import "github.com/agentic-identity-broker/internal/adapters/jwks"

// In Build() method:
jwksAdapter := jwks.NewAdapter()
```

---

## Phase 4: Implement CEL Authorization

### Step 4.1: Add CEL Dependency

```bash
go get github.com/google/cel-go
```

### Step 4.2: Create CEL Evaluator

Create `internal/domain/tokenexchange/cel.go`:

```go
package tokenexchange

import (
    "context"
    "fmt"
    "time"

    "github.com/google/cel-go/cel"
)

// CELEvaluator evaluates authorization expressions
type CELEvaluator struct {
    program cel.Program
    timeout time.Duration
}

// NewCELEvaluator creates a new evaluator with the given expression
func NewCELEvaluator(expression string, timeout time.Duration) (*CELEvaluator, error) {
    env, err := cel.NewEnv(
        cel.Variable("privileged_client", cel.MapType(cel.StringType, cel.DynType)),
        cel.Variable("user", cel.MapType(cel.StringType, cel.DynType)),
        cel.Variable("resource", cel.StringType),
        cel.Variable("scope", cel.ListType(cel.StringType)),
    )
    if err != nil {
        return nil, fmt.Errorf("creating CEL environment: %w", err)
    }
    
    ast, issues := env.Compile(expression)
    if issues != nil && issues.Err() != nil {
        return nil, fmt.Errorf("compiling CEL expression: %w", issues.Err())
    }
    
    prg, err := env.Program(ast)
    if err != nil {
        return nil, fmt.Errorf("creating CEL program: %w", err)
    }
    
    return &CELEvaluator{program: prg, timeout: timeout}, nil
}

// Evaluate runs the CEL expression with the given context
func (e *CELEvaluator) Evaluate(ctx context.Context, vars map[string]interface{}) (bool, error) {
    // Apply timeout
    ctx, cancel := context.WithTimeout(ctx, e.timeout)
    defer cancel()
    
    // Run evaluation in goroutine to respect timeout
    resultCh := make(chan bool, 1)
    errCh := make(chan error, 1)
    
    go func() {
        result, _, err := e.program.Eval(vars)
        if err != nil {
            errCh <- err
            return
        }
        
        boolResult, ok := result.Value().(bool)
        if !ok {
            errCh <- fmt.Errorf("CEL expression must return boolean, got %T", result.Value())
            return
        }
        
        resultCh <- boolResult
    }()
    
    select {
    case <-ctx.Done():
        return false, fmt.Errorf("CEL evaluation timed out")
    case err := <-errCh:
        return false, err
    case result := <-resultCh:
        return result, nil
    }
}
```

---

## Phase 5: Implement Token Exchange Service

### Step 5.1: Create Service Implementation

Create `internal/domain/tokenexchange/service.go`:

```go
package tokenexchange

import (
    "context"
    "fmt"

    "github.com/agentic-identity-broker/internal/ports"
    "github.com/lestrrat-go/jwx/v3/jws"
    "github.com/lestrrat-go/jwx/v3/jwt"
)

// Service implements ports.TokenExchangeService
type Service struct {
    jwks            ports.JWKSPort
    agentRepo       ports.AgentRepository
    serviceRepo     ports.ThirdpartyOAuth2ServiceRepository
    grantRepo       ports.UserGrantRepository
    sessionRepo     ports.UserSessionRepository
    oauth2Service   ports.OAuth2Service
    celEvaluator    *CELEvaluator
    config          ports.TokenExchangeConfig
}

// NewService creates a new token exchange service
func NewService(
    jwks ports.JWKSPort,
    agentRepo ports.AgentRepository,
    serviceRepo ports.ThirdpartyOAuth2ServiceRepository,
    grantRepo ports.UserGrantRepository,
    sessionRepo ports.UserSessionRepository,
    oauth2Service ports.OAuth2Service,
    config ports.TokenExchangeConfig,
) (*Service, error) {
    var celEval *CELEvaluator
    if config.CEL.AuthorizationExpression != "" {
        var err error
        celEval, err = NewCELEvaluator(
            config.CEL.AuthorizationExpression,
            time.Duration(config.CEL.Timeout)*time.Millisecond,
        )
        if err != nil {
            return nil, fmt.Errorf("creating CEL evaluator: %w", err)
        }
    }
    
    return &Service{
        jwks:          jwks,
        agentRepo:     agentRepo,
        serviceRepo:   serviceRepo,
        grantRepo:     grantRepo,
        sessionRepo:   sessionRepo,
        oauth2Service: oauth2Service,
        celEvaluator:  celEval,
        config:        config,
    }, nil
}

// Exchange performs the RFC 8693 token exchange
func (s *Service) Exchange(ctx context.Context, req TokenExchangeRequest) (*TokenExchangeResponse, error) {
    // 1. Validate grant_type
    if req.GrantType != "urn:ietf:params:oauth:grant-type:token-exchange" {
        return nil, &OAuth2Error{Code: "unsupported_grant_type"}
    }
    
    // 2. Validate client_assertion (privileged client identity)
    privilegedClient, err := s.validateClientAssertion(ctx, req.ClientAssertion, req.ClientAssertionType)
    if err != nil {
        return nil, err
    }
    
    // 3. Validate subject_token (user identity from upstream)
    subject, err := s.validateSubjectToken(ctx, req.SubjectToken, req.SubjectTokenType)
    if err != nil {
        return nil, err
    }
    
    // 4. Resolve target service
    service, err := s.resolveService(ctx, req.Resource)
    if err != nil {
        return nil, err
    }
    
    // 5. Find user grant
    grant, err := s.grantRepo.FindByUserAndService(ctx, subject.Subject, service.ID)
    if err != nil || grant == nil {
        return nil, &OAuth2Error{Code: "invalid_target", Description: "no grant for user"}
    }
    
    // 6. CEL authorization check
    if s.celEvaluator != nil {
        allowed, err := s.celEvaluator.Evaluate(ctx, map[string]interface{}{
            "privileged_client": privilegedClient.Claims,
            "user":              subject.Claims,
            "resource":          req.Resource,
            "scope":             parseScopes(req.Scope),
        })
        if err != nil {
            return nil, &OAuth2Error{Code: "server_error", Description: err.Error()}
        }
        if !allowed {
            return nil, &OAuth2Error{Code: "access_denied"}
        }
    }
    
    // 7. Get/refresh third-party token
    session, err := s.sessionRepo.FindByGrantID(ctx, grant.ID)
    if err != nil {
        return nil, &OAuth2Error{Code: "invalid_grant"}
    }
    
    // 8. Filter scope if requested
    responseScope := filterScope(grant.Scope, req.Scope)
    
    return &TokenExchangeResponse{
        AccessToken:     session.AccessToken,
        IssuedTokenType: "urn:ietf:params:oauth:token-type:access_token",
        TokenType:       "Bearer",
        ExpiresIn:       int64(session.ExpiresAt.Sub(time.Now()).Seconds()),
        Scope:           responseScope,
    }, nil
}

// Helper methods...
```

---

## Phase 6: Integration & Testing

### Step 6.1: Run Database Migration

Create `migrations/005_add_protected_resources.up.sql`:

```sql
ALTER TABLE thirdparty_oauth2_services 
ADD COLUMN protected_resources TEXT[] DEFAULT '{}';

CREATE INDEX idx_services_protected_resources 
ON thirdparty_oauth2_services USING GIN (protected_resources);
```

### Step 6.2: Update E2E Tests

Remove `Skip()` calls and add actual test implementations.

### Step 6.3: Run Full Test Suite

```bash
just verify
```

---

## Configuration Example

```yaml
oauth2_auth_server:
  token_exchange:
    enabled: true
    trusted_issuers:
      - "https://auth.example.com"
    require_resource_param: false
    max_clock_skew_seconds: 60
    cel:
      authorization_expression: |
        privileged_client.id in ["privileged-client-1", "privileged-client-2"] &&
        user.email.endsWith("@example.com")
      timeout_ms: 100
```

---

## Troubleshooting

| Error Code | Likely Cause | Solution |
|------------|--------------|----------|
| `invalid_client` | Bad client_assertion | Check JWT signature, privileged client registration |
| `invalid_request` | Bad subject_token | Check issuer in trusted list, token not expired |
| `invalid_target` | No grant or unknown resource | User needs to authorize service |
| `invalid_scope` | Scope exceeds grant | Request subset of granted scopes |
| `access_denied` | CEL denied | Check CEL expression and input values |
