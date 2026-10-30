# ADR 20260119-120000: Token Exchange JWKS Adapter Pattern

**Date**: 2026-01-19
**Status**: Accepted
**Feature**: 20260115-120000-token-exchange

## Context

The RFC 8693 OAuth 2.0 Token Exchange feature requires JWT validation of `client_assertion` and `subject_token` parameters from upstream OAuth2 servers. JWT validation is security-critical and must:

1. Use battle-tested cryptographic libraries (Constitution Principle III: Library-First Security)
2. Validate signatures against upstream server public keys (JWKS - JSON Web Key Sets)
3. Keep HTTP concerns (fetching JWKS) separate from domain logic (hexagonal architecture)

The primary challenge: **How do we fetch and cache JWKS while maintaining clean architectural boundaries between domain and infrastructure?**

### Constraints

- **JWKS fetching is HTTP I/O**: Must be abstracted into adapter layer (HTTP calls never in domain logic)
- **Library requirement**: lestrrat-go/jwx/v3 is already in codebase and provides `jwk.Cache` for transparent JWKS caching with configurable refresh intervals
- **Security-critical**: JWT validation cannot be skipped or made optional
- **Testability**: Domain services must depend on interfaces (JWKSPort), not concrete HTTP clients

### Current State

- lestrrat-go/jwx/v3 available in `go.mod` (used for existing JWT/JWE operations)
- jwx/v3 provides `jwk.Cache` with built-in:
  - Automatic JWKS fetching from configured URI
  - TTL-based caching with `MinRefreshInterval` and `RefreshInterval`
  - Thread-safe concurrent access
  - Background key rotation support
- No existing JWKS adapter layer (this is new)

## Decision

We implement a **dedicated JWKS adapter layer** following hexagonal architecture with the following components:

### Architecture Pattern

```
┌──────────────────────────────────────────────────────┐
│              Domain Layer                             │
│  (TokenExchangeService using JWKSPort interface)    │
└────────────────┬─────────────────────────────────────┘
                 │ depends on (interface)
          ┌──────▼──────────┐
          │   JWKSPort      │
          │   (interface)   │
          │                 │
          │ GetKeySet()     │
          │ GetKey()        │
          └────────┬────────┘
                   │ implements
          ┌────────▼─────────────┐
          │  JWKS Adapter        │
          │  (HTTP + caching)    │
          │                      │
          │ wraps jwk.Cache      │
          │ from lestrrat-go/jwx │
          └────────┬─────────────┘
                   │
          ┌────────▼─────────────┐
          │   lestrrat-go/jwx    │
          │   jwk.Cache          │
          │                      │
          │ (HTTP fetching,      │
          │  TTL caching,        │
          │  key rotation)       │
          └──────────────────────┘
```

### Components

#### 1. **JWKSPort Interface** (`internal/ports/jwks.go`)

Define the contract for JWKS operations without implementation details:

```go
// JWKSPort defines the interface for fetching and retrieving JWKs.
type JWKSPort interface {
    // GetKeySet retrieves the full JWKS from the upstream server.
    // Returns jwk.Set containing all public keys.
    // Errors: if JWKS fetch fails or URI is invalid
    GetKeySet(ctx context.Context) (jwk.Set, error)

    // GetKey retrieves a specific key by key ID from the cached JWKS.
    // Returns jwk.Key if found, or error if not found or fetch fails.
    GetKey(ctx context.Context, kid string) (jwk.Key, error)
}
```

#### 2. **JWKS Adapter** (`internal/adapters/jwks/adapter.go`)

Implements `JWKSPort` using lestrrat-go/jwx/v3 `jwk.Cache`:

```go
type Adapter struct {
    cache jwk.Cache
    uri   string
}

// NewAdapter creates a new JWKS adapter with caching.
func NewAdapter(jwksURI string, minRefresh, maxRefresh time.Duration) (*Adapter, error) {
    cache := jwk.NewCache(context.Background())

    // Configure refresh intervals per research.md recommendations
    cache.Register(jwksURI,
        jwk.WithMinRefreshInterval(minRefresh),      // 15min
        jwk.WithRefreshInterval(maxRefresh),          // 1hr
    )

    return &Adapter{
        cache: cache,
        uri:   jwksURI,
    }, nil
}

// GetKeySet retrieves the full JWKS (possibly cached).
func (a *Adapter) GetKeySet(ctx context.Context) (jwk.Set, error) {
    return a.cache.Get(ctx, a.uri)
}

// GetKey retrieves a specific key by ID from cached JWKS.
func (a *Adapter) GetKey(ctx context.Context, kid string) (jwk.Key, error) {
    set, err := a.GetKeySet(ctx)
    if err != nil {
        return nil, err
    }

    key, ok := set.LookupKeyID(kid)
    if !ok {
        return nil, fmt.Errorf("key %q not found in JWKS", kid)
    }

    return key, nil
}
```

#### 3. **Domain Usage** (`internal/domain/tokenexchange/service.go`)

TokenExchangeService depends on `JWKSPort` interface, not concrete adapter:

```go
type TokenExchangeService struct {
    jwks            ports.JWKSPort         // depends on interface, not concrete
    storage         ports.ThirdpartyServiceRepository
    grants          ports.UserGrantRepository
    logger          ports.LoggerPort
}

// ValidateClientAssertion validates JWT using JWKS.
func (s *TokenExchangeService) ValidateClientAssertion(
    ctx context.Context,
    token string,
) (*jwt.Claims, error) {
    // JWKS adapter transparently handles:
    // - Fetching JWKS from upstream server
    // - Caching with TTL-based refresh
    // - Key rotation
    // - Concurrent access
    //
    // Domain logic only calls interface method, unaware of HTTP/caching:
    jwks, err := s.jwks.GetKeySet(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
    }

    // Parse and verify JWT using JWKS...
}
```

#### 4. **Configuration** (`internal/ports/config.go` extension)

Token exchange configuration includes JWKS caching parameters:

```yaml
token_exchange:
  upstream_jwks_uri: https://oauth2.example.com/.well-known/jwks.json

  # JWKS caching configuration
  jwks:
    # Minimum interval between refresh attempts
    min_refresh_interval: 15m    # from research.md

    # Maximum cache TTL before forced refresh
    max_refresh_interval: 1h     # from research.md
```

### Caching Strategy

Per `specs/20260115-120000-token-exchange/research.md` findings:

- **MinRefreshInterval**: 15 minutes (lestrrat-go/jwx default)
  - Prevents excessive refresh attempts after failed fetches
  - Allows for key rotation without constant requests
  - Suitable for most OAuth2 deployments

- **RefreshInterval**: 1 hour (HTTP caching TTL)
  - Keys refreshed every hour proactively
  - Enables gradual key rotation
  - Balances staleness risk vs HTTP load

- **Error handling**: If fresh JWKS fetch fails, cached keys still available (graceful degradation)

### HTTP Abstraction Enforced

All HTTP operations (JWKS fetching) occur **exclusively in adapter layer**:

- `lestrrat-go/jwx/v3` makes HTTP calls internally within `jwk.Cache.Get()`
- Domain layer never calls HTTP directly; all HTTP I/O goes through `JWKSPort` interface
- Tests mock `JWKSPort` interface without needing HTTP stubbing
- Future changes to JWKS source (e.g., embedded keys, HSM) don't touch domain logic

## Rationale

### Why a Dedicated Adapter?

1. **HTTP Abstraction**: Keeps HTTP I/O (adapter responsibility) separate from domain logic (validation responsibility)
   - Domain service calls `s.jwks.GetKeySet()` without knowing it triggers HTTP
   - HTTP caching/retry logic isolated in adapter
   - Future storage/caching backend changes (Redis, file-based) only touch adapter

2. **Testability**: Domain tests mock `JWKSPort` interface without HTTP stubbing
   ```go
   // In unit tests:
   mockJWKS := &mockJWKSPort{} // No HTTP involved
   service := NewTokenExchangeService(mockJWKS, ...)
   // Test domain logic without flaky HTTP calls
   ```

3. **Library-First Security**: Delegates cryptography to lestrrat-go/jwx/v3 (battle-tested)
   - JWT parsing/verification: jwx handles it
   - JWKS fetching: jwx.Cache handles it
   - Key management: jwx handles it
   - No custom crypto code needed

4. **Caching Transparency**: lestrrat-go/jwx's `jwk.Cache` provides:
   - Transparent HTTP fetching (caller doesn't manage it)
   - Automatic TTL-based refresh
   - Thread-safe concurrent access
   - Background key rotation support
   - No need for custom caching layer

### Why NOT Inline HTTP in Domain?

**Rejected**: Making domain service directly call `http.Client` to fetch JWKS

- Violates hexagonal architecture (Principle VI)
- Domain depends on infrastructure (HTTP client)
- Hard to test (requires HTTP stubbing in unit tests)
- Mixing concerns (validation logic + network I/O)
- Future caching/retry changes affect domain code

### Why NOT Custom Caching Layer?

**Rejected**: Wrapping lestrrat-go/jwx with custom Redis/in-memory cache

- lestrrat-go/jwx's `jwk.Cache` is already production-grade
- Avoids premature optimization
- Reduces code maintenance burden
- Research.md confirms recommended intervals align with jwx defaults
- Can add custom caching layer later if performance requirements change

### Why NOT Direct HTTP Client in Domain?

**Rejected**: Injecting `*http.Client` directly into TokenExchangeService

- Violates port/adapter boundary (domain shouldn't know about HTTP details)
- Couples domain to HTTP library details (timeout config, retry logic)
- Makes domain service harder to test (HTTP stubbing more complex)
- Mixes concerns unnecessarily

## Consequences

### Positive

- **Security Insulated**: Domain logic never directly calls HTTP; all I/O goes through adapter
- **Testable**: Unit tests mock `JWKSPort` interface without HTTP complexity
- **Flexible**: Future changes to JWKS source (Redis cache, HSM, file-based) only touch adapter
- **Standards Compliant**: Uses lestrrat-go/jwx/v3 (library-first security per Principle III)
- **Clear Boundaries**: HTTP concerns (fetching, caching, retry) isolated in adapter; domain concerns (validation) isolated in service
- **Efficient**: lestrrat-go/jwx's built-in `jwk.Cache` handles key rotation, TTL management, concurrent access transparently
- **Auditable**: All JWKS operations logged at adapter level; domain logs only validation results

### Negative

- **Abstraction Overhead**: Additional interface layer (minor complexity)
- **Debugging**: Error traces pass through adapter/domain boundary (stack traces longer)

### Risks

- **Risk**: JWKS fetch timeout blocks token exchange requests
  - **Mitigation**: Configure context timeout in service, jwk.Cache honors contexts

- **Risk**: Stale JWKS keys after upstream rotation
  - **Mitigation**: 1-hour refresh interval + HTTP cache headers provide reasonable staleness tolerance

- **Risk**: jwk.Cache thread safety edge cases
  - **Mitigation**: lestrrat-go/jwx is mature, widely used; testing includes concurrent scenarios

## Alternatives Considered

### 1. Inline HTTP Client in Domain Service

**Decision**: Rejected

```go
// REJECTED: domain directly calls HTTP
type TokenExchangeService struct {
    httpClient *http.Client  // violates hexagonal architecture
}

func (s *TokenExchangeService) ValidateClientAssertion(...) {
    resp, err := s.httpClient.Get(s.jwksURI) // HTTP I/O in domain
}
```

**Rationale for rejection**:
- Violates hexagonal architecture (Principle VI: domain shouldn't know about HTTP)
- Domain logic depends on HTTP library internals (timeout, retry, etc.)
- Unit tests require HTTP stubbing (httptest.Server) making tests more complex
- Future changes to JWKS backend require domain changes

### 2. Custom In-Memory JWKS Cache

**Decision**: Rejected

```go
// REJECTED: custom caching layer when lestrrat provides it
type JWKSCache struct {
    mu       sync.RWMutex
    keys     map[string]jwk.Key
    ttl      time.Duration
    lastFetch time.Time
}
```

**Rationale for rejection**:
- Premature optimization (lestrrat-go/jwx's cache is production-grade)
- Duplicates functionality already in lestrrat-go/jwx
- Adds maintenance burden for key rotation, concurrent access, TTL management
- research.md confirms recommended caching intervals align with jwx defaults

### 3. Third-Party Wrapper Library

**Decision**: Rejected

**Rationale for rejection**:
- lestrrat-go/jwx's built-in `jwk.Cache` is sufficient
- Third-party wrapper adds dependency without clear benefit
- Reduces transparency of what's happening (lestrrat is well-documented)

## Implementation Details

### Configuration in `internal/ports/config.go`

```go
type TokenExchangeConfig struct {
    // JWKS caching configuration
    JWKS struct {
        MinRefreshInterval time.Duration `yaml:"min_refresh_interval" default:"15m"`
        MaxRefreshInterval time.Duration `yaml:"max_refresh_interval" default:"1h"`
    } `yaml:"jwks"`
}
```

### Adapter Factory Pattern (`internal/adapters/jwks/factory.go`)

```go
// Factory function for dependency injection
func NewAdapter(
    jwksURI string,
    config *ports.TokenExchangeConfig,
) (*Adapter, error) {
    return NewAdapterWithIntervals(
        jwksURI,
        config.JWKS.MinRefreshInterval,
        config.JWKS.MaxRefreshInterval,
    )
}
```

### Error Types (`internal/domain/tokenexchange/errors.go`)

```go
// JWKSFetchError indicates JWKS retrieval failed
type JWKSFetchError struct {
    URI       string
    KeyID     string
    Cause     error
}

// KeyNotFoundError indicates key doesn't exist in JWKS
type KeyNotFoundError struct {
    KeyID string
}
```

### Builder Integration (`internal/app/builder.go`)

```go
func (b *Builder) BuildTokenExchangeService() (*tokenexchange.Service, error) {
    jwksAdapter, err := jwks.NewAdapter(
        b.config.TokenExchange.UpstreamJWKSURI,
        b.config.TokenExchange.JWKS,
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create JWKS adapter: %w", err)
    }

    return tokenexchange.NewService(
        jwksAdapter,  // passes interface, not concrete adapter
        b.storage.ThirdpartyServices(),
        b.storage.UserGrants(),
        b.logger,
    ), nil
}
```

## Testing Strategy

### Unit Tests (Domain Level)

**Location**: `internal/domain/tokenexchange/service_test.go`

Mock `JWKSPort` interface without HTTP:

```go
type mockJWKS struct {
    keyset jwk.Set
    err    error
}

func (m *mockJWKS) GetKeySet(ctx context.Context) (jwk.Set, error) {
    return m.keyset, m.err
}

func (m *mockJWKS) GetKey(ctx context.Context, kid string) (jwk.Key, error) {
    // Return test key or error
}

func TestValidateClientAssertion_SucceedsWithValidKey(t *testing.T) {
    mockJWKS := &mockJWKS{keyset: testKeyset}
    service := tokenexchange.NewService(mockJWKS, ...)

    // Test validation without HTTP complexity
    claims, err := service.ValidateClientAssertion(ctx, validJWT)
    require.NoError(t, err)
    require.NotNil(t, claims)
}
```

### Integration Tests (Adapter Level)

**Location**: `tests/integration/tokenexchange/jwks_adapter_test.go`

Test adapter with real lestrrat-go/jwx:

```go
func TestJWKSAdapter_FetchesAndCaches(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Mock JWKS endpoint
        json.NewEncoder(w).Encode(testKeyset)
    }))
    defer server.Close()

    adapter, err := jwks.NewAdapter(server.URL, 1*time.Minute, 1*time.Hour)
    require.NoError(t, err)

    // First call fetches from HTTP
    keyset1, err := adapter.GetKeySet(context.Background())
    require.NoError(t, err)

    // Second call returns cached copy (no HTTP call)
    keyset2, err := adapter.GetKeySet(context.Background())
    require.NoError(t, err)
    require.Equal(t, keyset1, keyset2)
}
```

### E2E Tests (Feature Level)

**Location**: `tests/e2e/token_exchange_test.go`

E2E tests verify complete flow with real servers:

```go
It("should validate client_assertion using JWKS", func() {
    // Setup: create mock upstream JWKS endpoint
    // Execute: exchange token via /oauth2/token endpoint
    // Verify: client_assertion validated correctly
})
```

## References

- **Library**: lestrrat-go/jwx/v3 - JWT/JWE/JWK handling with JWKS caching
  - Documentation: https://github.com/lestrrat-go/jwx
  - JWKS Cache: https://pkg.go.dev/github.com/lestrrat-go/jwx/v3/jwk#Cache
  - Examples: https://github.com/lestrrat-go/jwx/examples

- **Architecture**:
  - Hexagonal Architecture: Alistair Cockburn, "Ports and Adapters Pattern"
  - SOLID Principles: Domain-Driven Design

- **Research**:
  - specs/20260115-120000-token-exchange/research.md - JWKS caching research and recommendations
  - specs/20260115-120000-token-exchange/spec.md - Token Exchange feature specification

- **Binding References**:
  - Constitution Principle II: Architecture Documentation & ADRs (Binding)
  - Constitution Principle III: Library-First Security (lestrrat-go/jwx/v3)
  - Constitution Principle VI: Hexagonal Architecture (port/adapter separation)
  - ADR 20251215-120300: Storage Layer Architecture (port/adapter patterns)

- **Implementation Plan**: specs/20260115-120000-token-exchange/plan.md

## Sign-Off

**Architecture Justification**: JWKS adapter pattern follows hexagonal architecture (Principle VI) by separating HTTP concerns (adapter) from domain validation logic (service). Using lestrrat-go/jwx/v3 for caching satisfies library-first security (Principle III). Port/adapter boundary clearly defined via `JWKSPort` interface enables testability and future flexibility.

**Compliance**:
- Principle II (Binding ADRs): This ADR documents major architectural decision; implementation must follow it
- Principle III (Library-First Security): Uses lestrrat-go/jwx/v3, no custom crypto
- Principle VI (Hexagonal Architecture): Clear port (JWKSPort) and adapter (jwks.Adapter) separation

**Implementation Status**: Accepted - Implementation begins with E2E tests written first (Principle XIII: End-to-End Acceptance Testing).

**Date Accepted**: 2026-01-19
**Status**: Accepted
