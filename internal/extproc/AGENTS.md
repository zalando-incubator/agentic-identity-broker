# ExtProc Token Exchange Service — `internal/extproc/`

**Use retrieval-led reasoning. Read source files before you make assumptions about types, interfaces, or behavior.**

This is a standalone process in `cmd/extproc-token-exchange/`. `internal/extproc/*` remains independent of broker domain, port, and adapter packages.

---

## Package Map

```
internal/extproc/
  config/
    config.go       gRPC, OAuth2, cache, circuit-breaker, OPA, and telemetry schema
    loader.go       Viper load, environment expansion, defaults, and Cobra flags
    validate.go     Core, authorization, and telemetry validation

  authorization/
    authorizer.go   `Authorizer` and `OPAAuthorizer`
    input.go        OPAInput map alias, MCPInput, ContextInput types
    input_builder.go Builds the OPA input document
    parser.go        ParseMCPMessage, ParseMCPBatch — JSON-RPC 2.0 parsing
    decision.go      `OPADecision` and result extraction
    doc.go           Package documentation

  server/
    server.go       ExtProc gRPC request processing and OPA integration
    exchanger.go    RFC 8693 exchange, cache, singleflight, stale fallback, and refresh
    circuit_breaker.go  Broker-failure circuit breaker
    *_test.go       Unit, cancellation, cache, security, circuit-breaker, and TLS coverage
```

The files outside this tree contain the entry point and wiring:

```
cmd/extproc-token-exchange/
  main.go       Cobra `Execute` call
  root.go       Configuration, telemetry bridge, logging, service wiring, and gRPC lifecycle
```

ExtProc E2E tests use a separate Ginkgo suite:

```
tests/e2e/extproc/
  extproc_suite_test.go  Separate suite entry point
  bootstrap/             ExtProc startup wrappers
  fixtures/, helpers/    Deterministic data and gRPC support
  *_test.go              Token exchange, OPA, agentgateway, and telemetry scenarios
```

Some agentgateway scenarios use Docker and `Ordered` to share the expensive container.

---

## Key Types and Interfaces

### `internal/extproc/config`

| Type | Purpose |
|---|---|
| `Config` | gRPC, OAuth2, cache, log, circuit breaker, OPA, and telemetry configuration |
| `AuthorizationConfig` | OPA enabled flag, policy, default decision, timeout, and body limit |
| `PolicyConfig` | Local Rego or OPA configuration file, package, and decision |
| `GRPCConfig` | `Bind`, `Port`, `MaxConcurrentStreams` |
| `OAuth2Config` | Token endpoint, issuer, client credentials, scopes, assertion type, timeout, and TLS |
| `TLSConfig` | `InsecureSkipVerify`, `CaBundlePath`, `AllowHTTP` |
| `CacheConfig` | `DefaultTTL`, `MaxTTL` |
| `CircuitBreakerConfig` | `Enabled`, `MaxFailures`, `ResetTimeout` |
| `LogConfig` | `Level`, `Format` |
| `TelemetryConfig` | OpenTelemetry service, signals, and exporter configuration |

**Loading pipeline** (in `loader.go`):

1. Viper uses the `EXTPROC_` prefix. It replaces dots in keys with underscores.
2. The service can load an optional YAML file through `EXTPROC_CONFIG_PATH`.
3. Explicit Cobra flags use `RegisterFlags()` and `bindFlags()`. They have the highest precedence.
4. `expandEnvVars()` expands `${VAR}` in supported string fields and exporter header values.
5. `Validate()` fails early. It collects all errors.

### `internal/extproc/authorization`

| Type/Symbol | Purpose |
|---|---|
| `Authorizer` interface | Port: `Evaluate(ctx, input OPAInput) (*OPADecision, error)` + `Stop(ctx)` |
| `OPAAuthorizer` struct | Production `Authorizer`. It has two backends: PreparedEvalQuery (path) or sdk.OPA (config_file) |
| `NewOPAAuthorizer(cfg, logger)` | Path mode compiles Rego at startup. Config-file mode denies access until the bundle is ready. |
| `OPAInput` map alias | OPA document from the opa-envoy-plugin-compatible base, with top-level `type`, `mcp`, and `context` keys |
| `MCPInput` struct | MCP protocol fields: `JSONRPC`, `Method`, `ToolName`, `Arguments`, `SessionID` |
| `ContextInput` struct | Authorization context fields, including `granted_permission_sets_available` and token-bound `granted_permission_sets` when present |
| `OPADecision` struct | Policy output: `Action string` (`"allow"` or `"deny"`), `Reasons []string` |
| `ParseDecision(any)` | Type-safe extraction of `action` and `reasons` from the OPA result map |
| `BuildOPAInput(protocol, body, headers, grantedPermissionSets)` | Builds the OPA input document with protocol-specific parsing |
| `ParseMCPMessage(body)` | Parses a JSON-RPC 2.0 single message → `*MCPMessage` |
| `ParseMCPBatch(body)` | Finds and parses JSON-RPC 2.0 batch messages (FR-023) |

### `internal/extproc/server`

| Type/Symbol | Purpose |
|---|---|
| `Exchanger` interface | Port: `Exchange(ctx, subjectToken, resourceURI string) (ExchangeResult, error)` + `Shutdown()` |
| `Server` struct | Implements `ExternalProcessorServer`. Fields: `cfg`, `exchanger`, `authorizer`, `logger` |
| `NewServer(cfg, exchanger, logger)` | Constructor with OPA disabled (`authorizer=nil`) |
| `NewServerWithAuthorizer(cfg, exchanger, authorizer, logger)` | Constructor with OPA enabled |
| `Server.Process(stream)` | Streaming gRPC RPC. It dispatches on the `req.Request` type. |
| `TokenExchanger` struct | Concrete `Exchanger` implementation |
| `NewTokenExchanger(cfg, logger)` | At startup, it gets a client assertion. It fails early and creates a background goroutine. |
| `ErrAssertionExpired` | Sentinel error. The caller returns 503. |
| `tokenCacheKey` struct | Map key for the token cache: `{subjectToken, resourceURI}`. The struct prevents separator injection. |
| `cachedToken` | `accessToken string`, `expiresAt time.Time` |
| `assertionState` | `value`, `issuedAt`, `expiresAt`. The service atomically swaps it through `atomic.Pointer[assertionState]`. |

---

## Architecture Invariants

### 1. Standalone process — isolated ExtProc packages

Do not import `internal/domain/`, `internal/ports/`, or `internal/adapters/` from `internal/extproc/`, except `internal/domain/approval/toolpattern` as authorized by ADR 035 for shared approval-pattern semantics. No other `internal/domain/` package is permitted.

The `cmd/extproc-token-exchange/` composition layer can import `internal/ports`.
It can also import `internal/adapters/telemetry` for OpenTelemetry (ADR 027).
Keep `mapTelemetryConfig` in `cmd/extproc-token-exchange/root.go`.

### 2. Two port interfaces: Exchanger and Authorizer

`Exchanger` in `server/server.go` and `authorization.Authorizer` in `authorization/authorizer.go` are the two hexagonal boundaries in this service.

- `Server` depends on both interfaces. Use `TokenExchanger` as the production `Exchanger` implementation. Use `OPAAuthorizer` as the production `Authorizer` implementation.
- Use `_test.go` helpers or `tests/e2e/extproc/bootstrap/` for ExtProc startup and readiness tests.
- Keep the `authorizer` field on `Server` as `authorization.Authorizer`. Do not add an adapter layer or `map[string]any` indirection.
- If `authorizer == nil` (OPA disabled), run token exchange during `RequestHeaders`. Do not buffer the body. This adds zero overhead.
- If a request has a body and OPA is enabled, exchange tokens in `RequestHeaders`. Then evaluate OPA in `RequestBody`.
- For header-only requests, evaluate first.
- Use mock or stub implementations of both interfaces in tests.

### 2a. OPA Authorization Pipeline (when enabled)

Read token-exchange inputs only from the flat `aib.tokenexchange` dynamic-metadata namespace. Do not use raw `Authorization`, `:path`, `:scheme`, `:authority`, or any other HTTP attribute as a token-exchange input.

Validate `subject_token` before `resource_uri`. Both fields must be strings. The subject token must be nonblank and have no case-insensitive `Bearer ` prefix. The resource URI must be nonblank, absolute, HTTP or HTTPS, and have a host. Pass accepted metadata values to `Exchanger.Exchange` without modification.

Invalid subject-token metadata returns the fixed 503 `invalid_subject_token` response. Invalid resource metadata returns the fixed 503 `invalid_resource` response. Validate before protocol metadata checks, MCP transport checks, OPA input construction, or OPA evaluation. Invalid metadata must not reach OPA.

Keep request headers for OPA input and MCP transport checks. In particular, `:method` remains a transport check. These headers cannot supply token-exchange inputs.

An Agentgateway `jwtAuth` policy with `mode: strict` must run before ExtProc. Set `preserveToken: false` explicitly. This removes the raw authorization header, but the ExtProc metadata producer can still project the validated token with `jwt.rawToken.unredacted()`.

```
Body-bearing requests:
RequestHeaders → validate subject metadata → validate resource metadata
               → protocol and transport checks → Exchanger.Exchange(subject, resource)
               → Authorization header mutation + BUFFERED body
RequestBody    → BuildOPAInput(protocol, body, headers, grantedPermissionSets)
               → OPAAuthorizer.Evaluate(ctx, opaInput) → allow: echo body
                                                      → deny: 403 ImmediateResponse {"error":"access_denied","error_description":"...reasons..."}

Header-only requests:
RequestHeaders(end_of_stream=true) → validate subject metadata → validate resource metadata
                                  → protocol and transport checks
                                  → authorization.BuildOPAInputHeadersOnly(protocol, headers)
                                  → OPAAuthorizer.Evaluate(ctx, opaInput) → allow: Exchanger.Exchange(subject, resource) + Authorization header mutation
                                                                         → deny: 403 ImmediateResponse `{"error":"access_denied","error_description":"...reasons..."}`
```

`authorization.BuildOPAInput` selects a parser by `protocol`:

- For `"mcp"`, `ParseMCPMessage` produces `type="mcp_tool_call"` for `tools/call`. It produces `type="mcp_method"` for other methods.
- For all other values, `type="unknown"` contains the raw body in `input.attributes.request.http.body`.

`OPAAuthorizer` has two backends:

- `rego.PreparedEvalQuery` runs when `authorization.policy.path` is set. This is a local Rego file that compiles once.
- `sdk.OPA` runs when `authorization.policy.config_file` is set. This is an OPA configuration YAML file that is bundle-aware.

All evaluation errors, timeouts, and undefined results return `deny`. This fails closed per SR-001.

For MCP, GET is the only header-only transport. POST carries the JSON-RPC body. Reject unsupported methods with 405.

### 3. Config is loaded once at startup

Call `LoadWithCommand()` in `cmd/extproc-token-exchange/root.go`. Do not read configuration elsewhere. Pass the loaded `*Config` by pointer.

### 4. Fail-fast at startup

- `Validate()` has 15 core rules, 9 authorization rules when enabled, and 4 telemetry rules when enabled. Its numbered comments run from 1 through 19.
- `NewTokenExchanger()` calls `refreshClientAssertion()` synchronously. It returns an error if this call fails.
- `buildHTTPClient()` reads and parses the CA bundle at construction time.

### 5. Token cache design

- Cache entries use `tokenCacheKey{subjectToken, resourceURI}` with `sync.RWMutex` and `singleflight`.
- The TTL comes from `expires_in` or `cache.default_ttl`. It does not exceed `cache.max_ttl`.
- Expired entries stay until their fixed `staleUntil` deadline. During transient broker errors, the exchanger can return this bounded stale token.
- `granted_permission_sets` uses the same cache window as the access token.

### 6. Client assertion refresh

- The service stores `assertionState` through `atomic.Pointer[assertionState]` for lock-free reads.
- The assertion refresh ticker runs every `min(max(DefaultTTL/2, 1s), 30s)`.
  Cache eviction runs every `max(DefaultTTL/2, 1s)`.
- The service refreshes the assertion when its remaining lifetime is less than `max(20% of total lifetime, 30s)`.
- If scopes are empty or blank, the client-credentials grant requests `openid`.
- A refresh error is logged. The prior assertion remains until it expires.
- If the assertion is nil or expired, `Exchange()` returns `ErrAssertionExpired`. The error signals 503 to Envoy.

### 7. Body/trailer pass-through

Envoy ExtProc needs a response type for each phase. Use `StreamedBodyResponse` for request and response bodies.

### 8. Metadata-only token-exchange input (ADR 036)

Both header paths must call `extractTokenExchangeInput()` as the single validation path. Do not restore raw-header extraction, resource-URI construction, fallback, dual-source precedence, or a configuration toggle. Do not log, return, or emit the subject token or rejected resource URI in diagnostics.

See accepted `adrs/036-extproc-metadata-token-exchange-input.md`. It changes the input trust boundary only. It does not change the standalone-process boundary in ADR 011 or the token cache in ADR 012.

---

## Validation Rules

`Validate()` has 15 core rules, 9 authorization rules when enabled, and 4 telemetry rules when enabled. See `config/validate.go`.

---

## Testing Conventions (this subtree)

- **Framework**: Use stdlib `testing` with `testify`.
  Use `require` for preconditions and `assert` for checks.
- **Package**: Prefer `package server_test` or `package config_test` for black-box tests.
  Use the package under test only for unexported helpers or test exports.
- **Mocking**: Use hand-written mock/stub implementations for `Exchanger` and `Authorizer`.
  Use `httptest.NewServer` for token-exchange endpoints.
- **Configuration helpers**: Call `extprocconfig.LoadFromViper(v)` with a configured `*viper.Viper`.
  This prevents environment pollution.
- **No Ginkgo here**: Use standard `t.Run` or table-driven subtests.
  Use Ginkgo/Gomega only in `tests/e2e/extproc/`.
- **Race detector**: Do `go test -race` for ExtProc changes.
- **Commands**: Run `just extproc-test` for unit tests with the race detector.
  Run `just test-e2e-extproc` for the ExtProc Ginkgo suite.

---

## Configuration Reference (selected defaults)

Read `config/config.go` and `config/loader.go` for the complete schema and defaults.

| Key | Default | Notes |
|---|---|---|
| `grpc.bind` | `0.0.0.0` | |
| `grpc.port` | `50051` | |
| `grpc.max_concurrent_streams` | `100` | |
| `oauth2.token_endpoint` | _(required)_ | RFC 8693 exchange endpoint |
| `oauth2.issuer` | _(required)_ | Derives the client-credentials endpoint if no override exists |
| `oauth2.client_id` | _(required)_ | |
| `oauth2.client_secret` | _(required)_ | The service never logs it |
| `oauth2.client_credentials_scopes` | `[]` | Requests `openid` when empty or blank |
| `oauth2.client_assertion_type` | `id_token` | `id_token` or `access_token` |
| `oauth2.exchange_timeout` | `5s` | |
| `oauth2.tls.insecure_skip_verify` | `false` | Development only |
| `oauth2.tls.allow_http` | `false` | Development only |
| `cache.default_ttl` | `5m` | |
| `cache.max_ttl` | `1h` | |
| `circuit_breaker.enabled` | `true` | |
| `circuit_breaker.max_failures` | `5` | |
| `circuit_breaker.reset_timeout` | `30s` | |
| `authorization.enabled` | `false` | Requires one policy source when enabled |
| `authorization.default_decision` | `deny` | Must remain fail-closed |
| `authorization.evaluation_timeout` | `100ms` | |
| `authorization.max_body_size` | `1048576` | Bytes |
| `telemetry.enabled` | `false` | |
| `telemetry.service_name` | `extproc-token-exchange` | |
| `telemetry.exporter.protocol` | `grpc` | |
| `telemetry.exporter.endpoint` | _(required when enabled)_ | |

Use `examples/config/extproc-token-exchange.yaml` for base token-exchange configuration.
Use `examples/config/extproc-opa-authorization.yaml` for OPA.
Use `examples/config/extproc-telemetry.yaml` for OpenTelemetry.

## Docker Compose

Docker Compose mounts `config.extproc.docker.yaml` at `/app/config.yaml`.
It sets `EXTPROC_CONFIG_PATH=/app/config.yaml`.
`agentgateway` connects to `extproc-token-exchange:50051` on the internal Compose network.

---

## Spec and Design Documents

| Document | Path |
|---|---|
| Feature spec | `specs/015-extproc-token-exchange/spec.md` |
| Implementation plan | `specs/015-extproc-token-exchange/plan.md` |
| Data model | `specs/015-extproc-token-exchange/data-model.md` |
| gRPC contract | `specs/015-extproc-token-exchange/contracts/extproc-grpc.md` |
| Configuration schema | `specs/015-extproc-token-exchange/contracts/configuration.md` |
| Docker Compose integration | `specs/015-extproc-token-exchange/contracts/docker-compose.md` |
| Quickstart | `specs/015-extproc-token-exchange/quickstart.md` |
