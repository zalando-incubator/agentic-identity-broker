# Internal contracts: policy source port, admission, and revision use

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Research**: R1, R2, R5, R6,
R8, R10, R11, R15, R16 | **Data model**: [../data-model.md](../data-model.md)

These seams are internal and need no sign-off by themselves. Their visible effects are in
[impersonation-opa-config.yaml](impersonation-opa-config.yaml),
[policy-input-decision.md](policy-input-decision.md), [observability.md](observability.md), and
[openapi-token-opa.yaml](openapi-token-opa.yaml).

## 1. Configuration types (`internal/ports/config.go`)

```go
// ImpersonationConfig gains the shared engine block.
OPA ImpersonationOPAEngineConfig `mapstructure:"opa"`

type ImpersonationOPAEngineConfig struct {
	Policy         ImpersonationOPAPolicySource `mapstructure:"policy"`
	StartupTimeout time.Duration                `mapstructure:"startup_timeout"`
}

// Exactly one member is set.
type ImpersonationOPAPolicySource struct {
	ConfigFile string `mapstructure:"config_file"`
	Path       string `mapstructure:"path"`
}

// ImpersonationRuleConfig.Authorization changes type; the YAML key is unchanged.
Authorization ImpersonationAuthorizationConfig `mapstructure:"authorization"`

type ImpersonationAuthorizationConfig struct {
	Type string                        `mapstructure:"type"` // "cel" or "opa"
	CEL  CELAuthorizationConfig        `mapstructure:"cel"`
	OPA  ImpersonationRuleOPADecision  `mapstructure:"opa"`
}

type ImpersonationRuleOPADecision struct {
	Package           string        `mapstructure:"package"`  // default "aib.broker.impersonation"
	Decision          string        `mapstructure:"decision"` // default "result"
	EvaluationTimeout time.Duration `mapstructure:"evaluation_timeout"`
}
```

The loader rejects every key under `rules[<i>].authorization.opa` other than the three above,
naming the key. This covers the former reserved `policy_url`/`policy_path` declarations and the
engine settings moved to `impersonation.opa`. It reads the raw rule map for this check, because
the typed decode drops unknown keys.

`AuthorizationConfig` and `OPAAuthorizationConfig` (used by `token_exchange.authorization`) are
unchanged as declarations, but the loader and ordinary-exchange service construction reject explicit
`token_exchange.authorization.type: opa` before serving requests (API-CFG). `internal/config/validator.go` and `impersonation.validateRuleInvariants` both enforce
the rules of the configuration contract (defense in depth, as today).

## 2. Port (`internal/ports/impersonation_policy.go`, new)

```go
// ImpersonationPolicyRole names a credential map in the policy context.
type ImpersonationPolicyRole string // "client_assertion", "actor_token", "subject_token"

// ImpersonationPolicyEvidence is the binding evidence of one policy revision.
type ImpersonationPolicyEvidence struct {
	References    map[ImpersonationPolicyRole]bool
	FieldBindings map[ImpersonationPolicyRole]map[string]bool // subject_token only: "sub", "email"
}

type ImpersonationPolicyAction string // "allow" or "deny"

// ImpersonationPolicyInput excludes raw credentials but contains sensitive identity claims.
type ImpersonationPolicyInput map[string]any

// Implement LogValue() slog.Value, String() string, and GoString() string on this type.
// Every method returns only the constant "[REDACTED]", including empty/nil values.

// ImpersonationPolicyDecisionRef names one rule's decision in the shared engine.
type ImpersonationPolicyDecisionRef struct {
	Rule              string // rule name, unique per configuration
	Package           string
	Decision          string
	EvaluationTimeout time.Duration
}

// ImpersonationPolicyRevision is immutable and safe for concurrent use. It holds one prepared
// query and one evidence set per decision reference, all built from the same snapshot.
type ImpersonationPolicyRevision interface {
	Evidence(rule string) ImpersonationPolicyEvidence
	Label() string       // operator manifest label, log/audit only
	AdmissionID() string // process-instance UUID plus engine admission sequence, log/audit only
	// Evaluate runs the rule's decision and returns allow or deny, or an error wrapping
	// ErrPolicyDecisionInvalid or ErrPolicyEvaluationFailed. It honors ctx cancellation and the
	// rule's own deadline.
	Evaluate(ctx context.Context, rule string, input ImpersonationPolicyInput) (ImpersonationPolicyAction, error)
}

// ImpersonationPolicySource owns the one shared OPA engine.
type ImpersonationPolicySource interface {
	// AwaitReady blocks until the engine is ready and the active admitted revision contains every
	// configured bundle in its own snapshot, or returns at the startup bound or ctx end.
	AwaitReady(ctx context.Context) error
	// Current returns the active revision; non-nil after AwaitReady succeeds.
	Current() ImpersonationPolicyRevision
	Stop(ctx context.Context)
}

// ImpersonationPolicyAdmission is supplied by the domain and called once per candidate revision
// with the evidence of every decision reference. A non-nil error rejects the whole candidate;
// ImpersonationPolicyAdmissionError names the first failing rule for diagnostics.
type ImpersonationPolicyAdmission func(evidence map[string]ImpersonationPolicyEvidence) error

type ImpersonationPolicyAdmissionError struct {
	Rule string
	Err  error
}

var (
	ErrPolicyDecisionInvalid  = errors.New("impersonation policy decision invalid")
	ErrPolicyEvaluationFailed = errors.New("impersonation policy evaluation failed")
)
```

## 3. Domain (`internal/domain/impersonation/`)

```go
// PolicySourceFactory starts the shared OPA engine without blocking on readiness.
type PolicySourceFactory func(cfg ports.ImpersonationOPAEngineConfig,
	decisions []ports.ImpersonationPolicyDecisionRef,
	admit ports.ImpersonationPolicyAdmission) (ports.ImpersonationPolicySource, error)
```

- `NewService` gains a `PolicySourceFactory` parameter. A nil factory with an OPA rule fails startup.
  With at least one OPA rule it starts one source with every OPA rule's decision reference, then
  calls `AwaitReady` with `impersonation.opa.startup_timeout`; on failure it stops the source and
  returns the engine error naming the last rejecting rule. With no OPA rule it starts nothing.
- `Service.Close(ctx)` stops the source. The builder registers it with the existing shutdown path.
- `bindingRequirements(cfg) []ports.ImpersonationPolicyRole` and `admit(requirements)` implement
  the per-rule check of [policy-input-decision.md § 4](policy-input-decision.md#4-binding-evidence-dec-bind).
  The OPA admission function applies it to every OPA rule's evidence in rule order and rejects the
  candidate at the first rule that fails (all-or-nothing, R11). The CEL
  compile path applies the same admission to `authorizationProgram` evidence, keeping today's error
  text (`requires the authorization predicate to reference subject_token (FR-007a)` and the CR-009
  message).
- `compiledRule` gets an internal `authorization` (`evidence()`, `label()`, `engine()`,
  `evaluate(ctx, inputs)`): CEL rules hold a static one; OPA rules obtain theirs per request from the
  revision read by `Impersonate`.
- `Impersonate` reads `rev := source.Current()` once, before the first rule, and passes it to every
  `evaluateRule` call of that request. An OPA rule's `authorization` is `rev` bound to the rule name;
  a CEL rule's is static. `evaluateRule` uses that one `authorization` for the `sub` binding
  (`classifySubject`), the `email` binding, and the decision. All OPA rules of one request therefore
  see the same policy and data. The pipeline order is unchanged.
- `Request` gains `HTTP RequestAttributes{Method, Path, RawQuery, Protocol string; Headers map[string][]string}`
  set by the handler. The domain builds the input (contract § 1) only for OPA rules, using
  `server.enduser.public_url` passed to `NewService`.
- Error mapping: `ErrPolicyEvaluationFailed` → `serverError("authorization evaluation failed")` with
  `client_authorization/opa_evaluation_failed`; `ErrPolicyDecisionInvalid` → same response with
  `client_authorization/opa_decision_invalid`; both are `abort` results.
- `AuditRecord` gains `AuthorizationEngine`, `PolicyRevision`, and `PolicyAdmissionID`.
- `input.type` is the constant `"impersonation"`. `context.agent` is copied from the resolved
  broker Agent as `{id, canonical_id?, external_id?}`, the ExtProc Grant Context agent shape
  plus `external_id`. Unset optional IDs are omitted, never `null` or `""`. The values are
  independent of credential claims, caller headers, and bundle mappings. A missing External ID
  makes the example governance lookup undefined, so it denies, with no UUID or Canonical ID
  fallback. Reserved ExtProc members (`principal`, `resource`, `grant`, `permission_sets`,
  `granted_permission_sets*`, `agent_session_id`, `mcp`) are never set
  (policy-input-decision.md § 1.1).
- Shared input vectors in `tests/testdata/opa-envoy-input/` must be consumed by both
  `internal/domain/impersonation/policy_input_test.go` and
  `internal/extproc/authorization/input_builder_test.go` for common Envoy fields and deliberate
  broker exclusions; no production cross-binary imports are introduced.
- Safe rendering tests belong in `internal/ports/impersonation_policy_test.go`: `%s`, `%v`, `%+v`,
  `%#v`, pointer/value formatting, `slog.Any` with JSON and text handlers all exclude distinctive
  synthetic claims and transport values. Evaluation still receives the full document, not its
  redacted representation; never convert it to a raw map for logging or diagnostics.

## 4. Adapter (`internal/adapters/policy/opa/`, new)

- `NewImpersonationPolicySource(cfg, decisions, admit, logger)` opens an adapter-owned `os.Root`
  over the builder-supplied `security.trust_material_dir`.
  - **Managed mode** (`policy.config_file`): reads and validates the bootstrap subset, bounds the
    regular-file read to 1 MiB on the same descriptor, rejects every OPA-native local-file reference
    before startup, creates an in-memory store with an `OnCommit` trigger, and starts `sdk.New` with
    `Ready`, `V1Compatible`, a discard console logger, and exactly one plugin factory in
    `sdk.Options.Plugins`: `aib_bearer_token_file` (R16), registered only when the bootstrap
    configures it. Its `Prepare` reads the token through the same `os.Root` before every bundle
    request and never logs it.
  - **Local mode** (`policy.path`): walks the bundle directory through `os.Root` with the file-type,
    count, and size limits of the configuration contract, loads it with OPA's bundle reader from
    those bytes, and builds one candidate. No SDK runtime and no polling are started.
  - The adapter obtains its tracer and meter from `otel.Tracer("tokenexchange")` and `otel.Meter("tokenexchange")`, never constructor-injected
    providers, as required by `coding-standards/OPENTELEMETRY.md` § 1.
- One candidate prepares a query and computes evidence for every decision reference from the same
  modules and data, then calls `admit` once with the evidence map.
- The trigger reads candidate modules, data, and activated bundle names from one transaction.
  A candidate carries its configured/activated bundle sets and must include all configured bundles
  before `admit` or any active-pointer swap, including initial loading. Historical SDK readiness
  never admits an earlier subset. It then computes capabilities (R4) and evidence (R5), calls
  `admit`, assigns a new `AdmissionID` only on successful admission, and swaps an
  `atomic.Pointer[revision]`. Trigger failures reject the candidate, never panic the broker.
- `revision.Evaluate` converts input with `ast.InterfaceToValue`, applies `context.WithTimeout`
  (the rule's `evaluation_timeout`), runs the rule's prepared query, and parses the decision
  (contract § 3).
- The adapter imports no other adapter and nothing from `internal/extproc`.

## 5. Builder and handler

- `internal/app/builder.go` constructs the factory with the injected logger and passes
  it to `impersonation.NewService`, and supplies `security.trust_material_dir` to the adapter factory.
- `internal/adapters/http/enduser/oauth2_token.go` fills `Request.HTTP` from the `*http.Request` and
  logs the three new audit attributes. It does not inspect policy results.
