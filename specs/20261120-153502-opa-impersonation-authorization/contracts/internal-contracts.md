# Internal contracts: policy source port, admission, and revision use

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Research**: R1, R2, R5, R6,
R8, R10, R11 | **Data model**: [../data-model.md](../data-model.md)

These seams are internal and need no sign-off by themselves. Their visible effects are in
[impersonation-opa-config.yaml](impersonation-opa-config.yaml),
[policy-input-decision.md](policy-input-decision.md), [observability.md](observability.md), and
[openapi-token-opa.yaml](openapi-token-opa.yaml).

## 1. Configuration types (`internal/ports/config.go`)

```go
// ImpersonationRuleConfig.Authorization changes type; the YAML key is unchanged.
Authorization ImpersonationAuthorizationConfig `mapstructure:"authorization"`

type ImpersonationAuthorizationConfig struct {
	Type string                         `mapstructure:"type"` // "cel" or "opa"
	CEL  CELAuthorizationConfig         `mapstructure:"cel"`
	OPA  ImpersonationOPAAuthorization  `mapstructure:"opa"`
}

type ImpersonationOPAAuthorization struct {
	Policy            ImpersonationOPAPolicy `mapstructure:"policy"`
	EvaluationTimeout time.Duration          `mapstructure:"evaluation_timeout"`
	StartupTimeout    time.Duration          `mapstructure:"startup_timeout"`
}

type ImpersonationOPAPolicy struct {
	ConfigFile string `mapstructure:"config_file"`
	Package    string `mapstructure:"package"`
	Decision   string `mapstructure:"decision"`
}
```

`AuthorizationConfig` and `OPAAuthorizationConfig` (used by `token_exchange.authorization`) are
unchanged. `internal/config/validator.go` and `impersonation.validateRuleInvariants` both enforce
the rules of the configuration contract (defense in depth, as today).

## 2. Port (`internal/ports/impersonation_policy.go`, new)

```go
// ImpersonationPolicyRole names a credential map in the policy context.
type ImpersonationPolicyRole string // "client_assertion", "actor_token", "subject_token"

// ImpersonationPolicyEvidence is the binding evidence of one policy revision.
type ImpersonationPolicyEvidence struct {
	References    map[ImpersonationPolicyRole]bool
	FieldBindings map[ImpersonationPolicyRole]map[string]bool // only "sub" and "email" are recorded
}

type ImpersonationPolicyAction string // "allow" or "deny"

// ImpersonationPolicyInput is the complete, credential-free input document (contract § 1).
type ImpersonationPolicyInput map[string]any

// ImpersonationPolicyRevision is immutable and safe for concurrent use.
type ImpersonationPolicyRevision interface {
	Evidence() ImpersonationPolicyEvidence
	Label() string // policy_revision for telemetry
	// Evaluate returns allow or deny, or an error wrapping ErrPolicyDecisionInvalid or
	// ErrPolicyEvaluationFailed. It honors ctx cancellation and its own deadline.
	Evaluate(ctx context.Context, input ImpersonationPolicyInput) (ImpersonationPolicyAction, error)
}

// ImpersonationPolicySource owns one rule's runtime.
type ImpersonationPolicySource interface {
	// AwaitReady blocks until a revision is admitted and every bundle activated once, or returns an
	// error at the configured startup bound or ctx end.
	AwaitReady(ctx context.Context) error
	// Current returns the active revision; non-nil after AwaitReady succeeds.
	Current() ImpersonationPolicyRevision
	Stop(ctx context.Context)
}

// ImpersonationPolicyAdmission is supplied by the domain and called for every candidate revision.
type ImpersonationPolicyAdmission func(ImpersonationPolicyEvidence) error

var (
	ErrPolicyDecisionInvalid  = errors.New("impersonation policy decision invalid")
	ErrPolicyEvaluationFailed = errors.New("impersonation policy evaluation failed")
)
```

## 3. Domain (`internal/domain/impersonation/`)

```go
// PolicySourceFactory starts one OPA rule's runtime without blocking on readiness.
type PolicySourceFactory func(ruleName string, cfg ports.ImpersonationOPAAuthorization,
	admit ports.ImpersonationPolicyAdmission) (ports.ImpersonationPolicySource, error)
```

- `NewService` gains a `PolicySourceFactory` parameter. A nil factory with an OPA rule fails startup.
  It starts every OPA rule's source, then calls `AwaitReady` concurrently with each rule's bound; on any
  failure it stops every started source and returns the indexed rule error.
- `Service.Close(ctx)` stops every source. The builder registers it with the existing shutdown path.
- `bindingRequirements(cfg) []ports.ImpersonationPolicyRole` and `admit(requirements)` implement
  [policy-input-decision.md § 4](policy-input-decision.md#4-binding-evidence-dec-bind). The CEL
  compile path applies the same admission to `authorizationProgram` evidence, keeping today's error
  text (`requires the authorization predicate to reference subject_token (FR-007a)` and the CR-009
  message).
- `compiledRule.authz` becomes an internal interface `authorizer { current() authorization }`, where
  `authorization` exposes `evidence()`, `label()`, `engine()`, and `evaluate(ctx, inputs)`. CEL returns a
  single static `authorization`; OPA wraps `Current()`.
- `evaluateRule` reads `auth := rule.authz.current()` once, before classification, and uses `auth`
  for the `sub` binding (`classifySubject`), the `email` binding, and the decision. The pipeline order
  is unchanged.
- `Request` gains `HTTP RequestAttributes{Method, Path, RawQuery, Protocol string; Headers map[string][]string}`
  set by the handler. The domain builds the input (contract § 1) only for OPA rules, using
  `server.enduser.public_url` passed to `NewService`.
- Error mapping: `ErrPolicyEvaluationFailed` → `serverError("authorization evaluation failed")` with
  `client_authorization/opa_evaluation_failed`; `ErrPolicyDecisionInvalid` → same response with
  `client_authorization/opa_decision_invalid`; both are `abort` results.
- `AuditRecord` gains `AuthorizationEngine` and `PolicyRevision`.

## 4. Adapter (`internal/adapters/policy/opa/`, new)

- `NewImpersonationPolicySource(ruleName, cfg, admit, logger)` reads and validates the
  bootstrap subset, creates an in-memory store with an `OnCommit` trigger, and starts `sdk.New` with
  `Ready`, `V1Compatible`, discard console logger, and no plugins. It obtains its tracer and meter
  from `otel.Tracer("tokenexchange")` and `otel.Meter("tokenexchange")`, never constructor-injected
  providers, as required by `coding-standards/OPENTELEMETRY.md` § 1.
- The trigger builds candidates (research R2), computes capabilities (R4) and evidence (R5), calls
  `admit`, and swaps an `atomic.Pointer[revision]`. Trigger failures never panic the broker; they
  reject the candidate.
- `revision.Evaluate` converts input with `ast.InterfaceToValue`, applies `context.WithTimeout`
  (`evaluation_timeout`), runs the prepared query, and parses the decision (contract § 3).
- The adapter imports no other adapter and nothing from `internal/extproc`.

## 5. Builder and handler

- `internal/app/builder.go` constructs the factory with the injected logger and passes
  it to `impersonation.NewService`.
- `internal/adapters/http/enduser/oauth2_token.go` fills `Request.HTTP` from the `*http.Request` and
  logs the two new audit attributes. It does not inspect policy results.
