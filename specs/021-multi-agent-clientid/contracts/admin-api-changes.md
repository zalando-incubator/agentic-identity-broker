# API Contract Changes: Multi-Agent OAuth2 Client Delegation

**Feature**: 021-multi-agent-clientid
**Date**: 2026-03-20
**Stakeholder confirmation**: In spec (breaking change acknowledged, no migration shim)

---

## Breaking Changes Summary

This feature introduces two breaking API changes:

1. **OAuth2 `client_id` parameter semantics change** (enduser OAuth2 API)
2. **Agent `client_id` uniqueness relaxation** (admin API)

Both changes have been explicitly approved in the feature spec and clarifications.

---

## 1. Enduser OAuth2 API — `client_id` Parameter Semantics

### Affected Endpoints
- `GET /oauth2/authorize` (end-user server :8000)
- `POST /oauth2/token` (end-user server :8000, standard grant types)

### Change Description

**Before**: The `client_id` parameter was matched against `agent.client_id` (the upstream OAuth2 client ID string, e.g., `"my-app"`).

**After**: The `client_id` parameter MUST be the agent's internal UUID (`agent.id`), e.g., `"a1b2c3d4-e5f6-..."`).

### OpenAPI Impact

Update `/api/enduser/openapi.yaml` parameter description for `client_id` on the authorize endpoint:

```yaml
# Before
- name: client_id
  description: "The OAuth2 client ID registered with the identity broker"

# After
- name: client_id
  description: >
    The internal agent identifier (UUID) registered with the identity broker.
    This MUST be the agent's `id` field, NOT the upstream OAuth2 `client_id`.
    Example: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
```

### Migration Impact
OAuth2 clients integrating with the broker must update their `client_id` parameter from `agent.client_id` to `agent.id`. No migration shim is provided.

---

## 2. Admin API — Agent `client_id` Uniqueness

### Affected Endpoints
- `POST /api/agents` (admin server :14000)
- `PUT /api/agents/{agent-id}` (admin server :14000)

### Change Description

**Before**: Creating two agents with the same `client_id` returned `409 Conflict`.

**After (feature disabled, default)**: Same behavior — `409 Conflict` returned when a duplicate `client_id` is detected.

**After (feature enabled)**: Multiple agents MAY share the same `client_id`. The `409 Conflict` for duplicate `client_id` is NOT returned.

### OpenAPI Impact

Update agent creation/update documentation in `/api/admin/openapi.yaml`:

```yaml
# POST /api/agents request body description update
description: >
  Register a new AI agent in the identity broker system.

  The `client_id` field is the upstream OAuth2 client ID used when proxying
  authorization requests to the upstream server.

  When `multi_agent_client.enabled = false` (default): `client_id` must be
  unique across all agents (409 Conflict if duplicate).

  When `multi_agent_client.enabled = true`: Multiple agents may share the
  same `client_id` (no uniqueness validation).
```

---

## 3. New Configuration Block

No new API endpoints. The `multi_agent_client` block is added to the YAML configuration under `oauth2_authorization_server`.

This is a **non-breaking addition** to the configuration schema — existing configs continue to work with `multi_agent_client.enabled = false` (implicit default).

---

## 4. Token Exchange CEL Policy (Operator-facing, not REST API)

### Affected Configuration
`token_exchange.claim_extraction.agent_id_expression` in YAML config.

### Change Description

**Feature disabled (recommended for existing operators)**:
```yaml
agent_id_expression: "resolveAgentIdByClientId(subject_token.azp)"
```
The new `resolveAgentIdByClientId` CEL function maps upstream `client_id` → internal `agent.id`.

**Feature enabled**:
```yaml
agent_id_expression: "subject_token.x_agent_id"  # use configured claim name
```

**Previous behavior (now invalid)**:
```yaml
agent_id_expression: "subject_token.azp"  # Was looking up by client_id directly
```

The previous expression using `subject_token.azp` directly is no longer valid because the token exchange service now looks up agents by `agent.id` (UUID), not `agent.client_id`.

### Migration
Operators using the default `subject_token.azp` expression MUST update to:
- `resolveAgentIdByClientId(subject_token.azp)` if keeping feature disabled
- OR `subject_token.<claim_name>` if enabling the feature

---

## 5. Error Response Changes

### New error scenario (feature enabled, token endpoint)

When the upstream token endpoint returns a token lacking the expected `agent_id_claim_name` claim, the broker now returns an OAuth2 error instead of forwarding the token:

```json
{
    "error": "server_error",
    "error_description": "upstream token missing required agent ID claim"
}
```

HTTP status: `500 Internal Server Error`

---

## Compatibility Changes

- **BREAKING**: The `client_id` parameter in OAuth2 authorize/token requests now resolves to `agent.id` (UUID), not `agent.client_id` (upstream client ID). Clients must update their `client_id` values.
- **BREAKING**: Token exchange CEL expression `agent_id_expression` must be updated from `subject_token.azp` to `resolveAgentIdByClientId(subject_token.azp)` (or similar claim-based expression when feature is enabled).
- **NEW**: `multi_agent_client` configuration block under `oauth2_authorization_server` enables multiple agents to share one upstream OAuth2 client ID.
- **NEW**: CEL helper function `resolveAgentIdByClientId(clientId)` available when feature is disabled for token exchange CEL policies.
