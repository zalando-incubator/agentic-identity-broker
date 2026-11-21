# Research: OPA policies for impersonation authorization

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Plan**: [plan.md](plan.md) | **Spec**: [spec.md](spec.md)

Each entry records a decision, its rationale, and the main alternatives. Entries marked
**Sign-off** are registered in [plan.md § Approval Items](plan.md#approval-items). Every other
entry follows existing repository patterns.

## Baseline (code read before design)

- `internal/domain/impersonation/rule.go` compiles one CEL predicate per rule and fails startup when
  an unsigned-subject rule does not reference `subject_token` (ADR `031-unverified-subject-unsigned-jwt`
  control 3) or an audience-absent role's variable is not referenced (ADR
  `034-audience-absent-impersonation-credentials` control 3). `validateRuleInvariants` rejects every
  `authorization.type` other than `cel`.
- `internal/domain/impersonation/service.go` `evaluateRule` classifies the subject with
  `rule.authz.Binds("subject_token","sub")` (ADR `045-agent-subject-impersonation` decision 2a), mints an
  unverified email only when `Binds("subject_token","email")` (ADR 031 control 4), then evaluates the
  predicate. An evaluation error is a terminal `500 server_error`; `false` falls through.
- `internal/ports/config.go` shares `AuthorizationConfig` between `token_exchange.authorization` and
  each impersonation rule. Its `opa` block (`policy_url`, `policy_path`) is reserved and ignored; it
  is not documented in `docs/configuration.md` or exposed by the Helm chart.
- `internal/extproc/authorization/authorizer.go` (ADR `028-opa-extproc-authorization`) embeds OPA
  v1.21.4 with `sdk.New` for `config_file` mode and `envoyauth.RequestToInput` for the Envoy input.
  It treats undefined and errors as deny and accepts any OPA configuration, including decision logs
  and discovery. The broker must not import `internal/extproc` (separate binary, ADR
  `011-extproc-standalone-binary`).
- OPA v1.21.4 public APIs used below were checked in the module source: `sdk.Options.Store`,
  `sdk.Options.Ready`, `storage.TriggerConfig.OnCommit`, `plugins.GetCompilerOnContext`,
  `bundle.ReadBundleNamesFromStore` / `ReadBundleRevisionFromStore`, `rego.Capabilities`,
  `rego.StrictBuiltinErrors`, `rego.EnablePrintStatements`, `ast.CapabilitiesForThisVersion`,
  `ast.Compiler.Graph.Dependencies`. The bundle plugin compiles each activation with a fresh
  `ast.NewCompiler()` that has every built-in, so OPA alone does not prohibit `http.send`.

## R1 — Policy engine placement and hexagonal boundary

**Decision**: Add a broker-only adapter `internal/adapters/policy/opa/` that implements a new port
`ports.ImpersonationPolicySource`. The domain keeps CEL in-process (as today) behind the same internal
`authorizer` abstraction. The builder injects an `impersonation.PolicySourceFactory` into
`impersonation.NewService`, like the existing `JWKSProviderFactory`.

**Rationale**: OPA bundle management performs network I/O and owns background goroutines, which is
adapter work (Principle VI). The binding rules (which references are mandatory, how evidence drives
classification and email minting) stay in the domain, which passes an admission function to the
adapter (R6). The broker cannot reuse the ExtProc package without coupling two deployables.

**Alternatives**: Import `internal/extproc/authorization` (cross-binary coupling, wrong failure
semantics: deny on error). Put OPA in `internal/domain/impersonation` (network I/O in the domain).

## R2 — Embedding mode: SDK with broker-owned revision snapshots

**Decision**: Per OPA rule, start `sdk.New` with the rule's bootstrap bytes, `V1Compatible: true`, a
non-blocking `Ready` channel, a discard `ConsoleLogger`, no extra plugins, and an in-memory `Store`
created by the adapter. The adapter registers a store `OnCommit` trigger. On each commit that carries
a compiler (`plugins.GetCompilerOnContext`), the adapter builds a **candidate revision** inside the
trigger:

1. Copy the compiled modules from that compiler and read the complete data document in the
   committing transaction.
2. Prepare a `rego.PreparedEvalQuery` for `data.<package>.<decision>` from those modules, a new
   in-memory store holding the data copy, the restricted capabilities of R4,
   `StrictBuiltinErrors(true)`, and print statements disabled.
3. Analyze references (R5) and call the domain admission function (R6).
4. Only if every step succeeds, atomically replace the active revision (`atomic.Pointer`).

The broker never evaluates through `sdk.OPA.Decision`.

**Rationale**: OPA's bundle plugin already provides polling, ETags, signing verification,
credentials, and atomic per-bundle activation (library-first, Principle III), and it reuses the OPA
configuration vocabulary that ExtProc operators know. It cannot reject an activation on the broker's
behalf: `bundle.Activate` uses the full-capability compiler, and pre-activation hooks do not receive
the compiled policy. A broker-owned immutable snapshot provides exactly the needed semantics:
a rejected candidate never becomes active, evaluation and binding evidence come from one object
(SR-005 revision coupling), and in-flight requests keep their snapshot during a swap.

**Consequence**: OPA's internal store can hold a revision the broker rejected. The next bundle poll
does not re-download an unchanged ETag, so the broker stays on the last admitted revision until the
operator publishes a new one. OPA's own bundle status is therefore not authoritative; the broker
emits its own activation and rejection telemetry ([contracts/observability.md](contracts/observability.md)).
The data document is held twice in memory (OPA store and active snapshot), plus a third copy briefly
during a candidate build.

**Alternatives**: Custom `bundle.Activator` registered through the process-global activator
registry (global mutable state shared by every rule, and the activator API is marked for extension
use, not a stable contract). The OPA `download` package with a broker-written bundle loader
(reimplements polling, signing, and multi-bundle roots). Evaluating through `sdk.OPA.Decision`
(cannot enforce capabilities or couple binding evidence to the evaluated revision).

## R3 — Bootstrap configuration subset (**Sign-off: API-BOOT**)

**Decision**: The bootstrap file is OPA-native YAML or JSON, read once at startup. Only the
top-level keys `services`, `bundles`, `keys`, and `labels` are accepted. Every other top-level key
fails startup, including `decision_logs`, `discovery`, `status`, `plugins`, `server`,
`persistence_directory`, `caching`, `distributed_tracing`, `nd_builtin_cache`, `storage`,
`default_decision`, and `default_authorization_decision`. `bundles.<name>.persist: true` fails
startup. At least one bundle is required. Diagnostics name the rule, the field
`authorization.opa.policy.config_file`, and the offending key name, but never quote values or raw
OPA parser errors, because the file may contain service credentials. Changes to the file need a
restart; bundle contents update live.

**Rationale**: Decision logs and status reports would export claim-bearing input or decision
metadata (SR-003). Discovery can rewrite the configuration at runtime, including enabling decision
logs, which would bypass this check. Plugins such as `envoy_ext_authz_grpc` would open listeners
inside the broker. Persistence adds disk state outside the broker's deployment contract. The
ExtProc schema is broader because it runs in a separate trust context under ADR
`028-opa-extproc-authorization`.

**Alternatives**: Accept the full OPA configuration as ExtProc does (exports sensitive claims,
runtime reconfiguration). Accept a broker-specific bundle schema instead of OPA's vocabulary
(contradicts the issue's request for an ExtProc-comparable bootstrap).

## R4 — Prohibiting policy-time networking (**Sign-off: DEC-BUILTINS**)

**Decision**: Candidate revisions compile with capabilities derived from
`ast.CapabilitiesForThisVersion()` minus every built-in whose declaration is `Nondeterministic`,
except `time.now_ns`. In OPA v1.21.4 this removes `http.send`, `net.lookup_ip_addr`, `opa.runtime`,
`rand.intn`, `uuid.rfc4122`, `io.jwt.encode_sign`, and `io.jwt.encode_sign_raw`. A module that calls
a removed built-in, including from a helper package that the decision does not reach, fails
compilation, so the candidate is rejected before activation and nothing is called. Print statements
are disabled.

**Rationale**: The nondeterministic flag is OPA's own classification of built-ins with external
effects or inputs. Deriving the allowlist from it fails closed for built-ins a future OPA upgrade
adds. `opa.runtime` exposes the runtime configuration, which can include bundle-service credentials.
`time.now_ns` is needed for time-based governance and has no external effect. Checking whole bundles
rather than only reachable rules keeps the check independent of R5's graph walk.

**Alternatives**: Remove only `http.send` and `net.lookup_ip_addr` (fails open for future network
built-ins; leaves `opa.runtime`). Also allow `rand.intn` and `uuid.rfc4122` (no governance use
case; reintroduces nondeterministic decisions).

## R5 — Binding evidence from the decision's dependency graph (**Sign-off: DEC-BIND**, ADR)

**Decision**: For each candidate, the adapter computes **binding evidence** from the compiled
modules:

1. The roots are the rules for `data.<package>.<decision>`. No such rule rejects the candidate.
2. Reachable rules are the transitive closure of `compiler.Graph.Dependencies` from the roots,
   including functions and rules in other packages.
3. A reachable expression with a `with` modifier whose target is `input` or starts with `input`
   voids all evidence for the candidate (no references and no field bindings).
4. In each reachable rule head and body, including nested comprehensions, every ref is examined.
   A ref is resolved through local aliases: a variable unified or assigned (`=`, `:=`, compiled
   `__localN__` rewrites) to a ref or alias that resolves to an input path inherits that path.
   Aliases are local to one rule body; they do not cross function or rule boundaries.
5. A resolved ref whose ground string prefix is `input.context.<role>` (role
   `client_assertion`, `actor_token`, or `subject_token`) is a **reference** to that role. If the
   next term is the ground string `email` or `sub`, it is also a **field binding**
   `<role>.<field>`. Non-ground (variable) keys at those positions, `input`, and `input.context`
   alone are not evidence.

Evidence is part of the immutable revision (R2). It is equivalent to the CEL checks: CEL
`References(v)` corresponds to a role reference and CEL `Binds(v, f)` to a field binding.

**Rationale**: The compiled dependency graph already resolves imports
(`import input.context.subject_token as s`) and cross-package calls, so the analysis uses OPA's
compiler rather than a broker parser (Principle III). Every imprecision removes evidence and so fails
closed: a missing reference rejects activation for mandatory references, and a missing field binding
withholds unverified email or keeps an unverified exact-target subject human. As with CEL, a
reference is a necessary guard, not proof of a restrictive policy (SR-005).

**Consequence**: Policies that pass identity maps through function arguments and select fields
inside the function, or that build references dynamically, provide no field-binding evidence. The
contract documents the supported forms with examples ([contracts/policy-input-decision.md](contracts/policy-input-decision.md) § 4).

**Alternatives**: Runtime tracing of which input paths an evaluation read (per-request overhead,
evidence varies by request and cannot gate activation). Operator assertion of binding (forbidden by
SR-005). Whole-bundle reference scanning (unreachable helper code could satisfy the guard).

## R6 — Admission and requirements stay in the domain

**Decision**: `compileRule` derives the rule's **binding requirements** from its configuration:
`subject_token` when `roles.subject.verification: none`, and the role variable for each role with
`audience_requirement: absent`. For OPA it passes an admission function to the factory; the adapter
calls it for every candidate (initial and update) and rejects the candidate on error. For CEL it
applies the same function once to the compiled predicate's evidence, replacing the two existing
inline checks with identical error text. The service reads one revision per rule evaluation and
uses that revision for subject classification (`sub` binding), unverified email minting (`email`
binding), and the decision.

**Rationale**: One rule for both engines keeps CEL behavior unchanged (FR-013, SC-008) and keeps
security logic out of the adapter (Principle VI). Reading one revision per rule evaluation satisfies
the concurrent-update requirement of Scenario 3.6 without locks.

**Alternatives**: Adapter-owned requirement logic (duplicates domain rules). Re-reading the active
revision for each step (a swap between steps could mix evidence and decision).

## R7 — Startup readiness bound (**Sign-off: API-CFG**)

**Decision**: Add `authorization.opa.startup_timeout` (duration, default `30s`, range `1s`–`5m`).
`NewService` starts every OPA rule's runtime, then waits concurrently. A rule is ready when the SDK
`Ready` channel is closed (every configured bundle activated once) **and** the adapter has admitted a
revision. If either does not happen within the rule's bound, the broker stops all started runtimes
and startup fails with a credential-free diagnostic naming the rule index, rule name, field, and the
last rejection reason class. An admitted revision can come from any activation inside the bound.

**Rationale**: FR-003 requires a bounded interval, and the spec left the setting to planning.
`30s` covers several OPA bundle retry intervals for a slow bundle server; `5m` caps rollout stalls.
Concurrent waiting keeps total startup time near the largest single bound.

**Alternatives**: A fixed, non-configurable bound (operators with large bundles cannot adapt). Reuse
`evaluation_timeout` (wrong order of magnitude). Start without a revision and fail requests (the
ExtProc behavior, forbidden by Scenario 3.3).

## R8 — Policy input construction (**Sign-off: API-INPUT**)

**Decision**: The HTTP handler passes request-local transport facts (method, path, raw query,
protocol, header map) into `impersonation.Request`. The domain builds the input:

- `attributes.request.http`: `method`, `path` (`URL.Path`), `host` and `scheme` from the configured
  `server.enduser.public_url`, `protocol` (`HTTP/1.1` or `HTTP/2.0`), and `headers` (lower-case
  names; multiple values joined with `, `; `authorization`, `proxy-authorization`, `cookie`, and
  pseudo-headers removed).
- `parsed_path` (path segments) and `parsed_query` (query values as string arrays).
- `parsed_body: null` and `truncated_body: false`. No `body`, `raw_body`, `source`, `destination`,
  or `id`.
- `context`: `client_assertion`, `actor_token`, `subject_token` (validated claim maps, `{}` when
  empty), `subject_unverified`, and `request` (`resource`, `grant_type`, `scope`, `agent_id`), built
  from the same values as the CEL activation.

Field names and shapes follow `envoyauth.RequestToInput`. The adapter converts the input with
`ast.InterfaceToValue` before evaluation; claim maps keep JSON types (numbers stay `json.Number`
precision, arrays and nested objects unchanged).

**Rationale**: Envoy-compatible predicates such as `input.attributes.request.http.method == "POST"`
and `input.parsed_path == ["oauth2","token"]` work unchanged (FR-005). The configured public URL is
trusted configuration; the request `Host` header and a TLS-terminating ingress would make `host` and
`scheme` caller- or topology-dependent. The credential-bearing form body never enters the input
(SR-003). The `context` namespace is built only from validated values, so headers cannot overwrite it
(Scenario 2.2).

**Alternatives**: Synthesize an ext_authz `CheckRequest` and call `envoyauth.RequestToInput` (adds
the opa-envoy-plugin dependency to the broker binary for a handful of fields, and its body parsing
must then be disabled). Include all headers (exposes `authorization` and cookies). Allowlist headers
(breaks reuse of header predicates such as tenant headers).

## R9 — Decision contract and failure mapping (**Sign-off: API-DECISION**, **API-DIAG**)

**Decision**: The decision must be exactly one result whose value is an object with a string
`action` of `allow` or `deny`. Other members are ignored and never logged or reflected. `allow`
satisfies the rule's authorization predicate; `deny` is a non-match (`403 access_denied` after
no-match precedence). Undefined, multiple results, non-object, missing or other `action` values
(including `approval_required`) return `500 server_error` with diagnostic
`client_authorization/opa_decision_invalid`. Evaluation errors, built-in errors under strict mode,
timeouts, and cancellation return `500 server_error` with `client_authorization/opa_evaluation_failed`.
Both abort rule evaluation without fallback. Response descriptions reuse the existing
`authorization evaluation failed`.

**Rationale**: FR-007 distinguishes deny from failure. CEL failures use
`client_authorization/cel_evaluation_failed`; reusing the CEL detail for OPA would mislead operators,
and separating an invalid decision from an evaluation failure points to policy authoring rather than
runtime problems.

**Alternatives**: One `policy_evaluation_failed` detail for both engines (changes the documented CEL
detail). Treat undefined as deny like ExtProc (contradicts FR-007).

## R10 — Configuration schema separation

**Decision**: Give each impersonation rule a dedicated `ImpersonationAuthorizationConfig` with
`type`, `cel`, and `opa` (`policy.config_file`, `policy.package`, `policy.decision`,
`evaluation_timeout`, `startup_timeout`). `token_exchange.authorization` keeps its existing
`AuthorizationConfig`, including the reserved and ignored `opa.policy_url` / `opa.policy_path`.
Validation for impersonation rules: `type` is `cel` or `opa`; the selected block is required; the
other block must be empty; `cel.evaluation_timeout` and `opa.evaluation_timeout` keep the
`10ms`–`5s` range. `policy.package` must be a dot-separated Rego package path; `policy.decision` a
single Rego identifier (default `result`).

**Rationale**: FR-009 keeps OPA out of ordinary third-party exchange. A dedicated type avoids adding
OPA settings to `token_exchange.authorization`. The reserved fields were never documented, so their
absence from impersonation rules changes no documented key. The YAML hierarchy of impersonation rules
is unchanged.

**Alternatives**: Extend the shared struct and reject OPA for third-party exchange at validation
(adds unused keys to a second schema; changes an existing, ignored setting into a startup failure).

## R11 — One runtime per rule

**Decision**: Each OPA rule gets its own SDK instance, store, and revision, even when two rules use
the same bootstrap file. Runtimes stop on broker shutdown and on startup failure.

**Rationale**: Rule-scoped diagnostics, timeouts, and revisions, and no cross-rule coupling of
admission (a revision admissible for one rule can be inadmissible for another with stricter binding
requirements). The cost is duplicate downloads for shared bundles, which operators can avoid by
splitting bundles.

**Alternatives**: Share a runtime per bootstrap file (revision admission would then need per-rule
fan-out and partial acceptance).

## R12 — Observability (**Sign-off: API-DIAG**)

**Decision**: Scope `tokenexchange` (existing broker token-exchange scope). Span
`tokenexchange.impersonation.opa.evaluate` per OPA evaluation. Counter
`tokenexchange.impersonation.opa.revisions` (`outcome`: `admitted`, `rejected`; `reason`: closed
set) and observable gauge `tokenexchange.impersonation.opa.revision.age` (seconds since the active
revision was admitted), both with attribute `rule` (operator-bounded rule name). Lifecycle log events
`oauth2_server.impersonation_policy.revision_admitted` and `...revision_rejected`. The existing
`impersonation_decision` audit event adds `authorization_engine` (`cel`/`opa`) and, for OPA,
`policy_revision` (bundle manifest revisions). Details: [contracts/observability.md](contracts/observability.md).

**Rationale**: Q1 accepts no broker-enforced maximum age, so operators need the age and rejection
signals to manage staleness. No principal, claim, token, or decision reason is recorded
(OPENTELEMETRY §6, LOGGING §6).

**Alternatives**: Rely on OPA status reporting (disabled by R3, and not authoritative under R2).

## R13 — Superseding ADR (**Sign-off: ADR-OPA**)

**Decision**: One new ADR, drafted as
[adr-20261121-061324-opa-impersonation-authorization.md](adr-20261121-061324-opa-impersonation-authorization.md).
It partly supersedes ADR `009-cel-for-authorization-policies` (OPA is no longer reserved for
impersonation rules) and supersedes the CEL-specific mechanism wording of ADR
`031-unverified-subject-unsigned-jwt` controls 3–4, ADR `034-audience-absent-impersonation-credentials`
control 3, and ADR `045-agent-subject-impersonation` decision 2a, for OPA rules only, with the
equivalent revision-coupled enforcement of R5–R6. It does not widen the unsigned-subject exception
(subject role, `verification: none`, local mode), so Constitution Principle I needs no amendment: its
carve-out requires "a startup-verified authorization predicate reference to `subject_token`" and
"an unverified email minted only when the predicate binds `subject_token.email`", which the design
enforces at startup and additionally at every activation. ADR `028-opa-extproc-authorization` is
unaffected.

**Alternatives**: Separate ADRs for engine selection and binding equivalence (two approvals for one
inseparable decision). Amend accepted ADRs in place (forbidden by Principle II).

## R14 — Test strategy

**Decision**: An in-process `httptest` bundle server serves tarballs built with OPA's `bundle.Write`
and controllable ETags, with OPA polling at 1-second delays, so update journeys finish in seconds.
Rego fixtures live under `tests/e2e/fixtures/opa/impersonation/`. Unit tests cover the analysis
(table of Rego forms), capability rejection, the input builder, decision parsing, the admission
function for both engines, and a race test that swaps revisions during concurrent evaluations. No
container suite is needed.

**Rationale**: Mirrors `tests/e2e/extproc/opa_authorization_test.go` (file and HTTP bundles) and the
existing impersonation E2E fixtures.
