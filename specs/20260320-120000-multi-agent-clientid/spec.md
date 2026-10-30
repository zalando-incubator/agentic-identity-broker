# Feature Specification: Multi-Agent OAuth2 Client Delegation

**Feature Branch**: `20260320-120000-multi-agent-clientid`
**Created**: 2026-03-20
**Status**: Draft
**Input**: User description: "Allow multiple agents managed in the broker to delegate to the same upstream OAuth2 client ID. Forward agent ID as extra parameter in authorize URL, check for new claim in authorization response and token endpoint return. Token exchange endpoint resolves agentID from subject token claims. Optional feature enabled via configuration with configurable parameter and claim names. CEL function for backwards compatibility when feature is enabled."

## Clarifications

### Session 2026-03-20

- Q: How should the broker behave when multi-agent client sharing is enabled but the upstream does not return the expected agent ID claim in the token? → A: Fail closed — return an OAuth2 error and do not pass the token to the client.
- Q: Should the uniqueness constraint on agent `client_id` be fully removed when the feature is enabled, or should it remain enforced and operators explicitly opt specific agents into sharing? → A: The constraint is relaxed globally when the feature is enabled; all agents may share a client_id. Individual opt-in is out of scope.
- Q: When the feature is enabled, can the CEL `resolveAgentIdByClientId` function be called for a client_id that maps to multiple agents? → A: The function is a backwards-compatibility aid and assumes 1:1 mapping; calling it on a shared client_id has undefined behavior and should be avoided in new policies.
- Q: How is the agent resolved from the incoming OAuth2 authorize request `client_id` parameter when multiple agents share an upstream client_id? → A: The `client_id` parameter in all incoming OAuth2 requests is always resolved against `agent.id` (the agent's internal identifier), not `agent.client_id` (the upstream OAuth2 client ID). This is a breaking change; backwards compatibility with existing clients is not required.
- Q: Does the broker need to validate the JWT signature when inspecting the token at the token endpoint for the agent ID claim? → A: No — the broker parses JWT claims without signature verification. Claim presence and registry match are sufficient; cryptographic validation is the responsibility of the token consumer (resource server), not the proxy.
- Q: When the feature is enabled, does the token exchange (ExtProc) service need a new agent ID extraction mechanism, or should the agent ID claim be exposed as a CEL variable? → A: Token claims are already fully exposed in the CEL evaluation context; no new extraction mechanism is needed. Operators reference the agent ID claim directly in their CEL policy (e.g., `claims.x_agent_id`). No broker-side changes to the CEL infrastructure are required.
- Q: Should the `resolveAgentIdByClientId` CEL helper function be kept, and if so when should it be registered? → A: Keep it. Register it only when the feature is **disabled** — in that mode upstream client_id uniqueness is enforced so the lookup is safe. When the feature is enabled the function must NOT be registered; operators use the agent ID claim directly. Both modes are long-lived operator choices.
- Q: What does the token exchange CEL policy look like for both feature modes? → A: Feature disabled: `agent_id_expression: "resolveAgentIdByClientId(subject_token.azp)"` (maps upstream client_id claim to agent.id via helper). Feature enabled: `agent_id_expression: "subject_token.x_agent_id"` (reads agent ID claim directly, using the configured claim name).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Multiple Agents Share One Upstream OAuth2 Client (Priority: P1)

An operator registers multiple distinct AI agents in the identity broker that all delegate through the same upstream OAuth2 client application. Each agent has its own identity, consent flow, and permission scope, but the upstream OAuth2 server only knows one registered client. The broker appends the specific agent ID as an extra parameter in the upstream authorization redirect so that the upstream server can embed the agent ID as a claim in the minted token, enabling downstream services to distinguish which agent obtained the token.

**Why this priority**: This is the core capability. Without many-to-one agent-to-client mapping working end-to-end, no other part of this feature delivers value.

**Independent Test**: Can be fully tested by configuring two agents with the same upstream client_id, initiating authorization for each, and verifying that (a) each authorization redirect carries the respective agent ID as the configured parameter, and (b) the tokens proxied back from the upstream each contain the correct agent ID claim.

**Acceptance Scenarios**:

1. **Given** the feature is enabled with configured parameter and claim names, **When** the broker processes an authorization request for an agent that shares its upstream client_id with other agents, **Then** the broker appends the configured agent ID parameter (with the agent's internal ID as its value) to the upstream authorization redirect URL
2. **Given** an authorization request arrives at `/oauth2/authorize?client_id=<value>`, **When** the broker resolves the agent, **Then** it looks up the agent whose internal identifier (`agent.id`) matches the `client_id` parameter value — regardless of whether the feature is enabled or disabled
3. **Given** the upstream authorization succeeds and the token endpoint returns a token, **When** the broker proxies the token response back to the client, **Then** it verifies the token contains the configured agent ID claim before returning it
4. **Given** the token contains the expected agent ID claim, **When** the token is returned to the requesting client, **Then** the claim value matches the internal ID of the agent that initiated the authorization flow
5. **Given** the upstream token response does not contain the expected agent ID claim, **When** the broker attempts to verify the claim, **Then** it returns an OAuth2 error response and does not forward the token to the client
6. **Given** the feature is disabled, **When** authorization and token exchange proceed, **Then** the system behaves exactly as before (no agent ID parameter forwarded, no claim verification, client_id uniqueness enforced per agent)

---

### User Story 2 - Token Exchange Resolves Agent from Token Claim (Priority: P2)

When the ExtProc token exchange service receives a subject token for exchange, it currently uses a CEL expression to extract the upstream `client_id` from the token claims and map it to an agent. Token claims are already fully exposed as variables in the CEL evaluation context. With multi-agent client sharing enabled, the upstream server embeds the agent ID as a token claim, and operators update their CEL policy to reference that claim directly (e.g., `claims.x_agent_id`) — no changes to the broker's CEL infrastructure are required.

**Why this priority**: Depends on P1 — the token must carry the agent ID claim before CEL policies can reference it. This is critical for downstream services that perform token exchange to identify which agent is acting on behalf of the user.

**Independent Test**: Can be tested by presenting a subject token containing the configured agent ID claim to the token exchange endpoint with a CEL policy that references the claim, and verifying the exchange resolves the correct agent identity.

**Acceptance Scenarios**:

1. **Given** the feature is enabled and a subject token contains the configured agent ID claim, **When** a CEL policy references the claim (e.g., `claims.x_agent_id`), **Then** the token exchange resolves the correct agent and completes successfully
2. **Given** the feature is enabled and a CEL policy references the agent ID claim, **When** a subject token is missing that claim, **Then** the CEL expression fails and the token exchange returns an error (standard CEL missing-variable behaviour)
3. **Given** the feature is disabled, **When** a CEL policy calls `resolveAgentIdByClientId(claims.client_id)`, **Then** it returns the matching agent's internal ID and the token exchange proceeds (function is registered only in disabled mode, where upstream client_id is unique)
4. **Given** the feature is enabled and a subject token contains an agent ID claim that does not match any registered agent, **When** the token exchange endpoint processes the token, **Then** it returns an error and does not perform the exchange

---

### User Story 3 - Operator Configures Multi-Agent Client Sharing (Priority: P3)

An operator enables and configures multi-agent client sharing via YAML configuration, specifying the parameter name to forward on the authorize URL and the claim name expected in returned tokens.

**Why this priority**: Configuration is a prerequisite but can be validated independently of the full end-to-end flow. Startup validation gives operators immediate feedback without needing a running upstream.

**Independent Test**: Can be tested by verifying that starting the broker with a valid enabled configuration succeeds, starting with an incomplete configuration (missing parameter or claim name) fails with a clear error, and starting with the feature disabled uses the unchanged default behavior.

**Acceptance Scenarios**:

1. **Given** a configuration with multi-agent client sharing enabled and both `agent_id_param_name` and `agent_id_claim_name` specified, **When** the broker starts, **Then** it initializes successfully and applies the configured names throughout all OAuth2 flows
2. **Given** a configuration with the feature enabled but `agent_id_param_name` absent, **When** the broker starts, **Then** it fails at startup with a clear error describing the missing parameter
3. **Given** a configuration with the feature enabled but `agent_id_claim_name` absent, **When** the broker starts, **Then** it fails at startup with a clear error describing the missing claim name
4. **Given** a configuration with `enabled: false` (or no `multi_agent_client` block), **When** the broker starts, **Then** it operates in the original single-agent-per-client mode with no behavioral change

---

### Edge Cases

- What happens when the upstream server does not return the expected agent ID claim? → The broker returns an OAuth2 error (server_error) and withholds the token from the client (fail closed).
- What happens when a request arrives with a `client_id` that does not match any `agent.id`? → The broker returns `invalid_client`, unchanged from the base behavior.
- How does consent checking work when multiple agents share an upstream client_id? → Consent is tied to `agent.id`; each agent requires its own grant independently.
- What happens when the agent ID claim in the token doesn't match any registered `agent.id`? → Token exchange fails with an error; unrecognized agent IDs are rejected.
- What happens if an operator calls `resolveAgentIdByClientId` when the feature is enabled? → The function is not registered in that mode; the CEL expression will fail with an unknown function error, surfacing the misconfiguration immediately.
- What happens if both the agent ID claim and an upstream client_id claim are present in the token when the feature is enabled? → The agent ID claim takes precedence for agent resolution.
- What happens when the feature is enabled but the upstream authorization server ignores the extra agent ID parameter and does not embed it as a claim? → The broker fails closed when verifying the token (claim absent error) rather than silently passing through tokens without agent attribution.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When the feature is enabled, the authorization endpoint MUST append the agent's internal ID as a query parameter (using the configured `agent_id_param_name`) to the upstream authorization redirect URL
- **FR-002**: The authorization endpoint MUST always resolve the agent by matching the incoming `client_id` parameter against `agent.id` (the agent's internal identifier). This applies regardless of whether the feature is enabled. The upstream proxy request then uses `agent.client_id` (the upstream OAuth2 client ID) as the `client_id` sent to the upstream server.
- **FR-003**: When the feature is enabled, the token endpoint MUST parse the JWT claims in the proxied token response (without signature verification) and verify that the configured `agent_id_claim_name` claim is present and its value matches the internal ID of the agent that initiated the request (i.e., the agent whose UUID was supplied as `client_id` in the token request) before returning the token to the client
- **FR-004**: When the feature is enabled and the token response lacks the expected agent ID claim, the system MUST return an OAuth2 error response and MUST NOT forward the token to the client
- **FR-005**: When the feature is disabled, the upstream proxy behavior is unchanged except that `agent.client_id` (not the incoming `client_id` / `agent.id`) is sent to the upstream server; no agent ID parameter is forwarded and no claim verification is performed
- **FR-006**: The system MUST reject startup if the feature is enabled but either `agent_id_param_name` or `agent_id_claim_name` is not configured
- **FR-007**: Token claims are already fully exposed in the CEL evaluation context; no broker-side changes to CEL infrastructure are required. Operators MUST update their token exchange CEL policy to reference the configured `agent_id_claim_name` claim (e.g., `claims.x_agent_id`) when the feature is enabled
- **FR-008**: If the agent ID claim is absent from the subject token, the CEL expression referencing it will fail, causing the token exchange to return an error — this is handled by standard CEL evaluation behaviour with no additional broker logic required
- **FR-009**: When the feature is **disabled**, the system MUST register a custom CEL helper function `resolveAgentIdByClientId(clientId: string) -> string` that, given an upstream client_id value (as it appears in a token claim), returns the matching agent's internal ID. This is safe in disabled mode because upstream client_id uniqueness is enforced.
- **FR-010**: The `resolveAgentIdByClientId` CEL function MUST only be registered when the feature is **disabled**. When the feature is enabled, the function MUST NOT be registered — operators reference the agent ID claim directly in their CEL policy (e.g., `claims.<agent_id_claim_name>`).
- **FR-011**: Consent checking MUST operate per `agent.id`, not per upstream client_id, so each agent's consent is independently managed even when multiple agents share the same upstream client_id
- **FR-012**: The system MUST emit structured audit log entries for agent ID parameter injection on the authorization endpoint and agent ID claim verification on the token endpoint when the feature is enabled

### Configuration Requirements

**Configuration Parameters**:
- **enabled**: Boolean. Whether multi-agent client sharing is active. Default: `false`. When `false`, all other `multi_agent_client` parameters are ignored and the system behaves as before.
- **agent_id_param_name**: String. The name of the query parameter appended to the upstream authorization redirect URL carrying the agent's internal ID (e.g., `"x_agent_id"`). Required when `enabled: true`, no default.
- **agent_id_claim_name**: String. The name of the JWT claim expected in tokens returned by the upstream server that contains the agent's internal ID (e.g., `"x_agent_id"`). Required when `enabled: true`, no default.

**Example YAML Configuration**:

*Feature disabled (default) — upstream client_id uniqueness enforced; `resolveAgentIdByClientId` CEL helper available:*
```yaml
oauth2_authorization_server:
  mode: "delegate_upstream"
  upstream_issuer_uri: "https://oauth2.example.com"
  upstream_authorize_endpoint: "https://oauth2.example.com/oauth2/authorize"
  upstream_token_endpoint: "https://oauth2.example.com/oauth2/token"
  public_base_url: "https://identity-broker.example.com"

  # multi_agent_client not set (or enabled: false) — single-agent-per-upstream-client mode

token_exchange:
  claim_extraction:
    principal_expression: "subject_token.sub"
    # resolveAgentIdByClientId is registered when multi_agent_client is disabled.
    # It maps the upstream client_id claim (e.g. azp) to the broker's agent.id.
    agent_id_expression: "resolveAgentIdByClientId(subject_token.azp)"
```

*Feature enabled — multiple agents may share the same upstream client_id; agent resolved from token claim:*
```yaml
oauth2_authorization_server:
  mode: "delegate_upstream"
  upstream_issuer_uri: "https://oauth2.example.com"
  upstream_authorize_endpoint: "https://oauth2.example.com/oauth2/authorize"
  upstream_token_endpoint: "https://oauth2.example.com/oauth2/token"
  public_base_url: "https://identity-broker.example.com"

  multi_agent_client:
    enabled: true
    agent_id_param_name: "x_agent_id"   # extra parameter forwarded on /oauth2/authorize
    agent_id_claim_name: "x_agent_id"   # claim the upstream embeds in minted tokens

token_exchange:
  claim_extraction:
    principal_expression: "subject_token.sub"
    # When multi_agent_client is enabled, read the agent ID directly from the token claim.
    # resolveAgentIdByClientId is NOT registered in this mode.
    agent_id_expression: "subject_token.x_agent_id"
```

**Configuration Location**: Added under `oauth2_authorization_server` in `examples/config/oauth2-authorization-server.yaml`; `token_exchange` snippet illustrates required CEL policy change in `examples/config/token-exchange.yaml`

### Security Requirements

- **SR-001**: The system MUST fail closed when the expected agent ID claim is absent from a token — no fallback to client_id-based lookup when the feature is enabled
- **SR-002**: The broker MUST reject any token containing an agent ID claim that does not match a registered agent; it MUST NOT silently pass through tokens with unrecognized agent IDs
- **SR-003**: The agent ID parameter appended to the upstream authorization URL MUST use only the broker-assigned internal agent ID value; the system MUST NOT accept user-supplied or externally influenced values for this parameter
- **SR-004**: Audit logs MUST include the agent ID claim verification outcome (success/failure, agent ID value) for all token endpoint responses when the feature is enabled
- **SR-005**: The `resolveAgentIdByClientId` CEL helper function MUST be read-only and MUST NOT be registered when the feature is enabled, preventing ambiguous upstream client_id lookups in shared-client mode

### Key Entities

- **Agent**: Existing entity with two distinct identifiers: `agent.id` (internal broker identifier, globally unique, UUID) and `agent.client_id` (upstream OAuth2 client ID, used only for proxying to upstream). OAuth2 clients communicate with the broker using `agent.id` as the `client_id` parameter — this is a breaking change from the prior behavior where `agent.client_id` was used. When the feature is enabled, the uniqueness constraint on `agent.client_id` is relaxed and multiple agents may share the same upstream client ID.
- **Multi-Agent Client Configuration**: A new configuration value object nested within `oauth2_authorization_server`. Contains `enabled`, `agent_id_param_name`, and `agent_id_claim_name`. All-or-nothing scoping: when enabled, applies to all agents.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can successfully register two or more agents sharing the same upstream client_id without configuration errors or startup failures
- **SC-002**: Authorization flows for agents sharing a client_id each produce tokens with the correct agent-specific claim with 100% accuracy across all test scenarios
- **SC-003**: The token exchange endpoint correctly resolves the agent from the token claim (not client_id) with 100% accuracy when the feature is enabled
- **SC-004**: When the feature is disabled, all OAuth2 flows function correctly with agents resolved by `agent.id` and proxied to upstream using `agent.client_id`
- **SC-005**: Operators receive clear startup error messages for missing or invalid multi-agent configuration within 5 seconds of startup
- **SC-006**: Enabling the feature introduces no measurable latency increase on the authorization or token proxy paths beyond the parameter injection and claim presence check

## Scope

### In Scope

- Breaking change: OAuth2 `client_id` parameter in all incoming requests is now resolved against `agent.id` (internal identifier), not `agent.client_id` (upstream client ID)
- `multi_agent_client` configuration block nested under `oauth2_authorization_server`
- Agent ID parameter injection in the upstream authorization redirect URL
- Agent ID claim presence verification in proxied token endpoint responses
- Operator guidance: CEL policy update to reference the agent ID claim directly when the feature is enabled (no broker code changes needed for token exchange)
- Custom CEL helper function `resolveAgentIdByClientId` registered only when the feature is **disabled** (safe lookup by upstream client_id in 1:1 mode)
- Startup validation of `multi_agent_client` configuration parameters
- Relaxed client_id uniqueness constraint on agents when the feature is enabled
- Structured audit logging for agent ID injection and claim verification

### Out of Scope

- Changes to the upstream OAuth2 server configuration or behavior
- Automatic claim mapping or transformation between different claim name formats
- Support for agent ID embedded in non-JWT (opaque) token formats
- Consent UI changes (consent remains per-agent as before)
- Migration tooling or backwards compatibility shims for existing OAuth2 clients using `agent.client_id` as the `client_id` parameter
- Migration tooling for converting single-agent setups to multi-agent configuration
- JWT signature validation at the token endpoint (signature verification is the resource server's responsibility, not the proxy's)
- Token introspection or cryptographic validation of agent ID claim integrity beyond claim presence and registry match
- Per-agent opt-in/opt-out of client sharing (the feature is all-or-nothing when enabled)

## Dependencies

- **OAuth2 Authorization Server Proxy** ([20251222-120100-oauth2-auth-server](../20251222-120100-oauth2-auth-server/spec.md)): This feature extends the existing proxy behavior; spec 20251222-120100 must be implemented
- **ExtProc Token Exchange** ([20260223-120000-extproc-token-exchange](../20260223-120000-extproc-token-exchange/spec.md)): Token claims are already exposed as CEL variables; no code changes required. Operators must update their CEL policy configuration to reference the agent ID claim.
- **Agent Registry** ([20251217-120000-domain-model-apis](../20251217-120000-domain-model-apis/spec.md)): Relaxed uniqueness on `client_id` must be supported by the agent registry
- **CEL Policy Evaluation** ([20260115-120000-token-exchange](../20260115-120000-token-exchange/spec.md)): The custom CEL function extension requires the CEL evaluation infrastructure to support pluggable function registration

## Assumptions

- The upstream OAuth2 server can be configured to receive an extra query parameter during authorization and embed it verbatim as a claim in the minted token
- The upstream server uses a standard JWT format for access tokens; the broker parses claims without signature verification (base64-decode only). JWT signature validation is delegated to token consumers (resource servers).
- The agent ID parameter name and token claim name are symmetric (same name on both sides is typical, but the configuration allows them to differ)
- OAuth2 clients integrating with the broker are updated to use `agent.id` as the `client_id` parameter; no migration shim is provided
- When the feature is disabled, uniqueness of `agent.client_id` (upstream client ID) per agent is still enforced by the agent registry
- The broker does not validate the cryptographic binding between the agent ID parameter sent during authorization and the claim in the minted token — it trusts the upstream server to embed the claim faithfully
- When the feature is disabled, CEL policies use `resolveAgentIdByClientId(claims.client_id)` to look up the agent; when enabled, policies reference the agent ID claim directly (e.g., `claims.x_agent_id`). The function is intentionally unavailable in enabled mode to prevent ambiguous lookups.
- The feature is all-or-nothing: when enabled, agent ID forwarding and claim verification apply to all authorization and token exchange flows
