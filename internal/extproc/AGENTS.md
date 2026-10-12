# ExtProc Token Exchange Service — `internal/extproc/`

All repository paths in this file are relative to the repository root.
Read source files before changes. Tree entries use the repository-root path at the start of each tree.

This standalone process starts in `cmd/extproc-token-exchange/`.
Its only broker-domain dependency is the shared matcher permitted by `adrs/035-shared-tool-pattern-matching.md`.

---

## Package Map

```
internal/extproc/
  config/
    config.go       gRPC, OAuth2, cache, circuit-breaker, OPA, approval, session, and telemetry schema
    loader.go       Viper load, environment expansion, defaults, and Cobra flags
    validate.go     Base, authorization, approval, session, and telemetry validation

  authorization/
    authorizer.go   `Authorizer` and `OPAAuthorizer`
    input.go        OPAInput map alias and ContextInput type
    input_builder.go InputBuilder: converts headers once, builds body-bearing OPA inputs
    parser.go        ParseMCPMessage: validates decoded JSON-RPC 2.0 messages
    decision.go      `OPADecision` and result extraction
    doc.go           Package documentation

  approval/
    cache.go         Approval cache, matching, reservations, and eviction
    client.go        Broker approval API client
    gate.go          Approval-gate request decision
    syncer.go        ETag long-poll synchronization
    precedence.go    ExtProc candidate ranking and selection

  httpclient/
    httpclient.go    Traced outbound HTTP client construction

  server/
    server.go       ExtProc gRPC request processing, OPA, and approval-gate integration
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
| `Config` | gRPC, OAuth2, cache, log, circuit breaker, OPA, approval, session, and telemetry configuration |
| `AuthorizationConfig` | OPA enabled flag, policy, default decision, timeout, and body limit |
| `PolicyConfig` | Local Rego or OPA configuration file, package, and decision |
| `ToolApprovalsConfig` | Approval gate enable flag, broker URL, long-poll timeout, idle TTL, request timeout, and maximum staleness |
| `SessionsConfig` | Agent-session extraction configuration |
| `GRPCConfig` | `Bind`, `Port`, `MaxConcurrentStreams` |
| `OAuth2Config` | Token endpoint, issuer, client credentials, scopes, assertion type, timeout, and TLS |
| `TLSConfig` | `InsecureSkipVerify`, `CaBundlePath`, `AllowHTTP` |
| `CacheConfig` | `DefaultTTL`, `MaxTTL` |
| `CircuitBreakerConfig` | `Enabled`, `MaxFailures`, `ResetTimeout` |
| `LogConfig` | `Level`, `Format` |
| `TelemetryConfig` | OpenTelemetry service, signals, and exporter configuration |

**Loading pipeline** (`internal/extproc/config/loader.go`):

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
| `mcp` input object | Map of decoded MCP fields and agentgateway target-server metadata. Omit fields irrelevant to the request type |
| `ContextInput` struct | Authorization context fields, including `granted_permission_sets_available` and token-bound `granted_permission_sets` when present |
| `OPADecision` struct | `Action`, `Reasons`, and optional `ApprovalContext`. Supports allow, deny, and approval-required outcomes |
| `ParseDecision(map[string]any)` | Fail-closed extraction. `ciba_required` remains deny |
| `NewInputBuilder(protocol, headers, targetServerName, contextInput)` | Converts Envoy-compatible headers once. `Build(body, parsed)` derives independent policy inputs for standalone and batch elements |
| `ParseMCPMessage(value)` | Validates one decoded JSON-RPC 2.0 message → `*MCPMessage` |
| `BuildOPAInputHeadersOnly(protocol, headers, targetServerName)` | Constructs the separate header-only policy input |

### `internal/extproc/server`

| Type/Symbol | Purpose |
|---|---|
| `Exchanger` interface | Port: `Exchange(ctx, subjectToken, resourceURI string) (ExchangeResult, error)` + `Shutdown()` |
| `ApprovalGate` interface | Port: `Evaluate(context.Context, approval.Invocation) approval.Outcome` |
| `Server` struct | Implements `ExternalProcessorServer`. Fields: `cfg`, `exchanger`, `authorizer`, `approvalGate`, `logger` |
| `NewServer(cfg, exchanger, logger)` | Constructor with OPA and approval handling disabled |
| `NewServerWithAuthorizer(cfg, exchanger, authorizer, logger)` | Constructor with OPA enabled |
| `NewServerWithApprovalGate(cfg, exchanger, authorizer, gate, logger)` | Constructor with OPA and approval handling enabled |
| `TokenExchanger` struct | Concrete `Exchanger` implementation |
| `NewTokenExchanger(cfg, logger)` | At startup, it gets a client assertion. It fails early and creates a background goroutine. |
| `ErrAssertionExpired` | Sentinel error. The caller returns 503. |
| `tokenCacheKey` struct | Map key for the token cache: `{subjectToken, resourceURI}`. The struct prevents separator injection. |
| `cachedToken` | Token, permission-set snapshot, broker identity, error state, expiry, and fixed stale deadline |
| `assertionState` | `value`, `issuedAt`, `expiresAt`. The service atomically swaps it through `atomic.Pointer[assertionState]`. |

---

## Architecture and Security

Read `ARCHITECTURE.md` and `.specify/memory/constitution.md` before boundary changes.

### Standalone boundary

- Do not import broker ports or adapters from `internal/extproc/`.
- Import only `internal/domain/approval/toolpattern` from the broker domain, as permitted by `adrs/035-shared-tool-pattern-matching.md`.
- Keep approval precedence selection in `internal/extproc/approval/`.
- Keep telemetry composition in `cmd/extproc-token-exchange/root.go`, including `mapTelemetryConfig`.

`adrs/027-extproc-telemetry-shared-dependency.md` permits the command layer to import `internal/ports` and `internal/adapters/telemetry`.
It does not permit these imports inside `internal/extproc/`.

### Service boundaries

`Exchanger` and `ApprovalGate` live in `internal/extproc/server/server.go`.
`Authorizer` lives in `internal/extproc/authorization/authorizer.go`.

- Inject `TokenExchanger`, `OPAAuthorizer`, and `approval.Gate` through these interfaces.
- Keep the typed interface fields on `Server`. Do not add an adapter layer or map-based indirection.
- Use mock or stub implementations for these interfaces in tests.
- Use `tests/e2e/extproc/bootstrap/` for startup and readiness coverage.

### Metadata-only exchange input

`adrs/036-extproc-metadata-token-exchange-input.md` governs this boundary.
Feature 043 supersedes older raw-header descriptions, including those in Feature 015.

- Read exchange inputs only from the flat `aib.tokenexchange` dynamic-metadata namespace.
- Use `extractTokenExchangeInput()` in both header paths.
- Validate `subject_token` before `resource_uri`, before protocol checks, transport checks, or OPA evaluation.
- Require string values for both fields.
- Reject blank subject tokens and case-insensitive `Bearer ` prefixes.
- Require a nonblank, absolute HTTP or HTTPS resource URI with a host.
- Pass accepted values to `Exchanger.Exchange` without modification.
- Return fixed 503 `invalid_subject_token` or `invalid_resource` responses for invalid metadata.
- Do not restore raw-header fallback, resource construction, dual-source precedence, or a configuration toggle.
- Do not expose subject credentials or rejected resource URIs in diagnostics.

Agentgateway must apply strict `jwtAuth` before ExtProc, with `preserveToken: false`.
The metadata producer can project the validated token through `jwt.rawToken.unredacted()`.
Headers remain inputs for OPA and transport checks, not token exchange.

### OPA processing

Read `adrs/028-opa-extproc-authorization.md` for the two policy backends and the default-off rationale.
Broker token exchange and CEL authorization remain active when OPA is disabled.

- If OPA is disabled, exchange during `RequestHeaders` without body buffering.
- If OPA is enabled, exchange body-bearing requests during `RequestHeaders`.
- Evaluate those bodies during `RequestBody`.
- For header-only requests, evaluate OPA before exchange.
- Keep GET as the only header-only MCP transport. Reject unsupported MCP methods with 405.
- Deny evaluation errors, timeouts, undefined results, and unsupported actions.

```text
Body-bearing requests:
RequestHeaders → validate subject metadata → validate resource metadata
               → protocol and transport checks → exchange
               → Authorization mutation + BUFFERED body
RequestBody    → decode once → InputBuilder.Build → OPA
               → allow: echo body
               → deny: 403 access_denied
               → approval_required: standalone tool-call gate

Header-only requests:
RequestHeaders → validate subject metadata → validate resource metadata
               → protocol and transport checks → header-only OPA input
               → allow: exchange + Authorization mutation
               → deny: 403 access_denied
```

`InputBuilder.Build` uses `ParseMCPMessage` for protocol `mcp`.
It produces `mcp_tool_call` for `tools/call`, or `mcp_method` for other methods.
Other protocols produce `type="unknown"` with the raw body in `input.attributes.request.http.body`.
Local policy paths use `rego.PreparedEvalQuery`. OPA configuration files use the bundle-aware SDK.

### Approval gate and cache

Read `adrs/036-extproc-approval-cache-and-gate.md` and the Feature 026 contracts before approval changes.

- Gate only standalone MCP tool calls after `approval_required`. Deny approval-required batches and `ciba_required`.
- Scope approval records with broker-provided `principal` and `agent_id`. Do not parse tokens for identity.
- Keep approvals outside OPA. Permission sets come from the exchange snapshot.
- Keep configured `context.agent_session_id` separate from fixed MCP `mcp.session_id`.
- Replace full snapshots on `200`. Refresh confirmation time on `304`.
- Keep one long-poll loop per instance, with active session IDs.
- Enforce `tool_approvals.max_staleness` and reject older targeted ETags.
- Require a successful authoritative read before approval creation after a cache miss.
- Deny failed or malformed reads. Do not treat them as confirmed misses.
- Reserve `once` approvals locally. Forward only after the consume endpoint returns `200`.
- Release reservations on consume failure.
- Preserve broker-supplied URLs, decision telemetry, and synchronization-age telemetry.

`specs/026-extproc-approval-sync/contracts/broker-approval-client.md` defines credentials, statuses, timestamps, and snapshot handling.
It excludes browser-only `GET /api/approvals/{id}` and cross-replica at-most-once guarantees.
`adrs/018-approval-endpoint-auth-boundaries.md` defines the endpoint-specific authentication.
The shared client in `internal/extproc/httpclient/httpclient.go` rejects redirects.

### Configuration and startup

`adrs/011-extproc-standalone-binary.md` authorizes the separate Viper/Cobra schema and `EXTPROC_` prefix.
Constitution Principle VII scopes configuration to its deployable application.

- Load configuration once with `LoadWithCommand()` in `cmd/extproc-token-exchange/root.go`.
- Pass the loaded `*Config` to consumers. Do not read configuration elsewhere.
- Preserve precedence: explicit CLI flags, environment variables, YAML, then defaults.
- Keep schema and startup validation in `internal/extproc/config/`.
- Deliver ExtProc configuration through its own deployment artifact, not the broker Helm ConfigMap or Deployment.

`NewTokenExchanger()` gets the initial client assertion synchronously and fails startup on error.
`httpclient.New()` reads and parses the CA bundle at construction.
Approval bootstrap differs: an unavailable approval endpoint leaves an empty cache and logs a warning.
Request-time approval checks still fail closed.
Read `internal/extproc/config/validate.go` for current validation rules rather than a fixed rule count.

### Token cache and assertion refresh

- Keep the struct cache key `{subjectToken, resourceURI}`, `sync.RWMutex`, and `singleflight`.
- Bound the TTL from `expires_in` or `cache.default_ttl` by `cache.max_ttl`.
- Keep token-bound permission sets and broker identity in the same cache window.
- Keep bounded stale-token fallback separate from approval freshness rules.
- Do not extend the fixed `staleUntil` deadline after transient failures.

`adrs/012-extproc-in-memory-token-cache.md` governs token caching.
The exchanger stores assertion state through `atomic.Pointer[assertionState]`.
Assertion refresh runs every `min(max(DefaultTTL/2, 1s), 30s)`.
Cache eviction runs every `max(DefaultTTL/2, 1s)`.
Refresh starts when remaining lifetime is less than `max(20% of total lifetime, 30s)`.
Empty or blank scopes request `openid`.
A refresh error retains the prior assertion only until expiry.
An absent or expired assertion returns `ErrAssertionExpired`, which signals 503.

### Body and trailer responses

Return the correct Envoy response type for each phase.
Keep `StreamedBodyResponse` for body pass-through.

## Testing Conventions (this subtree)

- **Framework**: Use stdlib `testing` with `testify`.
  Use `require` for preconditions and `assert` for checks.
- **Package**: Use the existing package convention. External-package tests cover public behavior.
  Same-package tests also cover approval internals and unexported helpers.
- **Mocking**: Use hand-written mocks for `Exchanger`, `Authorizer`, and `ApprovalGate`.
  Use `httptest.NewServer` for broker endpoints.
- **Configuration helpers**: Call `extprocconfig.LoadFromViper(v)` with a configured `*viper.Viper`.
  This prevents environment pollution.
- **No Ginkgo here**: Use standard `t.Run` or table-driven subtests.
  Use Ginkgo/Gomega only in `tests/e2e/extproc/`.
- **Race detector**: Do `go test -race` for ExtProc changes.
- **Commands**: Run `just extproc-test` for unit tests with the race detector.
  Run `just test-e2e-extproc` for the ExtProc Ginkgo suite.

---

## Configuration Reference (selected defaults)

Read `internal/extproc/config/config.go` and `internal/extproc/config/loader.go` for the complete schema and defaults.

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
| `tool_approvals.enabled` | `false` | Requires `authorization.enabled` when true |
| `tool_approvals.url` | `""` | Required when approval handling is enabled |
| `tool_approvals.long_poll_timeout_seconds` | `30` | Must be 1–120 when approval handling is enabled |
| `tool_approvals.approval_cache_idle_ttl` | `5m` | Must be positive when approval handling is enabled |
| `tool_approvals.request_timeout` | `5s` | Must be positive when approval handling is enabled |
| `tool_approvals.max_staleness` | `60s` | Freshness bound for approval-cache matches |
| `sessions.extraction.http_header` | `Mcp-Session-Id` | Must be a valid, non-empty HTTP field name |
| `telemetry.enabled` | `false` | |
| `telemetry.service_name` | `extproc-token-exchange` | |
| `telemetry.exporter.protocol` | `grpc` | |
| `telemetry.exporter.endpoint` | _(required when enabled)_ | |

Use `examples/config/extproc-token-exchange.yaml` for base token-exchange configuration.
Use `examples/config/extproc-opa-authorization.yaml` for OPA.
Use `examples/config/extproc-telemetry.yaml` for OpenTelemetry.
Use `examples/config/extproc-tool-approvals.yaml` for approval gating.

## Docker Compose

`docker-compose.yml` mounts `configs/config.extproc.docker.yaml` at the container path `/app/config.yaml`.
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
| Baseline tasks | `specs/015-extproc-token-exchange/tasks.md` |
| Approval specification, design, and tasks | `specs/026-extproc-approval-sync/spec.md`, `specs/026-extproc-approval-sync/plan.md`, `specs/026-extproc-approval-sync/tasks.md` |
| Approval HTTP and OPA contracts | `specs/026-extproc-approval-sync/contracts/broker-approval-client.md`, `specs/026-extproc-approval-sync/contracts/opa-input-approval.md` |
| Approval configuration and API additions | `specs/026-extproc-approval-sync/contracts/extproc-approval-config.yaml`, `specs/026-extproc-approval-sync/contracts/broker-api-changes.yaml` |
| Metadata specification, design, and tasks | `specs/043-extproc-metadata-input/spec.md`, `specs/043-extproc-metadata-input/plan.md`, `specs/043-extproc-metadata-input/tasks.md` |
| Metadata wire contract | `specs/043-extproc-metadata-input/contracts/extproc-metadata-input.md` |
| Broker OpenAPI | `api/enduser/openapi.yaml` |
| Operator guides | `docs/guides/token-exchange-gateway.md`, `docs/configuration.md` |

Use the current approval contracts for snapshot semantics.
Older Feature 026 narrative still describes deltas and the former `internal/toolpattern` location.
Accepted ADR 035 and ADR 036, the contracts, and current source define the current behavior.
