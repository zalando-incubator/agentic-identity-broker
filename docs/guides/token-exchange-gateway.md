---
title: "Set up token exchange at the gateway"
description: Deploy the ExtProc token-exchange sidecar with Agentgateway. Agentgateway validates a JWT and sends trusted metadata for RFC 8693 exchange.
---

# Set up token exchange at the gateway

The ExtProc token-exchange service (`extproc-token-exchange`) is a standalone gRPC sidecar.
Agentgateway validates the inbound JWT and publishes trusted token-exchange inputs as Envoy dynamic metadata.
The sidecar performs [RFC 8693 token exchange](/docs/concepts/token-exchange) with those inputs.

The sidecar accepts only `aib.tokenexchange.subject_token` and `aib.tokenexchange.resource_uri`.
It does not read `Authorization`, request pseudo-headers, or other raw HTTP attributes as token-exchange inputs.
After a successful exchange, it replaces the downstream `Authorization` header with the exchanged credential.

This guide describes the separate Agentgateway policy and ExtProc sidecar configuration.

## What you need

- An Agentgateway v1.5.0 instance with the ExtProc policy. Configure it with the `jwtAuth` syntax shown in this guide.
- A trusted JWT issuer and a JWKS source for the Agentgateway JWT provider.
- A reachable broker token endpoint at `POST /oauth2/token` on end-user port 8000.
- OAuth2 client credentials for the ExtProc sidecar. The sidecar uses them to obtain a client assertion for the broker.
- The sidecar container image (`build/docker/Dockerfile.extproc`). The sidecar stores state only in an in-memory cache.
- A broker CEL policy if you restrict gateway exchanges. You can also add an OPA policy for the proxied request.

## How the exchange flows

The Agentgateway policy runs before the ExtProc sidecar receives the request:

```mermaid
sequenceDiagram
    participant A as AI agent
    participant G as Agentgateway
    participant X as ExtProc sidecar
    participant B as Broker
    participant T as Third-party service
    A->>G: Request with bearer JWT
    G->>G: jwtAuth validates JWT and removes header
    G->>X: ExtProc metadata: subject_token, resource_uri
    X->>B: RFC 8693 exchange
    B-->>X: Third-party access token
    X->>G: Replace Authorization header
    G->>T: Request with third-party token
    T-->>G: Response
    G-->>A: Response
```

1. Agentgateway applies `jwtAuth` with `mode: strict`. This policy validates the JWT before the ExtProc policy runs.
2. The ExtProc policy evaluates `jwt.rawToken.unredacted()` and its resource expression. It publishes both results in `aib.tokenexchange` metadata.
3. The sidecar validates both metadata fields. It rejects absent or invalid values and never derives inputs from raw HTTP attributes.
4. The sidecar exchanges the metadata values through the broker `/oauth2/token` endpoint. The broker returns the stored third-party token.
5. The sidecar replaces `Authorization` with `Bearer <exchanged token>`. Agentgateway forwards the request.

### Configure the Agentgateway producer

This block is Agentgateway policy configuration. It is not ExtProc sidecar configuration.
Keep `jwtAuth` before `extProc`. The latter expression needs the validated JWT claims from the former policy.

```yaml
binds:
  - port: 4000
    listeners:
      - routes:
          - policies:
              jwtAuth:
                mode: strict
                preserveToken: false
                providers:
                  - issuer: "https://issuer.example.com"
                    audiences:
                      - "mcp-server"
                    jwks:
                      url: "https://issuer.example.com/.well-known/jwks.json"
              extProc:
                host: "extproc-token-exchange:50051"
                failureMode: failClosed
                metadataContext:
                  aib.tokenexchange:
                    subject_token: "jwt.rawToken.unredacted()"
                    resource_uri: "'https://mcp.example.com/mcp'"
            backends:
              - mcp:
                  targets:
                    - name: tools
                      mcp:
                        host: "https://mcp.example.com/mcp"
```

`preserveToken: false` removes the validated request header before ExtProc runs. Agentgateway retains a claims copy for the following metadata expression.
The claims copy stays inside Agentgateway. Only the fields produced by `metadataContext` cross the gRPC boundary.

Use `jwt.rawToken.unredacted()` exactly. `jwt.rawToken` without `.unredacted()` is redacted and does not produce the raw subject credential.
The `subject_token` result must be the raw bearer credential with no `Bearer ` prefix. The sidecar passes an accepted value to the broker unchanged.

`aib.tokenexchange` is one literal metadata namespace, not a nested configuration path. Both fields must be strings.
Set `resource_uri` to a CEL string literal. It must resolve to an absolute `http` or `https` URL with a non-empty host.

The configured `resource_uri` must exactly match the protected resource that is registered with the broker.

### Incremental authorization metadata (approved design)

Feature 055's implementation is pending. Its approved additive challenge contract allows
Agentgateway to supply optional `resource_metadata_url` beside `resource_uri` in the literal
`aib.tokenexchange` namespace. The producer is the Agentgateway route, not the broker or the
ExtProc process configuration. See the
[complete route example](../../examples/config/incremental-authorization-agentgateway.yaml),
[broker overlay](../../examples/config/incremental-authorization.yaml), and
[configuration validation rules](../configuration.md#incremental-service-authorization-approved-design).

Omit that field for the default variant; no metadata URL is inferred. An absent/invalid value
omits only the optional `resource_metadata` challenge attribute. The approved challenge also
carries the broker's service `scope` and preserves `error_uri` and URL elicitation. The running
baseline behavior described below remains unchanged until implementation completes.

ADRs 011 and 036 keep the producer/consumer deployment boundary explicit. The
`charts/agentic-identity-broker/` chart does not deploy Agentgateway or the standalone ExtProc
sidecar and must not receive this route metadata setting. Keep strict JWT authentication,
`preserveToken: false` and `failureMode: failClosed`; optional metadata is not an authentication
source and must never be sourced from an HTTP request header.

### Fail-closed behavior

The sidecar rejects the request if either required metadata field is absent, empty, non-string, or invalid.
It validates `subject_token` before `resource_uri`. If both are invalid, it returns `invalid_subject_token`.
The response is a 503 JSON immediate response. Diagnostics never contain the subject credential or rejected resource URI.

If exchange fails, the sidecar does not forward the original credential. Generic errors, broker 4xx responses without `error_uri` (including 429), 5xx errors, network errors, and timeouts return HTTP 500. The body is `{"error":"token_exchange_failed","error_description":"token exchange request failed"}`.

If a non-transient broker error contains `error_uri`, the sidecar returns HTTP 403 for every protocol. It forwards the broker URI unchanged in the RFC 6750 challenge and JSON-RPC `-32042` URL elicitation body (`id: null`). For provider-session recovery, the broker URI starts that service's third-party OAuth2 authorization with PKCE and returns to `/sessions` after the provider callback.

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
WWW-Authenticate: Bearer error="insufficient_scope", error_description="User session is unavailable. Please re-authenticate.", error_uri="https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"

{"jsonrpc":"2.0","id":null,"error":{"code":-32042,"message":"User session is unavailable. Please re-authenticate.","data":{"elicitations":[{"mode":"url","elicitationId":"<fresh uuid v4>","url":"https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions","message":"User session is unavailable. Please re-authenticate."}]}}}
```

The challenge always has `error="insufficient_scope"`. It adds `error_description` and `error_uri` only when each value uses its RFC 6750 §3 character set. It does not provide `scope`, `resource_metadata`, or `realm`. Generic failures have no `WWW-Authenticate` header. Expired assertions and open circuits also have no challenge and keep their 503 responses.

An mcp-go client can read the URL from a tool-call error after initialization, but not from an `initialize` error.

For automatic step-up, an MCP client must discover RFC 9728 protected-resource metadata for the gateway's MCP URL. This `jwtAuth` route does not provide metadata by itself. Add a separate gateway route for `/.well-known/oauth-protected-resource/mcp`. It must return the MCP resource URL and its authorization server. Match the MCP route on `/mcp`, so route order does not affect dispatch. [Feature 023's example journey](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/023-extproc-mcp-elicitation/spec.md) shows the route and authorization-code retry.

The Compose gateway includes this public metadata route. It advertises `http://localhost:4000/mcp` as the resource and `http://localhost:3000` as the authorization server. Vite proxies the broker's authorization-server metadata and OAuth2 endpoints on port 3000. The `/mcp` route still applies strict JWT validation and ExtProc exchange. Compose also exposes `WWW-Authenticate` to browser clients through CORS.

The existing JWT 401 response has no `resource_metadata` link. Clients must try the well-known URL when that link is absent.

The broker does not advertise a dynamic client registration endpoint in its Compose metadata. A client needs a broker-registered identity and an allowed redirect URI, or a supported Client ID Metadata Document. URL-only discovery does not register an arbitrary agent.

### Diagnostic telemetry

Exchange logs, spans, and metrics use the bounded `token_exchange.*` classification contract in the [token-exchange reference](../reference/token-exchange.md#diagnostic-attributes).
ExtProc records the operation stage that it observes. It does not guess an unreported broker stage or exchange profile.
Its exchange kind is `unknown` when the broker profile is not observable.
Successful metric observations use `none` for failure stage and detail. Successful logs and spans omit these failure-only fields.

Telemetry excludes resource paths, endpoint URLs, broker descriptions, nested errors, credentials, request headers and bodies, and process command arguments.
Outbound spans preserve trace propagation and the real network request without recording these values.
OPA audit data uses bounded actions, result codes, protocols, and allowlisted MCP methods. It also includes policy reasons, tool names, and target server names.
Direct protocol recovery responses still contain the recovery URI. Recovery telemetry values do not instruct ExtProc to retry automatically.


## Configure the sidecar

This is ExtProc sidecar configuration. It does not configure Agentgateway `jwtAuth`, JWKS, `preserveToken`, or `metadataContext`.
The sidecar environment prefix is `EXTPROC_`. Each key maps to `EXTPROC_<SECTION>_<KEY>`.
For example, `grpc.port` maps to `EXTPROC_GRPC_PORT`. String values support `${VAR}` substitution for secret injection.

```yaml
grpc:
  bind: "0.0.0.0"
  port: 50051
  max_concurrent_streams: 100

oauth2:
  # The broker's RFC 8693 token endpoint (required)
  token_endpoint: "https://identity-broker.example.com/oauth2/token"
  # The upstream OAuth2 issuer the sidecar authenticates against (required)
  issuer: "https://upstream-oauth2.example.com"
  # Client credentials for the sidecar itself
  client_id: "extproc-gateway"
  client_secret: "${EXTPROC_OAUTH2_CLIENT_SECRET}"
  # Explicit client_credentials endpoint; defaults to {issuer}/oauth/token
  # client_credentials_endpoint: "https://upstream-oauth2.example.com/oauth/token"
  client_assertion_type: "id_token"
  exchange_timeout: "5s"
  tls:
    # Leave false in production; the endpoints must be https unless this is true
    allow_http: false

cache:
  default_ttl: "5m"   # TTL for a cached token when the response omits expires_in
  max_ttl: "1h"       # hard cap on how long any exchanged token is cached

log:
  level: "info"
  format: "json"
```

The Agentgateway `extProc.host` value points to the sidecar `grpc.bind:grpc.port` listener. The default listener is `0.0.0.0:50051`.
Key sidecar settings follow:

- **`oauth2.token_endpoint`** — The broker `/oauth2/token` endpoint. The sidecar sends each exchange to this endpoint.
- **`oauth2.issuer`**, **`client_id`**, and **`client_secret`** — The sidecar obtains a client assertion through the upstream client-credentials grant. It refreshes the assertion in the background.
- **`oauth2.tls.allow_http`** — Keep this value `false`. The token endpoint and issuer use `https` unless you explicitly allow HTTP for local development.
- **`cache.default_ttl` / `cache.max_ttl`** — The sidecar uses `default_ttl` only for absent or null `expires_in`. Explicit zero or negative lifetime is not cacheable. Positive lifetime uses a fresh-cache margin of `max(10s, 10% of lifetime)`, limited to half the lifetime. `max_ttl` caps all cache use. During transient outages, a previously cached token can remain available until reported lifetime plus five seconds, within that cap. This fallback can serve past reported expiry; the fresh-cache margin does not guarantee validity in transit.

Inject the client secret through the environment. Do not commit it. The sidecar redacts sensitive values from logs.

## The two-gate model

Two independent policy gates can guard a request. Both gates must allow the request:

| Gate | Where | Question it answers |
|---|---|---|
| Broker CEL | Broker `POST /oauth2/token` | Can this gateway exchange a token for this resource? |
| ExtProc OPA | Sidecar request path | Can this proxied request or MCP tool call continue? |

The broker CEL policy is configured on the broker. It decides whether exchange is allowed.
The optional sidecar OPA policy can restrict the proxied request after exchange. It cannot
give access that the broker denied. See
[token exchange](/docs/concepts/token-exchange) for the broker CEL policy.

### Optional OPA gate

To evaluate a proxied request, including MCP tool calls, with a Rego policy, add an
`authorization` block. The sidecar buffers and evaluates requests that contain a body. An
undefined result denies the request.

```yaml
authorization:
  enabled: true
  policy:
    # A local Rego file or directory...
    path: "/etc/extproc/policy.rego"
    # ...or an OPA config file for bundles/discovery (mutually exclusive with path)
    # config_file: "/etc/extproc/opa-config.yaml"
    package: "aib.extproc.authz"
    decision: "result"
  default_decision: "deny"   # keep as deny for fail-closed behavior
  evaluation_timeout: 100ms
  max_body_size: 1048576     # bytes buffered for evaluation (1 MiB)
```

:::warning
Keep `default_decision: "deny"`. If a policy result is undefined or evaluation times out,
the OPA gate denies the request.
:::
### Enable approval gating

Set `tool_approvals.enabled: true` only with OPA authorization enabled. When OPA returns `approval_required` for a standalone MCP `tools/call`, ExtProc checks its local approval cache, refreshes the caller's broker-authoritative `(principal, agent_id)` scope on a miss, and creates a pending approval only after a successful confirmed miss. It returns the broker-provided URL as MCP error `-32042`. The original subject token is used only for create and one-time consume; ExtProc never parses either token for cache identity.

```yaml
tool_approvals:
  enabled: true
  url: "https://identity-broker.example.com"
  long_poll_timeout_seconds: 30
  approval_cache_idle_ttl: 5m
  request_timeout: 5s
sessions:
  extraction:
    http_header: "Mcp-Session-Id"
```

The cache is warmed asynchronously at startup and then refreshed by one ETag long-poll loop. A failed refresh, malformed response, missing approval identity, or failed one-time consume denies the request. `ciba_required`, batches, and header-only requests never enter the approval elicitation flow.
## Use a trusted Compose JWT

The Compose gateway trusts upstream JWTs from `http://upstream-oauth2:9001` and broker-issued local JWTs from `http://localhost:3000`.

An upstream JWT must contain `token-exchange-broker` in `aud` and identify a registered agent in `azp`. A local JWT must contain the same audience and its broker-issued `agent_id` claim.

A broker-issued CIMD token keeps its metadata URL in `agent_id`. Compose also signs the registered agent UUID as `azp`. The issuer-gated exchange policy uses that UUID only for broker-issued tokens. When upstream tokens do not carry an `agent_id`, token exchange resolves their `azp` through the client ID registry. Tokens issued before this policy change may lack `azp` and require a new authorization flow.

Compose uses in-memory broker storage. A restart discards grants, provider sessions, signing keys, and manually registered agents. The startup script re-registers Visual Studio Code and Claude Code with new agent UUIDs. Obtain a new token, approve delegation, and reconnect the provider after a restart.

### Use a sample-client token

1. Open `http://localhost:9002/`.
2. Select **Login as Local Client** or **Login as Proxy Client**.
3. If you select the local client, connect the listed third-party service.
4. Approve the requested delegation.
5. Select **Call MCP Tool: whoami (allowed)**.

The sample client sends its issued JWT to Agentgateway. It does not expose the credential in a command argument or output.

Before you call the gateway, create an active user grant and a third-party OAuth2 session for the JWT subject. The broker rejects a token without either record.

Give the JWT to a local MCP client as its bearer token. Do not print the credential or put it in command arguments or command history.



## Deployment notes

- **No database.** The sidecar stores only an in-memory cache. Start one sidecar instance
  for each gateway pod. Each instance has its own cache.
- **Container image.** To build a release image, use `just docker-build-extproc`. The command
  builds Linux artifacts for amd64 and arm64. Then it packages the default Dockerfile
  target. To build a source-based development image, run:

  ```bash
  docker build --target development --file build/docker/Dockerfile.extproc .
  ```

  Configure the sidecar with a YAML file or `EXTPROC_` environment variables.
- **Placement.** Start the sidecar with the gateway. Token exchange then occurs at the edge,
  before the request reaches the third-party service. See
  [architecture](/docs/concepts/architecture) for the sidecar location.

## Related

- [Token exchange](/docs/concepts/token-exchange) — the concept and the broker-side flow.
- [Token exchange reference](/docs/reference/token-exchange) — the RFC 8693 request and
  response contract.
- [Architecture](/docs/concepts/architecture) — the sidecar's place in the system.
