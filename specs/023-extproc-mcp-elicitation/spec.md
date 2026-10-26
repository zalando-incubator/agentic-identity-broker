# Feature Specification: ExtProc MCP URL Elicitation for Session Errors

**Feature Branch**: `023-extproc-mcp-elicitation`
**Created**: 2026-03-26
**Status**: Implemented
**Amended**: 2026-10-26 — User Story 1 Amendment (HTTP 403 re-authentication challenge) and User Story 3 (step-up recovery)

## Overview

When an AI agent's delegated session has expired, the ExtProc token exchange service must signal that the user must re-authenticate — not silently fail. This feature closes two gaps:

1. The identity broker includes a service-specific third-party OAuth2 authorization URL (`error_uri`) in its RFC 6749 error response for recoverable session failures.
2. ExtProc translates that `error_uri` into a JSON-RPC `-32042` `URLElicitationRequiredError` returned directly to the calling agent, following the MCP 2025-11-05 specification.

As a prerequisite improvement, JWT validation error messages are also made more informative for operators by including expected `iss`/`aud` values.

---

## User Scenarios & Testing

### User Story 1 — Agent Receives Actionable Re-Auth Signal (Priority: P1)

An AI agent makes an MCP tool call via agentgateway. The user's delegated OAuth2 session to the third-party service has expired. Instead of receiving a generic failure, the agent receives a structured error containing the URL the user must visit to re-authorise.

**Why this priority**: Without this, expired sessions silently block agents with no recovery path. The MCP `URLElicitationRequiredError` is the standard mechanism for agents to request user action.

**Independent Test**: Can be tested end-to-end by triggering a token exchange against a mock broker that returns `error_uri`, and verifying the mcp-go client returns `*mcp.URLElicitationRequiredError` with a non-empty `elicitations[0].url`.

**Acceptance Scenarios**:

1. **Given** an agent sends an MCP request through agentgateway with a valid Bearer token, **when** the identity broker returns `{"error":"invalid_grant","error_uri":"https://..."}` from the token exchange endpoint, **then** the agent receives a JSON-RPC error with code `-32042`, `data.elicitations[0].mode = "url"`, and `data.elicitations[0].url` equal to the broker's `error_uri`.

2. **Given** an agent sends an MCP request through agentgateway, **when** the broker returns a token exchange error *without* `error_uri`, **then** ExtProc returns a standard `500 Internal Server Error` (no elicitation).

3. **Given** an agent sends an MCP request through agentgateway, **when** ExtProc returns a JSON-RPC `-32042` elicitation response, **then** the HTTP status code of the agentgateway response is `200 OK` (per JSON-RPC 2.0 — errors travel over HTTP 200).

### User Story 1 Amendment - HTTP 403 Re-authentication Challenge (2026-10-26)

This amendment changes only the ExtProc HTTP status (Scenario 3 and FR-002's former HTTP 200) and Feature 043's former non-MCP 503. The JSON-RPC body (FR-002–FR-004) and headers-phase delivery (FR-006) remain authoritative. Feature 013's 2026-10-26 User Story 5 Amendment defines the broker `error_uri` target. The stakeholder confirmed both changes on 2026-10-26.

For a non-transient broker exchange error with `error_uri`, ExtProc returns HTTP 403 for every protocol and processing mode. It sets `Content-Type: application/json` and exactly one `www-authenticate` mutation. The challenge attributes occur in this order: `error`, `error_description`, `error_uri`. The `error` value is `insufficient_scope`. ExtProc includes a nonempty description only when every byte matches RFC 6750 §3 `%x20-21 / %x23-5B / %x5D-7E`. It includes the URI only when every byte matches `%x21 / %x23-5B / %x5D-7E`. Invalid attributes are omitted, not escaped. The challenge never includes `scope`, `resource_metadata`, or `realm`. The body remains the existing URL-elicitation JSON-RPC error with `id: null` and a fresh UUID v4 `elicitationId`. ExtProc does not mutate `Authorization` on failure.

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
WWW-Authenticate: Bearer error="insufficient_scope", error_description="User session is unavailable. Please re-authenticate.", error_uri="https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"

{"jsonrpc":"2.0","id":null,"error":{"code":-32042,"message":"User session is unavailable. Please re-authenticate.","data":{"elicitations":[{"mode":"url","elicitationId":"<fresh uuid v4>","url":"https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions","message":"User session is unavailable. Please re-authenticate."}]}}}
```

**Acceptance Scenarios**:

1. **Scenario 1: Challenge through agentgateway.** Given an agent sends an MCP request through agentgateway with a valid Bearer token, When the broker rejects the exchange with `400 invalid_grant` and `error_uri`, Then the response is HTTP 403 with `WWW-Authenticate: Bearer error="insufficient_scope", error_description="<broker description>", error_uri="<error_uri>"`. Its JSON-RPC body has `id` null, code `-32042`, and exactly one `url` elicitation equal to `error_uri`.
2. **Scenario 2: mcp-go after initialize.** Given an mcp-go client initialized through agentgateway, When a later `tools/call` hits the same rejection, Then `CallTool` returns `mcp.URLElicitationRequiredError` whose only elicitation URL is `error_uri`.
3. **Scenario 3: OPA mode.** Given ExtProc runs with OPA authorization enabled, When a body-bearing MCP request's exchange is rejected with `error_uri`, Then the same 403 challenge is returned from the request-headers phase.
4. **Scenario 4: Non-MCP protocol.** Given agentgateway protocol metadata `a2a`, When the exchange is rejected with `error_uri`, Then ExtProc returns the same 403 challenge instead of 503 `token exchange requires re-authentication`.

**Edge Cases**:

- Transient 5xx, 429 without `error_uri`, no `error_uri`, expired assertion, and open circuit keep their 500/503 responses without `WWW-Authenticate`.
- A description or URI outside its RFC 6750 set omits that header attribute. The body keeps the value.
- Repeats within the 5 s re-authentication cooldown return the same challenge.
- A header-only `GET` gets the same 403.

---

### User Story 2 — Operator Diagnoses JWT Validation Failures (Priority: P2)

An operator troubleshooting a token exchange failure wants to know precisely *why* a JWT was rejected — not just that validation failed. Error messages must include the expected issuer and audience values so the operator can identify misconfiguration without digging into source code.

**Why this priority**: Operational visibility; does not block agent functionality but significantly reduces mean time to resolution.

**Independent Test**: Triggering a token exchange with a JWT signed for the wrong issuer yields an error description containing the expected issuer value (e.g., `expected iss="https://auth.example.com"`).

**Acceptance Scenarios**:

1. **Given** a JWT with an incorrect `iss` claim, **when** the token exchange service validates it, **then** the error description includes `expected iss=` followed by the configured issuer value.

2. **Given** a JWT with an incorrect `aud` claim, **when** the token exchange service validates it, **then** the error description includes `expected aud=` followed by the configured audience value.

3. **Given** a JWT that has expired, **when** validation runs, **then** the error description states the token has expired (without leaking raw token content).

### User Story 3 - Step-up-capable MCP Client Re-authorizes and Retries (Priority: P2)

A step-up-capable MCP client uses the gateway's protected-resource metadata to re-authorize after the re-authentication challenge and retries its tool call.

**Why this priority**: MCP 2025-11-25 step-up authorization enables recovery without a manual retry. This also lays groundwork for incremental authorization.

**Independent Test**: A go-sdk client calls an MCP tool through agentgateway, completes an authorization-code exchange against an advertised authorization server, and retries successfully.

**Acceptance Scenarios**:

1. **Scenario 1: Step-up authorization and retry.** Given a go-sdk client with an authorization-code OAuth handler holding a valid token, and a gateway serving RFC 9728 metadata at `/.well-known/oauth-protected-resource/mcp` that names the authorization server, When a `tools/call` receives the 403 challenge, Then the client makes exactly one authorization request at that server for resource `<gateway>/mcp` and obtains a new token. The retried call succeeds, and the broker exchanges the new token.

---

### Edge Cases

- **Elicitation from headers phase only**: agentgateway 0.12.0 commits the upstream request to the backend immediately upon receiving any `RequestHeaders` response. `ImmediateResponse` can therefore only be returned from the headers phase — a body-phase `ImmediateResponse` is never reached. The JSON-RPC `id` is set to `null` (per JSON-RPC 2.0 §5) because the request body is unavailable at headers phase.
- **Broker error without `error_uri`**: broker errors that lack `error_uri` (e.g., `access_denied`) must not produce an elicitation — they fall through to the existing 500/503 path.
- **"Got" claim value not exposed**: jwx error types do not expose the actual claim value (only the expected value). Client-facing messages include only the expected value; the full error detail (containing the actual value) is available in operator logs via `WithCause`.

---

## Concrete Example Journeys

**Cast**: Alice (`alice@example.com`) uses the Research Assistant (agent ID `a1b2c3d4-e5f6-7890-abcd-ef1234567890`) at `https://gw.example.com/mcp`. The broker is `https://broker.example.com`. The resolved third-party service is `550e8400-e29b-41d4-a716-446655440000`. Its recovery URL is `https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions`.

### Journey 1 — A provider session expires mid-session (User Story 1 Amendment, Scenarios 1–3)

**Configuration**: ExtProc has `authorization.enabled: true`. The response is identical when this value is `false`.

**Step 1 — 10:00:00Z: Research Assistant calls a tool**

```http
POST /mcp HTTP/1.1
Host: gw.example.com
Authorization: Bearer <agent-token>
Content-Type: application/json

{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"whoami","arguments":{}}}
```

**Step 2 — 10:00:01Z: ExtProc exchanges the token and receives a broker error**

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded

grant_type=urn:ietf:params:oauth:grant-type:token-exchange&subject_token=<agent-token>&resource=https%3A%2F%2Fgw.example.com%2Fmcp&...

HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"invalid_grant","error_description":"User session is unavailable. Please re-authenticate.","error_uri":"https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"}
```

The broker response matches `OAuth2Error` in `api/enduser/openapi.yaml`.

**Step 3 — 10:00:01Z: The gateway sends the 403 response to Research Assistant**

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
WWW-Authenticate: Bearer error="insufficient_scope", error_description="User session is unavailable. Please re-authenticate.", error_uri="https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"

{"jsonrpc":"2.0","id":null,"error":{"code":-32042,"message":"User session is unavailable. Please re-authenticate.","data":{"elicitations":[{"mode":"url","elicitationId":"<fresh uuid v4>","url":"https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions","message":"User session is unavailable. Please re-authenticate."}]}}}
```

**Step 4 — 10:02:00Z: Alice reconnects the service**

After initialize, mcp-go returns `URLElicitationRequiredError` for the failed tool call. Alice opens the service-specific `error_uri`. The broker starts provider OAuth with PKCE. The provider callback returns Alice to `/sessions`, and the next tool call succeeds.

### Journey 2 — A step-up-capable client re-authorizes and retries (User Story 3, Scenario 1)

**Step 1 — 11:00:00Z: The client receives a challenge**

The gateway returns the 403 from Journey 1, step 3, for a tool call made with the initial token.

This client starts a separate broker authorization-code step-up through `/oauth2/authorize`. A user who follows the broker-provided `error_uri` instead starts OAuth for the resolved third-party service; that link never targets broker `/oauth2/authorize`.

**Step 2 — 11:00:01Z: The client discovers protected-resource metadata**

```http
GET /.well-known/oauth-protected-resource/mcp HTTP/1.1
Host: gw.example.com

HTTP/1.1 200 OK
Content-Type: application/json

{"resource":"https://gw.example.com/mcp","authorization_servers":["https://broker.example.com"]}
```

The gateway needs a route for this metadata. Without the route, clients cannot discover the authorization server:

```yaml
- name: protected-resource-metadata
  matches: [{path: {exact: /.well-known/oauth-protected-resource/mcp}}]
  backends: [{host: prm.example.internal:8080}]
```

**Step 3 — 11:00:02Z: The client discovers authorization-server metadata**

```http
GET /.well-known/oauth-authorization-server HTTP/1.1
Host: broker.example.com
```

**Step 4 — 11:00:03Z: The browser visits the authorization endpoint**

```http
GET /oauth2/authorize?response_type=code&client_id=a1b2c3d4-e5f6-7890-abcd-ef1234567890&redirect_uri=…&state=…&code_challenge=…&code_challenge_method=S256&… HTTP/1.1
Host: broker.example.com
```

The trailing `…` represents client-added parameters, such as RFC 8707 `resource`. The documented `/oauth2/authorize` parameters omit it, and `internal/adapters/http/enduser/oauth2_authorize.go` does not read it. The broker finds Alice's expired delegated session and redirects to consent (`internal/domain/oauth2/service.go`). After consent, the broker resumes authorization and issues a code. The consent flow retains the original authorization context in a session token, as described in `specs/031-unified-session-token/spec.md`.

**Step 5 — 11:01:00Z: The client exchanges the code for a new token**

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&code=<code>&redirect_uri=<registered-redirect-uri>&code_verifier=<pkce-verifier>

HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"<synthetic-signed-jwt>","token_type":"Bearer","expires_in":600}
```

The synthetic decoded token illustrates the claims minted by `internal/domain/oauth2server/strategies.go`:

```json
{"iss":"https://broker.example.com","sub":"alice@example.com","iat":1793012460,"exp":1793013060,"jti":"<uuid>","agent_id":"a1b2c3d4-e5f6-7890-abcd-ef1234567890","scope":""}
```

**Step 6 — 11:01:01Z: The client retries the tool call**

```http
POST /mcp HTTP/1.1
Host: gw.example.com
Authorization: Bearer <synthetic-signed-jwt>
Content-Type: application/json

{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"whoami","arguments":{}}}

HTTP/1.1 200 OK
Content-Type: application/json

{"jsonrpc":"2.0","id":8,"result":{"content":[{"type":"text","text":"auth_scheme=Bearer token_digest=<exchanged-token-digest>"}]}}
```

ExtProc exchanges the new token before the upstream MCP server handles the retried call.

### Journey 3 — An A2A client gets the same challenge (User Story 1 Amendment, Scenario 4)

**Step 1 — 12:00:00Z: An A2A request reaches ExtProc**

The gateway supplies protocol metadata `a2a` and valid token-exchange metadata. The broker rejects the exchange with `error_uri`.

**Step 2 — 12:00:01Z: The gateway returns the re-authentication challenge**

The former `503 {"error":"service_unavailable","error_description":"token exchange requires re-authentication"}` becomes the identical 403 response shown in Journey 1, step 3.

User Story 2 (JWT diagnostics) is unchanged by the 2026-10-26 amendment.

---

## Requirements

### Functional Requirements

- **FR-001**: For a recoverable session failure, the broker token exchange endpoint MUST set `error_uri` to `<public URL>/api/third-party/{serviceId}/oauth2/authorize?redirect_uri=<url.QueryEscape(<public URL>/sessions)>`. It MUST use the configured public URL and resolved service UUID. This supersedes the earlier direct sessions-page target (Feature 013 User Story 5 Amendment, 2026-10-26).

- **FR-002**: ExtProc MUST parse the `error_uri` field from non-2xx token exchange responses. When present on a non-transient error, it MUST return HTTP 403 with a `WWW-Authenticate: Bearer error="insufficient_scope"` challenge and a JSON-RPC `-32042` error body. The body contains an `elicitations` array with one entry of `mode="url"` and `url` set to the `error_uri` value (amended 2026-10-26; formerly HTTP 200).

- **FR-003**: The JSON-RPC `-32042` elicitation response MUST include a unique `elicitationId` (UUID v4) per response to allow clients to correlate elicitation flows.

- **FR-004**: The JSON-RPC `id` field in the elicitation response MUST be `null` when ExtProc cannot determine the request id (i.e., when responding from the headers phase before the body is available).

- **FR-005**: JWT validation error descriptions MUST include the expected `iss` or `aud` value when the corresponding claim does not match, using typed error matching rather than string inspection.

- **FR-006**: ExtProc MUST return `ImmediateResponse` from the **request headers phase** (not body phase) when an elicitation is triggered. This is required for compatibility with agentgateway 0.12.0, which forwards the request to the backend immediately after processing a headers-phase response.

- **FR-007**: When `error_uri` is absent from a broker error response, ExtProc MUST fall through to existing error handling (500 for unclassified errors, 503 for assertion-expired errors). These responses carry no `WWW-Authenticate` header.

- **FR-008**: ExtProc MUST return the FR-002 response for every protocol value (absent, `mcp`, `a2a`, or any other) and in every processing mode (OPA disabled, OPA body-bearing, OPA header-only).

- **FR-009**: The challenge MUST contain only the `error`, `error_description`, and `error_uri` attributes, in that order. It carries no `scope`, `resource_metadata`, or `realm` until a source for them exists.

### Security Requirements

- **SR-001**: The `error_uri` value MUST be sourced from the broker response and must not be constructed from user-supplied input.
- **SR-002**: JWT validation MUST use typed error matching against the JWT library's error hierarchy — `strings.Contains` on error messages is prohibited as a validation mechanism.
- **SR-003**: Token values (subject tokens, access tokens) MUST NOT appear in client-facing error messages.
- **SR-004**: The broker MUST construct the `error_uri` authorization URL from its configured `PublicURL` and the resolved service UUID. It MUST encode the fixed same-origin `/sessions` return URI, not use a caller-supplied URL.
- **SR-005**: Each challenge attribute value MUST consist only of its RFC 6750 §3 characters (`error_description`: %x20-21 / %x23-5B / %x5D-7E; `error_uri`: %x21 / %x23-5B / %x5D-7E). A value outside its set omits the attribute; it is never escaped.

---

## Success Criteria

### Measurable Outcomes

- **SC-001**: When the broker returns `error_uri`, an agent client receives HTTP 403 with the `insufficient_scope` challenge and the `-32042` body. An mcp-go client gets `URLElicitationRequiredError` with a non-empty URL on a post-initialize call. Both are verified end-to-end through a real agentgateway container.
- **SC-002**: JWT issuer mismatch errors include the expected issuer value in the error description, verifiable in unit tests without running a full server.
- **SC-003**: Broker errors without `error_uri` continue to produce 500 responses (no regression in existing error handling).
- **SC-004**: All unit and E2E tests pass with the race detector enabled (`go test -race`).
- **SC-005**: A go-sdk client with an authorization-code OAuth handler completes exactly one step-up authorization, and its retried call succeeds. This is verified end-to-end through a real agentgateway container.

---

## Assumptions

- agentgateway version is 0.12.0. Future versions may process body-phase `ImmediateResponse` correctly; this spec is valid for 0.12.0 behaviour.
- The MCP `URLElicitationRequiredError` (-32042) is as defined in the MCP specification 2025-11-05.
- The broker's configured `ServerInstanceConfig.PublicURL` supplies both the authorization URL origin and the fixed `/sessions` return origin. The service-specific GET starts provider OAuth with PKCE; the callback returns to `/sessions`. No new configuration field is needed.
- RFC 6749 §5.2 `error_uri` is an optional extension field; existing clients that do not inspect it are unaffected.
- (2026-10-26) The E2E gateway is agentgateway v1.5.0 (`cr.agentgateway.dev/agentgateway:v1.5.0`). It copies ImmediateResponse status, headers, and body into its direct response.
- The challenge follows MCP 2025-11-25 Authorization, Scope Challenge Handling.
- mcp-go v1.1.0 (and latest v1.4.0) reports any non-401 4xx on `initialize` as `ErrLegacySSEServer`. The URL is machine-readable to mcp-go only after successful initialize.
- Without `scope`, recovery depends on the authorization server re-running consent. With a refresh token, TS SDK 1.32.2 refreshes instead of re-authorizing. If the cause persists, it fails with `Server returned 403 after trying upscoping`; users then follow `error_uri`.
- Without `resource_metadata`, step-up needs the gateway to serve protected-resource metadata at the MCP endpoint's well-known path (Journey 2 route or agentgateway `mcpAuthentication.resourceMetadata`). The documented `jwtAuth` deployment does not serve it.
