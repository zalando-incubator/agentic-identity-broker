# Internal contracts: policy source port, admission, and revision use

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Research**: R1, R2, R5, R6,
R8, R10, R11, R12, R15, R16 | **Data model**: [../data-model.md](../data-model.md)

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
engine settings moved to `impersonation.opa`. It reuses the loader's existing mapstructure
decode-hook pattern (`rejectNonSequenceSubjectCategoriesHook` in `internal/config/loader.go`): a
hook keyed on `ImpersonationRuleOPADecision` rejects unknown keys of that map before the typed
decode drops them. The validator then reports the error under the indexed
`oauth2_authorization_server.impersonation.rules[<i>] (<name>)` prefix. An absent `impersonation.opa`
block or `{}` is accepted when no rule selects OPA; a populated block without an OPA rule fails.

`AuthorizationConfig` and `OPAAuthorizationConfig` (used by `token_exchange.authorization`) are
unchanged as declarations, but the loader and ordinary-exchange service construction reject explicit
`token_exchange.authorization.type: opa` before serving requests (API-CFG). `internal/config/validator.go` and `impersonation.validateRuleInvariants` both enforce
the rules of the configuration contract (defense in depth, as today).

## 2. Port (`internal/ports/impersonation_policy.go`, new)

```go
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
// query per decision reference, all built from the same snapshot.
type ImpersonationPolicyRevision interface {
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
// with, per rule name, whether that rule's decision rule exists. The adapter sets the flag to true
// only when ast.Compiler.GetRulesExact(data.<package>.<decision>) on the candidate's broker-owned
// compiler returns at least one rule; a value only in data documents or only rules nested below
// the ref yields false (decision_undefined). A non-nil error rejects the whole
// candidate; ImpersonationPolicyAdmissionError names the first failing rule for diagnostics.
type ImpersonationPolicyAdmission func(decisionDefined map[string]bool) error

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

- `NewService` appends two trailing parameters, `policySources PolicySourceFactory, publicURL string`;
  CEL-only callers pass `nil, ""`. A nil factory with an OPA rule fails startup. `NewService` takes
  no context (`Builder.Build()` has none): with at least one OPA rule it starts one source with
  every OPA rule's decision reference, then calls `AwaitReady` with
  `context.WithTimeout(context.Background(), startup_timeout)`; on failure it stops the source and
  returns the engine error naming the last rejecting rule. With no OPA rule it starts nothing.
  `validateRuleInvariants` accepts `type: opa`, and the four OPA defaults (`startup_timeout` 30s,
  `package`, `decision`, `evaluation_timeout` 100ms) are applied here at use.
- `Service.Close(ctx context.Context) error` stops the source and returns nil when no source runs.
  The builder registers it through `App.chainShutdown`.
- The OPA admission function checks every OPA rule in rule order and rejects the candidate at the
  first rule whose decision rule is absent (all-or-nothing, R11). A decision rule that exists but
  is undefined for a request is a runtime failure (`ErrPolicyDecisionInvalid`, terminal), never a
  deny. OPA rules have no reference or
  field-binding requirements ([policy-input-decision.md § 4](policy-input-decision.md#4-exceptional-credential-modes-dec-bind-withdrawn)).
  CEL rules keep their inline startup checks in `cel.go` and today's error text
  (`requires the authorization predicate to reference subject_token (FR-007a)` and the CR-009
  message).
- `compiledRule` gets an internal `authorization`; CEL rules hold a static one and OPA rules obtain
  theirs per request from the revision read by `Impersonate`:

  ```go
  type authorization interface {
  	label() string  // the rule name, for diagnostics and audit
  	engine() string // "cel" or "opa", the audit authorization_engine value
  	evaluate(ctx context.Context, in policyInputs) (bool, error)
  	// bindsSubjectField reports whether this rule may use the unverified subject field ("sub" or
  	// "email"): CEL returns the compiled predicate's binding; OPA always returns true.
  	bindsSubjectField(field string) bool
  }

  type policyInputs struct {
  	clientClaims, actorClaims, subjectClaims map[string]any
  	subjectUnverified                        bool
  	request                                  map[string]any // CEL request variable
  	http                                     RequestAttributes
  	agent                                    *AgentContext // id, canonical_id?, external_id?
  }
  ```

  CEL `evaluate` runs the compiled predicate on the CEL activation built from `policyInputs`; OPA
  `evaluate` builds the input document (contract § 1) and calls the request's revision with the rule
  name. Phase 0 introduces the seam with only the CEL implementation and keeps behavior unchanged.
- `Impersonate` reads `rev := source.Current()` once, before the first rule, and passes it to every
  `evaluateRule` call of that request. An OPA rule's `authorization` is `rev` bound to the rule name;
  a CEL rule's is static. For a CEL rule, `classifySubject` and email minting keep using the
  predicate's `sub` and `email` bindings. For an OPA rule, an unsigned subject whose raw `sub` equals
  the target UUID is agent category, and the extracted email is minted when nonempty and the
  decision allows. All OPA rules of one request see the same policy and data. The pipeline order is unchanged.
- `Request` gains `HTTP RequestAttributes{Method, Path, RawQuery, Protocol, ContentType string; Headers, Form map[string][]string}`
  set by the handler on `req.HTTP` after `impersonation.ParseRequest(formData)` returns. `Form` is a copy of the `url.ParseQuery` result of the body the handler already parses, taken before omitted-value normalization (`dropOmittedValues`).
  The domain builds `parsed_body` from it and always deletes `client_assertion`, `actor_token`, `subject_token`, `client_secret`, `refresh_token`, `code`, and `code_verifier`
  itself, so a handler change cannot leak them; `parsed_body` is `null` unless
  `ContentType` is `application/x-www-form-urlencoded`. The domain builds the input (contract § 1) only for OPA rules, using
  `server.enduser.public_url` passed to `NewService`.
- Error mapping: `ErrPolicyEvaluationFailed` → `serverError("authorization evaluation failed")` with
  `client_authorization/opa_evaluation_failed`; `ErrPolicyDecisionInvalid` → same response with
  `client_authorization/opa_decision_invalid`; both are `abort` results.
- `AuditRecord` gains `AuthorizationEngine`, `PolicyRevision`, and `PolicyAdmissionID`, logged on the
  existing `impersonation.decision.recorded` event.
- CEL principal and email extraction on an OPA rule uses `defaultEvaluationTimeout` (100 ms,
  `cel.go`), because an OPA rule has no CEL timeout; `authorization.opa.evaluation_timeout` bounds
  only the OPA decision.
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

- `NewImpersonationPolicySource(cfg, decisions, admit, logger)` reads every file through
  `internal/fscontain` (§ 6) over that file's own directory (the bundle directory in local mode); it
  takes no directory setting (configuration contract `file_containment`).
  - **Managed mode** (`policy.config_file`): reads and validates the bootstrap subset, bounds the
    regular-file read to 1 MiB on the same descriptor, rejects every OPA-native local-file reference
    before startup, creates an in-memory store with an `OnCommit` trigger, and starts `sdk.New` with
    `Ready`, `V1Compatible`, a discard console logger, and exactly one plugin factory in
    `sdk.Options.Plugins`: `aib_bearer_token_file` (R16), registered only when the bootstrap
    configures it. Its `Prepare` reads the token through `fscontain` over the token file's
    directory before every bundle request and never logs it. If OPA v1.21.4 does not resolve
    `credentials.plugin` through `sdk.Options.Plugins`, the fallback is inline credentials only and
    the plugin key is rejected.
  - After `sdk.New`, the adapter registers a bundle status listener
    (`sdk.OPA.Plugin("bundle")` as `*bundle.Plugin`, `Register`) that feeds the bundle download
    counter, the download-age gauge, and the rate-limited `bundle_download_failed` log
    ([observability.md](observability.md)).
  - **Local mode** (`policy.path`): walks the bundle directory with `fscontain.Dir.Walk` and the
    file-type, count, and size limits of the configuration contract, loads it with OPA's bundle reader from
    those bytes, and builds one candidate. No SDK runtime and no polling are started.
  - The adapter obtains its tracer and meter from `otel.Tracer("tokenexchange")` and `otel.Meter("tokenexchange")`, never constructor-injected
    providers, as required by `coding-standards/OPENTELEMETRY.md` § 1.
- One candidate prepares a query for every decision reference from the same modules and data, then
  calls `admit` once with each rule's decision-defined flag.
- The trigger reads candidate modules, data, and activated bundle names from one transaction.
  A candidate carries its configured/activated bundle sets and must include all configured bundles
  before `admit` or any active-pointer swap, including initial loading. Historical SDK readiness
  never admits an earlier subset. It then compiles with restricted capabilities (R4), calls
  `admit`, assigns a new `AdmissionID` only on successful admission, and swaps an
  `atomic.Pointer[revision]`. Trigger failures reject the candidate, never panic the broker.
- `revision.Evaluate` converts input with `ast.InterfaceToValue`, applies `context.WithTimeout`
  (the rule's `evaluation_timeout`), runs the rule's prepared query, and parses the decision
  (contract § 3).
- Admission flags: for each decision reference, `GetRulesExact` on the candidate's broker-owned
  compiler for `data.<package>.<decision>`; at least one rule means defined.
- The adapter imports no other adapter and nothing from `internal/extproc`.

## 5. Builder and handler

- `internal/app/builder.go` constructs the factory with the injected logger and passes
  it to `impersonation.NewService`. The adapter factory takes no directory setting.
- `internal/adapters/http/enduser/oauth2_token.go` fills `Request.HTTP` from the `*http.Request`
  after `impersonation.ParseRequest`, passing the pre-normalization form copy through
  `handleTokenExchange`, and logs the three new audit attributes on
  `impersonation.decision.recorded`. It does not inspect policy results.

## 6. Shared file containment (`internal/fscontain/`, new; extracted in Phase 0)

Extracted from `internal/adapters/jwks/trust_directory.go`, `trust_directory_unix.go`, and
`trust_directory_other.go`, so the jwks and OPA adapters share one implementation without
importing each other.

```go
// OpenDir opens an os.Root over path after filepath.EvalSymlinks. path must be absolute and must
// not resolve to "/".
func OpenDir(path string) (*Dir, error)

// ReadFile opens name relative to the root with openFlags (O_RDONLY|O_NONBLOCK on Unix, so a FIFO
// cannot block), checks on the same descriptor that it is a regular file, and reads at most max
// bytes; a larger file fails with ReasonFileTooLarge.
func (d *Dir) ReadFile(name string, max int64) ([]byte, error)

// Walk visits regular files below the root in lexical order, calling accept on each relative name
// and fn on the bytes of every accepted file. It fails with ReasonTooManyFiles or
// ReasonFileTooLarge when maxFiles or the cumulative maxBytes is exceeded.
func (d *Dir) Walk(maxFiles int, maxBytes int64, accept func(name string) bool,
	fn func(name string, data []byte) error) error

func (d *Dir) Close() error

type Reason string // escaping_link, root_directory, not_absolute, not_regular, file_too_large, too_many_files, unreadable

type Error struct {
	Reason Reason
	Name   string // relative name, never file content
	Err    error
}
```

Files: `fscontain.go`, `open_unix.go`, `open_other.go`, `fscontain_test.go`, `open_unix_test.go`
(the FIFO and escape cases move from `trust_directory_unix_test.go`). Escape classification keeps
the stable "path escapes from parent" match. `jwks/trust_directory.go` keeps its long-lived root,
calls `ReadFile(name, trustanchor.MaxKeySetBytes)`, and maps each `Reason` to its existing
`trustanchor.Category`, so jwks error text and categories stay byte-for-byte unchanged; the
existing jwks tests guard the extraction. The OPA adapter opens one `Dir` per file directory and
maps `Reason` to the configuration contract's reason classes.
