# Feature Specification: Principal and Agent ID in the ExtProc OPA Authorization Input

**Feature Branch**: `055-extproc-opa-identity-context`

**Created**: 2026-10-29

**Status**: Draft

**Predecessors**: `specs/020-extproc-opa-authorization/` introduced the OPA input document and its `context` object. `specs/026-extproc-approval-sync/` added the verified `principal` and `agent_id` to the token-exchange response (FR-017) and the `approval_required` decision.

**Input**: User description: "Expose the calling user and agent in the ExtProc OPA input, so a policy can require a tool approval for one specific user and one specific tool."

## Clarifications

### Session 2026-10-29

- Q: Policies can no longer identify the user or agent. Spec 020 SR-005 passed the Bearer token to OPA for this, but feature 043 (ADR 036) moved the subject token into gateway metadata, and the gateway strips the `Authorization` header. Where should the identity come from? → A: From the broker. The token-exchange response already carries the `principal` and `agent_id` the broker extracted from the signature-verified subject token (spec 026 FR-017). ExtProc passes those values to OPA. It does not parse the subject token.
- Q: Which field names, and where in the input document? → A: `context.principal` and `context.agent_id`. The `context` object holds the token-bound authorization context (`granted_permission_sets`, `agent_session_id`). The names match the token-exchange response and the approval API.
- Q: Header-only MCP requests are evaluated before token exchange. Should ExtProc exchange earlier so they also carry the identity? → A: No. Header-only evaluations keep their current order and omit both fields.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Policy requires approval for one user's tool calls (Priority: P1)

An operator wants one user to approve each call an agent makes to one tool, for example while trialling tool approvals with a single colleague. Everyone else keeps calling the tool without a prompt. The operator writes a static Rego rule that returns `approval_required` for that user and tool.

**Why this priority**: This is the reason for the feature. Without the user in the input document, a policy can only gate a tool for everyone or no one.

**Independent Test**: Enable tool approvals and load a policy that requires approval for `get_user` when `input.context.principal` is Alice's. Send `get_user` once with Alice's subject token and once with Bob's. Verify that Alice's call returns a URL elicitation and Bob's call is forwarded.

**Acceptance Scenarios**:

1. **Given** tool approvals are enabled and a policy returns `approval_required` for `get_user` when `input.context.principal == "alice@example.com"`, **When** Alice's agent calls `get_user`, **Then** ExtProc creates a pending approval and returns a JSON-RPC `-32042` URL elicitation with the broker's approval URL, and the call is not forwarded.
2. **Given** the same policy, **When** Bob's agent calls `get_user`, **Then** ExtProc forwards the call with the exchanged token and creates no approval.

---

### User Story 2 - Policy applies a rule to one agent (Priority: P2)

An operator wants to stop one agent from calling a tool, whichever user it acts for.

**Why this priority**: `agent_id` comes from the same exchange response at no extra cost. Agent-scoped rules are the second most common policy shape after user-scoped ones.

**Independent Test**: Load a policy that denies `get_user` when `input.context.agent_id` is the Research Assistant's ID. Call `get_user` through that agent and verify a 403.

**Acceptance Scenarios**:

1. **Given** a policy denies `get_user` when `input.context.agent_id == "7c9e6679-7425-40de-944b-e07fc1f90ae7"`, **When** that agent calls `get_user`, **Then** ExtProc returns HTTP 403 `access_denied` with the policy's reason and does not forward the call.

---

### User Story 3 - Identity is present only when the broker verified it (Priority: P2)

A policy author needs to know when the fields can be missing, so that an identity rule never matches an unknown caller by accident.

**Why this priority**: A policy that reads a field ExtProc did not set must see it as undefined, never as an empty or placeholder identity.

**Independent Test**: Load a policy that denies any request whose `input.context.principal` is defined. Send a header-only MCP request, and a tool call whose token exchange returns no `principal` or `agent_id`. Verify that neither is denied.

**Acceptance Scenarios**:

1. **Given** a policy that denies when `input.context.principal` or `input.context.agent_id` is defined, **When** a header-only MCP request is evaluated, **Then** the request is allowed, because header-only evaluation runs before token exchange.
2. **Given** the same policy and a token-exchange response without `principal` and `agent_id`, **When** a tool call is evaluated, **Then** the call is allowed, because ExtProc omits both fields.

---

### Edge Cases

- **JSON-RPC batch**: Every element of a batch is evaluated with the same `principal` and `agent_id`, because one token exchange serves the whole request.
- **Only one field returned**: ExtProc sets the field it received and omits the other.
- **Non-MCP request with a body**: The fields are set as for MCP requests. The token exchange runs in the headers phase for every body-bearing request.
- **OPA disabled**: No input document is built, so the feature has no effect.
- **Tool approvals disabled**: A matching `approval_required` rule still becomes a 403, as today. The fields only change which requests the rule matches.

## Concrete Example Journeys *(mandatory)*

### Journey 1 — An operator trials approvals with one user (User Story 1, Scenarios 1–2)

**Cast**: The broker is published at `https://broker.example.com`. ExtProc runs in front of the `directory-mcp` MCP server. Alice (`alice@example.com`) and Bob (`bob@example.com`) both use the *Research Assistant* agent, whose canonical agent ID is `7c9e6679-7425-40de-944b-e07fc1f90ae7`.

ExtProc configuration (excerpt):

```yaml
authorization:
  enabled: true
  policy:
    path: /policies
    package: aib.extproc.authz
tool_approvals:
  enabled: true
  url: https://broker.example.com
```

Policy in `/policies/approvals.rego`:

```rego
package aib.extproc.authz

default result := {"action": "allow"}

result := {"action": "approval_required", "approval_context": {"description": "Approve looking up a user", "risk_level": "low"}} if {
	input.mcp.tool_name == "get_user"
	input.context.principal == "alice@example.com"
}
```

**Step 1 — 09:00:00: Alice's agent calls `get_user`**

The gateway validates Alice's token and hands it to ExtProc in metadata. ExtProc exchanges it in the headers phase:

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&subject_token=SYNTHETIC-ALICE-SUBJECT-TOKEN
&subject_token_type=urn:ietf:params:oauth:token-type:access_token
&resource=https://directory-mcp.example.com/mcp
&client_assertion=SYNTHETIC-EXTPROC-ASSERTION
&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "access_token": "dir_at_SYNTHETIC_ALICE",
  "token_type": "Bearer",
  "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "expires_in": 3600,
  "principal": "alice@example.com",
  "agent_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7"
}
```

**Step 2 — 09:00:00: ExtProc evaluates the body**

The agent's request carries `Mcp-Session-Id: sess-alice-1` and this body:

```json
{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": {"name": "get_user", "arguments": {"user_id": "u-42"}}}
```

The OPA input document now carries the identity (excerpt):

```json
{
  "type": "mcp_tool_call",
  "mcp": {"method": "tools/call", "tool_name": "get_user", "target_server_name": "directory-mcp", "...": "..."},
  "context": {
    "granted_permission_sets_available": false,
    "agent_session_id": "sess-alice-1",
    "principal": "alice@example.com",
    "agent_id": "7c9e6679-7425-40de-944b-e07fc1f90ae7"
  }
}
```

The policy returns `approval_required`. ExtProc creates a pending approval with `POST /api/approvals` (feature 026, unchanged) and gets back the approval URL `https://broker.example.com/approvals/3f2b8c1e-9a4d-4e7b-b1c2-5d6e7f8a9b0c`. It answers the agent without forwarding the call:

```http
HTTP/1.1 200 OK
Content-Type: application/json

{
  "jsonrpc": "2.0",
  "id": 7,
  "error": {
    "code": -32042,
    "message": "approval required",
    "data": {
      "elicitations": [
        {
          "mode": "url",
          "elicitationId": "d1c7a0e2-5b8f-4c3a-9e61-0f2a4b6c8d10",
          "url": "https://broker.example.com/approvals/3f2b8c1e-9a4d-4e7b-b1c2-5d6e7f8a9b0c",
          "message": "approval required"
        }
      ]
    }
  }
}
```

**Step 3 — 09:05:00: Bob's agent calls `get_user`**

The exchange returns `"principal": "bob@example.com"` and the same `agent_id`. The input document carries `"principal": "bob@example.com"`, the rule does not match, and the default allows the call. ExtProc forwards it to `directory-mcp` with `Authorization: Bearer dir_at_SYNTHETIC_BOB`. No approval is created.

**Observable result**: Alice sees an approval prompt for `get_user`. Bob's identical call goes through. Without this feature, `input.context.principal` would be undefined and the rule would match neither user.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: ExtProc MUST add `context.principal` and `context.agent_id` to the OPA input of every evaluation that runs after token exchange. That covers standalone body evaluations and every element of a JSON-RPC batch, for every `type`.
- **FR-002**: The values MUST be the `principal` and `agent_id` fields of the token-exchange response for the same request (spec 026 FR-017), passed through unmodified. ExtProc MUST NOT derive them from the subject token, a request header, or any other source.
- **FR-003**: When the token-exchange response omits a field, ExtProc MUST omit the matching input field. It MUST NOT emit an empty string or a placeholder.
- **FR-004**: Header-only evaluations (`mcp_headers_only`) MUST NOT carry either field. This feature MUST NOT change the order of OPA evaluation and token exchange.
- **FR-005**: This feature MUST NOT change the decision for any policy that does not read `context.principal` or `context.agent_id`.

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: The fields MUST come only from the broker's response to a token exchange authenticated with ExtProc's client assertion (FR-002). A caller cannot choose the identity a policy sees by forging token claims or headers (Constitution Principle I).
- **SR-002**: `context.principal` identifies a person. This feature MUST NOT add it to ExtProc's own logs, span attributes, or metrics. Masking it in OPA decision logs is OPA's responsibility, as spec 020 SR-005 already states for the Bearer token.
- **SR-003**: This feature restores what spec 020 SR-005 intended, letting policies identify the user and agent, with verified values instead of the raw token. It does not reintroduce the `Authorization` header into the input document.

### Key Entities

- **ContextInput** *(extended)*: the token-bound authorization context in the OPA input document. It gains two optional string fields, `principal` and `agent_id`. Both are recomputed for every request and never persisted.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A policy author can gate one tool for one user with a single static Rego rule, with no ExtProc instance or policy bundle per user.
- **SC-002**: Every existing OPA policy that does not reference the new fields produces the same decisions before and after this feature.
- **SC-003**: No OPA input document ever contains an empty or placeholder `principal` or `agent_id`.

## Assumptions

- The broker returns `principal` and `agent_id` on token-exchange responses (spec 026 FR-017). Brokers that predate it simply leave both fields out of the OPA input.
- No configuration, migration, or HTTP API change is needed. The change is internal to the OPA input document, which is consumed only by the embedded OPA engine.
- The input contract change is documented in `contracts/opa-input-schema-delta.md` and in an amendment to ADR 028.
