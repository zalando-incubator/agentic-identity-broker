# Quickstart: Multi-Agent OAuth2 Client Delegation

**Feature**: 20260320-120000-multi-agent-clientid

---

## Overview

This feature allows multiple agents registered in the identity broker to delegate through the same upstream OAuth2 application (client). It introduces a breaking change in how the `client_id` OAuth2 parameter is resolved: **it now maps to the agent's internal UUID (`agent.id`)**, not the upstream client ID string.

---

## Feature Modes

### Mode 1: Feature Disabled (default)

```yaml
oauth2_authorization_server:
  upstream_issuer_uri: "https://auth.example.com"
  upstream_authorize_endpoint: "https://auth.example.com/oauth2/authorize"
  upstream_token_endpoint: "https://auth.example.com/oauth2/token"
  public_base_url: "https://identity-broker.example.com"
  # multi_agent_client not set — disabled by default

token_exchange:
  claim_extraction:
    principal_expression: "subject_token.sub"
    # resolveAgentIdByClientId maps upstream client_id → internal agent.id
    # Safe because client_id uniqueness is enforced in disabled mode.
    agent_id_expression: "resolveAgentIdByClientId(subject_token.azp)"
```

**Behavior**:
- Agents MUST have unique `client_id` values
- OAuth2 `client_id` parameter resolved against `agent.id` (UUID)
- Upstream proxy uses `agent.client_id` when calling the upstream server
- `resolveAgentIdByClientId` CEL function available in token exchange policies

### Mode 2: Feature Enabled

```yaml
oauth2_authorization_server:
  upstream_issuer_uri: "https://auth.example.com"
  upstream_authorize_endpoint: "https://auth.example.com/oauth2/authorize"
  upstream_token_endpoint: "https://auth.example.com/oauth2/token"
  public_base_url: "https://identity-broker.example.com"
  multi_agent_client:
    enabled: true
    agent_id_param_name: "x_agent_id"   # parameter added to authorize redirect URL
    agent_id_claim_name: "x_agent_id"   # claim expected in minted tokens

token_exchange:
  claim_extraction:
    principal_expression: "subject_token.sub"
    # Read agent ID directly from the token claim (configured name)
    agent_id_expression: "subject_token.x_agent_id"
```

**Behavior**:
- Multiple agents may share the same `client_id`
- Each authorize redirect to upstream includes `?x_agent_id=<agent.id>`
- Upstream must embed agent ID as `x_agent_id` claim in minted tokens
- Token proxy verifies `x_agent_id` claim before returning token to client
- `resolveAgentIdByClientId` CEL function NOT available (not registered)

---

## Migration Guide for Existing Setups

### Step 1: Update OAuth2 clients

All OAuth2 clients must change their `client_id` parameter:
- **Before**: `client_id=my-upstream-client-id`
- **After**: `client_id=<agent-uuid>` (the `id` field from `GET /api/agents`)

Retrieve your agent's UUID:
```bash
curl -s https://broker.example.com/api/agents | jq '.[].id'
```

### Step 2: Update token exchange CEL policy

If using token exchange, update `agent_id_expression`:
```yaml
# Before
agent_id_expression: "subject_token.azp"

# After (feature disabled)
agent_id_expression: "resolveAgentIdByClientId(subject_token.azp)"
```

### Step 3: Configure upstream OAuth2 server (if enabling the feature)

Configure your upstream OAuth2 server to:
1. Accept an extra query parameter on the authorize endpoint (e.g., `x_agent_id`)
2. Embed that parameter as a claim in minted access tokens

---

## Registering Multiple Agents with a Shared Client

```bash
# Create agent 1 (shared upstream client)
curl -X POST https://broker.example.com/api/agents \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "shared-upstream-client",
    "display_name": "Research Agent",
    "description": "AI research assistant"
  }'
# Returns: {"id": "a1b2c3d4-...", "client_id": "shared-upstream-client", ...}

# Create agent 2 (same upstream client — only valid when feature is enabled)
curl -X POST https://broker.example.com/api/agents \
  -H "Content-Type: application/json" \
  -d '{
    "client_id": "shared-upstream-client",
    "display_name": "Data Agent",
    "description": "AI data analyst"
  }'
# Returns: {"id": "b2c3d4e5-...", "client_id": "shared-upstream-client", ...}
```

---

## Audit Log Events

When the feature is enabled, the following structured log events are emitted:

| Event | Level | Key Fields |
|---|---|---|
| `AgentIDParamInjected` | Info | `agent_id`, `param_name`, `upstream_url` |
| `AgentIDClaimVerified` | Info | `agent_id`, `claim_name`, `claim_value` |
| `AgentIDClaimMissing` | Error | `agent_id`, `claim_name`, `error` |
| `AgentIDClaimMismatch` | Error | `expected_agent_id`, `claim_value`, `claim_name` |

---

## Testing

```bash
# Run all tests (fast/package + integration + E2E)
just verify

# Run only multi-agent E2E tests
ginkgo -v --label-filter="multi-agent" ./tests/e2e/...
```
