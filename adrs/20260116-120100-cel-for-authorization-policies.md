# ADR 20260116-120100: CEL (Common Expression Language) for Authorization Policies

**Status**: Accepted
**Date**: 2026-01-16
**Feature**: 20260115-120000-token-exchange (User Story 4 - Gateway Authorization via CEL)

## Context

The OAuth 2.0 Token Exchange feature (RFC 8693) requires flexible, configurable authorization policies beyond basic JWT signature validation. Specifically:

1. **Challenge**: Gateway authorization must be configurable without code changes - administrators need to express authorization rules (e.g., "allow only gateways from trusted issuers") via configuration
2. **Requirement**: Claim extraction (extracting principal and agent_client_id from subject_token) must also be configurable - different deployments may use different JWT claim names
3. **Challenge**: Authorization expressions must be secure - expressions cannot access system resources, call arbitrary functions, or bypass security controls
4. **Challenge**: Expressions must be validated at startup to fail fast (Constitution Principle I: Security-First) rather than failing during request handling

We evaluated multiple options for expression languages and ultimately selected google/cel-go as the foundation for this architecture.

## Decision

We adopt **Common Expression Language (CEL)** for:

1. **Authorization Policy Evaluation**: Validate client_assertion (gateway JWT) claims against configurable CEL expressions
2. **Claim Extraction**: Extract principal and agent_client_id from subject_token using configurable CEL expressions
3. **Implementation Library**: google/cel-go (v0.20+), a Google-maintained, production-grade CEL evaluator

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                 Configuration Layer                      │
│ (YAML: token_exchange.authorization.cel.expression)     │
└────────────────────┬────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│               CEL Evaluator (Domain)                     │
│     internal/domain/tokenexchange/cel_evaluator.go      │
└────────────────────┬────────────────────────────────────┘
                     │
        ┌────────────┴────────────┐
        │                         │
        ▼                         ▼
┌──────────────────┐      ┌──────────────────┐
│  Compile Phase   │      │  Evaluate Phase  │
│  (Startup)       │      │  (Per Request)   │
├──────────────────┤      ├──────────────────┤
│ Parse syntax     │      │ Build context:   │
│ Validate schema  │      │ - claims.*       │
│ Fail fast on     │      │ - request.*      │
│ syntax errors    │      │ Evaluate expr    │
│ Cache compiled   │      │ Return bool      │
│ programs         │      │ <100ms timeout   │
└──────────────────┘      └──────────────────┘
```

### Configuration Structure

**token_exchange configuration**:

```yaml
token_exchange:
  claim_extraction:
    # Principal extraction: default subject_token.sub
    principal_expression: "subject_token.sub"
    # Agent ID extraction: default subject_token.azp
    agent_id_expression: "subject_token.azp"

  authorization:
    # Type: "cel" or "opa" (OPA reserved for future)
    type: "cel"
    cel:
      # Gateway authorization: default "true" (allow all)
      expression: "claims.iss == 'https://upstream.example.com' && claims.aud == 'token-exchange'"

  refresh:
    enabled: true
```

### CEL Expression Context

**Authorization Expression** receives these variables:

```go
// claims: extracted from client_assertion (gateway JWT)
claims.sub         // Gateway identifier
claims.iss         // Issuer (upstream OAuth2 server)
claims.aud         // Audience
claims.exp         // Expiration timestamp
claims.iat         // Issued at timestamp
claims.{custom}    // Any custom claims in the JWT

// request: HTTP token exchange request metadata
request.resource       // Resource parameter (target service URI)
request.grant_type    // Always "urn:ietf:params:oauth:grant-type:token-exchange"
request.scope         // Optional scope parameter
```

**Examples**:

```
// Allow any gateway from trusted issuer
claims.iss == 'https://upstream.example.com'

// Allow only specific gateway
claims.sub == 'my-gateway@example.com'

// Allow based on custom claim
claims.trusted_gateway == true

// Complex: allow if issuer matches AND audience matches
claims.iss == 'https://upstream.example.com' && claims.aud == 'token-exchange'

// Time-based: deny if token issued before cutoff
claims.iat > now.seconds(cutoff_time)
```

**Claim Extraction Expressions** receive these variables:

```go
// For principal_expression
subject_token // The subject_token JWT as a map of claims

// For agent_id_expression
subject_token // The subject_token JWT as a map of claims
```

**Examples**:

```
// Default: extract from 'sub' claim
subject_token.sub

// Custom: extract from 'azp' claim (authorized party)
subject_token.azp

// Complex: extract from nested claim or use fallback
subject_token.azp || subject_token.client_id

// Custom mapping: extract from custom claim
subject_token.agent_identifier
```

### Implementation Details

**Location**: `internal/domain/tokenexchange/cel_evaluator.go`

```go
type CELEvaluator struct {
    authProgram       cel.Program  // Compiled authorization expression
    principalExpr     cel.Program  // Compiled principal extraction
    agentExpr         cel.Program  // Compiled agent_client_id extraction
    evaluationTimeout time.Duration
}

// NewCELEvaluator: Compile all expressions at startup
// Returns error if any expression has syntax errors
func NewCELEvaluator(config *TokenExchangeConfig) (*CELEvaluator, error)

// EvaluateAuthorization: Evaluate gateway authorization
// Returns (true|false, error) - false means access denied
func (e *CELEvaluator) EvaluateAuthorization(
    clientAssertionClaims map[string]interface{},
    requestContext TokenExchangeRequestContext,
) (bool, error)

// ExtractPrincipal: Extract user principal from subject_token
// Returns error if principal claim not found or expression evaluation fails
func (e *CELEvaluator) ExtractPrincipal(
    subjectTokenClaims map[string]interface{},
) (string, error)

// ExtractAgentClientID: Extract agent identifier from subject_token
// Returns error if agent claim not found or expression evaluation fails
func (e *CELEvaluator) ExtractAgentClientID(
    subjectTokenClaims map[string]interface{},
) (string, error)
```

### Security Properties

**Sandboxed Execution**:
- CEL has no access to Go's built-in functions, file system, network, or system resources
- Only library-provided functions and custom safe functions are available
- Variables passed to CEL are the only data accessible to expressions

**Fail-Closed Design**:
- CEL expressions MUST compile successfully at startup or application fails to start (Principle I: Security-First)
- If an expression evaluation fails during request handling, authorization is DENIED (fail-closed)
- All evaluation errors are logged for audit trail

**Denial-of-Service Protection**:
- 100ms evaluation timeout prevents infinite loops or runaway evaluations
- Expressions cannot call external services or perform I/O
- Resource usage (memory, CPU) is bounded by evaluation timeout

**No Custom Crypto**:
- CEL expressions cannot implement cryptographic operations
- All crypto (JWT validation, HMAC, etc.) is performed via vetted libraries outside CEL
- CEL is used ONLY for boolean logic and data transformation (Principle III: Library-First Security)

## Rationale

### Why CEL?

1. **Security**: CEL is explicitly designed for sandboxed policy evaluation - it cannot access system resources or arbitrary functions
2. **Expressiveness**: CEL supports complex boolean logic, string operations, comparisons, and conditionals
3. **Standardization**: CEL is a standard (Google Cloud's Policy Language standard), not a custom DSL
4. **Maturity**: google/cel-go is maintained by Google, battle-tested in production (Google Cloud IAM, Kubernetes policies, etc.)
5. **Simplicity**: Non-developers can understand and modify expressions (e.g., `claims.iss == 'example.com'`)

### Why Not Alternatives?

1. **JSON Path** (rejected): Insufficient expressiveness for authorization logic (cannot evaluate boolean conditions)
2. **Rego/OPA** (rejected for now): More complex to integrate, either as a library or as a seperate process but more flexible as it allows bringing context data. 
3. **Lua Scripting** (rejected): Less standardized than CEL, higher security surface (can access arbitrary libraries), not designed for sandboxed policy evaluation
4. **Custom Boolean Language** (rejected): Violates Principle II (Architecture Documentation) - non-standard, increases maintenance burden
5. **No Expression Support** (rejected): Defeats the requirement for configurable, flexible authorization (must change code for each new policy)

### Why Compile at Startup?

Per Constitution Principle I (Security-First):
- Fail-fast on invalid expressions - don't discover syntax errors during request handling
- Invalid configuration prevents application startup (not a runtime surprise)
- Ensures all expressions are valid before accepting requests
- Reduces error handling complexity in request path

## Consequences

### Positive

- **Flexible, Policy-Driven Authorization**: New authorization rules can be deployed without code changes or recompilation
- **Configurable Claim Extraction**: Different deployments can extract claims from different JWT claim names via configuration
- **Security**: CEL is sandboxed and cannot access system resources or call arbitrary functions
- **Performance**: Expressions compiled at startup and cached; evaluation typically <10ms per request (far below 100ms timeout)
- **Maintainability**: Single, well-maintained library (google/cel-go) avoids maintenance burden
- **Fail-Fast**: Invalid expressions detected at startup, not during request handling
- **Auditability**: All authorization decisions logged with expression context

### Negative

- **New Dependency**: Adds google/cel-go library (v0.20+, ~2.5MB, no transitive dependencies)
- **Learning Curve**: Operators (admins, on-call engineers) must learn CEL syntax (minimal - similar to Go expressions)
- **Expression Complexity**: Complex authorization rules may be hard to understand or debug
- **Limited Extensibility**: Adding new custom functions to CEL requires code changes (cannot be done purely via configuration)

### Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| Operator misconfigures CEL expression | Fail-fast at startup; provide clear error messages; document expression syntax in ARCHITECTURE.md |
| CEL expression has performance bug (infinite loop) | 100ms timeout prevents runaway evaluation; monitoring for slow authorizations |
| Expression accesses sensitive data unintentionally | CEL is sandboxed; no access to file system, network, or other secrets |
| Default expression "true" is too permissive | Document security implications; recommend explicit policies in production |

## Migration Path

**Current State (Before)**: Authorization is hardcoded (JWT validation + UserGrant check only)

**After ADR 20260116-120100 Implementation**:
1. Basic authorization still works: default CEL expression is `"true"` (allow all valid gateways)
2. Administrators can optionally configure stricter policies in YAML
3. Claim extraction expressions customize claim names for different upstream OAuth2 servers

**No breaking changes**: Existing configurations work without modification.

## References

- **CEL Specification**: https://github.com/google/cel-spec
- **google/cel-go Library**: https://github.com/google/cel-go
- **CEL Playground**: https://playpen.cel.dev/
- **Implementation**: `internal/domain/tokenexchange/cel_evaluator.go`
- **Configuration**: `internal/config/schema.go` (TokenExchangeConfig section)
- **Tests**: `internal/domain/tokenexchange/cel_evaluator_test.go` and `tests/e2e/token_exchange_test.go` (User Story 4 scenarios)
- **Documentation**: `docs/token-exchange.md` and `docs/configuration.md`
- **Related ADR**: ADR 20260119-120000 (Token Exchange JWKS Adapter Pattern)

## Constitution Compliance

This ADR aligns with project constitution principles:

- **Principle I (Security-First)**: CEL expressions fail-closed, validated at startup, sandboxed from system resources
- **Principle II (Architecture Documentation & ADRs)**: This ADR documents the binding CEL architecture for broker-side RFC 8693 token-exchange authorization and claim extraction. ExtProc request/tool authorization is a separate service boundary and must be documented separately.
- **Principle III (Library-First Security)**: Uses google/cel-go (vetted library), NOT custom expression language or custom policy engine
- **Principle VII (Configuration-Driven Design)**: Authorization expressions and claim extraction are purely configuration (YAML), not code changes
- **Principle VIII (Test-Driven Development)**: E2E tests written first for all CEL scenarios (User Story 4, Scenarios 1-6)

## Appendix: CEL Expression Reference

### Built-in Functions

google/cel-go provides standard functions:

```
// String operations
string.contains(s, substring)  // Check substring
string.startsWith(s, prefix)   // Check prefix
string.endsWith(s, suffix)     // Check suffix
s.toUpper()
s.toLower()

// Comparison
==, !=, <, <=, >, >=

// Logical
&&, ||, !

// Collections
list[index]
map.get(key, default_value)

// Type conversions
int(s)
string(i)
bool(s)

// Time
now  // Timestamp object
timestamp.seconds()
```

### Example Expressions

**Restrictive: Allow only specific gateways**
```cel
claims.sub in ['gateway-1@example.com', 'gateway-2@example.com']
```

**Issuer-based: Allow any gateway from trusted issuer**
```cel
claims.iss == 'https://upstream.example.com'
```

**Audience-based: Ensure token is for token-exchange endpoint**
```cel
'token-exchange' in claims.aud || claims.aud == 'token-exchange'
```

**Complex: Multiple conditions**
```cel
claims.iss == 'https://upstream.example.com' &&
claims.aud == 'token-exchange' &&
claims.exp > now.seconds()
```

**Time-based: Deny revoked gateways (issued before cutoff)**
```cel
claims.iat > 1704067200  // Tokens issued after Jan 1, 2024
```

---

**Sign-off**: Accepted by Architecture Review Board
**Implementation Date**: 2026-01-16
**Status**: Complete
