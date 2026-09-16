---
title: Changelog
description: Notable changes, breaking changes, and migration guidance for the Agentic Identity Broker.
---

# Changelog

## [NEXT VERSION] — Breaking Changes

### Third-Party Public Clients (042)

- **CHANGE:** The `Service` read schema no longer unconditionally requires `client_secret`; public services omit it because they hold no credential.

### Root-Mounted Consent SPA (038)

- **BREAKING**: The canonical consent browser paths start at `/`. The sessions page is `/sessions`. Former `/consent` paths do not receive server-side redirects.
- **Confirmation**: The user approved ADR 035 in this conversation.

### Protected Resource Subresources (035)

- **BREAKING:** Full-service `PUT /api/services/{id}` retains protected resources when
  `protected_resources` is omitted. When the field is present, including an empty set, it
  replaces the set. The request must contain the current strong `If-Match` ETag. A request
  without it returns `428`. A stale ETag returns `412` and does not change the set.
- **NEW:** Administrators can add a protected resource with
  `POST /api/services/{id}/protected-resources` and `resource_uri`. The retained idempotent
  member-addressed `PUT` can also add a resource. Dedicated service subresource endpoints
  remove, rename, and list resources.
- **Confirmation**: Stakeholder approved these API changes, including the body-based POST add operation, in this conversation before implementation.

### Multi-Agent OAuth2 Client Delegation (021)

- **BREAKING:** OAuth2 authorize and token requests now resolve `client_id` as `agent.id`
  (UUID). They do not resolve `agent.client_id` (upstream client ID). Clients must replace
  the upstream OAuth2 client ID with the internal agent UUID.
- **BREAKING:** Update token-exchange CEL `agent_id_expression` from `subject_token.azp` to
  `resolveAgentIdByClientId(subject_token.azp)`. Use a claim-based expression when the
  multi-agent feature is enabled. The previous expression resolved `client_id`. Token
  exchange now resolves `agent.id`.
- **NEW:** `multi_agent_client` under `oauth2_authorization_server` lets agents share an
  upstream OAuth2 client ID. See `examples/config/oauth2-authorization-server.yaml` and
  `docs/configuration.md`.
- **NEW:** Token-exchange CEL policies can use
  `resolveAgentIdByClientId(clientId string) string` when `multi_agent_client.enabled =
  false`. This function maps an upstream `client_id` to the broker `agent.id` UUID.

#### Migration Guide

**Step 1: Update OAuth2 `client_id` values**

OAuth2 clients must replace the upstream OAuth2 client ID in authorize and token requests
with the agent internal UUID:

```
# Before
client_id=my-upstream-app

# After
client_id=a1b2c3d4-e5f6-7890-abcd-ef1234567890  # agent.id UUID
```

**Step 2: Update token exchange CEL expression**

Operators must update `token_exchange.claim_extraction.agent_id_expression` in their configuration:

```yaml
# Before (no longer valid — resolves by client_id, not agent.id)
agent_id_expression: "subject_token.azp"

# After — feature disabled (default): resolve upstream client_id → agent.id UUID
agent_id_expression: "resolveAgentIdByClientId(subject_token.azp)"

# After — feature enabled: use the agent ID claim injected by the broker
agent_id_expression: "subject_token.x_agent_id"  # use your configured claim name
```

**Step 3 (optional): Enable multi-agent client sharing**

If you want multiple agents to share one upstream OAuth2 client ID, add the following to your configuration under `oauth2_authorization_server`:

```yaml
multi_agent_client:
  enabled: true
  agent_id_param_name: "x_agent_id"   # query param appended to upstream authorize redirect
  agent_id_claim_name: "x_agent_id"   # JWT claim in upstream token carrying the agent's internal UUID
```

No action is required if you leave `multi_agent_client.enabled = false` (default). Existing per-agent `client_id` uniqueness enforcement continues to apply.
