# Quickstart: OPA-Based Authorization in ExtProc

**Branch**: `20260314-120100-extproc-opa-authorization`
**Date**: 2026-03-14

## Prerequisites

- Go 1.25+
- Existing ExtProc Token Exchange Service setup (see `specs/20260223-120000-extproc-token-exchange/`)
- Docker (for E2E tests with agentgateway)

## New Dependency

```bash
go get github.com/open-policy-agent/opa/v1@latest
```

This adds the OPA SDK. The `mcp-go` library is already a dependency.

## File Map

### New Files

```
internal/extproc/
├── authorization/
│   ├── authorizer.go           # Authorizer interface + OPAAuthorizer (wraps sdk.OPA)
│   ├── authorizer_test.go      # Unit tests for OPAAuthorizer
│   ├── input.go                # OPAInput, MCPInput, RequestInput, ContextInput types
│   ├── input_builder.go        # Builds OPAInput from protocol, body, headers
│   ├── input_builder_test.go   # Unit tests for input construction
│   ├── decision.go             # OPADecision type + parsing from sdk.DecisionResult
│   ├── decision_test.go        # Unit tests for decision parsing
│   ├── parser.go               # MCP protocol parser (JSON-RPC → MCPInput)
│   └── parser_test.go          # Unit tests for MCP parsing

tests/e2e/extproc/
├── opa_authorization_test.go   # OPA authorization E2E tests (agentgateway + MCP client)
├── fixtures/
│   └── policies/               # Test Rego policy files
│       ├── allow_readonly.rego # Allows read-only tools, denies destructive
│       ├── deny_all.rego       # Denies everything (for default-deny testing)
│       └── allow_all.rego      # Allows everything (for passthrough testing)

examples/config/
└── extproc-opa-authorization.yaml  # Example config with OPA authorization
```

### Modified Files

```
internal/extproc/
├── config/
│   ├── config.go               # Add AuthorizationConfig struct
│   ├── loader.go               # Add default values + flag registration for authorization
│   └── validate.go             # Add authorization validation rules

├── server/
│   ├── server.go               # Add Authorizer field, modify Process() for OPA flow
│   └── server_test.go          # Add unit tests for OPA-enabled request processing

cmd/extproc-token-exchange/
└── root.go                     # Initialize OPA Authorizer in startup, pass to Server

internal/extproc/AGENTS.md      # Update with authorization package documentation
```

## Implementation Steps (Bird's-Eye View)

This section is the original implementation sketch. The shipped code uses `rego.PreparedEvalQuery` for
`policy.path`, `sdk.OPA` only for `policy.config_file`, and split `NewServer` /
`NewServerWithAuthorizer` constructors. Use `internal/extproc/AGENTS.md` and
`internal/extproc/server/server.go` as the source of truth for exact current APIs.

### Step 1: Configuration

Add `AuthorizationConfig` to `config.go`:

```go
type Config struct {
    GRPC          GRPCConfig          `mapstructure:"grpc"`
    OAuth2        OAuth2Config        `mapstructure:"oauth2"`
    Cache         CacheConfig         `mapstructure:"cache"`
    Log           LogConfig           `mapstructure:"log"`
    Authorization AuthorizationConfig `mapstructure:"authorization"` // NEW
}
```

Add defaults in `loader.go`, validation in `validate.go`.

### Step 2: OPA Authorizer

Create `internal/extproc/authorization/authorizer.go`:

```go
type Authorizer interface {
    Evaluate(ctx context.Context, input *OPAInput) (*OPADecision, error)
    Stop(ctx context.Context)
}

type OPAAuthorizer struct {
    opa      *sdk.OPA
    path     string        // Decision path: /aib/extproc/authz/result
    timeout  time.Duration
    logger   *slog.Logger
}

func NewOPAAuthorizer(cfg *config.AuthorizationConfig, logger *slog.Logger) (*OPAAuthorizer, error)
```

`NewOPAAuthorizer` uses `rego.PreparedEvalQuery` when `policy.path` is set and `sdk.New()` only when `policy.config_file` is set.

### Step 3: Protocol Parser

Create `internal/extproc/authorization/parser.go`:

```go
func ParseMCPMessage(body []byte) (*MCPInput, error)
```

Uses `encoding/json` with mcp-go type definitions to parse JSON-RPC 2.0 messages. Extracts `tool_name` and `arguments` for `tools/call` method.

### Step 4: Input Builder

Create `internal/extproc/authorization/input_builder.go`:

```go
func BuildOPAInput(protocol string, body []byte, headers map[string]string) (*OPAInput, error)
```

Combines protocol detection + body parsing + header extraction into a complete `OPAInput`.

### Step 5: Server Integration

Modify `server.go` to support OPA authorization:

1. Add `authorizer Authorizer` field to `Server`
2. Add `requestState` to track per-stream state between phases:
   ```go
   type requestState struct {
       bearerToken string
       resourceURI string
       headers     map[string]string
       protocol    string
   }
   ```
3. In `processRequestHeaders`:
   - If no Bearer token → passthrough (unchanged)
   - If OPA disabled → exchange immediately (unchanged)
   - If OPA enabled → extract metadata, store state, request body buffering via `mode_override`
4. In `processRequestBody`:
   - If no OPA state → echo body (unchanged)
   - If OPA state present → parse body → build OPA input → evaluate → allow/deny

### Step 6: Startup Wiring

Modify `cmd/extproc-token-exchange/root.go`:

```go
// After creating TokenExchanger, before creating Server:
var authorizer authorization.Authorizer
if cfg.Authorization.Enabled {
    authorizer, err = authorization.NewOPAAuthorizer(&cfg.Authorization, logger)
    if err != nil {
        return fmt.Errorf("failed to initialize OPA authorizer: %w", err)
    }
    defer authorizer.Stop(ctx)
}

svc := server.NewServer(cfg, exchanger, authorizer, logger)
```

## Testing Patterns

### Unit Tests

Test `OPAAuthorizer` with inline Rego policies:

```go
func TestOPAAuthorizer_AllowDecision(t *testing.T) {
    policy := `package aib.extproc.authz
import rego.v1
default result := {"action": "allow"}
`
    authorizer := newTestAuthorizer(t, policy)
    defer authorizer.Stop(context.Background())

    input := &OPAInput{Type: "mcp_tool_call", ...}
    decision, err := authorizer.Evaluate(context.Background(), input)
    require.NoError(t, err)
    assert.Equal(t, "allow", decision.Action)
}
```

### E2E Tests (agentgateway + MCP client)

Follow existing `agentgateway_e2e_test.go` pattern:

```go
var _ = Describe("OPA Authorization via Agentgateway", Ordered, func() {
    // Setup: agentgateway Docker + ExtProc with OPA + Mock MCP server + Mock identity broker
    
    It("should allow read-only tool calls and exchange token", func() {
        result, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
            Params: mcp.CallToolParams{Name: "list_repositories"},
        })
        Expect(err).NotTo(HaveOccurred())
        // Verify tool result received + token exchanged
    })
    
    It("should deny destructive tool calls with 403", func() {
        _, err := mcpClient.CallTool(ctx, mcp.CallToolRequest{
            Params: mcp.CallToolParams{Name: "delete_repository"},
        })
        Expect(err).To(HaveOccurred())
        // Verify 403 response
    })
})
```

## Key Design Decisions

1. **Prepared query + OPA SDK**: `rego.PreparedEvalQuery` keeps local Rego path mode simple; the OPA SDK handles bundle management and other `config_file` features
2. **Protocol parser using mcp-go types**: Type-safe JSON-RPC parsing
3. **mode_override for body buffering**: Only when OPA is enabled and Bearer token present
4. **Headers-phase exchange for body-bearing requests**: proxies like agentgateway only apply header mutations from the first ExtProc response; header-only requests still exchange only after OPA allow
5. **Per-stream state**: Shared between RequestHeaders and RequestBody phases via closure or struct
6. **Authorizer interface**: Enables mock injection for unit tests
