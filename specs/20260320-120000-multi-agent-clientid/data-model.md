# Data Model: Multi-Agent OAuth2 Client Delegation

**Feature**: 20260320-120000-multi-agent-clientid
**Date**: 2026-03-20

---

## Existing Entities (Modified)

### Agent (`internal/domain/storage/agent.go`)

The `Agent` entity gains no new fields. The **semantics** of the two identity fields are clarified:

| Field | Type | Description | Change |
|---|---|---|---|
| `ID` | `id.AgentID` (UUID) | Internal broker identifier — globally unique. This is what OAuth2 clients pass as `client_id`. | **Now used as the incoming `client_id` parameter** |
| `ClientID` | `id.ClientID` (string) | Upstream OAuth2 client ID used when proxying to the upstream server. | **Uniqueness constraint relaxed when feature is enabled** |

**Validation change**: The `ValidateForCreate()` uniqueness check on `ClientID` moves from the DB constraint to the admin API handler layer, conditioned on the `MultiAgentClient.Enabled` flag.

**Uniqueness rules**:
- `Agent.ID` (UUID) — always UNIQUE (DB primary key)
- `Agent.ClientID` — UNIQUE when `multi_agent_client.enabled = false`; shared allowed when `enabled = true`

---

## New Value Object

### MultiAgentClientConfig

**Location**: `internal/ports/config.go` nested under `OAuth2AuthServerConfig`

```go
// MultiAgentClientConfig holds configuration for multi-agent OAuth2 client sharing.
// When Enabled is true, multiple agents may share the same upstream OAuth2 client ID.
// All-or-nothing: applies to all agents when enabled.
type MultiAgentClientConfig struct {
    // Enabled enables multi-agent client sharing. Default: false.
    // When false, uniqueness of agent.ClientID is enforced at the application layer.
    Enabled bool `mapstructure:"enabled"`

    // AgentIDParamName is the query parameter name appended to the upstream
    // authorization redirect URL carrying the agent's internal ID.
    // Required when Enabled is true. Example: "x_agent_id".
    AgentIDParamName string `mapstructure:"agent_id_param_name"`

    // AgentIDClaimName is the JWT claim name expected in tokens returned by the
    // upstream server that contains the agent's internal ID.
    // Required when Enabled is true. Example: "x_agent_id".
    AgentIDClaimName string `mapstructure:"agent_id_claim_name"`
}
```

**Validation rules** (enforced at startup by `OAuth2AuthServerConfig.Validate()`):
- If `Enabled = true` and `AgentIDParamName == ""` → startup error: `oauth2_authorization_server.multi_agent_client.agent_id_param_name is required`
- If `Enabled = true` and `AgentIDClaimName == ""` → startup error: `oauth2_authorization_server.multi_agent_client.agent_id_claim_name is required`

---

## Configuration Changes (`ports/config.go`)

`OAuth2AuthServerConfig` gets one new field:

```go
type OAuth2AuthServerConfig struct {
    UpstreamIssuerURI         string                `mapstructure:"upstream_issuer_uri"`
    UpstreamAuthorizeEndpoint string                `mapstructure:"upstream_authorize_endpoint"`
    UpstreamTokenEndpoint     string                `mapstructure:"upstream_token_endpoint"`
    SupportedResponseTypes    []string              `mapstructure:"supported_response_types"`
    SupportedGrantTypes       []string              `mapstructure:"supported_grant_types"`
    UpstreamTimeout           time.Duration         `mapstructure:"upstream_timeout"`
    Mode                      string                `mapstructure:"mode"`
    // NEW: optional multi-agent client sharing configuration
    MultiAgentClient          MultiAgentClientConfig `mapstructure:"multi_agent_client"`
}
```

---

## OAuth2 Config Domain Type (`domain/oauth2/service.go`)

`OAuth2Config` (domain-layer config passed to `oauth2.Service`) gains a new field:

```go
type OAuth2Config struct {
    UpstreamAuthorizeEndpoint string
    UpstreamTokenEndpoint     string
    PublicURL                 string
    SupportedResponseTypes    []string
    SupportedGrantTypes       []string
    // NEW
    MultiAgentClient          MultiAgentClientConfig
}
```

---

## CEL Evaluator Config (`domain/tokenexchange/cel_evaluator.go`)

`CELEvaluatorConfig` gains an optional resolver function:

```go
type CELEvaluatorConfig struct {
    PrincipalExpression      string
    AgentIDExpression  string
    AuthorizationExpression  string
    EvaluationTimeout        time.Duration
    // NEW: non-nil only when multi_agent_client.enabled = false
    // Injected by app/builder.go from AgentRepository.GetByClientID.
    // When nil, resolveAgentIdByClientId is NOT registered as a CEL function.
    ResolveAgentIDByClientID func(clientID string) (agentID string, err error)
}
```

---

## New Domain Service (adapter layer)

### MultiAgentVerifier (`internal/domain/oauth2/` or adapter layer)

Responsible for claim verification at the token endpoint proxy. Can be implemented as a thin struct in the enduser handler or as an injected function:

```go
// MultiAgentTokenVerifier verifies agent ID claims in proxied token responses.
type MultiAgentTokenVerifier struct {
    Config          MultiAgentClientConfig
    AgentRepository ports.AgentRepository
    Logger          *slog.Logger
}

// VerifyTokenResponse parses a JSON token response, extracts the access_token JWT,
// parses its claims without signature verification, and verifies:
// 1. The configured agent_id_claim_name is present.
// 2. Its value matches the agent.ID that initiated the flow.
// Returns nil on success, error if claim is absent or doesn't match.
func (v *MultiAgentTokenVerifier) VerifyTokenResponse(ctx context.Context, responseBody []byte, expectedAgentID id.AgentID) error
```

---

## Database Migration

### Migration 008: Drop `client_id` UNIQUE constraint on `agents`

**File**: `migrations/008_drop_agent_client_id_unique.up.sql`
```sql
-- Drop UNIQUE constraint on agents.client_id to support multi-agent client sharing.
-- Uniqueness is now enforced at the application layer when multi_agent_client is disabled.
ALTER TABLE agents DROP CONSTRAINT IF EXISTS agents_client_id_key;
DROP INDEX IF EXISTS idx_agents_client_id;
-- Recreate index WITHOUT UNIQUE for performance (GetByClientID still used for CEL helper)
CREATE INDEX IF NOT EXISTS idx_agents_client_id ON agents(client_id);
```

**File**: `migrations/008_drop_agent_client_id_unique.down.sql`
```sql
DROP INDEX IF EXISTS idx_agents_client_id;
CREATE UNIQUE INDEX idx_agents_client_id ON agents(client_id);
ALTER TABLE agents ADD CONSTRAINT agents_client_id_key UNIQUE USING INDEX idx_agents_client_id;
```

---

## State Transitions & Flows

### Authorization Flow (feature enabled)

```
OAuth2 Client → /oauth2/authorize?client_id=<agent.ID>
   └─ Handler: id.ParseAgentID(clientID) → agentID
   └─ Service: agentRepo.Get(agentID) → agent
   └─ Service: buildUpstreamAuthorizeURL → adds ?<agent_id_param_name>=<agent.ID>
   └─ HTTP 302 redirect → upstream
```

### Token Proxy Flow (feature enabled)

```
OAuth2 Client → /oauth2/token
   └─ proxyToUpstream() → upstream token endpoint
   └─ upstream response buffered (not streamed)
   └─ MultiAgentTokenVerifier.VerifyTokenResponse()
       └─ Parse JSON body → extract access_token
       └─ jwt.ParseInsecure(access_token) → claims
       └─ claims.Get(agent_id_claim_name) → claimValue
       └─ id.ParseAgentID(claimValue) → check against expected agentID
   └─ success: write buffered response to client
   └─ failure: write OAuth2 error response (server_error)
```

### Token Exchange: resolveAgentIdByClientId (feature disabled)

```
Token Exchange: CEL expression evaluation
   └─ resolveAgentIdByClientId(subject_token.azp)
       └─ injected closure → agentRepo.GetByClientID(ctx, clientID)
       └─ returns agent.ID.String() (UUID)
   └─ id.ParseAgentID(result) → agentID
   └─ agentRepo.Get(agentID) → agent
```

---

## Glossary Additions (for ARCHITECTURE.md)

| Term | Definition |
|---|---|
| **MultiAgentClientConfig** | Configuration value object enabling multiple agents to share one upstream OAuth2 client ID. Contains feature gate (`enabled`), `agent_id_param_name`, and `agent_id_claim_name`. |
| **resolveAgentIdByClientId** | CEL helper function (registered only when feature is disabled) that maps an upstream `client_id` claim value to the broker's internal `agent.id`. Safe because client_id uniqueness is enforced in disabled mode. |
