# Research: Multi-Agent OAuth2 Client Delegation

**Feature**: 20260320-120000-multi-agent-clientid
**Date**: 2026-03-20

---

## 1. CEL Custom Function Registration with Repository Access

### Decision
Inject a `func(string) string` closure into `CELEvaluatorConfig` for the `resolveAgentIdByClientId` lookup. The closure is wired by `app/builder.go` from the `AgentRepository`, keeping the domain layer free of infrastructure imports.

### Pattern (cel-go v0.27.0)

```go
// In CELEvaluatorConfig (domain/tokenexchange/cel_evaluator.go)
type CELEvaluatorConfig struct {
    // ...existing fields...
    // ResolveAgentIDByClientID is injected when feature is disabled.
    // nil means feature is enabled; function must NOT be registered.
    ResolveAgentIDByClientID func(clientID string) (agentID string, err error)
}

// In compileExpression() — extend cel.NewEnv() options when function is non-nil:
cel.Function("resolveAgentIdByClientId",
    cel.Overload(
        "resolveAgentIdByClientId_string",
        []*cel.Type{cel.StringType},
        cel.StringType,
        cel.UnaryBinding(func(v ref.Val) ref.Val {
            clientID, ok := v.(types.String)
            if !ok {
                return types.NewErr("resolveAgentIdByClientId: expected string")
            }
            agentID, err := e.config.ResolveAgentIDByClientID(string(clientID))
            if err != nil {
                return types.NewErr("resolveAgentIdByClientId: %v", err)
            }
            return types.String(agentID)
        }),
    ),
)
```

### Builder wiring (`app/builder.go`)

```go
var resolverFn func(string) (string, error)
if !cfg.OAuth2AuthServer.MultiAgentClient.Enabled {
    resolverFn = func(clientID string) (string, error) {
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        agent, err := agentRepo.GetByClientID(ctx, id.ClientID(clientID))
        if err != nil {
            return "", err
        }
        return agent.ID.String(), nil
    }
}
celConfig := tokenexchange.CELEvaluatorConfig{
    // ...
    ResolveAgentIDByClientID: resolverFn,
}
```

### Rationale
- Domain layer (tokenexchange/) stays free of `ports/` storage dependencies — only `func` is injected.
- CEL evaluation is synchronous (100ms timeout goroutine handles the timeout boundary already).
- nil/non-nil on the injected func is the feature-mode gate — no extra flag needed.

### Alternatives Considered
- **Pre-built lookup map at startup**: Stale if agents are added/removed at runtime; rejected.
- **Async CEL extension**: Not supported by cel-go; rejected.

---

## 2. JWT Claim Parsing Without Signature Verification

### Decision
Use `jwt.ParseInsecure([]byte(tokenString))` from `github.com/lestrrat-go/jwx/v3/jwt`.

### API

```go
import "github.com/lestrrat-go/jwx/v3/jwt"

token, err := jwt.ParseInsecure([]byte(accessTokenString))
if err != nil {
    // malformed JWT — return server_error
}
var claimValue string
if err := token.Get(agentIDClaimName, &claimValue); err != nil {
    // claim absent — return error per FR-004
}
```

`jwt.ParseInsecure` is semantically self-documenting (no verification intent). Equivalent alternative: `jwt.ParseString(s, jwt.WithVerify(false), jwt.WithValidate(false))`.

### Rationale
- The broker is a proxy, not the resource server; per spec and out-of-scope decision, JWT signature validation is the token consumer's responsibility.
- No JWKS lookup needed for this path, keeping the token endpoint latency low.

---

## 3. Upstream Authorize URL Injection Point

### Decision
Modify `buildUpstreamAuthorizeURL()` in `internal/domain/oauth2/service.go` to accept the agent ID and multi-agent config, adding the parameter to the query string.

### Location
`internal/domain/oauth2/service.go`, `buildUpstreamAuthorizeURL()` (~line 156).
Injection point: after all standard params are set, before `u.RawQuery = q.Encode()`.

```go
if s.config.MultiAgentClient.Enabled {
    q.Set(s.config.MultiAgentClient.AgentIDParamName, agent.ID.String())
}
u.RawQuery = q.Encode()
```

The `agent` is already looked up before this call (via `Get(agentID)`) so its ID is available.

### Rationale
Pure domain layer change; no adapter modifications needed for URL building.

---

## 4. Token Response Claim Verification Injection Point

### Decision
Modify `proxyToUpstream()` in `internal/adapters/http/enduser/oauth2_token.go` to buffer the response body, parse the access_token JWT, and verify the agent ID claim before forwarding.

### Location
`internal/adapters/http/enduser/oauth2_token.go`, `proxyToUpstream()` (~line 214).
Replace `io.Copy(w, upstreamResp.Body)` (line 261) with buffered read + optional claim check.

### Dependency injection
Add a `MultiAgentVerifier` interface (or func field) on `OAuth2TokenHandler`:

```go
type MultiAgentVerifier interface {
    VerifyAgentIDClaim(ctx context.Context, tokenJSON []byte, agentID id.AgentID) error
    Enabled() bool
}
```

This keeps the handler aligned with hexagonal architecture — verification logic is in a domain service or domain helper, injected via the handler struct.

### Rationale
- Adapter layer cannot contain business logic. Verification logic lives in domain.
- Interface injection allows testing without a running upstream.

---

## 5. Breaking Change: Agent Lookup by Internal ID

### Decision
Change all agent resolution paths from `GetByClientID(clientID)` to `Get(agentID)`. The incoming OAuth2 `client_id` parameter is now expected to be the agent's internal UUID.

### Affected files
| File | Change |
|---|---|
| `domain/oauth2/service.go` | Replace `GetByClientID(req.ClientID)` with `id.ParseAgentID(string(req.ClientID))` + `Get(agentID)` |
| `domain/tokenexchange/service.go:227` | Replace `GetByClientID(id.NewClientID(agentClientID))` with `id.ParseAgentID(agentClientID)` + `Get(agentID)` |

UUID parse errors return `invalid_client` (401/400) for the authorize endpoint and token exchange error for token exchange.

---

## 6. Client_ID Uniqueness Constraint

### Decision
Drop the `UNIQUE` constraint on `agents.client_id` in the DB via migration `008_drop_agent_client_id_unique.up.sql`. Uniqueness in disabled mode is enforced at the admin API handler layer via a pre-check lookup.

### Why application-layer not DB-layer
- Feature mode is determined by config, not DB state; cannot use DB constraints conditionally.
- The existing memory adapter already enforces uniqueness programmatically — aligning postgres to the same pattern.

### Admin handler change
```go
if !multiAgentEnabled {
    existing, err := agentRepo.GetByClientID(ctx, clientID)
    if err == nil && existing != nil && existing.ID != agent.ID { // for updates
        return 409 Conflict "client_id already in use"
    }
}
```

---

## 7. Token Exchange: Agent ID Resolution Flow

### Feature disabled mode (default)
CEL expression: `resolveAgentIdByClientId(subject_token.azp)` → returns agent.id (UUID string)
Token exchange service: `Get(id.ParseAgentID(agentClientID))` ← **UUID lookup**

### Feature enabled mode
CEL expression: `subject_token.x_agent_id` → returns agent.id (UUID string)
Token exchange service: `Get(id.ParseAgentID(agentClientID))` ← **same UUID lookup**

Both modes resolve to the same `Get(agentID)` call. The only difference is how the CEL expression yields the UUID.

---

## Summary

| Unknown | Decision |
|---|---|
| CEL custom function with DB access | Inject `func(string)(string,error)` closure via `CELEvaluatorConfig` |
| JWT claim parsing without signature | `jwt.ParseInsecure([]byte(tokenString))` |
| Authorize URL injection point | `buildUpstreamAuthorizeURL()`, q.Set before q.Encode() |
| Token response verification | Buffer response in `proxyToUpstream()`, inject `MultiAgentVerifier` |
| Agent lookup breaking change | `id.ParseAgentID()` + `Get()` in both authorize and token exchange |
| Client_id uniqueness | Drop DB UNIQUE constraint; enforce at admin handler layer when disabled |
