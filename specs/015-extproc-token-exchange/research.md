# Research: Envoy ExtProc Token Exchange Service

**Branch**: `015-extproc-token-exchange`  
**Date**: 2026-02-23  
**Status**: Complete
> **Implementation note (superseded raw input):**
> [Feature 043](../043-extproc-metadata-input/contracts/extproc-metadata-input.md) and [ADR 036](../../adrs/036-extproc-metadata-token-exchange-input.md) replace raw `Authorization` and pseudo-header input, no-Bearer pass-through, and raw resource validation.
> This document retains the standalone process, configuration, cache, circuit-breaker, and exchange mechanics that remain applicable.


## Research Questions & Findings

### R-001: Envoy ExtProc gRPC Streaming Protocol

**Question**: How does the Envoy ExtProc streaming gRPC interface work, and what are the key requirements for implementing a compatible server?

**Research**:
- Agentgateway implements the Envoy ExtProc gRPC protocol defined in `envoy.service.ext_proc.v3.ExternalProcessor`
- The server implements a **bidirectional streaming** `Process` RPC
- Each incoming message is a `ProcessingRequest` that wraps one of: `RequestHeaders`, `RequestBody`, `RequestTrailers`, `ResponseHeaders`, `ResponseBody`, `ResponseTrailers`
- The server must respond with a `ProcessingResponse` for each request, matching the type
- Reference implementation at kgateway: uses `srv.Recv()` in a loop, switches on request type, calls `srv.Send(resp)`
- Headers and Body are **always sent** by agentgateway (unlike Envoy which allows partial processing mode)
- Body is always in **streaming mode** — the server must process both headers and body

**Decision**: Implement the ExtProc server using `envoy.service.ext_proc.v3.ExternalProcessor` with streaming `Process` method. Token exchange logic triggers on `RequestHeaders` phase. Body and trailer phases pass through unchanged.

**Rationale**: This is the standard Envoy ExtProc protocol, compatible with both Envoy and agentgateway. Processing at the `RequestHeaders` phase is sufficient because token exchange only needs the Authorization header and request URI.

**Alternatives considered**:
- Unary gRPC: Not supported by Envoy/agentgateway ExtProc spec
- HTTP middleware: Would not integrate with agentgateway's ExtProc architecture

### R-002: Agentgateway ExtProc Configuration

**Question**: How is ExtProc configured in agentgateway?

**Research**:
- ExtProc is a route-level or gateway-level policy in agentgateway config
- Configuration is minimal: `host` (address:port) and `failureMode` (failOpen/failClosed)
- Example: `extProc: { host: "extproc-service:50051", failureMode: failClosed }`
- Agentgateway connects to the ExtProc server via gRPC (no TLS option visible in docs)
- `failClosed` is the default and recommended mode

**Decision**: Use `failClosed` mode in the docker-compose agentgateway configuration. ExtProc service listens on port 50051 (gRPC default).

**Rationale**: `failClosed` ensures requests are blocked when the ExtProc service is unavailable, which aligns with Security-First (Constitution Principle I).

### R-002a: Agentgateway `:path` Pseudo-Header Behavior

**Question**: Does agentgateway populate the `:path` pseudo-header with a relative path or a full absolute URI when sending ExtProc `RequestHeaders`?

**Research**:
- Agentgateway routes MCP requests from clients to backend MCP servers via HTTP streamable transport
- When agentgateway sends ExtProc `RequestHeaders`, the `:path` contains the **full backend target URL** (e.g., `http://mcp-server:9003/mcp`), not the client-facing relative path
- This behavior is consistent with agentgateway acting as a reverse proxy that rewrites `:path` to the backend target before ExtProc processing
- **IMPORTANT**: This assumption MUST be verified in E2E tests during Phase 2f. If `:path` is relative, FR-004 and the URI validation must be revised to construct the resource URI from `:path` + `:authority` + `:scheme`

**Decision**: Assume `:path` contains a full absolute URI. Validate this assumption in E2E tests. If the assumption is incorrect, revise FR-004 to construct the resource from available headers.

**Rationale**: The token exchange `resource` parameter per RFC 8693 should be an absolute URI identifying the target resource. Agentgateway's backend routing naturally provides this.

### R-003: Agentgateway MCP Backend with Streamable HTTP

**Question**: How to configure agentgateway to proxy MCP requests to a downstream MCP server?

**Research**:
- Agentgateway supports MCP backends with multiple target types: `stdio`, `mcp` (HTTP streamable)
- HTTP streamable MCP uses `mcp: { host: "http://server:port/path" }` syntax
- The MCP server mock should expose a Streamable HTTP endpoint
- Agentgateway automatically manages MCP session state (session pinning, encrypted session IDs)
- Backend authentication can pass through the Authorization header using `backendAuth: { passthrough: {} }`

**Decision**: Configure agentgateway with an MCP backend pointing to the MCP server mock via HTTP streamable transport. The ExtProc policy will intercept requests to perform token exchange before they reach the MCP server.

**Rationale**: HTTP streamable is the standard modern MCP transport. Using ExtProc for token exchange before routing to MCP backends provides transparent token handling.

### R-004: Token Exchange HTTP Flow (RFC 8693)

**Question**: How should the ExtProc service perform token exchange against the identity broker?

**Research**:
- The identity broker exposes `POST /oauth2/token` for RFC 8693 token exchange (spec `013-token-exchange`)
- Request format: `application/x-www-form-urlencoded` with parameters:
  - `grant_type=urn:ietf:params:oauth:grant-type:token-exchange`
  - `subject_token=<bearer_token>` (the incoming Bearer token)
  - `subject_token_type=urn:ietf:params:oauth:token-type:access_token`
  - `resource=<request_uri>` (the resource being accessed)
  - `client_assertion=<jwt>` (obtained via client credentials)
  - `client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer`
- Response: JSON with `access_token`, `token_type`, `issued_token_type`, `expires_in`
- The ExtProc service needs to first obtain its own ID token (client assertion) using client_id/secret at the authorization server's token endpoint

**Decision**: ExtProc service uses a two-step process: (1) obtain a client assertion (ID token) using client_credentials grant, (2) perform RFC 8693 token exchange with the identity broker's `/oauth2/token` endpoint.

**Rationale**: This follows the exact flow specified in spec 013 where the client_assertion JWT identifies the privileged client.

### R-005: Client Assertion Acquisition

**Question**: How should the ExtProc service obtain a client assertion to present to the token exchange endpoint?

**Research**:
- The client_assertion must be a JWT issued by the upstream OAuth2 server
- The ExtProc service has `client_id` and `client_secret` configured
- Standard approach: Use `client_credentials` grant at the OAuth2 authorization server's token endpoint to obtain an ID token
- The ID token serves as the client_assertion for token exchange
- The client assertion should be obtained **at application startup** to ensure it is available for the first token exchange request without added latency
- A background goroutine should refresh the assertion before it expires (e.g., at 80% of TTL) so the hot path never blocks on assertion acquisition
- The client assertion cache is separate from exchanged token cache (different lifecycle)

**Decision**: Obtain a client assertion (ID token) via `client_credentials` grant **at startup**. Run a background refresh goroutine that re-acquires the assertion before expiry (at 80% of TTL or 30 seconds before expiry, whichever is earlier). The token exchange hot path reads the cached assertion without any blocking HTTP call. If the initial startup acquisition fails, the service must fail to start (fail-fast).

The `id_token` field from the client credentials response is used as the `client_assertion` (not `access_token`). Operators must configure the upstream OAuth2 server to include an `id_token` in `client_credentials` grant responses (typically requires `scope=openid`).

**Rationale**: Acquiring the client assertion at startup and refreshing in the background removes an HTTP round-trip from the token exchange critical path, reducing latency. Fail-fast on startup ensures misconfigured credentials are detected immediately rather than on first request.

**Alternatives considered**:
- On-demand lazy acquisition: Adds latency to the first token exchange request and to every request after assertion expiry
- Pre-configured static JWT: Inflexible, requires manual rotation
- Direct client_id/client_secret on token exchange: Spec requires JWT client_assertion

### R-006: Token Caching Strategy

**Question**: What caching strategy should the ExtProc service use for exchanged tokens?

**Research**:
- Cache key: `(subject_token, resource_uri)` pair — uniquely identifies the context
- Cache value: `(access_token, expiry_time)` from token exchange response
- TTL: Use `expires_in` from response; fallback to `extproc.cache.default_ttl` if absent
- Concurrency: Use `golang.org/x/sync/singleflight` for single-flight refresh (one exchange per key, others wait)
- Eviction: Check expiry before reuse; expired entries trigger fresh exchange
- Implementation: `sync.RWMutex` + `map[string]*cachedToken` with lazy eviction
- No need for LRU or size limits — spec says "simple in memory cache"

**Decision**: Implement a simple in-memory cache with `sync.RWMutex` + `map`. Use `singleflight.Group` for concurrent refresh deduplication. Cache key is a Go struct `tokenCacheKey{subjectToken, resourceURI}` used directly as a map key. This is simpler, faster (no hash computation per lookup), and avoids separator-injection collisions that affect `hash(a + sep + b)` approaches.

**Rationale**: Simple, thread-safe, and meets all spec requirements. Singleflight prevents thundering herd on cache miss. The struct key approach was chosen over sha256-based keys after a security review identified that the `|` separator character can appear in JWT tokens and URI paths, creating theoretical hash collisions.

A background periodic eviction goroutine (ticker at `default_ttl / 2`) sweeps expired entries from the map. A `cache.max_ttl` cap prevents indefinite caching from servers returning very large `expires_in` values.

**Alternatives considered**:
- sync.Map: Less control over eviction
- External cache (Redis): Over-engineered for this use case per spec ("simple in memory cache")
- groupcache: Adds unnecessary dependency

### R-007: Configuration Infrastructure Reuse

**Question**: How should the ExtProc service reuse the existing configuration infrastructure without sharing structures?

**Research**:
- The existing config system uses Viper/Cobra with `internal/config/loader.go`
- The loader is env-prefixed with `IDENTITY_BROKER_*`
- The spec says "same configuration _structure_ but with completely different content" and "its own schema"
- Approach: Create a new config loader in `internal/extproc/config/` that uses Viper/Cobra but defines its own Config struct and env prefix
- The new loader can reuse patterns from `internal/config/loader.go` (multi-source loading, env expansion, validation) but with different schema
- New env prefix: `EXTPROC_*` to avoid collision

**Decision**: Create a standalone config package in `internal/extproc/config/` with its own `Config` struct, loader, and validator. Reuse the Viper/Cobra pattern but with prefix `EXTPROC_*` and schema-specific validation. The new config has sections: `grpc`, `oauth2`, `cache`.

**Rationale**: Follows spec requirement of separate schema while maintaining architectural consistency. New env prefix avoids collision with identity broker config.

### R-008: Separate E2E Test Harness

**Question**: How should the separate E2E test harness be structured?

**Research**:
- Spec requires: "completely separate e2e tests, the existing test harness cannot be used"
- The existing E2E tests in `tests/e2e/` use Ginkgo/Gomega with production bootstrap (app.Builder)
- The new ExtProc is a separate application — it doesn't use `app.Builder`
- The new E2E tests need to spin up: (1) the ExtProc gRPC server, (2) a mock identity broker token exchange endpoint, (3) a mock OAuth2 token endpoint
- Use Ginkgo/Gomega to maintain consistency with project patterns (ADR 007)
- Test location: `tests/e2e/extproc/` with its own `extproc_suite_test.go`

**Decision**: Create a separate Ginkgo test suite in `tests/e2e/extproc/` with its own bootstrap, fixtures, and helpers. Tests spin up the ExtProc server with mock HTTP services for the token exchange and OAuth2 endpoints.

**Rationale**: Follows spec requirement for separate test harness while maintaining Ginkgo/Gomega consistency (ADR 007).

### R-009: Go gRPC Dependencies for ExtProc

**Question**: What Go dependencies are needed for the ExtProc gRPC server?

**Research**:
- `google.golang.org/grpc` — gRPC server framework
- `github.com/envoyproxy/go-control-plane` — Envoy API protos including `envoy.service.ext_proc.v3`
- `google.golang.org/grpc/health/grpc_health_v1` — gRPC health checking
- The go-control-plane package provides the compiled protobuf types for:
  - `envoy.service.ext_proc.v3.ExternalProcessor` (service interface)
  - `envoy.service.ext_proc.v3.ProcessingRequest` / `ProcessingResponse`
  - `envoy.config.core.v3.HeaderValue`, `HeaderValueOption`
- Reference kgateway implementation uses these exact packages

**Decision**: Add `google.golang.org/grpc` and `github.com/envoyproxy/go-control-plane` as dependencies. Use the standard gRPC server with health check service.

**Rationale**: Standard approach, proven in kgateway reference implementation.

### R-010: MCP Server Mock Design

**Question**: How should the MCP server mock be designed to print Bearer token claims?

**Research**:
- The mock should be a simple Go HTTP server implementing Streamable HTTP MCP transport
- MCP Streamable HTTP uses `POST /mcp` with JSON-RPC 2.0 messages
- The mock needs to handle `initialize`, `tools/list`, and `tools/call` methods
- On `tools/call`, it should extract the Bearer token from the Authorization header, decode JWT claims, and return them as the tool result
- MCP Streamable HTTP requires: `Content-Type: application/json`, `Mcp-Session-Id` header for session management
- The mock should expose one tool: `show_claims` that returns the decoded JWT claims from the request's Bearer token

**Decision**: Implement a minimal Go MCP server under `mocks/mcp-server/` that supports Streamable HTTP transport. It exposes a `show_claims` tool that decodes and returns the Bearer token's JWT claims.

**Rationale**: Minimal implementation that demonstrates the token exchange working end-to-end: the original bearer token is exchanged for a downstream token which is then visible as claims in the MCP tool response.

### R-011: Sample Agent MCP Client Button

**Question**: How should the sample agent be augmented to call agentgateway MCP endpoints?

**Research**:
- The current sample agent is a simple HTML-rendered Go HTTP server with OAuth2 login flow
- It renders inline HTML in handler functions (no templating engine)
- To add MCP client functionality, we need a "Call MCP Tool" button on the user info page
- The button should trigger an HTTP request to the agentgateway's MCP endpoint using the user's Bearer token
- MCP over Streamable HTTP: POST to agentgateway with JSON-RPC messages, Authorization: Bearer header
- Flow: (1) Initialize MCP session, (2) Call `show_claims` tool, (3) Display result

**Decision**: Add a new handler in the sample agent that acts as a basic MCP client. Add a "Call MCP Tool" button to the user home page. The handler initializes an MCP session via agentgateway, calls the `show_claims` tool, and displays the returned claims.

**Rationale**: Demonstrates the full end-to-end flow: user authenticates → sample agent gets token → calls agentgateway → ExtProc exchanges token → MCP server sees exchanged token claims.

### R-012: Docker Compose Architecture

**Question**: How should the three new containers (agentgateway, ExtProc service, MCP server mock) be arranged in docker-compose?

**Research**:
- Existing containers: identity-broker, upstream-oauth2, third-party-oauth2, sample-agent, frontend
- New containers needed:
  1. `agentgateway` — runs agentgateway binary with config for MCP + ExtProc
  2. `extproc-token-exchange` — the new ExtProc gRPC service built from `cmd/extproc-token-exchange/`
  3. `mcp-server-mock` — simple Go MCP server mock built from `mocks/mcp-server/`
- Network: all on existing `aib-network`
- Dependencies: extproc-token-exchange depends on identity-broker (for token exchange); agentgateway depends on extproc-token-exchange and mcp-server-mock
- Sample-agent needs access to agentgateway for MCP calls

**Decision**: Add three new services to `docker-compose.yml`:
- `agentgateway`: Uses official Docker image `ghcr.io/agentgateway/agentgateway:latest` with mounted config
- `extproc-token-exchange`: Built from `Dockerfile.mock` with `SERVICE_PATH=cmd/extproc-token-exchange`
- `mcp-server-mock`: Built from `Dockerfile.mock` with `SERVICE_PATH=mocks/mcp-server/cmd/mcp-server`

**Rationale**: Uses existing Dockerfile.mock pattern for Go services. Official agentgateway image avoids need to build Rust from source.

### R-013: ExtProc Error Response Mechanism

**Question**: How does an ExtProc server return error responses (500, 503) to the client via Envoy/agentgateway?

**Research**:
- ExtProc can return an **immediate response** to terminate request processing
- Use `ProcessingResponse.ImmediateResponse` field with `ImmediateResponse` struct containing:
  - `Status` (HTTP status code via `envoy.type.v3.HttpStatus`)
  - `Headers` (response headers)
  - `Body` (response body string)
- This causes agentgateway to send the response directly to the client without forwarding to the backend
- For normal header modification, use `ProcessingResponse.RequestHeaders` with `HeaderMutation` to modify/add/remove headers

**Decision**: Use `ImmediateResponse` for error cases (missing bearer, failed exchange, invalid URI) and `HeaderMutation` on `RequestHeaders` response for successful token replacement.

**Rationale**: This is the standard Envoy ExtProc mechanism for short-circuiting requests on error.

## Summary of Decisions

| # | Decision | Technology/Approach |
|---|----------|-------------------|
| R-001 | ExtProc gRPC streaming | `envoy.service.ext_proc.v3.ExternalProcessor` |
| R-002 | Agentgateway ExtProc config | `extProc: { host, failureMode: failClosed }` |
| R-003 | MCP backend | HTTP streamable transport via agentgateway |
| R-004 | Token exchange | HTTP POST to identity broker `/oauth2/token` |
| R-005 | Client assertion | `client_credentials` grant → ID token → cache |
| R-006 | Token cache | `sync.RWMutex` + `map` + `singleflight` |
| R-007 | Configuration | Separate schema in `internal/extproc/config/`, Viper/Cobra |
| R-008 | E2E tests | Separate Ginkgo suite in `tests/e2e/extproc/` |
| R-009 | gRPC deps | `google.golang.org/grpc` + `go-control-plane` |
| R-010 | MCP mock | Go HTTP server, Streamable HTTP, `show_claims` tool |
| R-011 | Sample agent | MCP client button calling agentgateway |
| R-012 | Docker compose | 3 new services: agentgateway, extproc, mcp-mock |
| R-013 | Error responses | `ImmediateResponse` for errors, `HeaderMutation` for success |
