# End-to-End Test Suite

## Overview

This directory contains comprehensive end-to-end (E2E) tests for the Agentic Identity Broker using Ginkgo/Gomega. These tests validate the complete OAuth2 authorization server proxy functionality against acceptance scenarios defined in the specification.

**Test Scope**: E2E tests verify the entire system integration with a real HTTP server, production app initialization, and full request/response cycles. These are distinct from unit tests which test individual components in isolation.

## E2E vs Integration Tests

| Aspect | E2E Tests | Integration Tests |
|--------|-----------|-------------------|
| **Scope** | Full system with HTTP server | Single components (storage, handlers) |
| **Server** | Real httptest.Server | Mocked or no server |
| **Storage** | In-memory adapters | Direct storage access |
| **Location** | `/tests/e2e/**_test.go` | `/internal/**/*_test.go` |
| **Framework** | Ginkgo/Gomega | Testing/testify |
| **Run Command** | `ginkgo -v ./tests/e2e/` | `go test -v ./...` |
| **Test Count** | 62 scenarios | Hundreds of unit tests |

## Test Architecture Overview

### Layer 1: Bootstrap (`bootstrap/`)

**Purpose**: Creates production app instances and HTTP test servers for E2E testing.

**Components**:
- `test_server.go` - Wraps httptest.Server with authenticated request methods
  - `NewEndUserTestServer()` - Creates end-user server (OAuth2, consent routes)
  - `NewAdminTestServer()` - Creates admin server (agent/service management routes)
  - `NewTestServer()` - Flexible server creation with options pattern
  - `AuthenticatedGET/POST()` - Makes requests with X-Remote-User injection
  - `PublicGET()` - Makes unauthenticated requests
  - `DirectRequest()` - Advanced request control

- `server_factory.go` - Builds production app instances
  - `NewServerFactory()` - Creates factory with config
  - `BuildApp()` - Constructs fully-wired app.App using dependency injection
  - Handles app builder pattern: dependencies → app creation → ready to serve

- `storage.go` - Initializes storage adapters for testing
  - `NewTestStorage()` - Creates fresh in-memory storage
  - `StorageFactory` - Manages storage lifecycle

**Key Design**:
- Uses production code paths (no test doubles)
- httptest.Server for HTTP layer testing
- In-memory storage for fast tests without database
- Dependency injection for app configuration
- **Separate servers for end-user and admin routes** - Tests now use distinct server instances matching production architecture:
  - End-user server: OAuth2 endpoints, consent UI, public APIs
  - Admin server: Agent and service management APIs
  - Use `NewEndUserTestServer()` for OAuth2/consent tests
  - Use `NewAdminTestServer()` for agent/service management tests
  - Use both servers in tests that need both route types (see `agent_permission_requirements_test.go`)
- See `test_server.go` for detailed flow documentation

### Layer 2: Fixtures (`fixtures/`)

**Purpose**: Generates stable test data (agents, grants, principals, configurations).

**Key Characteristics**:
- Returns production domain types (not test-specific objects)
- Deterministic for principals/configs (same input = same output)
- Non-deterministic for entities (fresh UUIDs each call for isolation)
- All data passes domain validation
- No external dependencies (files, network)

**Components**:
- `principals.go` - Test user identities
  - `DefaultPrincipal()` - "user@example.com"
  - `AnotherPrincipal()` - "another-user@example.com"
  - `AdminPrincipal()` - "admin@example.com"

- `agents.go` - Registered OAuth2 client agents
  - `ValidAgent()` - Basic agent with required fields
  - `AgentWithClientID(id)` - Custom client ID
  - `AgentWithURLs()` - Governance and documentation URLs

- `grants.go` - User permissions for agents
  - `ActiveGrant()` - Valid for 1 hour
  - `ExpiredGrant()` - Expired 1 hour ago
  - `GrantExpiringIn()` - Custom expiration
  - `IndefiniteGrant()` - Never expires
  - `GrantWithService()` - For specific OAuth2 service

- `config.go` - Application configuration
  - `DefaultOAuth2Config()` - Safe E2E defaults
  - `OAuth2ConfigWithUpstream()` - Custom upstream server
  - `OAuth2ConfigWithTimeout()` - Custom upstream timeout
  - `OAuth2ConfigWithPublicURL()` - Custom public URL

**Usage Example**:
```go
agent := fixtures.ValidAgent()
grant := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID)
config := fixtures.OAuth2ConfigWithUpstream(mockServer.URL)
```

See `FIXTURE_EXAMPLES.md` for comprehensive fixture documentation.

### Layer 3: Helpers (`helpers/`)

**Purpose**: Utility functions for common test operations.

**Components**:
- `http_helpers.go` - HTTP response utilities
  - Response body parsing helpers
  - Status code assertions
  - Header extraction

- `mock_upstream.go` - Mock OAuth2 upstream server
  - `NewMockUpstreamOAuth2Server()` - Creates mock server
  - Implements OAuth2 endpoints (authorize, token, metadata)
  - Captures requests for verification
  - Close() for cleanup

**Usage Example**:
```go
mockUpstream := helpers.NewMockUpstreamOAuth2Server()
defer mockUpstream.Close()

config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)
```

### Layer 4: Matchers (`matchers/`)

**Purpose**: Custom Gomega matchers for OAuth2 assertions.

**Components**:
- `oauth2_matchers.go` - OAuth2-specific matchers
  - `HaveOAuth2Error(code, description)` - Verify error responses
  - `BeRedirectTo()` - Verify redirect targets
  - `ContainOAuth2Metadata()` - Verify metadata structure

**Usage Example**:
```go
Expect(body).To(matchers.HaveOAuth2Error("invalid_client", ""))
Expect(resp.StatusCode).To(Equal(http.StatusFound))
```

## Running E2E Tests

### Quick Start

```bash
# Run the functional backend E2E suite (excludes performance-labelled specs)
just test-e2e-backend

# Run the functional backend suite with coverage report
just test-e2e-backend-coverage

# Watch the functional backend suite during development (auto-rerun on changes)
just test-e2e-backend-watch

# Run the dedicated warmed local-mode, signed-subject 100-request SC-001 measurement
just test-e2e-performance

# Run all functional backend, ExtProc, and frontend E2E suites
just test-e2e

# Run specific functional backend suite
ginkgo -v --label-filter="!performance" ./tests/e2e/oauth2_authorize_test.go

# Run functional tests matching pattern
ginkgo -v --label-filter="!performance" --focus="should redirect to consent" ./tests/e2e/

### Ginkgo Command Reference

```bash
# Run all functional backend tests in verbose mode
ginkgo -v --label-filter="!performance" ./tests/e2e/

# Run the dedicated performance-labelled SC-001 measurement
ginkgo -v --procs=1 --label-filter="performance" ./tests/e2e/

# Run functional backend tests with coverage
ginkgo -v --label-filter="!performance" --cover ./tests/e2e/

# Generate functional backend HTML coverage report
ginkgo -v --label-filter="!performance" --coverprofile=coverage/e2e-backend.out ./tests/e2e/
go tool cover -html=coverage/e2e-backend.out -o coverage/e2e-backend.html

# Watch functional backend tests
ginkgo watch -v --label-filter="!performance" ./tests/e2e/

# Run a functional backend suite file
ginkgo -v --label-filter="!performance" ./tests/e2e/oauth2_authorize_test.go

# Run functional backend tests matching pattern
ginkgo -v --label-filter="!performance" --focus="Authorization" ./tests/e2e/

# Run functional backend tests excluding pattern
ginkgo -v --label-filter="!performance" --skip="Edge Cases" ./tests/e2e/

# Functional backend parallel execution (4 workers)
ginkgo -v --procs=4 --label-filter="!performance" ./tests/e2e/

# Functional backend JUnit reporter
ginkgo -v --label-filter="!performance" --reporter=junit ./tests/e2e/

# Show pending functional backend tests
ginkgo -v --label-filter="!performance" --pending ./tests/e2e/

# Set random seed for functional backend test execution order
ginkgo -v --label-filter="!performance" --seed=12345 ./tests/e2e/
```

`just test-e2e-performance` is a manually run, warmed local-mode, signed-subject 100-request check. Copy its verbose `SC-001: <ok>/100 succeeded; p95=<duration> max=<duration> min=<duration>` result into PR #464; functional recipes intentionally exclude this machine-sensitive measurement.

### Running Tests Locally

1. **Ensure Ginkgo is installed**:
   ```bash
   go install github.com/onsi/ginkgo/v2/ginkgo@latest
   go install github.com/onsi/gomega/...@latest
   ```

2. **Run tests from project root**:
   ```bash
   cd /path/to/agentic-identity-broker
   just test-e2e-backend
   ```

3. **View coverage**:
   ```bash
   just test-e2e-backend-coverage
   # Opens coverage/e2e-backend.html in your browser
   ```

## Test Organization & Structure

### Directory Layout

```
tests/e2e/
├── README.md                          # This file
├── e2e_suite_test.go                 # Test suite entry point (Ginkgo setup)
├── bootstrap/                         # Server and storage initialization
│   ├── test_server.go                # HTTP test server wrapper
│   ├── server_factory.go             # App instance builder
│   └── storage.go                    # Storage adapter initialization
├── fixtures/                          # Test data factories
│   ├── principals.go                 # User identities
│   ├── agents.go                     # OAuth2 client agents
│   ├── grants.go                     # User permissions
│   ├── config.go                     # Application configuration
│   ├── FIXTURE_EXAMPLES.md           # Detailed fixture guide
│   ├── IMPLEMENTATION_SUMMARY.md     # Fixture implementation notes
│   └── examples_test.go              # Fixture verification tests
├── helpers/                           # Utility functions
│   ├── http_helpers.go               # HTTP response utilities
│   ├── mock_upstream.go              # Mock OAuth2 server
│   └── mock_upstream_test.go         # Mock server tests
├── matchers/                          # Custom Gomega matchers
│   ├── oauth2_matchers.go            # OAuth2 assertions
│   └── oauth2_matchers_test.go       # Matcher tests
└── Test Files (62 scenarios total)
    ├── oauth2_authorize_test.go      # Authorization endpoint tests (23 scenarios)
    ├── oauth2_token_test.go          # Token endpoint tests (12 scenarios)
    ├── oauth2_metadata_test.go       # Metadata endpoint tests (8 scenarios)
    ├── oauth2_security_test.go       # Security and validation tests (12 scenarios)
    └── oauth2_edge_cases_test.go     # Edge case handling (7 scenarios)
```

### Test File Structure

Each test file follows this Ginkgo pattern:

```go
package e2e_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    // ... imports
)

var _ = Describe("Feature Name", func() {
    var (
        server *bootstrap.TestServer
        testStorage *storageadapter.Adapter
        // ... test variables
    )

    BeforeEach(func() {
        // Setup: Create fresh fixtures and server for each test
        server, _ = createTestServer()
        testStorage.Agents().Create(context.Background(), agent)
    })

    AfterEach(func() {
        // Cleanup: Close server and storage
        server.Close()
    })

    Describe("when condition", func() {
        It("should verify behavior", func() {
            // Act: Make request
            resp, _ := server.AuthenticatedGET("/endpoint", principal)

            // Assert: Verify response
            Expect(resp.StatusCode).To(Equal(http.StatusOK))
        })
    })
})
```

## Test Suites

### 1. Authorization Endpoint (`oauth2_authorize_test.go`)

**Priority**: P1 - Entry point for OAuth2 flows

**23 Scenarios**:
- Client ID validation (agent lookup)
- Invalid/missing client_id handling
- Consent check (active grant verification)
- Redirect to consent UI when needed
- Proxy to upstream when consent exists
- Parameter preservation in proxy
- Redirect behavior (302 vs 303)

**Maps to Spec**: `specs/009-oauth2-auth-server/spec.md` - User Story 1

### 2. Token Endpoint (`oauth2_token_test.go`)

**Priority**: P2 - Authorization code exchange

**12 Scenarios**:
- Authorization code exchange for access token
- Parameter forwarding (grant_type, code, client_id, client_secret)
- Refresh token handling
- Content-type support (form-encoded, JSON)
- Error response proxying
- Status code preservation

**Maps to Spec**: `specs/009-oauth2-auth-server/spec.md` - User Story 2

### 3. Metadata Endpoint (`oauth2_metadata_test.go`)

**Priority**: P3 - OAuth2 discovery

**8 Scenarios**:
- RFC 8414 compliance
- Issuer URL setting
- Endpoints exposure (authorize, token)
- Supported response types and grant types
- Supported auth methods
- Cache headers

**Maps to Spec**: `specs/009-oauth2-auth-server/spec.md` - User Story 3

### 4. Security & Validation (`oauth2_security_test.go`)

**Priority**: P1 - Security requirements

**12 Scenarios**:
- CSRF protection validation
- State parameter preservation
- Redirect URI validation
- Upstream TLS certificate validation
- Error handling (upstream unavailable)
- Timeout handling
- Malformed request handling

**Maps to Spec**: `specs/009-oauth2-auth-server/spec.md` - Edge Cases & Security

### 5. Edge Cases (`oauth2_edge_cases_test.go`)

**Priority**: Variable - Complex scenarios

**7 Scenarios**:
- Grant expiration handling
- Multiple agents with same client_id (error)
- Missing redirect_uri handling
- PKCE parameter transparency
- Session expiration during flow
- Parameter limit handling
- Concurrent request handling

**Maps to Spec**: `specs/009-oauth2-auth-server/spec.md` - Edge Cases

## Ginkgo Test Organization Patterns

### Container Nodes

Ginkgo provides several container nodes for organizing tests hierarchically:

**`Describe(description, func())`**
- Groups related test scenarios
- Typically used for: endpoint/feature name at top level, then nested for conditions
- Example: `Describe("OAuth2 Authorization Endpoint", func() { ... })`
- Can be nested for progressive organization

**`Context(description, func())`**
- Adds additional preconditions or variants within a Describe
- Use "and" to connect to parent condition
- Example: `Context("and no grant exists", func() { ... })`
- Equivalent to nested Describe, but semantically indicates a variant/sub-condition

**`When(description, func())`**
- Alternative to Context, same functionality
- Use for BDD-style "when" clauses if preferred
- Less commonly used in this codebase

**`It(description, func())`**
- Individual test case - the actual test that runs
- **Critical: Each It() block maps to ONE acceptance scenario from spec.md**
- Use "should" to describe expected behavior
- Example: `It("should redirect to consent UI", func() { ... })`

### BDD Mapping Pattern

Our tests follow BDD (Behavior-Driven Development) structure mapping to Given/When/Then:

| BDD Element | Ginkgo Element | Purpose | Example |
|-------------|----------------|---------|---------|
| **Feature** | Outer `Describe` | Component/endpoint under test | `Describe("OAuth2 Authorization Endpoint", ...)` |
| **Given** (preconditions) | Nested `Describe` + `BeforeEach` | Scenario preconditions | `Describe("when a valid request arrives", ...)` + agent setup |
| **And** (additional conditions) | `Context` + `BeforeEach` | Variants/sub-conditions | `Context("and no grant exists", ...)` |
| **When** (action) | First part of `It` block | The action being tested | Making HTTP request in It block |
| **Then** (assertion) | `Expect` statements in `It` | Expected outcome | `Expect(resp.StatusCode).To(Equal(...))` |

**Example mapping:**
```
Given a valid authorization request arrives (Describe + BeforeEach creates agent)
And no grant exists (Context - deliberately don't create grant)
When the system processes the request (It block makes HTTP call)
Then it should redirect to consent UI (Expect statements verify redirect)
```

### Hierarchical Test Organization

Tests should be organized hierarchically with progressive setup, where each level adds specific preconditions:

```go
var _ = Describe("OAuth2 Authorization Endpoint", func() {
    var (
        server      *bootstrap.TestServer
        testStorage *storageadapter.Adapter
        agent       *storage.Agent
    )

    BeforeEach(func() {
        // Suite-level setup - runs for EVERY test in this Describe
        server, testStorage = createTestServer()
    })

    AfterEach(func() {
        // Suite-level cleanup - runs after EVERY test
        server.Close()
    })

    Describe("when a valid authorization request arrives", func() {
        BeforeEach(func() {
            // Scenario-level setup - runs for all tests in this Describe
            // All tests in this block need a valid agent
            agent = fixtures.ValidAgent()
            testStorage.Agents().Create(context.Background(), agent)
        })

        Context("and no grant exists", func() {
            // No additional BeforeEach needed - grant intentionally absent

            It("should redirect to consent UI", func() {
                // Test: valid agent + no grant -> consent redirect
                resp, err := server.AuthenticatedGET(...)
                Expect(resp).To(matchers.HaveOAuth2Redirect("/agents/"))
            })

            It("should preserve original request URL in redirect_uri parameter", func() {
                // Test: valid agent + no grant -> URL preserved
                resp, err := server.AuthenticatedGET(...)
                redirectURL, _ := helpers.ExtractRedirectURL(resp)
                Expect(redirectURL.Query().Get("redirect_uri")).To(ContainSubstring(...))
            })
        })

        Context("and active grant exists", func() {
            BeforeEach(func() {
                // Context-level setup - ONLY for tests with active grant
                // Runs AFTER scenario-level BeforeEach (agent already created)
                grant := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID)
                testStorage.UserGrants().Create(context.Background(), grant)
            })

            It("should proxy request to upstream OAuth2 server", func() {
                // Test: valid agent + active grant -> proxy to upstream
                resp, err := server.AuthenticatedGET(...)
                Expect(resp).To(matchers.HaveOAuth2Redirect(mockUpstream.Server.URL))
            })

            It("should preserve all OAuth2 parameters in proxy request", func() {
                // Test: valid agent + active grant -> parameters preserved
                resp, err := server.AuthenticatedGET(
                    "/oauth2/authorize?client_id=...&scope=openid+profile&code_challenge=abc",
                    fixtures.DefaultPrincipal().String(),
                )
                redirectURL, _ := helpers.ExtractRedirectURL(resp)
                Expect(redirectURL.Query().Get("scope")).To(Equal("openid profile"))
                Expect(redirectURL.Query().Get("code_challenge")).To(Equal("abc"))
            })
        })

        Context("and expired grant exists", func() {
            BeforeEach(func() {
                // Context-level setup - expired grant variant
                grant := fixtures.ExpiredGrant(fixtures.DefaultPrincipal().String(), agent.ID)
                testStorage.UserGrants().Create(context.Background(), grant)
            })

            It("should treat expired grant as non-existent and redirect to consent", func() {
                // Test: valid agent + expired grant -> same as no grant
                resp, err := server.AuthenticatedGET(...)
                Expect(resp).To(matchers.HaveOAuth2Redirect("/agents/"))
            })
        })
    })

    Describe("when authorization request has invalid client_id", func() {
        // No BeforeEach - deliberately no agent created to test invalid client

        It("should return OAuth2 error response indicating invalid_client", func() {
            // Test: no agent exists -> invalid_client error
            resp, err := server.AuthenticatedGET(
                "/oauth2/authorize?client_id=non-existent&redirect_uri=...",
                fixtures.DefaultPrincipal().String(),
            )
            Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
        })
    })
})
```

**Key Benefits of This Structure:**

1. **Shared Setup**: Common preconditions (agent creation) are set up once for all related tests
2. **Variant-Specific Setup**: Each Context adds only what's unique to that variant (grant state)
3. **Test Isolation**: Each test still gets fresh storage from suite-level BeforeEach
4. **Clear Hierarchy**: Structure mirrors the logical flow of acceptance scenarios
5. **DRY Principle**: No duplicate setup code across similar tests
6. **Easy Maintenance**: Adding a new variant (e.g., "and grant expires in 5 minutes") is just a new Context block

**BeforeEach Execution Order:**

For a test in the innermost Context, BeforeEach blocks execute from outermost to innermost:

```
1. Suite-level BeforeEach (creates server, storage)
2. Scenario-level BeforeEach (creates agent)
3. Context-level BeforeEach (creates grant)
4. Test runs (It block)
5. Context-level AfterEach (if any)
6. Scenario-level AfterEach (if any)
7. Suite-level AfterEach (closes server, storage)
```

### Test Naming Conventions

Follow these conventions for consistency and readability:

**Describe blocks:**
- Top level: Feature/component name
  - `Describe("OAuth2 Authorization Endpoint", ...)`
  - `Describe("OAuth2 Token Endpoint", ...)`
- Nested: Condition using "when"
  - `Describe("when a valid authorization request arrives", ...)`
  - `Describe("when upstream OAuth2 server is unreachable", ...)`

**Context blocks:**
- Use "and" to add conditions
  - `Context("and no grant exists", ...)`
  - `Context("and active grant exists", ...)`
  - `Context("and expired grant exists", ...)`
- Describe state variants or additional preconditions
- Keep descriptions focused on the ONE thing that differs from sibling Contexts

**It blocks:**
- Use "should" + expected behavior
  - `It("should redirect to consent UI", ...)`
  - `It("should preserve all OAuth2 parameters", ...)`
  - `It("should return OAuth2 error invalid_client", ...)`
- Be specific about the behavior being tested
- Match acceptance scenario language from spec.md when possible
- Include enough detail to understand what's being tested without reading the code

**Examples from actual tests:**

Good naming:
```go
Describe("when a valid authorization request arrives", func() {
    Context("and no grant exists", func() {
        It("should redirect to consent UI with agent ID in path", func() {
```

Avoid:
```go
Describe("authorization", func() {  // Too vague
    It("works", func() {  // Not descriptive
```

### Common Patterns

**Pattern 1: Testing Multiple Variants of Same Scenario**

When testing the same action under different preconditions:

```go
Describe("when processing authorization request", func() {
    var agent *storage.Agent

    BeforeEach(func() {
        agent = fixtures.ValidAgent()
        testStorage.Agents().Create(context.Background(), agent)
    })

    // Each Context tests same action with different grant state
    Context("and no grant exists", func() {
        It("should redirect to consent", func() { /* test */ })
    })

    Context("and active grant exists", func() {
        BeforeEach(func() { /* create active grant */ })
        It("should proxy to upstream", func() { /* test */ })
    })

    Context("and expired grant exists", func() {
        BeforeEach(func() { /* create expired grant */ })
        It("should redirect to consent", func() { /* test */ })
    })
})
```

**Pattern 2: Testing Multiple Behaviors Under Same Preconditions**

When multiple things should happen under the same conditions:

```go
Context("and active grant exists", func() {
    BeforeEach(func() {
        // Single setup for all related tests
        grant := fixtures.ActiveGrant(...)
        testStorage.UserGrants().Create(context.Background(), grant)
    })

    // Multiple Its test different aspects of same behavior
    It("should proxy to upstream", func() { /* test redirect happens */ })
    It("should preserve OAuth2 parameters", func() { /* test params preserved */ })
    It("should preserve state parameter for CSRF", func() { /* test state preserved */ })
    It("should preserve PKCE parameters", func() { /* test PKCE preserved */ })
})
```

**Pattern 3: Testing Error Conditions**

Error scenarios often don't need Context nesting:

```go
Describe("when authorization request is malformed", func() {
    It("should return invalid_request when client_id is missing", func() { /* test */ })
    It("should return invalid_request when redirect_uri is missing", func() { /* test */ })
    It("should return unsupported_response_type when response_type is invalid", func() { /* test */ })
})
```

## Mapping Tests to Acceptance Scenarios

### Core Principle

**Rule: Each `It()` block maps to ONE acceptance scenario from spec.md**

This 1:1 mapping ensures:
- **Complete coverage**: Every spec scenario has a test
- **Traceability**: Can map test failures back to spec requirements
- **Clear intent**: Test names explain what's being validated
- **Review alignment**: Stakeholders can verify acceptance criteria are tested

### Mapping Approach

When implementing tests from spec scenarios:

#### 1. Reference the Spec Scenario

Add comments in your test code linking to the spec:

```go
// Scenario 1.1 from specs/009-oauth2-auth-server/spec.md (User Story 1)
// Spec: "Given a valid authorization request, When client_id is valid,
//        Then system looks up Agent and validates it exists"
It("should look up the agent by client_id and validate it exists", func() {
    // Test implementation
})
```

#### 2. Name Tests to Match Spec Language

Use the spec's terminology in your test names:

- **Spec says**: "system redirects to consent UI when no grant exists"
- **Test should be**: `It("should redirect to consent UI when no user consent exists", ...)`

- **Spec says**: "parameters are preserved during proxy to upstream"
- **Test should be**: `It("should preserve all OAuth2 parameters in proxy request", ...)`

#### 3. Organize Tests to Match Spec Structure

The spec's user stories and scenarios should map to test file organization:

- **Spec User Story 1** → `Describe("OAuth2 Authorization Endpoint", ...)`
  - Scenario 1.1 → First `It()` in relevant Context
  - Scenario 1.2 → Second `It()` or separate Context if preconditions differ
  - Scenario 1.3 → Additional `It()` or Context

- **Spec User Story 2** → `Describe("OAuth2 Token Endpoint", ...)`
  - Scenarios 2.1-2.6 → Individual `It()` blocks

### Mapping Example

Here's how spec scenarios map to test organization:

**From spec.md - User Story 1: Authorization Endpoint**

```
Scenario 1.1: Valid client lookup
Given: User authenticated, valid client_id
When: Authorization request arrives
Then: System looks up agent and processes request

Scenario 1.2: Invalid client_id
Given: User authenticated, invalid client_id
When: Authorization request arrives
Then: System returns invalid_client error

Scenario 1.3: No grant exists
Given: User authenticated, valid client_id, no grant
When: Authorization request arrives
Then: System redirects to consent UI with preserved request

Scenario 1.4: Active grant exists
Given: User authenticated, valid client_id, active grant
When: Authorization request arrives
Then: System proxies to upstream with preserved parameters
```

**Maps to test code:**

```go
// oauth2_authorize_test.go
var _ = Describe("OAuth2 Authorization Endpoint", func() {
    // Scenarios 1.1, 1.3, 1.4 all need valid agent
    Describe("when a valid authorization request arrives", func() {
        BeforeEach(func() {
            agent = fixtures.ValidAgent()
            testStorage.Agents().Create(context.Background(), agent)
        })

        // Scenario 1.1: Valid client lookup
        It("should look up the agent by client_id and validate it exists", func() {
            // Line 86
        })

        // Scenario 1.3: No grant exists
        Context("and no grant exists", func() {
            It("should redirect to consent UI with agent ID in path", func() {
                // Line 180
            })

            It("should preserve original request URL in redirect_uri parameter", func() {
                // Line 190
            })
        })

        // Scenario 1.4: Active grant exists
        Context("and active grant exists", func() {
            BeforeEach(func() {
                grant := fixtures.ActiveGrant(...)
                testStorage.UserGrants().Create(context.Background(), grant)
            })

            It("should proxy request to upstream OAuth2 server", func() {
                // Line 220
            })

            It("should preserve all OAuth2 parameters in proxy request", func() {
                // Line 230
            })
        })
    })

    // Scenario 1.2: Invalid client_id (different precondition - no agent)
    Describe("when authorization request has invalid client_id", func() {
        It("should return OAuth2 error response indicating invalid_client", func() {
            // Line 150
        })
    })
})
```

### Mapping Table

| Spec Reference | Test File | Describe/Context | It Block | Line |
|----------------|-----------|------------------|----------|------|
| User Story 1, Scenario 1.1 | oauth2_authorize_test.go | "when a valid authorization request arrives" | "should look up the agent by client_id" | 86 |
| User Story 1, Scenario 1.2 | oauth2_authorize_test.go | "when authorization request has invalid client_id" | "should return OAuth2 error invalid_client" | 150 |
| User Story 1, Scenario 1.3 | oauth2_authorize_test.go | "when a valid request arrives" → "and no grant exists" | "should redirect to consent UI" | 180 |
| User Story 1, Scenario 1.4 | oauth2_authorize_test.go | "when a valid request arrives" → "and active grant exists" | "should proxy to upstream" | 220 |
| User Story 2, Scenario 2.1 | oauth2_token_test.go | "when client exchanges authorization code" | "should proxy to upstream token endpoint" | 45 |
| User Story 2, Scenario 2.2 | oauth2_token_test.go | "when token request contains all parameters" | "should forward all parameters unchanged" | 65 |
| User Story 3, Scenario 3.1 | oauth2_metadata_test.go | "when client requests server metadata" | "should return RFC 8414 compliant document" | 30 |

**Maintaining the Mapping:**

1. When adding a new acceptance scenario to spec.md, immediately add a corresponding `It()` block
2. When writing a new test, reference the spec scenario in a comment
3. During code review, verify that each `It()` block has a clear spec mapping
4. Keep this README's mapping table updated as tests evolve

## Adding New E2E Tests

### Overview

When adding new E2E tests for a feature:

1. **Start with the spec**: Identify acceptance scenarios from spec.md
2. **Choose organization**: Decide if it's a new endpoint/feature or variant of existing
3. **Use hierarchical structure**: Leverage Describe/Context/BeforeEach for shared setup
4. **Follow naming conventions**: Use "when", "and", "should" pattern
5. **Map clearly**: Each It() = one acceptance scenario

### Step 1: Identify Acceptance Scenarios

Start with acceptance scenarios from your spec:

```markdown
Feature: Parameter preservation in OAuth2 proxy

Scenario 1: PKCE parameters preserved
Given: User authenticated, valid client_id, active grant
When: Authorization request with PKCE parameters
Then: System proxies to upstream with PKCE parameters intact

Scenario 2: Custom parameters preserved
Given: User authenticated, valid client_id, active grant
When: Authorization request with custom parameters
Then: System proxies to upstream with custom parameters intact
```

### Step 2: Determine Test Organization

**Question 1: Is this a new feature/endpoint or variant of existing?**

- **New feature/endpoint** → Create new test file with top-level Describe
- **Variant of existing** → Add Context to existing Describe

**Question 2: Do multiple tests share preconditions?**

- **Yes** → Use nested Describe with shared BeforeEach
- **No** → Flat structure with individual It blocks

**Question 3: Are you testing multiple behaviors under same preconditions?**

- **Yes** → Single Context with multiple It blocks
- **No** → Separate Context blocks for each variant

### Step 3: Write Hierarchical Tests

**Example: Adding new parameter preservation tests to existing endpoint**

```go
// In oauth2_authorize_test.go - add to existing Describe

Describe("when a valid authorization request arrives", func() {
    var agent *storage.Agent

    BeforeEach(func() {
        // Shared setup: All tests need a valid agent
        agent = fixtures.ValidAgent()
        err := testStorage.Agents().Create(context.Background(), agent)
        Expect(err).ToNot(HaveOccurred())
    })

    Context("and active grant exists", func() {
        BeforeEach(func() {
            // Shared setup: All tests in this Context need active grant
            grant := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID)
            err := testStorage.UserGrants().Create(context.Background(), grant)
            Expect(err).ToNot(HaveOccurred())
        })

        // Scenario 1: PKCE parameters preserved
        It("should preserve PKCE parameters in proxy to upstream", func() {
            // Given: (setup above) valid agent + active grant

            // When: Authorization request with PKCE parameters
            resp, err := server.AuthenticatedGET(
                fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&code_challenge=abc123&code_challenge_method=S256",
                    agent.ClientID),
                fixtures.DefaultPrincipal().String(),
            )
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()

            // Then: PKCE parameters preserved in upstream redirect
            redirectURL, err := helpers.ExtractRedirectURL(resp)
            Expect(err).ToNot(HaveOccurred())
            Expect(redirectURL.Query().Get("code_challenge")).To(Equal("abc123"))
            Expect(redirectURL.Query().Get("code_challenge_method")).To(Equal("S256"))
        })

        // Scenario 2: Custom parameters preserved
        It("should preserve custom parameters in proxy to upstream", func() {
            // Given: (setup above) valid agent + active grant

            // When: Authorization request with custom parameter
            resp, err := server.AuthenticatedGET(
                fmt.Sprintf("/oauth2/authorize?client_id=%s&redirect_uri=https://example.com/cb&response_type=code&custom_param=value123",
                    agent.ClientID),
                fixtures.DefaultPrincipal().String(),
            )
            Expect(err).ToNot(HaveOccurred())
            defer resp.Body.Close()

            // Then: Custom parameter preserved
            redirectURL, err := helpers.ExtractRedirectURL(resp)
            Expect(err).ToNot(HaveOccurred())
            Expect(redirectURL.Query().Get("custom_param")).To(Equal("value123"))
        })
    })
})
```

**Example: Adding a new feature with its own test file**

```go
// tests/e2e/oauth2_device_flow_test.go - new file for new feature

package e2e_test

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    // ... imports
)

var _ = Describe("OAuth2 Device Authorization Flow", func() {
    var (
        server         *bootstrap.TestServer
        testStorage    *storageadapter.Adapter
        mockUpstream   *helpers.MockUpstreamOAuth2Server
        storageFactory *bootstrap.StorageFactory
    )

    BeforeEach(func() {
        // Suite-level setup for all device flow tests
        mockUpstream = helpers.NewMockUpstreamOAuth2Server()
        config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)

        storageFactory = bootstrap.NewStorageFactory(logger)
        testStorage, _ = storageFactory.NewTestStorage()

        serverFactory := bootstrap.NewServerFactory(config, logger)
        appInstance, _ := serverFactory.BuildApp(testStorage)
        
        // Use appropriate server type based on routes needed
        // For end-user routes (OAuth2, consent):
        server, _ = bootstrap.NewEndUserTestServer(appInstance, logger)
        // For admin routes (agent/service management):
        // adminServer, _ = bootstrap.NewAdminTestServer(appInstance, logger)
    })

    AfterEach(func() {
        if server != nil {
            server.Close()
        }
        if mockUpstream != nil {
            mockUpstream.Close()
        }
        if testStorage != nil {
            storageFactory.CloseStorage(testStorage)
        }
    })

    Describe("when device initiates authorization", func() {
        It("should return device code and user code", func() {
            // Test implementation
        })

        It("should include verification URI in response", func() {
            // Test implementation
        })
    })

    Describe("when user enters user code", func() {
        Context("and code is valid", func() {
            It("should display consent screen", func() {
                // Test implementation
            })
        })

        Context("and code has expired", func() {
            It("should return expired_token error", func() {
                // Test implementation
            })
        })
    })
})
```

### Step 4: Verify Coverage and Mapping

1. **Run the test**:
   ```bash
   # Run specific test with focus
   ginkgo -v --focus="PKCE parameters" ./tests/e2e/

   # Run full test file
   ginkgo -v ./tests/e2e/oauth2_authorize_test.go

   # Run all E2E tests
   just test-e2e-backend
   ```

2. **Check coverage**:
   ```bash
   # Generate coverage report
   just test-e2e-backend-coverage

   # View in browser (coverage/e2e-backend.html)
   ```

3. **Verify spec mapping**:
   - Ensure each new `It()` block has a comment referencing the spec scenario
   - Add entry to "Mapping Tests to Acceptance Scenarios" section in this README
   - Update mapping table with new test locations

4. **Check test output readability**:
   ```bash
   ginkgo -v ./tests/e2e/
   ```

   Output should be readable and show clear test hierarchy:
   ```
   OAuth2 Authorization Endpoint
     when a valid authorization request arrives
       and active grant exists
         ✓ should preserve PKCE parameters in proxy to upstream
         ✓ should preserve custom parameters in proxy to upstream
   ```

### Step 5: Test Isolation Best Practices

**Always**:
- Create fresh fixtures in BeforeEach
- Use independent storage (fresh for each test)
- Close server in AfterEach
- Use descriptive test names matching spec scenarios

**Never**:
- Assume test execution order
- Share state between tests
- Hardcode timestamps
- Depend on external services

## Troubleshooting Common Issues

### Issue: Tests fail with "connection refused"

**Cause**: httptest.Server not starting correctly

**Solution**:
```go
// Verify server initialization
Expect(server).ToNot(BeNil())
Expect(server.BaseURL()).ToNot(BeEmpty())
```

### Issue: "invalid_client" errors in all tests

**Cause**: Agent not being created before authorization request

**Solution**:
```go
BeforeEach(func() {
    agent := fixtures.ValidAgent()
    err := testStorage.Agents().Create(context.Background(), agent)
    Expect(err).ToNot(HaveOccurred())  // Verify creation succeeded
})
```

### Issue: Mock upstream server not receiving requests

**Cause**: Config not pointing to mock server URL

**Solution**:
```go
mockUpstream := helpers.NewMockUpstreamOAuth2Server()
config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)
// Verify mock URL is in config
Expect(config.OAuth2AuthServer.UpstreamIssuer).To(Equal(mockUpstream.Server.URL))
```

### Issue: Tests hang or timeout

**Cause**: Goroutine leak or blocked channel

**Solution**:
```bash
# Run tests with timeout
ginkgo -v --timeout=30s ./tests/e2e/

# Check for goroutine leaks
go test -v -race ./tests/e2e/...
```

### Issue: "principal is required" errors

**Cause**: Using PublicGET for authenticated endpoints

**Solution**:
```go
// For authenticated endpoints, use:
resp, err := server.AuthenticatedGET(path, principal.String())

// For public endpoints (health, metadata), use:
resp, err := server.PublicGET(path)
```

### Issue: Coverage reports show 0% for internal code

**Cause**: Code in internal packages not being tested through E2E

**Solution**:
- These packages should have unit tests in `internal/**/*_test.go`
- E2E tests verify integration, not individual packages
- Run the full verification gate: `just verify` (static checks → fast/package tests → integration suites → E2E suites)

## Performance & CI/CD

### Test Execution Time

- **Full E2E Suite**: ~30-60 seconds (62 tests)
- **Single Test**: 100-500ms
- **With Coverage**: Add 10-20% overhead
- **Watch Mode**: Runs affected tests only

### CI/CD Integration

```bash
# In GitHub Actions or similar CI:
ginkgo -v --cover ./tests/e2e/
if [ $? -ne 0 ]; then
    echo "E2E tests failed"
    exit 1
fi
```

### Running in Parallel

For faster CI execution, run tests in parallel:

```bash
# Run with 4 workers
ginkgo -v --procs=4 ./tests/e2e/

# Specify number of workers based on CPU
ginkgo -v --procs=$(nproc) ./tests/e2e/
```

## Dependencies

### Test Framework
- `github.com/onsi/ginkgo/v2` - BDD test framework
- `github.com/onsi/gomega` - Assertion library

### Production Code (Tested)
- `github.com/agentic-identity-broker/agentic-identity-broker/internal/...` - Main app
- `github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/routing` - Route setup
- `github.com/go-chi/chi/v5` - HTTP router

### Test Utilities
- `net/http/httptest` - HTTP test server
- `context` - Context propagation

## Quick Reference for AI Agents

This section provides a condensed reference for AI agents writing new E2E tests. Follow these patterns to maintain consistency with the existing test suite.

### Test Structure Template

```go
var _ = Describe("Feature/Endpoint Name", func() {
    var (
        server         *bootstrap.TestServer
        testStorage    *storageadapter.Adapter
        storageFactory *bootstrap.StorageFactory
        // ... other suite-level variables
    )

    BeforeEach(func() {
        // Suite-level setup: runs for ALL tests
        storageFactory = bootstrap.NewStorageFactory(logger)
        testStorage, _ = storageFactory.NewTestStorage()
        // ... create server
    })

    AfterEach(func() {
        // Suite-level cleanup: runs after ALL tests
        server.Close()
        storageFactory.CloseStorage(testStorage)
    })

    Describe("when [condition]", func() {
        BeforeEach(func() {
            // Scenario-level setup: shared preconditions
        })

        Context("and [additional condition]", func() {
            BeforeEach(func() {
                // Context-level setup: variant-specific preconditions
            })

            It("should [expected behavior]", func() {
                // Given: (setup from BeforeEach blocks)

                // When: Make HTTP request
                resp, err := server.AuthenticatedGET(path, principal)
                Expect(err).ToNot(HaveOccurred())
                defer resp.Body.Close()

                // Then: Assert expected outcome
                Expect(resp.StatusCode).To(Equal(http.StatusOK))
            })
        })
    })
})
```

### Naming Cheat Sheet

| Element | Pattern | Example |
|---------|---------|---------|
| Top-level Describe | Feature/endpoint name | `Describe("OAuth2 Authorization Endpoint", ...)` |
| Nested Describe | "when [condition]" | `Describe("when a valid request arrives", ...)` |
| Context | "and [condition]" | `Context("and no grant exists", ...)` |
| It | "should [behavior]" | `It("should redirect to consent UI", ...)` |

### Container Node Decision Tree

```
Do tests share preconditions?
├─ YES → Use nested Describe with shared BeforeEach
│   └─ Do you have variants of the same condition?
│       ├─ YES → Use Context blocks for each variant
│       └─ NO → Use multiple It blocks in same Describe
└─ NO → Use flat Describe with individual It blocks
```

### Common Patterns Quick Reference

**Pattern: Testing variants under same precondition**
```go
Describe("when [condition]", func() {
    BeforeEach(func() { /* shared setup */ })

    Context("and [variant A]", func() {
        BeforeEach(func() { /* variant A setup */ })
        It("should [behavior A]", func() { /* test */ })
    })

    Context("and [variant B]", func() {
        BeforeEach(func() { /* variant B setup */ })
        It("should [behavior B]", func() { /* test */ })
    })
})
```

**Pattern: Testing multiple behaviors under same conditions**
```go
Context("and [condition]", func() {
    BeforeEach(func() { /* single setup for all */ })

    It("should [behavior 1]", func() { /* test aspect 1 */ })
    It("should [behavior 2]", func() { /* test aspect 2 */ })
    It("should [behavior 3]", func() { /* test aspect 3 */ })
})
```

**Pattern: Testing error conditions**
```go
Describe("when [error condition]", func() {
    // Often no BeforeEach needed for error scenarios

    It("should [error behavior 1]", func() { /* test */ })
    It("should [error behavior 2]", func() { /* test */ })
})
```

### Key Rules

1. **Each It() = One acceptance scenario** from spec.md
2. **Use Context for "and"** - additional conditions or variants
3. **BeforeEach cascades** - outer runs before inner
4. **Fresh storage per test** - created in suite-level BeforeEach
5. **Close resources in AfterEach** - prevent resource leaks
6. **Reference spec in comments** - add scenario number and description
7. **Use fixtures for test data** - from `tests/e2e/fixtures/`
8. **Use helpers for common operations** - from `tests/e2e/helpers/`
9. **Use matchers for assertions** - from `tests/e2e/matchers/` when available
10. **Test HTTP contract** - focus on requests/responses, not internals

### Fixtures Quick Reference

```go
// Principals (users)
fixtures.DefaultPrincipal()           // "user@example.com"
fixtures.AnotherPrincipal()           // "another-user@example.com"
fixtures.AdminPrincipal()             // "admin@example.com"

// Agents (OAuth2 clients)
fixtures.ValidAgent()                 // Basic agent with required fields
fixtures.AgentWithClientID("id")      // Custom client ID
fixtures.AgentWithURLs()              // With governance/docs URLs

// Grants (user permissions)
fixtures.ActiveGrant(principal, agentID)      // Valid for 1 hour
fixtures.ExpiredGrant(principal, agentID)     // Expired 1 hour ago
fixtures.GrantExpiringIn(principal, agentID, duration)
fixtures.IndefiniteGrant(principal, agentID)  // Never expires

// Config
fixtures.DefaultOAuth2Config()                   // Safe defaults
fixtures.OAuth2ConfigWithUpstream(url)           // Custom upstream
fixtures.OAuth2ConfigWithTimeout(seconds)        // Custom timeout
fixtures.OAuth2ConfigWithPublicURL(url)          // Custom public URL
```

### Server Request Methods

```go
// Authenticated requests (includes X-Remote-User header)
server.AuthenticatedGET(path, principal)
server.AuthenticatedPOST(path, body, contentType, principal)

// Public requests (no authentication header)
server.PublicGET(path)
server.PublicPOST(path, body, contentType)

// Advanced control
server.DirectRequest(req)
```

### Custom Matchers

```go
// OAuth2 error responses
Expect(resp).To(matchers.HaveOAuth2Error("invalid_client"))
Expect(body).To(matchers.HaveOAuth2Error("invalid_request", "description"))

// Redirects
Expect(resp).To(matchers.BeRedirectTo("/agents/123"))

// Metadata
Expect(body).To(matchers.ContainOAuth2Metadata("issuer", "https://..."))
```

### Running Tests

```bash
# Run the backend E2E suite
just test-e2e-backend

# Run the backend suite with coverage
just test-e2e-backend-coverage

# Watch the backend suite (auto-rerun on changes)
just test-e2e-backend-watch

# Run all backend, ExtProc, and frontend E2E suites
just test-e2e

# Run specific test
ginkgo -v --focus="should redirect to consent" ./tests/e2e/

# Run specific file
ginkgo -v ./tests/e2e/oauth2_authorize_test.go
```

### Anti-Patterns to Avoid

This section shows common mistakes found in real test code (e.g., `oauth2_edge_cases_test.go`) and how to refactor them correctly.

---

#### Anti-Pattern 1: Heavy Setup in It() Blocks

❌ **Don't**: Create server, storage, and fixtures inside every It block

```go
// From oauth2_edge_cases_test.go - ANTI-PATTERN
Describe("OAuth2 Edge Cases", func() {
    It("should handle upstream server unavailability gracefully", func() {
        // Given: Upstream URL points to non-existent server
        config := fixtures.DefaultOAuth2Config()
        config.OAuth2AuthServer.UpstreamAuthorizeEndpoint = "http://localhost:19999/authorize"

        // Create fresh storage
        storage, err := storageFactory.NewTestStorage()
        Expect(err).ToNot(HaveOccurred())

        // Create server
        factory := bootstrap.NewServerFactory(config, logger)
        built, err := factory.BuildApp(storage)
        Expect(err).ToNot(HaveOccurred())

        // Use appropriate server type for your test
        server, err = bootstrap.NewEndUserTestServer(built, logger)
        Expect(err).ToNot(HaveOccurred())

        // Create and store test agent
        agent := fixtures.ValidAgent()
        err = storage.Agents().Create(ctx, agent)
        Expect(err).ToNot(HaveOccurred())

        // Finally... the actual test
        resp, err := server.AuthenticatedGET(...)
        // assertions
    })

    It("should return 401 when principal header is missing", func() {
        // All the same setup repeated again! 15+ lines of duplicate code
        config := fixtures.DefaultOAuth2Config()
        storage, err := storageFactory.NewTestStorage()
        // ... etc
    })
})
```

**Problems:**
- 15-20 lines of setup in every test
- Duplicate code across all tests
- Hard to read - the actual test is buried
- Hard to maintain - changes require updating every test

✅ **Do**: Use hierarchical BeforeEach blocks for shared setup

```go
// CORRECT PATTERN
Describe("OAuth2 Edge Cases", func() {
    var (
        server      *bootstrap.TestServer
        testStorage *storageadapter.Adapter
        config      *ports.Config
        agent       *storage.Agent
    )

    BeforeEach(func() {
        // Shared setup - runs for ALL tests
        config = fixtures.DefaultOAuth2Config()
        testStorage, _ = storageFactory.NewTestStorage()

        factory := bootstrap.NewServerFactory(config, logger)
        appInstance, _ := factory.BuildApp(testStorage)
        // Use appropriate server type for your test
        server, _ = bootstrap.NewEndUserTestServer(appInstance, logger)
    })

    AfterEach(func() {
        if server != nil {
            server.Close()
        }
    })

    Describe("when agent exists", func() {
        BeforeEach(func() {
            // Only tests needing an agent get this setup
            agent = fixtures.ValidAgent()
            testStorage.Agents().Create(ctx, agent)
        })

        It("should handle upstream server unavailability gracefully", func() {
            // Clean, focused test - just the unique parts
            config.OAuth2AuthServer.UpstreamAuthorizeEndpoint = "http://localhost:19999/authorize"

            resp, err := server.AuthenticatedGET(
                fmt.Sprintf("/oauth2/authorize?client_id=%s&...", agent.ClientID),
                fixtures.DefaultPrincipal().String(),
            )

            Expect(resp.StatusCode).To(Or(
                Equal(http.StatusBadGateway),
                Equal(http.StatusServiceUnavailable),
            ))
        })
    })

    Describe("when authentication is missing", func() {
        // No agent created - testing unauthenticated access

        It("should return 401 Unauthorized", func() {
            // Simple, clear test
            resp, err := server.PublicGET("/oauth2/authorize?client_id=test")
            Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
        })
    })
})
```

**Benefits:**
- Setup code written once, runs for all tests
- Tests are 3-5 lines instead of 30+
- Easy to see what's being tested
- Adding new tests is trivial

---

#### Anti-Pattern 2: Flat Test Structure

❌ **Don't**: Put all tests at the same level without grouping

```go
// From oauth2_edge_cases_test.go - ANTI-PATTERN
Describe("OAuth2 Edge Cases and Error Scenarios", func() {
    // Suite-level BeforeEach only

    It("should handle upstream server unavailability gracefully", func() {
        // Full setup: config, storage, server, agent
        // Test with agent and upstream failure
    })

    It("should return 401 Unauthorized when principal header is missing", func() {
        // Full setup: config, storage, server (no agent needed)
        // Test without authentication
    })

    It("should return invalid_request error when client_id is missing", func() {
        // Full setup: config, storage, server (no agent needed)
        // Test missing parameter
    })

    It("should return 400 Bad Request when Content-Type is application/json", func() {
        // Full setup: config, storage, server (no agent needed)
        // Test wrong content type
    })

    It("should forward code_challenge and code_challenge_method parameters", func() {
        // Full setup: config, storage, server, agent, grant
        // Test PKCE forwarding
    })

    // 20 more tests, all at same level, all with full setup...
})
```

**Problems:**
- No organization by preconditions
- Can't tell which tests need what setup
- Duplicate setup even when tests have similar needs
- Hard to navigate - all tests look the same

✅ **Do**: Group tests by shared preconditions using nested Describe/Context

```go
// CORRECT PATTERN
Describe("OAuth2 Edge Cases and Error Scenarios", func() {
    var (
        server      *bootstrap.TestServer
        testStorage *storageadapter.Adapter
    )

    BeforeEach(func() {
        // Minimal suite-level setup
        testStorage, _ = storageFactory.NewTestStorage()
    })

    Describe("upstream server failures", func() {
        var agent *storage.Agent

        BeforeEach(func() {
            // Tests in this group need an agent
            agent = fixtures.ValidAgent()
            testStorage.Agents().Create(ctx, agent)

            // All need server with unreachable upstream
            config := fixtures.DefaultOAuth2Config()
            config.OAuth2AuthServer.UpstreamAuthorizeEndpoint = "http://localhost:19999/authorize"
            server = createServerWithConfig(config, testStorage)
        })

        It("should handle connection refused gracefully", func() { /* test */ })
        It("should handle timeout gracefully", func() { /* test */ })
        It("should not expose internal error details", func() { /* test */ })
    })

    Describe("authentication failures", func() {
        BeforeEach(func() {
            // All tests in this group need a running server
            config := fixtures.DefaultOAuth2Config()
            server = createServerWithConfig(config, testStorage)
        })

        It("should return 401 when X-Remote-User header is missing", func() { /* test */ })
        It("should return 400 when principal exceeds max length", func() { /* test */ })
    })

    Describe("malformed OAuth2 requests", func() {
        BeforeEach(func() {
            config := fixtures.DefaultOAuth2Config()
            server = createServerWithConfig(config, testStorage)
        })

        It("should return invalid_request when client_id is missing", func() { /* test */ })
        It("should return invalid_request when response_type is missing", func() { /* test */ })
        It("should return unsupported_response_type for invalid response_type", func() { /* test */ })
    })

    Describe("PKCE parameter handling", func() {
        var agent *storage.Agent
        var mockUpstream *helpers.MockUpstreamOAuth2Server

        BeforeEach(func() {
            // PKCE tests need agent + grant + mock upstream
            mockUpstream = helpers.NewMockUpstreamOAuth2Server()
            config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.URL())
            server = createServerWithConfig(config, testStorage)

            agent = fixtures.ValidAgent()
            testStorage.Agents().Create(ctx, agent)

            grant := fixtures.ActiveGrant(fixtures.DefaultPrincipal().String(), agent.ID)
            testStorage.UserGrants().Create(ctx, grant)
        })

        AfterEach(func() {
            mockUpstream.Close()
        })

        It("should forward code_challenge parameter to upstream", func() { /* test */ })
        It("should forward code_challenge_method parameter to upstream", func() { /* test */ })
    })

    Describe("security validations", func() {
        var agent *storage.Agent

        BeforeEach(func() {
            config := fixtures.DefaultOAuth2Config()
            server = createServerWithConfig(config, testStorage)
            agent = fixtures.ValidAgent()
            testStorage.Agents().Create(ctx, agent)
        })

        It("should safely handle XSS attempts in state parameter", func() { /* test */ })
        It("should safely handle SQL injection attempts in client_id", func() { /* test */ })
    })
})
```

**Benefits:**
- Clear organization by feature area
- Shared setup only where needed
- Easy to find related tests
- Test output is hierarchical and readable

---

#### Anti-Pattern 3: Not Declaring Shared Variables

❌ **Don't**: Recreate variables in every test

```go
Describe("Feature", func() {
    BeforeEach(func() {
        // Nothing declared at suite level
    })

    It("test 1", func() {
        agent := fixtures.ValidAgent()  // Local variable
        testStorage.Agents().Create(ctx, agent)

        resp, _ := server.AuthenticatedGET(
            fmt.Sprintf("/path?client_id=%s", agent.ClientID),  // Have to use agent
            fixtures.DefaultPrincipal().String(),
        )
    })

    It("test 2", func() {
        agent := fixtures.ValidAgent()  // Recreate in every test
        testStorage.Agents().Create(ctx, agent)
        // ...
    })
})
```

✅ **Do**: Declare shared variables at Describe level

```go
Describe("OAuth2 Authorization", func() {
    var (
        server      *bootstrap.TestServer
        testStorage *storageadapter.Adapter
        agent       *storage.Agent  // Declared at suite level
    )

    BeforeEach(func() {
        // Initialize shared variables
        testStorage, _ = storageFactory.NewTestStorage()
        agent = fixtures.ValidAgent()
        testStorage.Agents().Create(ctx, agent)
    })

    It("should validate agent exists by client_id", func() {
        // Just use the agent variable - no redeclaration needed
        resp, _ := server.AuthenticatedGET(
            fmt.Sprintf("/oauth2/authorize?client_id=%s&...", agent.ClientID),
            fixtures.DefaultPrincipal().String(),
        )
        Expect(resp.StatusCode).To(Equal(http.StatusFound))
    })

    It("should include agent ID in consent redirect", func() {
        // Same agent variable available here - shared across tests
        resp, _ := server.AuthenticatedGET(
            fmt.Sprintf("/oauth2/authorize?client_id=%s&...", agent.ClientID),
            fixtures.DefaultPrincipal().String(),
        )
        location := resp.Header.Get("Location")
        Expect(location).To(ContainSubstring(agent.ID))
    })
})
```

---

#### Anti-Pattern 4: Testing Multiple Things in One It()

❌ **Don't**: Test multiple behaviors in a single It block

```go
It("should handle edge cases", func() {
    // Test 1: Missing client_id
    resp1, _ := server.AuthenticatedGET("/oauth2/authorize?redirect_uri=...", principal)
    Expect(resp1.StatusCode).To(Equal(http.StatusBadRequest))

    // Test 2: Missing redirect_uri
    resp2, _ := server.AuthenticatedGET("/oauth2/authorize?client_id=test", principal)
    Expect(resp2.StatusCode).To(Equal(http.StatusBadRequest))

    // Test 3: Missing response_type
    resp3, _ := server.AuthenticatedGET("/oauth2/authorize?client_id=test&redirect_uri=...", principal)
    Expect(resp3.StatusCode).To(Equal(http.StatusBadRequest))
})
```

✅ **Do**: One behavior per It block

```go
It("should return invalid_request when client_id is missing", func() {
    resp, _ := server.AuthenticatedGET("/oauth2/authorize?redirect_uri=...", principal)
    Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
})

It("should return invalid_request when redirect_uri is missing", func() {
    resp, _ := server.AuthenticatedGET("/oauth2/authorize?client_id=test", principal)
    Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
})

It("should return invalid_request when response_type is missing", func() {
    resp, _ := server.AuthenticatedGET("/oauth2/authorize?client_id=test&redirect_uri=...", principal)
    Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
})
```

---

#### Anti-Pattern 5: Vague or Overly Long Test Names

❌ **Don't**: Use vague or excessively verbose names

```go
It("works", func() {  // Too vague

It("test", func() {  // Too vague

It("should handle the case when the upstream OAuth2 authorization server is completely unreachable and unavailable due to network issues or server being down and return an appropriate error response to the client without exposing internal implementation details", func() {
    // Way too long - 30+ words
})
```

✅ **Do**: Be specific but concise (5-12 words typical)

```go
It("should return 502 Bad Gateway when upstream is unreachable", func() {

It("should handle upstream timeout gracefully", func() {

It("should not expose internal errors to client", func() {
```

---

#### Anti-Pattern 6: Hardcoding Test Data

❌ **Don't**: Hardcode values that could use fixtures

```go
It("test", func() {
    // Hardcoded agent data
    agent := &storage.Agent{
        ID:       "test-agent-id",
        ClientID: "test-client-123",
        Name:     "Test Agent",
        // ... 10 more fields
    }
    testStorage.Agents().Create(ctx, agent)

    // Hardcoded principal
    resp, _ := server.AuthenticatedGET(path, "user@example.com")

    // Hardcoded grant
    grant := &storage.UserGrant{
        ID:        "grant-123",
        Principal: "user@example.com",
        AgentID:   "test-agent-id",
        ExpiresAt: time.Now().Add(1 * time.Hour),
        // ... more fields
    }
})
```

✅ **Do**: Use fixtures for consistent test data

```go
It("should proxy request to upstream when grant exists", func() {
    // Given: Use fixtures - all fields correctly initialized
    agent := fixtures.ValidAgent()
    testStorage.Agents().Create(ctx, agent)

    principal := fixtures.DefaultPrincipal().String()
    grant := fixtures.ActiveGrant(principal, agent.ID)
    testStorage.UserGrants().Create(ctx, grant)

    // When: Make request
    path := fmt.Sprintf("/oauth2/authorize?client_id=%s&...", agent.ClientID)
    resp, _ := server.AuthenticatedGET(path, principal)

    // Then: Verify redirect to upstream
    Expect(resp.StatusCode).To(Equal(http.StatusFound))
})
```

---

#### Anti-Pattern 7: Not Using Context for Variants

❌ **Don't**: Repeat setup for similar test variants

```go
Describe("when processing requests", func() {
    It("should handle request with no grant", func() {
        agent := fixtures.ValidAgent()
        testStorage.Agents().Create(ctx, agent)
        // No grant created

        resp, _ := server.AuthenticatedGET(path, principal)
        Expect(resp).To(matchers.BeRedirectTo("/"))
    })

    It("should handle request with active grant", func() {
        agent := fixtures.ValidAgent()  // Duplicate agent setup
        testStorage.Agents().Create(ctx, agent)
        grant := fixtures.ActiveGrant(principal, agent.ID)
        testStorage.UserGrants().Create(ctx, grant)

        resp, _ := server.AuthenticatedGET(path, principal)
        Expect(resp).To(matchers.BeRedirectTo(mockUpstream.URL))
    })

    It("should handle request with expired grant", func() {
        agent := fixtures.ValidAgent()  // Duplicate agent setup again
        testStorage.Agents().Create(ctx, agent)
        grant := fixtures.ExpiredGrant(principal, agent.ID)
        testStorage.UserGrants().Create(ctx, grant)

        resp, _ := server.AuthenticatedGET(path, principal)
        Expect(resp).To(matchers.BeRedirectTo("/"))
    })
})
```

✅ **Do**: Use Context blocks for variants with shared base setup

```go
Describe("when processing authorization requests", func() {
    var (
        agent          *storage.Agent
        principal      string
        mockUpstream   *helpers.MockUpstreamOAuth2Server
    )

    BeforeEach(func() {
        // Shared: all variants need an agent and principal
        agent = fixtures.ValidAgent()
        testStorage.Agents().Create(ctx, agent)
        principal = fixtures.DefaultPrincipal().String()

        // Setup mock upstream for proxy tests
        mockUpstream = helpers.NewMockUpstreamOAuth2Server()
        config.OAuth2AuthServer.UpstreamAuthorizeEndpoint = mockUpstream.URL() + "/authorize"
    })

    AfterEach(func() {
        mockUpstream.Close()
    })

    Context("and no grant exists", func() {
        // Deliberately don't create grant

        It("should redirect to consent UI", func() {
            path := fmt.Sprintf("/oauth2/authorize?client_id=%s&...", agent.ClientID)
            resp, _ := server.AuthenticatedGET(path, principal)
            Expect(resp).To(matchers.BeRedirectTo("/agents/" + agent.ID))
        })
    })

    Context("and active grant exists", func() {
        BeforeEach(func() {
            // Only this variant needs active grant
            grant := fixtures.ActiveGrant(principal, agent.ID)
            testStorage.UserGrants().Create(ctx, grant)
        })

        It("should proxy to upstream", func() {
            path := fmt.Sprintf("/oauth2/authorize?client_id=%s&...", agent.ClientID)
            resp, _ := server.AuthenticatedGET(path, principal)
            Expect(resp).To(matchers.BeRedirectTo(mockUpstream.URL()))
        })
    })

    Context("and expired grant exists", func() {
        BeforeEach(func() {
            // Only this variant needs expired grant
            grant := fixtures.ExpiredGrant(principal, agent.ID)
            testStorage.UserGrants().Create(ctx, grant)
        })

        It("should redirect to consent UI", func() {
            path := fmt.Sprintf("/oauth2/authorize?client_id=%s&...", agent.ClientID)
            resp, _ := server.AuthenticatedGET(path, principal)
            Expect(resp).To(matchers.BeRedirectTo("/agents/" + agent.ID))
        })
    })
})
```

---

#### Anti-Pattern 8: Excessive Nesting

❌ **Don't**: Nest Context inside Context excessively

```go
Describe("Feature", func() {
    Context("when A", func() {
        Context("and B", func() {
            Context("and C", func() {
                Context("and D", func() {  // Too deep! (4 levels)
                    It("should do something", func() {
                        // Test buried 5 levels deep
                    })
                })
            })
        })
    })
})
```

✅ **Do**: Keep nesting to 2-3 levels maximum

```go
Describe("Feature", func() {  // Level 1: Feature
    Describe("when A", func() {  // Level 2: Scenario
        Context("and B", func() {  // Level 3: Variant
            It("should do something", func() {  // Level 4: Test
                // Clear, readable depth
            })
        })
    })
})

// Or combine conditions if too deep
Describe("Feature", func() {
    Describe("when A and B", func() {  // Combine related conditions
        It("should do something", func() {
            // Simpler structure
        })
    })
})
```

---

#### Anti-Pattern 9: Not Following Given/When/Then

❌ **Don't**: Mix setup, action, and assertions without structure

```go
It("should do something", func() {
    resp, _ := server.AuthenticatedGET(path, principal)
    agent := fixtures.ValidAgent()
    Expect(resp.StatusCode).To(Equal(200))
    testStorage.Agents().Create(ctx, agent)
    body, _ := io.ReadAll(resp.Body)
    grant := fixtures.ActiveGrant(principal, agent.ID)
    Expect(body).To(ContainSubstring("success"))
})
```

✅ **Do**: Structure tests with clear Given/When/Then sections

```go
Describe("OAuth2 token exchange", func() {
    var (
        server      *bootstrap.TestServer
        testStorage *storageadapter.Adapter
        agent       *storage.Agent
        principal   string
        grant       *storage.UserGrant
    )

    BeforeEach(func() {
        // Given: Setup preconditions (in BeforeEach, not It block!)
        testStorage, _ = storageFactory.NewTestStorage()
        // Use appropriate server type for your test
        server, _ = bootstrap.NewEndUserTestServer(appInstance, logger)

        agent = fixtures.ValidAgent()
        testStorage.Agents().Create(ctx, agent)

        principal = fixtures.DefaultPrincipal().String()
        grant = fixtures.ActiveGrant(principal, agent.ID)
        testStorage.UserGrants().Create(ctx, grant)
    })

    It("should return access token when code is valid", func() {
        // When: Exchange authorization code for token
        formData := url.Values{
            "grant_type":   []string{"authorization_code"},
            "code":         []string{"test_code_123"},
            "client_id":    []string{agent.ClientID},
            "redirect_uri": []string{"https://example.com/cb"},
        }
        resp, err := server.DirectRequest(
            "POST",
            "/oauth2/token",
            principal,
            map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
            strings.NewReader(formData.Encode()),
        )
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()

        // Then: Verify successful token response
        Expect(resp.StatusCode).To(Equal(http.StatusOK))
        body, _ := io.ReadAll(resp.Body)
        Expect(body).To(ContainSubstring("access_token"))
    })

    It("should return error when code is invalid", func() {
        // When: Exchange invalid authorization code
        formData := url.Values{
            "grant_type":   []string{"authorization_code"},
            "code":         []string{"invalid_code"},
            "client_id":    []string{agent.ClientID},
            "redirect_uri": []string{"https://example.com/cb"},
        }
        resp, err := server.DirectRequest(
            "POST",
            "/oauth2/token",
            principal,
            map[string]string{"Content-Type": "application/x-www-form-urlencoded"},
            strings.NewReader(formData.Encode()),
        )
        Expect(err).ToNot(HaveOccurred())
        defer resp.Body.Close()

        // Then: Verify error response
        Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
    })
})
```

---

### Summary: Key Rules to Follow

1. **Use BeforeEach** for setup shared across multiple tests
2. **Use Context** to group test variants with similar preconditions
3. **Declare variables** at Describe level, initialize in BeforeEach
4. **One behavior per It()** - don't test multiple things
5. **Use fixtures** for test data instead of hardcoding
6. **Keep nesting shallow** - 2-3 levels max
7. **Use descriptive names** - "should [behavior]" format, 5-12 words
8. **Follow Given/When/Then** - structure tests clearly
9. **Keep It() blocks concise** - 5-15 lines typical, not 30+
10. **Group related tests** - use nested Describe/Context for organization

## Frontend E2E Testing with Playwright

This section covers E2E testing for the frontend UI using Playwright and Ginkgo.

### Running Frontend E2E Tests

**Built Mode** (Production-like, self-contained, recommended for CI):

```bash
# Run all frontend E2E tests (built mode with headless browser)
just test-e2e-frontend

# Run with browser visible for debugging
HEADLESS=false just test-e2e-frontend

# Run with visible browser AND verbose output
HEADLESS=false ginkgo -v ./tests/e2e/frontend/

# Run specific test
ginkgo -v --focus="should display service scopes" ./tests/e2e/frontend/
```

**Dev Mode** (Interactive development with Vite HMR, requires `just web-dev`):

Dev mode connects Playwright to the Vite dev server (http://localhost:3000) for hot module reloading. The test backend automatically runs on fixed port 8000 to match Vite's proxy configuration.

```bash
# Terminal 1: Start Vite dev server with HMR
just web-dev

# Terminal 2: Run tests in dev mode
E2E_FRONTEND_MODE=dev ginkgo -v ./tests/e2e/frontend/

# Or with visible browser for debugging
HEADLESS=false E2E_FRONTEND_MODE=dev ginkgo -v ./tests/e2e/frontend/
```

**Key Differences:**
- **Built Mode**: Frontend assets bundled and served by Go backend on random port, no HMR, test-isolated
- **Dev Mode**: Frontend served by Vite (port 3000) with HMR, backend on fixed port 8000, proxied API requests via Vite

**Architecture Details (Dev Mode):**
- Frontend: http://localhost:3000 (Vite dev server with HMR)
- Backend: http://localhost:8000 (Test server on fixed port)
- Vite proxy: http://localhost:3000/api/* → http://localhost:8000/api/*
- Authentication: X-Remote-User header injected by Vite proxy (dev@example.com)

### Test Organization

Frontend E2E tests are located in `/tests/e2e/frontend/`:

```
tests/e2e/frontend/
├── frontend_suite_test.go        # Suite entry point (browser/Playwright setup)
├── consent_flow_test.go          # Tests for consent management flow
├── bootstrap/
│   └── playwright.go             # Playwright initialization
├── fixtures/
│   └── frontend.go               # Frontend-specific test data
├── pages/                        # Page Object Models
│   ├── consent_page.go           # Consent management page
│   ├── grant_management_page.go  # Grant management page
│   └── [page_name]_page.go       # Add new pages here
├── helpers/
│   └── ui_helpers.go             # UI-specific helper functions
└── README.md                      # This file
```

### Selector Best Practices

When writing frontend E2E tests, use this selector hierarchy (best to worst):

#### 1. **Semantic Selectors (BEST)** ✅

Use `GetByRole`, `GetByLabel`, `GetByText` - these test what users actually see:

```go
// Find button by its accessible role and visible text
button := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Approve & Delegate"})

// Find input by its associated label
input := cp.page().GetByLabel("Expiration")

// Find element by visible text
link := cp.page().GetByText("Learn More")

// Find checkbox/radio by label
checkbox := cp.page().GetByRole("checkbox")
```

**Why semantic selectors are best:**
- Test what users actually see (accessibility-first)
- Resistant to styling/CSS changes
- Self-documenting test intent
- Improve accessibility of your application

#### 2. **data-testid Attributes** - Use as fallback only

When semantic selectors aren't sufficient:

```go
// Use only for complex components without clear semantic role
badge := cp.page().Locator("[data-testid='status-badge']")
```

**When to use:**
- Complex components with no clear semantic role
- When multiple elements share the same role/label
- Last resort before brittle selectors

**Never use for:**
- Simple buttons, links, form inputs (use semantic selectors)
- Components with clear ARIA roles

#### 3. **IDs** - Acceptable but risky ⚠️

```go
// Use sparingly - can change for styling reasons
button := cp.page().Locator("#submit-button")
```

#### 4. **CSS Classes** - Brittle ❌

```go
// AVOID - breaks whenever CSS is refactored
// button := cp.page().Locator(".btn.primary.large")
```

#### 5. **XPath** - Last resort ❌

```go
// AVOID - extremely fragile and hard to maintain
// button := cp.page().Locator("xpath=//div[@class='card']//button[1]")
```

### Page Objects Pattern

Page Objects encapsulate UI selectors and interactions into high-level methods. This makes tests more readable and maintainable.

#### Structure

```go
package pages

// ConsentPage represents a specific page/feature
type ConsentPage struct {
    page    playwright.Page
    baseURL string
}

// NewConsentPage creates a page object
func NewConsentPage(page playwright.Page, baseURL string) *ConsentPage {
    return &ConsentPage{
        page:    page,
        baseURL: baseURL,
    }
}

// High-level methods describe user actions
func (cp *ConsentPage) NavigateToAgent(ctx context.Context, agentID string) error {
    // Implementation: find element, click, wait, etc.
}

func (cp *ConsentPage) GetAvailableScopes(ctx context.Context) ([]string, error) {
    // Implementation: extract data from page
}

func (cp *ConsentPage) DelegateService(ctx context.Context, serviceName string) error {
    // Implementation: perform action
}
```

#### Benefits

1. **Readability**: Tests read like specifications
   ```go
   // Clear intent - no need to understand selectors
   err := consentPage.DelegateService(ctx, "GitHub")
   ```

2. **Maintainability**: Change selectors in one place
   ```go
   // If GitHub service selector changes, update once in page object
   // All tests using DelegateService() automatically use new selector
   ```

3. **Reusability**: Share complex interactions across tests
   ```go
   // Multiple tests can use the same high-level method
   err := consentPage.SelectScope(ctx, "repo")
   err := consentPage.ClearScope(ctx, "repo")
   ```

#### Page Object Guidelines

1. **High-level methods** - Describe what the user does, not how
   ```go
   // ✅ Good: What user does
   func (cp *ConsentPage) DelegateService(ctx context.Context, name string) error

   // ❌ Bad: How it's implemented
   func (cp *ConsentPage) ClickServiceButton(ctx context.Context, selector string) error
   ```

2. **No selectors in tests** - Selectors only in page object
   ```go
   // ✅ Good: Test uses page object method
   err := consentPage.DelegateService(ctx, "GitHub")

   // ❌ Bad: Test contains selectors
   button := page.GetByRole("button", opts)
   err := button.Click()
   ```

3. **Handle waits in page object** - Tests shouldn't manage timing
   ```go
   // ✅ Good: Page object handles waiting
   func (cp *ConsentPage) GetAvailableScopes(ctx context.Context) ([]string, error) {
       deadline := time.Now().Add(10 * time.Second)
       for {
           indicators := cp.page.Locator("div.inline-flex.items-center")
           if count, _ := indicators.Count(); count > 0 {
               // Extract scopes
               return scopes, nil
           }
           if time.Now().After(deadline) {
               return nil, fmt.Errorf("scopes not found after timeout")
           }
           time.Sleep(100 * time.Millisecond)
       }
   }

   // ❌ Bad: Test manages timing
   scopes, _ := consentPage.GetAvailableScopes(ctx)
   time.Sleep(2 * time.Second)  // Hack to wait
   ```

4. **Return meaningful data** - Extract what tests need
   ```go
   // ✅ Good: Returns extracted data
   func (cp *ConsentPage) GetAgentName(ctx context.Context) (string, error)
   func (cp *ConsentPage) GetAvailableScopes(ctx context.Context) ([]string, error)

   // ❌ Bad: Returns raw elements
   func (cp *ConsentPage) GetAgentHeading(ctx context.Context) (playwright.Locator, error)
   ```

5. **Disambiguate repeated controls** - When cards share a button or radio label, scope the locator to a card or use `.First()` for an explicit first-card contract. Exercise the page object with multiple matching cards.

### Ginkgo By() for Longer Test Sequences

For complex tests with multiple steps, use Ginkgo's `By()` function to organize and report progress:

#### Structure

```go
It("should complete multi-step approval workflow", func() {
    // By() organizes longer tests into logical steps
    // Each By() prints as a progress line in test output

    By("navigating to agent page")
    err := consentPage.NavigateToAgent(ctx, testAgentID)
    Expect(err).NotTo(HaveOccurred())

    By("verifying agent details display")
    agentName, err := consentPage.GetAgentName(ctx)
    Expect(err).NotTo(HaveOccurred())
    Expect(agentName).To(Equal("Test Agent"))

    By("viewing available scopes")
    scopes, err := consentPage.GetAvailableScopes(ctx)
    Expect(err).NotTo(HaveOccurred())
    Expect(scopes).To(HaveLen(2))

    By("delegating GitHub service")
    err = consentPage.DelegateService(ctx, "GitHub")
    Expect(err).NotTo(HaveOccurred())

    By("taking screenshot for verification")
    err = consentPage.TakeScreenshot(ctx, "service_delegated")
    Expect(err).NotTo(HaveOccurred())

    GetLogger().Info("Workflow completed successfully")
})
```

#### Test Output with By()

```
Consent Flow
  should complete multi-step approval workflow
    [By] navigating to agent page
    [By] verifying agent details display
    [By] viewing available scopes
    [By] delegating GitHub service
    [By] taking screenshot for verification
    ✓ completed successfully (1.234s)
```

#### Guidelines for By()

1. **One action per By()** - Keep steps focused
   ```go
   // ✅ Good
   By("navigating to agent page")
   err := consentPage.NavigateToAgent(ctx, agentID)

   // ❌ Bad - multiple actions in one step
   By("navigating and verifying")
   consentPage.NavigateToAgent(ctx, agentID)
   consentPage.GetAgentName(ctx)
   ```

2. **Use imperative form** - "verb the noun"
   ```go
   // ✅ Good verbs for By()
   By("navigating to consent page")
   By("viewing available scopes")
   By("delegating GitHub service")
   By("verifying successful completion")

   // ❌ Bad
   By("consent page navigation")
   By("available scopes are visible")
   ```

3. **Include context in assertions** - Make failures clear
   ```go
   By("delegating GitHub service")
   err := consentPage.DelegateService(ctx, "GitHub")
   Expect(err).NotTo(HaveOccurred(), "Failed to delegate GitHub service")

   By("verifying service appears delegated")
   isDelegated, err := consentPage.IsServiceDelegated(ctx, "GitHub")
   Expect(err).NotTo(HaveOccurred(), "Failed to check service status")
   Expect(isDelegated).To(BeTrue(), "GitHub service should show as delegated")
   ```

4. **Use for complex workflows only** - Simple tests don't need By()
   ```go
   // ✅ Good: By() for multi-step workflow
   It("should complete full consent workflow", func() {
       By("navigating to page")
       // ...
       By("clicking button")
       // ...
       By("verifying result")
       // ...
   })

   // ✅ OK: Simple test without By()
   It("should display agent name", func() {
       err := consentPage.NavigateToAgent(ctx, agentID)
       Expect(err).NotTo(HaveOccurred())

       name, err := consentPage.GetAgentName(ctx)
       Expect(err).NotTo(HaveOccurred())
       Expect(name).NotTo(BeEmpty())
   })
   ```

#### Example: Multi-Step Workflow

```go
var _ = Describe("Consent Workflow", func() {
    var (
        consentPage *pages.ConsentPage
        testAgentID string
    )

    BeforeEach(func() {
        // Setup...
    })

    It("should complete grant approval workflow with service delegation", func() {
        By("navigating to agent consent page")
        err := consentPage.NavigateToAgent(ctx, testAgentID)
        Expect(err).NotTo(HaveOccurred(), "Navigation failed")

        By("viewing agent details and required services")
        agentName, err := consentPage.GetAgentName(ctx)
        Expect(err).NotTo(HaveOccurred())
        Expect(agentName).NotTo(BeEmpty())

        By("retrieving list of available scopes")
        scopes, err := consentPage.GetAvailableScopes(ctx)
        Expect(err).NotTo(HaveOccurred())
        Expect(scopes).NotTo(BeEmpty(), "Should have at least one scope")

        By("delegating GitHub service access")
        err = consentPage.DelegateService(ctx, "GitHub")
        Expect(err).NotTo(HaveOccurred())

        By("verifying service shows as delegated")
        isDelegated, err := consentPage.IsMandatoryServiceConnected(ctx, "GitHub")
        Expect(err).NotTo(HaveOccurred())
        Expect(isDelegated).To(BeTrue(), "GitHub should show as connected")

        By("taking final verification screenshot")
        err = consentPage.TakeScreenshot(ctx, "workflow_completed")
        Expect(err).NotTo(HaveOccurred())

        GetLogger().Info("Consent workflow completed successfully",
            "agent_name", agentName,
            "scopes_found", len(scopes),
        )
    })
})
```

### Frontend E2E Testing Best Practices

#### 1. **Always use page objects** - Never put selectors in tests

```go
// ✅ Good
err := consentPage.DelegateService(ctx, "GitHub")

// ❌ Bad - selector in test
button := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Delegate"})
err := button.Click()
```

#### 2. **Test user workflows** - Not implementation details

```go
// ✅ Good - tests what user does
func (cp *ConsentPage) DelegateService(ctx context.Context, name string) error {
    serviceHeading := cp.page().GetByRole("heading", opts)
    button := serviceHeading.GetByRole("button", opts)
    return button.Click()
}

// ❌ Bad - tests React state directly
Expect(consentPage.GetServiceState(ctx, "GitHub")).To(Equal("delegated"))
```

#### 3. **Wait for elements implicitly** - Not with sleep()

```go
// ✅ Good - Playwright waits automatically
button := cp.page().GetByRole("button", opts)
err := button.Click()  // Waits for clickable

// ❌ Bad - explicit waits
time.Sleep(2 * time.Second)  // Race condition!
button := cp.page().GetByRole("button", opts)
err := button.Click()
```

#### 4. **Use descriptive assertion messages**

```go
// ✅ Good
Expect(scopes).NotTo(BeEmpty(), "Should have at least one scope available from service")
Expect(isDelegated).To(BeTrue(), "GitHub service should show as delegated after clicking")

// ❌ Bad
Expect(scopes).NotTo(BeEmpty())
Expect(isDelegated).To(BeTrue())
```

#### 5. **Organize related tests with Context**

```go
// ✅ Good - organized by user path
Describe("OAuth2 Consent Flow", func() {
    Describe("when user views service details", func() {
        It("should display service name", func() { /* */ })
        It("should display required scopes", func() { /* */ })
    })

    Describe("when user delegates service", func() {
        It("should mark service as connected", func() { /* */ })
        It("should show success message", func() { /* */ })
    })
})

// ❌ Bad - random organization
Describe("Consent Tests", func() {
    It("displays name", func() { /* */ })
    It("confirms delegation", func() { /* */ })
    It("shows scopes", func() { /* */ })
    It("delegation success", func() { /* */ })
})
```

### Common Frontend Testing Patterns

#### Pattern: Testing Visibility

```go
It("should display service scopes", func() {
    By("navigating to service page")
    err := consentPage.NavigateToAgent(ctx, agentID)
    Expect(err).NotTo(HaveOccurred())

    By("retrieving visible scopes")
    scopes, err := consentPage.GetAvailableScopes(ctx)
    Expect(err).NotTo(HaveOccurred())

    By("verifying expected scopes are displayed")
    Expect(scopes).To(ContainElement("repo"))
    Expect(scopes).To(ContainElement("user"))
})
```

#### Pattern: Testing User Interaction

```go
It("should delegate service on button click", func() {
    By("navigating to service page")
    err := consentPage.NavigateToAgent(ctx, agentID)
    Expect(err).NotTo(HaveOccurred())

    By("clicking delegate button")
    err = consentPage.DelegateService(ctx, "GitHub")
    Expect(err).NotTo(HaveOccurred())

    By("verifying service shows as delegated")
    isDelegated, err := consentPage.IsMandatoryServiceConnected(ctx, "GitHub")
    Expect(err).NotTo(HaveOccurred())
    Expect(isDelegated).To(BeTrue())
})
```

#### Pattern: Testing Error Handling

```go
It("should show error message on failure", func() {
    By("navigating to page")
    err := consentPage.NavigateToAgent(ctx, agentID)
    Expect(err).NotTo(HaveOccurred())

    By("attempting invalid action")
    // Simulate error condition

    By("verifying error message appears")
    errorMsg, err := consentPage.GetErrorMessage(ctx)
    Expect(err).NotTo(HaveOccurred())
    Expect(errorMsg).To(ContainSubstring("required"))
})
```

### Debugging Frontend Tests

#### Enable Visible Browser

```bash
# Run with browser visible for debugging
HEADLESS=false ginkgo -v --focus="test-name" ./tests/e2e/frontend/
```

#### Add Screenshots in Tests

```go
By("taking screenshot at this point")
err := consentPage.TakeScreenshot(ctx, "debug-point-name")
Expect(err).NotTo(HaveOccurred())
// Screenshot saved to tests/e2e/frontend/coverage/screenshots/
```

Run `E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend` for maintained screenshots.
Capture fails if one of the three bundled fonts does not load.
The capture hides elements marked `data-screenshot-dynamic`. The normal page still shows these elements.

#### Add Detailed Logging

```go
By("checking service status")
isDelegated, err := consentPage.IsMandatoryServiceConnected(ctx, "GitHub")
GetLogger().Info("Service check complete",
    "service", "GitHub",
    "is_delegated", isDelegated,
    "error", err,
)
```

## See Also

- [/tests/e2e/fixtures/README.md](fixtures/README.md) - Detailed fixture documentation
- [/tests/e2e/fixtures/FIXTURE_EXAMPLES.md](fixtures/FIXTURE_EXAMPLES.md) - Comprehensive fixture examples
- [/tests/e2e/bootstrap/test_server.go](bootstrap/test_server.go) - Server implementation details
- [/tests/e2e/pages/](pages/) - Page Object implementations
- [/specs/009-oauth2-auth-server/spec.md](../../specs/009-oauth2-auth-server/spec.md) - Feature specification
- `/ARCHITECTURE.md` - System architecture overview
- `/justfile` - Available test commands
