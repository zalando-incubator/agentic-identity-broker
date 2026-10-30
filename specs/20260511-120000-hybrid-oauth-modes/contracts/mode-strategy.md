# Contracts: Hybrid OAuth Server Modes

## Configuration Contract

The OAuth2 authorization server configuration uses nested sections per mode.

### YAML Structure

```yaml
oauth2_authorization_server:
  mode: "proxy" | "local" | "hybrid"  # required, no default

  proxy:                               # required in proxy + hybrid modes
    upstream_issuer_uri: string
    upstream_authorize_endpoint: string
    upstream_token_endpoint: string
    upstream_timeout: duration          # default: "30s"

  local:                               # required in local + hybrid modes
    token_ttl: duration                 # default: "1h"
    token_claims_expression: string
    supported_grant_types: []string     # default: ["authorization_code"]

  cimd:                                # optional, forbidden in proxy mode
    enabled: bool
    # ... existing CIMD config unchanged
```

### Validation Rules

| Mode | `proxy` section | `local` section | `cimd` section |
|------|----------------|-----------------|----------------|
| `proxy` | Required | Forbidden | Forbidden |
| `local` | Forbidden | Required | Optional |
| `hybrid` | Required | Required | Optional |
| `issue_token` | Error: "renamed to 'local'" | — | — |

## Mode Strategy (Domain Layer)

Domain logic determining whether a classified agent is permitted. Not a port — all implementations are internal domain code in `internal/domain/oauth2/`.

```go
type ClientMode int

const (
    ProxyClient ClientMode = iota  // Agent.ClientID set
    CIMDClient                         // client_uris set, no ClientID
    LocalClient                        // neither set
)

type ModeStrategy interface {
    AcceptsClientMode(mode ClientMode) bool
    Name() OAuthServerMode
}
```

### Mode Strategy Behavior

| Mode | ProxyClient | CIMDClient | LocalClient |
|------|-----------|----------------|-----------------|
| `proxy` | Accept | Reject | Reject |
| `local` | Reject | Accept | Accept |
| `hybrid` | Accept | Accept | Accept |

## Dispatching Strategies (Adapter Layer)

For hybrid mode, the builder wires dispatching `AuthorizationProceedStrategy` and `TokenGrantStrategy` implementations in `internal/adapters/http/enduser/`. These wrap both proxy and local strategies and delegate based on `agent.ClientMode()`. Domain code is not involved in dispatch.

## Agent Resolution Contract

Resolution uses the request `client_id` format:

| Request `client_id` format | Resolution method | Post-resolution check |
|---------------------------|-------------------|----------------------|
| Valid URL | `AgentRepository.GetByClientURI(uri)` | None |
| Valid UUID | `AgentRepository.Get(id)` | If agent has `client_uris` → reject |
| Other | Reject: invalid format | — |

The upstream `Agent.ClientID` is never used for resolution and never exposed to relying parties.
