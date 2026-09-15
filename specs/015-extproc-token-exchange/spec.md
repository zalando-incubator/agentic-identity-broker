# Feature Specification: Envoy ExtProc Token Exchange Service

**Feature Branch**: `015-extproc-token-exchange`  
**Created**: 2026-02-23  
**Status**: Draft  
**Input**: User description: "support for token exchange via Envoy's ExtProc interface. I want to implement a new application in this codebase (resulting in a new process defined in cmd/) in this project that implements the ExtProc interface that Envoy exposes. The new cmd should use the same configuration _structure_ but with completely different content. It is designed to be usable with agentgateway so that an agent can call an MCP tool or another agent with transparent token exchange. The new application should only share the infrastructure of the configuration but should not share any structures, i.e. it has its own schema. The ExtProc interface should be implemented to transparently exchange a Bearer token of the request using the identity broker's token exchange capability as specified in specs/013-token-exchange/spec.md and specs/013-token-exchange/quickstart.md. The resource that is required as part of the token exchange should be the request URI that the ExtProc interface receives. The exchanged token should be cached until the lifetime of the token expires. A simple in memory cache is sufficient. The configuration should include: - server and port for the grpc interface - the OAuth2 Authorization Server that supports the token exhange RFC - ClientID + Secret to obtain an id token to be used as the client assertion in the token exchange. We do not need to introduce new domain objects as this is a separate application. Also this must define completely separate e2e tests, the existing test harness cannot be used."
> **Implementation note (superseded raw input):**
> [Feature 043](../043-extproc-metadata-input/contracts/extproc-metadata-input.md) and [ADR 036](../../adrs/036-extproc-metadata-token-exchange-input.md) replace raw `Authorization` and pseudo-header input, no-Bearer pass-through, and raw resource validation.
> This document retains the standalone process, configuration, cache, circuit-breaker, and exchange mechanics that remain applicable.


## Clarifications

### Current input contract

- Q: How does ExtProc obtain token-exchange inputs? → A: Feature 043 and ADR 036 define the metadata-only input contract and its validation responses.
- Q: Which exchange errors return a 500 response? → A: The Feature 043 response matrix defines generic and exception outcomes.
- Q: When exchanged token response lacks an expiry, what TTL should the cache use? → A: Use extproc.cache.default_ttl.
- Q: How should concurrent requests refresh an expired cached token? → A: Use single-flight refresh.


## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.

  Assign priorities (P1, P2, P3, etc.) to each story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently

  E2E ACCEPTANCE TESTING (Constitution Principle XIII):
  Each acceptance scenario below MUST have a corresponding end-to-end (E2E) test in tests/e2e/.
  - Each scenario maps 1:1 to one It() block in E2E tests
  - E2E tests MUST be written BEFORE implementation begins (red-green development)
  - E2E tests MUST FAIL initially, proving they test actual functionality
  - E2E tests change minimally during implementation (fixture adjustments only)
  - E2E tests turn GREEN when implementation satisfies acceptance criteria
  - Use Ginkgo/Gomega framework following patterns in tests/e2e/README.md
  - Test file naming: tests/e2e/[feature]_test.go
  - Include comment references to spec scenarios in E2E test files
-->

### User Story 1 - Transparent Token Exchange for Agent Requests (Priority: P1)

An operator deploys an Envoy External Processing (ExtProc) service so that incoming requests from agents can have their Bearer tokens exchanged for downstream service tokens without changes to the agent’s call pattern.

**Why this priority**: This is the core capability enabling transparent token exchange for agentgateway and downstream MCP tooling.

**Independent Test**: See Feature 043 for the metadata-only input and exchange test. This feature retains the standalone exchange mechanics.


**Acceptance Scenarios**:

1. **Superseded**: Feature 043 defines metadata-only subject-token and resource-URI provenance.

2. **Given** a successful token exchange response, **When** ExtProc responds to Envoy, **Then** the Authorization header is replaced with the exchanged token and the request continues
3. **Given** token exchange fails. **When** ExtProc processes validated metadata. **Then** it returns the Feature 043 response-matrix outcome and does not forward a credential.


---

### User Story 2 - Token Exchange Cache for Repeated Calls (Priority: P2)

An operator wants repeated requests with the same Bearer token and resource to avoid unnecessary exchanges while the exchanged token is still valid.

**Why this priority**: Reduces latency and load on the authorization server while preserving security.

**Independent Test**: Perform two requests with identical tokens and resource and verify the second request reuses the cached token without a new exchange call.

**Acceptance Scenarios**:

1. **Given** a valid exchanged token stored in cache, **When** a new request arrives with the same subject token and resource, **Then** ExtProc uses the cached token without calling token exchange
2. **Given** a cached exchanged token is expired, **When** a new request arrives, **Then** ExtProc performs a fresh token exchange and updates the cache

---

### User Story 3 - Operable Configuration and Startup Validation (Priority: P3)

An operator configures the ExtProc service with gRPC settings and OAuth2 authorization server details so it starts predictably and fails fast on invalid configuration.

**Why this priority**: Reliable startup and validation are required to deploy the service safely.

**Independent Test**: Launch the service with valid configuration and verify it starts; launch with invalid configuration and verify startup fails with a clear error.

**Acceptance Scenarios**:

1. **Given** valid configuration for gRPC and token exchange settings, **When** the service starts, **Then** it binds to the configured host/port and logs a startup summary
2. **Given** missing or invalid required configuration values, **When** the service starts, **Then** it exits with a configuration validation error
3. **Given** no `client_credentials_scopes` are configured, **When** the ExtProc service acquires a client assertion, **Then** it sends `scope=openid` to the client_credentials endpoint
4. **Given** `client_credentials_scopes` are configured (e.g., `["openid", "profile", "email"]`), **When** the ExtProc service acquires a client assertion, **Then** it sends the configured scopes to the client_credentials endpoint

---

### Edge Cases

- Feature 043 defines metadata-only input rejection, including absent credentials, invalid resources, and all raw HTTP attributes.
- Feature 043 defines the exchange-error response matrix, including generic 500 responses and 503 exceptions.
- When the exchanged token response lacks an expiration time, use the default cache TTL.


## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST run as a standalone ExtProc gRPC service that can be deployed alongside Envoy
- **FR-002–FR-004**: Superseded. Feature 043 and ADR 036 define metadata-only subject-token and resource-URI input provenance.

- **FR-005**: System MUST call the identity broker’s token exchange capability as defined in `specs/013-token-exchange/spec.md`
- **FR-006**: System MUST obtain a client assertion by performing a `client_credentials` grant (with `scope=openid`) against the configured authorization server and extracting the `id_token` from the response to use as the client assertion (per RFC 7523)
- **FR-007**: System MUST send token exchange requests with the client assertion and subject token, and MUST replace the Authorization header with the exchanged access token on success
- **FR-008**: System MUST reject requests when token exchange fails or is unauthorized, without forwarding the original token
- **FR-009**: Superseded. Feature 043 rejects missing metadata and forbids no-Bearer pass-through.
- **FR-010**: The system MUST return the generic 500 ExtProc response for ordinary exchange failures. Feature 043 defines `error_uri`, expired-assertion, and circuit-open exceptions.

- **FR-011**: System MUST cache exchanged tokens keyed by subject token and resource until the exchanged token’s expiration time
- **FR-012**: System MUST use extproc.cache.default_ttl when the exchanged token response lacks an expiration time
- **FR-013**: Superseded. Feature 043 defines metadata validation and fixed 503 rejection responses.

- **FR-014**: System MUST ensure only one token exchange occurs per subject token/resource when refreshing expired cache entries
- **FR-015**: System MUST evict expired cache entries before reuse and MUST periodically sweep expired entries via a background goroutine to bound memory growth
- **FR-018**: System MUST cap cached token TTL at `extproc.cache.max_ttl` regardless of the `expires_in` value from the token exchange response
- **FR-016**: System MUST allow configuration to be loaded via the shared configuration infrastructure while using its own schema
- **FR-017**: System MUST provide a standalone E2E test harness for ExtProc scenarios that does not reuse the existing E2E harness
- **FR-019**: System MUST protect outbound token exchange calls with a circuit breaker that opens after a configurable number of consecutive server-side failures (`circuit_breaker.max_failures`), fast-failing subsequent requests with 503 until a probe succeeds after `circuit_breaker.reset_timeout`
- **FR-020**: The circuit breaker MUST only count server-side errors (5xx HTTP status codes, network errors, timeouts) as failures. Client errors (4xx HTTP status codes, e.g., invalid subject tokens returning 400 or 401) MUST NOT count towards the failure threshold and MUST NOT trip the circuit.
- **FR-021**: System MUST support disabling the circuit breaker via `circuit_breaker.enabled: false`, in which case all token exchange calls bypass the circuit breaker entirely and no requests are fast-failed due to circuit state

### Configuration Requirements *(if applicable - document before implementation)*

**Configuration Parameters**:
- **extproc.grpc.bind**: string, gRPC bind address, default `0.0.0.0`
- **extproc.grpc.port**: integer, gRPC port, default `50051`
- **extproc.grpc.max_concurrent_streams**: integer, maximum concurrent gRPC streams, default `100`
- **extproc.oauth2.token_endpoint**: string (URL), OAuth2 token endpoint that supports RFC 8693 token exchange
- **extproc.oauth2.issuer**: string (URL), issuer identifier for the authorization server
- **extproc.oauth2.client_id**: string, client identifier for obtaining a client assertion
- **extproc.oauth2.client_secret**: string, client secret for obtaining a client assertion
- **extproc.oauth2.client_credentials_endpoint**: string (URL, optional), explicit token endpoint for client_credentials grant; defaults to `{issuer}/oauth/token`
- **extproc.oauth2.exchange_timeout**: duration, timeout for outbound token exchange HTTP calls, default `5s`
- **extproc.oauth2.tls.insecure_skip_verify**: boolean, skip TLS certificate verification (DEV ONLY), default `false`
- **extproc.oauth2.tls.ca_bundle_path**: string, path to custom CA bundle file, default `""`
- **extproc.oauth2.tls.allow_http**: boolean, allow HTTP endpoints (DEV ONLY; disables SR-002 enforcement), default `false`
- **extproc.cache.default_ttl**: duration, fallback TTL when exchanged token lacks expiry information, default `5m`
- **extproc.cache.max_ttl**: duration, maximum cache TTL cap regardless of `expires_in`, default `1h`
- **extproc.log.level**: string, log level (debug/info/warn/error), default `info`
- **extproc.log.format**: string, log format (text/json), default `text`
- **extproc.circuit_breaker.enabled**: boolean, enable/disable circuit breaker for token exchange calls, default `true`
- **extproc.circuit_breaker.max_failures**: integer (>= 1), consecutive server-side (5xx) failures before the circuit opens, default `5`. Only 5xx errors, network errors, and timeouts count as failures; 4xx client errors are excluded.
- **extproc.circuit_breaker.reset_timeout**: duration, time the circuit stays open before allowing a single probe request, default `30s`

> **Note**: The complete configuration schema including validation rules and environment variable mapping is specified in [contracts/configuration.md](contracts/configuration.md).

**Example YAML Configuration**:
```yaml
grpc:
  bind: "0.0.0.0"
  port: 50051
oauth2:
  token_endpoint: "https://identity-broker.example.com/oauth2/token"
  issuer: "https://identity-broker.example.com"
  client_id: "extproc-gateway"
  client_secret: "${EXTPROC_CLIENT_SECRET}"
  tls:
    allow_http: false  # Set to true for local development only
cache:
  default_ttl: "5m"
  max_ttl: "1h"
circuit_breaker:
  enabled: true
  max_failures: 5
  reset_timeout: "30s"
log:
  level: "info"
  format: "text"
```

**Configuration Location**: Will be added to `examples/config/extproc-token-exchange.yaml` and referenced in `examples/config/README.md`

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: Security controls MUST be enabled by default and fail closed when token exchange or validation fails
- **SR-002**: Token exchange requests MUST use secure transport and verify TLS certificates by default
- **SR-003**: Client secrets and exchanged tokens MUST be treated as sensitive and redacted from logs
- **SR-004**: Security-critical operations MUST emit structured audit logs

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Requests with invalid or unauthorized tokens are rejected 100% of the time
- **SC-002**: The ExtProc service starts with valid configuration in under 10 seconds and fails within 2 seconds on invalid configuration
