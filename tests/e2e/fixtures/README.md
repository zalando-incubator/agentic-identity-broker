# E2E Test Data Fixtures

This package provides test data factories for E2E acceptance tests. Fixtures generate valid domain objects (agents, grants, principals, and configurations) with realistic test data.

## Overview

Fixtures are stable test data generators used across E2E test scenarios. They follow deterministic patterns to ensure reproducible test behavior.

**Key Characteristics:**
- Use production domain types from `internal/domain/storage/` and `internal/ports/`
- Generate fresh UUIDs for agents and grants (non-deterministic IDs)
- Return deterministic data for principals and configurations
- All fixtures return valid domain objects (pass domain validation)
- No external dependencies (file I/O, network calls)
- Self-contained and reusable across multiple tests

## Fixture Categories

### 1. Principals (`principals.go`)

Represent test users in authentication contexts. Used for `X-Remote-User` header in authenticated requests.

```go
// Returns a test user with X-Remote-User header support
principal := fixtures.DefaultPrincipal()
// principal.String() = "user@example.com"

// Use in authenticated requests
resp, err := server.AuthenticatedGET("/oauth2/authorize?...", principal, headers)
```

**Available Principals:**
- `DefaultPrincipal()` - Email: `user@example.com`
- `AnotherPrincipal()` - Email: `another-user@example.com`
- `AdminPrincipal()` - Email: `admin@example.com`

**Principal Type:**
```go
type Principal struct {
    Email string
}

// String returns the principal identifier for X-Remote-User header
func (p Principal) String() string { return p.Email }
```

### 2. Agents (`agents.go`)

Represent AI agents registered in the identity broker.

```go
// Basic valid agent with required fields
agent := fixtures.ValidAgent()
// ClientID: "test-client-valid"
// DisplayName: "Test Agent Valid"
// ID: generated UUID (fresh each call)

// Agent with custom client ID
agent := fixtures.AgentWithClientID("custom-client-id")

// Agent with all URL fields populated
agent := fixtures.AgentWithURLs()
// Includes: GovernanceURL, UserDocumentationURL, AgentInterfaceURL
```

**Available Agent Fixtures:**
- `ValidAgent()` - Basic valid agent with all required fields
- `AnotherAgent()` - Alternative agent with different client ID
- `AgentWithClientID(clientID string)` - Agent with specific client ID
- `AgentWithURLs()` - Agent with governance and documentation URLs

**Agent Properties:**
```go
type Agent struct {
    ID                   string     // Generated UUID
    ClientID             string     // Unique client identifier
    DisplayName          string     // Required, max 255 chars
    Description          string     // Required, max 1000 chars
    GovernanceURL        *string    // Optional, validated URL
    UserDocumentationURL *string    // Optional, validated URL
    AgentInterfaceURL    *string    // Optional, validated URL
    CreatedAt            time.Time
    UpdatedAt            time.Time
}
```

### 3. User Grants (`grants.go`)

Represent user permissions delegated to agents for OAuth2 services.

```go
// Active grant (expires in 1 hour)
grant := fixtures.ActiveGrant("user@example.com", agent.ID)
// grant.IsActive() == true

// Expired grant (expired 1 hour ago)
grant := fixtures.ExpiredGrant("user@example.com", agent.ID)
// grant.IsActive() == false

// Grant with custom expiration
grant := fixtures.GrantExpiringIn("user@example.com", agent.ID, 24*time.Hour)
// grant.IsActive() == true (expires in 24 hours)

// Grant that never expires
grant := fixtures.IndefiniteGrant("user@example.com", agent.ID)
// grant.ValidUntil == nil
// grant.IsActive() == true (indefinitely)

// Grant for specific OAuth2 service
grant := fixtures.GrantWithService(
    "user@example.com",
    agent.ID,
    "github-service",
    []string{"repo", "user"},
)

// Grant with multiple services
grant := fixtures.GrantWithMultipleServices("user@example.com", agent.ID)
// Includes: github, google, microsoft services
```

**Available Grant Fixtures:**
- `ActiveGrant(principal, agentID)` - Active for 1 hour
- `ExpiredGrant(principal, agentID)` - Expired 1 hour ago
- `GrantExpiringIn(principal, agentID, duration)` - Expires in specified duration
- `IndefiniteGrant(principal, agentID)` - Never expires (ValidUntil = nil)
- `GrantWithService(principal, agentID, serviceID, scopes)` - Single service grant
- `GrantWithMultipleServices(principal, agentID)` - Multiple OAuth2 services

**Grant Properties:**
```go
type UserGrant struct {
    ID                    string         // Generated UUID
    Principal             string         // User identifier
    AgentID               string         // Agent reference
    ValidUntil            *time.Time     // Expiration (nil = indefinite)
    DelegatedOAuth2Tokens []DelegatedToken
    CreatedAt             time.Time
    UpdatedAt             time.Time
}

type DelegatedToken struct {
    ThirdpartyOAuth2ServiceID string
    Scopes                    []string
}
```

### 4. Configurations (`config.go`)

Represent application configuration for E2E testing. All configs use safe defaults suitable for E2E tests with in-memory storage.

```go
// Default configuration with sensible test defaults
config := fixtures.DefaultOAuth2Config()
// Uses: in-memory storage, localhost:8000, upstream at localhost:19000

// Configuration with custom upstream server
config := fixtures.OAuth2ConfigWithUpstream("http://mock-upstream:8080")
// Automatically sets authorize/token endpoints

// Configuration with custom timeout
config := fixtures.OAuth2ConfigWithTimeout(60) // seconds

// Configuration with custom log level
config := fixtures.OAuth2ConfigWithLogLevel("debug")

// Configuration with custom public URL
config := fixtures.OAuth2ConfigWithPublicURL("https://broker.example.com")
```

**Available Config Fixtures:**
- `DefaultOAuth2Config()` - Safe defaults for E2E testing
- `OAuth2ConfigWithUpstream(url)` - Custom upstream OAuth2 server
- `OAuth2ConfigWithTimeout(seconds)` - Custom upstream timeout
- `OAuth2ConfigWithLogLevel(level)` - Custom log level
- `OAuth2ConfigWithPublicURL(url)` - Custom public URL
- `OAuth2ConfigWithStorage(backend)` - Custom storage backend

**Config Properties (Default Values):**
```go
Storage:
    Backend: "memory"               // In-memory storage (no DB needed)
    Read Timeout: 5 seconds
    Write Timeout: 5 seconds

Server (EndUser):
    Port: 8000
    Bind: "127.0.0.1"
    PublicURL: "http://localhost:8000"
    Auth Header: "X-Remote-User"

OAuth2AuthServer:
    UpstreamIssuer: "http://localhost:19000"
    UpstreamAuthorize: "http://localhost:19000/authorize"
    UpstreamToken: "http://localhost:19000/token"
    SupportedResponseTypes: ["code"]
    SupportedGrantTypes: ["authorization_code", "refresh_token"]
    Timeout: 30 seconds
    Mode: "proxy"

Log:
    Level: "info"
    Format: "text"
```

## Usage Examples

### Basic E2E Test Setup

```go
import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "tests/e2e/fixtures"
    "tests/e2e/bootstrap"
)

var _ = Describe("OAuth2 Authorization", func() {
    var server *bootstrap.TestServer
    var testStorage *storage.Adapter
    var config *ports.Config

    BeforeEach(func() {
        // Create fresh config and storage for each test
        config = fixtures.DefaultOAuth2Config()
        testStorage = bootstrap.NewTestStorage()

        // Build server using production app.Builder
        var err error
        server, err = bootstrap.NewServerFactory(config).
            WithStorage(testStorage).
            Build()
        Expect(err).ToNot(HaveOccurred())
    })

    AfterEach(func() {
        if server != nil {
            server.Shutdown(context.Background())
        }
    })

    It("should authorize valid client", func() {
        // Setup: Register agent and grant using fixtures
        agent := fixtures.ValidAgent()
        Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())

        grant := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID)
        Expect(testStorage.UserGrants().Create(context.Background(), grant)).ToNot(HaveOccurred())

        // Execute: Make authenticated request
        resp, err := server.AuthenticatedGET(
            "/oauth2/authorize?client_id="+agent.ClientID+"&...",
            fixtures.DefaultPrincipal(),
            nil,
        )

        // Verify
        Expect(err).ToNot(HaveOccurred())
        Expect(resp.StatusCode).To(Equal(http.StatusOK))
    })
})
```

### Fixtures with Custom Values

```go
// Create agent with specific client ID
clientID := "my-custom-client"
agent := fixtures.AgentWithClientID(clientID)

// Create grant for specific principal
principal := "john.doe@company.com"
grant := fixtures.ActiveGrant(principal, agent.ID)

// Configure upstream for mock server
mockUpstream := helpers.NewMockUpstreamOAuth2Server()
config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)
```

### Testing Grant Expiration

```go
It("should reject expired grants", func() {
    agent := fixtures.ValidAgent()
    testStorage.Agents().Create(context.Background(), agent)

    // Create expired grant
    expiredGrant := fixtures.ExpiredGrant(
        fixtures.DefaultPrincipal().String(),
        agent.ID,
    )
    testStorage.UserGrants().Create(context.Background(), expiredGrant)

    // Should redirect to consent (not proxy to upstream)
    resp, _ := server.AuthenticatedGET(
        "/oauth2/authorize?client_id="+agent.ClientID+"&...",
        fixtures.DefaultPrincipal(),
        nil,
    )
    Expect(resp.StatusCode).To(Equal(http.StatusFound))
    // Redirect should be to consent, not upstream
})
```

## Fixture Characteristics

### Deterministic Fixtures

Fixtures that return the same data for same input:
- **Principals**: Always return same email for `DefaultPrincipal()`
- **Configurations**: Always return config with same values for `DefaultOAuth2Config()`
- Use these for predictable test behavior

### Non-Deterministic Fixtures

Fixtures that generate fresh UUIDs each time:
- **Agents**: Each call generates unique ID (via UUID)
- **Grants**: Each call generates unique ID (via UUID)
- Use these to ensure test isolation and prevent ID collisions

## Domain Type Validation

All fixtures return valid domain objects that pass domain validation:

```go
// Agent validation
agent := fixtures.ValidAgent()
err := agent.Validate()  // No error

// Grant validation
grant := fixtures.ActiveGrant("user@example.com", "agent-1")
err := grant.Validate()  // No error

// Config validation
config := fixtures.DefaultOAuth2Config()
err := config.OAuth2AuthServer.Validate()  // No error
```

## Testing Fixtures

All fixtures are tested in `examples_test.go`:

```bash
go test -v ./tests/e2e/fixtures/...
```

Tests verify:
1. All fixtures compile without errors
2. Fixtures return non-nil values
3. Fixture data matches expected values
4. Domain validation passes
5. Principal String() method returns email
6. Grant IsActive() method works correctly
7. Config OAuth2AuthServer validation passes
8. Determinism: same input = same output for principals/configs
9. Non-determinism: different UUIDs each call for agents/grants

## Adding New Fixtures

When adding new fixture functions:

1. Follow naming convention: `{Type}{Variant}(params...)`
   - Example: `AgentWithClientID(clientID string)`

2. Return production domain types
   - `*storage.Agent`
   - `*storage.UserGrant`
   - `*ports.Config`
   - `Principal`

3. Generate fresh UUIDs for entities
   - Use `uuid.New().String()` for ID fields

4. Include comprehensive docstring
   - Describe what the fixture creates
   - Document any non-default values
   - Include usage examples

5. Add test coverage in `examples_test.go`
   - Verify fixture returns non-nil value
   - Verify fixture data matches expectations
   - Verify domain validation passes

## Dependencies

Fixtures use only production code:
- `github.com/google/uuid` - UUID generation
- `internal/domain/storage` - Domain types
- `internal/ports` - Port interfaces and config types

No external test dependencies (Ginkgo, Gomega, etc.) are imported by fixtures.

## See Also

- `/tests/e2e/bootstrap/` - Server and storage setup (volatile layer)
- `/tests/e2e/helpers/` - HTTP and mock server utilities
- `/tests/e2e/matchers/` - Custom Gomega matchers
- `/specs/20251222-120100-oauth2-auth-server/spec.md` - Acceptance scenarios mapped to E2E tests
