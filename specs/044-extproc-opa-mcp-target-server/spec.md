# Feature Specification: MCP Target Server Name in ExtProc OPA Authorization Input

**Feature Branch**: `044-extproc-opa-mcp-target-server`

**Created**: 2026-09-16

**Status**: Draft

**Predecessor**: `specs/020-extproc-opa-authorization/` — introduced OPA-based authorization in ExtProc, including the `agentgateway.protocol` metadata extraction pattern and the protocol-namespaced OPA input document (`input.mcp.*`).

**Input**: User description: "Leverage a context attribute sent by agentgateway (`metadataContext.agentgateway.mcp_server`) to identify the specific MCP server a request targets, and make that identifier available in the OPA policy input document so policies can be written per MCP server."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Operator Writes MCP-Server-Scoped Tool Policies (Priority: P1)

An operator manages several MCP servers behind the same agentgateway/ExtProc deployment (e.g., a "github-mcp" server and a "salesforce-mcp" server). The operator wants a single OPA policy bundle that applies different tool-call rules depending on which MCP server the request targets, without deploying separate ExtProc instances or policy bundles per server.

**Why this priority**: This is the core value of the feature — without the target MCP server identifier in the input document, OPA policies cannot distinguish which upstream MCP server a `tools/call` request is destined for, forcing operators into one-policy-per-deployment workarounds.

**Independent Test**: Send an MCP `tools/call` request through ExtProc with OPA enabled and a policy that allows `delete_record` only when `input.mcp.target_server_name == "salesforce-mcp"`. Verify the same tool call is denied when the gateway metadata identifies a different MCP server (e.g., "github-mcp") and allowed when it identifies "salesforce-mcp".

**Acceptance Scenarios**:

1. **Given** OPA is enabled with a policy referencing `input.mcp.target_server_name`, **When** a `tools/call` request arrives with `metadataContext.agentgateway.mcp_server = "github-mcp"`, **Then** the OPA input document contains `mcp.target_server_name = "github-mcp"` and the policy decision reflects the server-specific rule.
2. **Given** the same policy, **When** an identical tool call arrives but the gateway metadata identifies a different MCP server, **Then** the OPA decision differs according to the per-server rule (e.g., allow for one server, deny for another).
3. **Given** OPA is enabled with a policy that does not reference `input.mcp.target_server_name` at all, **When** requests arrive from any MCP server, **Then** authorization behavior is unchanged from before this feature (zero regression for policies not using the new field).

---

### User Story 2 - MCP Target Server Identity Available for Header-Only Requests (Priority: P2)

An operator wants to gate MCP session establishment (e.g., the initial SSE/GET handshake) per MCP server, not just tool calls, so that an entire MCP server can be restricted to specific agents at the session level.

**Why this priority**: Header-only MCP requests (`mcp_headers_only`) are evaluated before token exchange and already carry `mcp.session_id`; extending the same header-derived context to include the target MCP server identifier lets policies apply server-level gating consistently across both header-only and body-bearing phases.

**Independent Test**: Send a header-only MCP request (end-of-stream in `RequestHeaders`) through ExtProc with OPA enabled and a policy referencing `input.mcp.target_server_name`. Verify the OPA input document for the header-only evaluation includes the same `mcp.target_server_name` value as a subsequent body-bearing request in the same session.

**Acceptance Scenarios**:

1. **Given** OPA is enabled, **When** a header-only MCP request arrives with `metadataContext.agentgateway.mcp_server = "github-mcp"`, **Then** the `mcp_headers_only` OPA input document contains `mcp.target_server_name = "github-mcp"` alongside the existing `mcp.session_id`.
2. **Given** a policy that denies all requests where `input.mcp.target_server_name == "restricted-mcp"`, **When** a header-only request targets that server, **Then** ExtProc returns a 403 before token exchange is attempted.

---

### User Story 3 - Fail-Closed Rejection When MCP Target Server Metadata Is Absent (Priority: P3)

An operator upgrades agentgateway routes to this feature, but a route is misconfigured and forwards an MCP request without the `mcp_server` metadata attribute. ExtProc must reject the request rather than silently authorizing it without target-server context.

**Why this priority**: Fail-closed safety — `mcp_server` is mandatory for MCP requests once this feature is enabled, mirroring how missing `protocol` metadata is already mandatory (spec 020, FR-003). A misconfigured route must not silently bypass MCP-server-scoped policies.

**Independent Test**: Send an MCP `tools/call` request through ExtProc with OPA enabled where the gateway metadata includes `protocol` but omits `mcp_server`. Verify the request is rejected with HTTP 403 at the headers phase, before OPA is ever invoked.

**Acceptance Scenarios**:

1. **Given** OPA is enabled and `metadataContext.agentgateway` contains `protocol` but no `mcp_server` field, **When** a request is evaluated, **Then** ExtProc rejects the request with HTTP 403 at the headers phase, mirroring the existing missing-`protocol` rejection, and OPA is never invoked.
2. **Given** `mcp_server` metadata is present but set to an empty string, **When** a request is evaluated, **Then** ExtProc treats it the same as absent and rejects with HTTP 403.

---

### Edge Cases

- What happens when `metadataContext.agentgateway.mcp_server` is present but set to an empty string? → Treated the same as absent: ExtProc rejects the request with HTTP 403 (empty is not a meaningful MCP server name).
- What happens when the protocol is not `mcp` (e.g., `a2a` or unrecognized) but `mcp_server` metadata is present anyway? → The field is ignored; `mcp.target_server_name` is only populated when `type` is one of the MCP-namespaced values (`mcp_tool_call`, `mcp_method`, `mcp_headers_only`), consistent with the existing protocol-namespaced input design from spec 020.
- What happens when OPA is disabled entirely? → No behavior change; ExtProc does not build an OPA input document at all in that mode (per FR-001 of spec 020), so this feature has no effect.
- How does the system handle a batch JSON-RPC request (multiple messages in one body)? → The same `mcp.target_server_name` value applies to every message in the batch, since it is derived once from gateway metadata per request, not per JSON-RPC message.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: ExtProc MUST read the MCP target server identifier from `MetadataContext.FilterMetadata["agentgateway"]["mcp_server"]`, using the same extraction mechanism already established for `protocol` (spec 020).
- **FR-002**: When present and non-empty, ExtProc MUST expose the extracted value as `mcp.target_server_name` in the OPA input document, alongside the existing `mcp.tool_name`, `mcp.method`, and `mcp.session_id` fields.
- **FR-003**: ExtProc MUST populate `mcp.target_server_name` for all three MCP input variants: `mcp_tool_call`, `mcp_method`, and `mcp_headers_only`.
- **FR-004**: When `mcp_server` metadata is absent or an empty string on an MCP request, ExtProc MUST reject the request with HTTP 403 at the headers phase, before OPA is ever invoked — mirroring how missing `protocol` metadata is already mandatory (spec 020 FR-003). `mcp.target_server_name` is therefore always present and non-empty in any OPA input document that reaches evaluation.
- **FR-005**: ExtProc MUST NOT populate `mcp.target_server_name` when the request's protocol is not `mcp` (i.e. the request's `type` discriminator is `unknown`), even if `mcp_server` metadata happens to be present. `type` is itself derived from `protocol` (spec 020), so this is a single gate, not two independent checks.
- **FR-006**: The `mcp.target_server_name` value MUST be passed through unmodified (no case-folding, trimming beyond the empty-string check, or validation against an allow-list) so that OPA policies retain full control over matching logic.
- **FR-007**: This feature MUST NOT change the OPA decision outcome for any existing policy that does not reference `input.mcp.target_server_name` (zero regression for policies unaware of the new field).

### Domain Model *(if applicable - document before API or database design)*

**Value Objects** (things without identity):

- **MCPInput** *(scope broadened)*: The existing MCP protocol-fields value object (spec 020) is redefined to cover MCP protocol fields **plus** agentgateway-routed target-server context, and gains one new field, `TargetServerName`, carrying the gateway-identified MCP server name. This field is mandatory for MCP requests (see FR-004) — it is a deliberate, documented broadening of `MCPInput`'s scope (not an accidental mixing of concerns): `TargetServerName` is routing metadata, not an MCP JSON-RPC wire field, but it lives alongside the protocol fields because it is only ever meaningful for MCP requests (see FR-005) and callers need one place to look for "everything about this MCP request." Immutable per evaluation, same as today.

*Domain terms to add to ARCHITECTURE.md Glossary:*

- **MCP Target Server (context attribute)**: The logical name of the upstream MCP server a request targets, sourced from agentgateway's `metadataContext.agentgateway.mcp_server` and surfaced to OPA policies as `input.mcp.target_server_name`. Distinct from `MCPInput.SessionID`, which identifies a client session rather than a server.

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: The `mcp.target_server_name` value MUST be treated as non-sensitive operational metadata (a configuration-supplied server name, not a credential or user-identifying attribute) and MAY be logged and passed unredacted, consistent with existing ExtProc structured logging practices.
- **SR-002**: The existing fail-closed baseline from spec 020 (SR-001: default deny when no policy rule matches) is extended by this feature: absent or empty `mcp_server` metadata on an MCP request MUST itself cause a 403 rejection at the headers phase (FR-004), rather than being left to the configured policy's own rules.

### Key Entities

- **MCP target server identifier**: A string value, supplied by agentgateway per route/request, identifying which upstream MCP server a request targets. Not persisted by ExtProc; recomputed per request from gateway metadata.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Policy authors can write a single Rego rule referencing `input.mcp.target_server_name` to apply different tool-call decisions across MCP servers, without deploying separate ExtProc instances or policy bundles per server.
- **SC-002**: 100% of existing OPA policies that do not reference `input.mcp.target_server_name` produce identical allow/deny decisions before and after this feature ships (zero regression).
- **SC-003**: Requests through agentgateway routes that have not been configured to send `mcp_server` metadata are rejected with HTTP 403 at the headers phase, with no OPA evaluation attempted, so misconfigured routes fail closed rather than silently authorizing without target-server context.
- **SC-004**: An operator can distinguish MCP-target-server-specific authorization failures from other denials by inspecting the `target_server_name` field surfaced in structured OPA decision logs and trace spans.

## Assumptions

- agentgateway is configured to send `mcp_server` as a sibling field to `protocol` under the same `metadataContext.agentgateway` filter-metadata namespace already used and documented in spec 020 (`FilterMetadata["agentgateway"]["mcp_server"]`).
- Like `protocol`, `mcp_server` is treated as mandatory metadata for MCP requests: every agentgateway route serving MCP traffic must be configured to send it before enabling OPA authorization, since ExtProc rejects requests with HTTP 403 when it is absent or empty.
- The MCP target server identifier is a single string per request (not a list), matching the single-value nature of `protocol` and `session_id` extraction.
- No new configuration parameters, database migrations, or public API changes are required — this is an internal extension of the existing OPA input document schema established in spec 020, exposed only to the embedded OPA evaluation engine.
- `input.mcp`'s documented scope (spec 020's frozen contract) is deliberately broadened by this feature to cover "MCP protocol fields plus agentgateway-routed target-server context," rather than staying strictly protocol-only. The naming `target_server_name` (rather than a bare `server`) was chosen to avoid ambiguity with other possible "server" meanings (e.g., the ExtProc process itself) and to make the routing-metadata origin explicit to policy authors.
