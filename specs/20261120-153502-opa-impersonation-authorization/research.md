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
  `rego.StrictBuiltinErrors`, `rego.EnablePrintStatements`, `ast.CapabilitiesForThisVersion`. The bundle plugin compiles each activation with a fresh
  `ast.NewCompiler()` that has every built-in, so OPA alone does not prohibit `http.send`.

## R1 — Policy engine placement and hexagonal boundary

**Decision**: Add a broker-only adapter `internal/adapters/policy/opa/` that implements a new port
`ports.ImpersonationPolicySource`. The domain keeps CEL in-process (as today) behind the same internal
`authorizer` abstraction. The builder injects an `impersonation.PolicySourceFactory` into
`impersonation.NewService`, like the existing `JWKSProviderFactory`.

**Rationale**: OPA bundle management performs network I/O and owns background goroutines, which is
adapter work (Principle VI). Admission (every OPA rule's decision exists, all-or-nothing) stays in
the domain, which passes an admission function to the adapter (R6). The broker cannot reuse the ExtProc package without coupling two deployables.

**Alternatives**: Import `internal/extproc/authorization` (cross-binary coupling, wrong failure
semantics: deny on error). Put OPA in `internal/domain/impersonation` (network I/O in the domain).

## R2 — Embedding mode: SDK with broker-owned revision snapshots

**Decision**: In managed mode, start one `sdk.New` for the shared engine (R11) with the bootstrap
bytes, `V1Compatible: true`, a non-blocking `Ready` channel, a discard `ConsoleLogger`, no plugins
other than the broker's `aib_bearer_token_file` credential plugin when configured (R16), and an
in-memory `Store` created by the adapter. The adapter registers a store `OnCommit` trigger. On each commit that carries
a compiler (`plugins.GetCompilerOnContext`), the adapter builds a **candidate revision** inside the
trigger:

1. Copy the compiled modules, complete data document, and activated bundle-name set from the
   same committing transaction. Record the configured bundle-name set as candidate completeness
   data. Reject with `bundles_incomplete` unless every configured bundle is present in this
   snapshot; partial initial candidates never enter admission or become serving revisions.
2. Prepare one `rego.PreparedEvalQuery` per OPA rule for its `data.<package>.<decision>` from those
   modules, one new in-memory store holding the data copy, the restricted capabilities of R4,
   `StrictBuiltinErrors(true)`, and print statements disabled.
3. Check that each OPA rule's decision exists and call the domain admission function once for all
   rules (R6, R11).
4. Only if every step succeeds, atomically replace the active revision (`atomic.Pointer`).

The broker never evaluates through `sdk.OPA.Decision`.

**Rationale**: OPA's bundle plugin already provides polling, ETags, signing verification,
credentials, and atomic per-bundle activation (library-first, Principle III), and it reuses the OPA
configuration vocabulary that ExtProc operators know. It cannot reject an activation on the broker's
behalf: `bundle.Activate` uses the full-capability compiler, and pre-activation hooks do not receive
the compiled policy. A broker-owned immutable snapshot provides exactly the needed semantics:
a rejected candidate never becomes active, every OPA rule of a request evaluates one object, and
in-flight requests keep their snapshot during a swap.

**Consequence**: OPA's internal store can hold a revision the broker rejected. The next bundle poll
does not re-download an unchanged ETag, so the broker stays on the last admitted revision until the
operator publishes a new one. OPA's own bundle status is therefore not authoritative; the broker
emits its own activation and rejection telemetry ([contracts/observability.md](contracts/observability.md)).
The data document is held twice in memory (OPA store and active snapshot), plus a third copy briefly
during a candidate build.

**Alternatives**: Custom `bundle.Activator` registered through the process-global activator
registry (process-global mutable state, and the activator API is marked for extension
use, not a stable contract). The OPA `download` package with a broker-written bundle loader
(reimplements polling, signing, and multi-bundle roots). Evaluating through `sdk.OPA.Decision`
(cannot enforce capabilities or keep one revision per request).

## R3 — Bootstrap configuration subset (**Sign-off: API-BOOT**)

**Decision (Proposed: API-BOOT)**: The bootstrap file (`impersonation.opa.policy.config_file`) is
OPA-native YAML or JSON, read once at startup.
It must be an absolute path. The adapter opens an `os.Root` over the file's own parent directory
(not `/`) and opens the file through it, with same-descriptor regular-file checks and a 1 MiB bound.
The same rule covers the token file (R16); a local bundle uses an `os.Root` over the bundle directory
(R15). No directory setting is involved: neither `security.trust_material_dir` nor a new one.

*Containment revision (2026-11-24)*: an earlier draft required every OPA file to lie under
`security.trust_material_dir`. Because one admin writes both the directory and the paths inside it,
that adds no boundary against the admin, only typo protection. It also widened ADR
`040-trust-anchor-local-key-sets`, which defines the trust directory as the only place for
public key-set files and warns against putting more there, and it forced secrets and large policy
bundles into one projected volume sized for small key sets. What containment does protect against is
content written by someone else: a rotated token file replaced with a link to another credential,
or a bundle file linking outside the bundle. A root over each file's own directory covers both.
Alternatives: keep `security.trust_material_dir` (scope creep on an accepted ADR); a dedicated
`impersonation.opa.directory` (same lack of a boundary against the admin, one more setting); plain
`os.Open` (loses the protection against content written by others). Bootstrap-referenced local file reads are not supported: reject bearer
`token_path`, TLS CA/certificate/private-key file paths, bundle `file://` resources, and any other
OPA configuration field that would cause a local file read. Validate against a closed member
allowlist for the pinned OPA v1.21.4 configuration types before `sdk.New` (revised 2026-11-26;
the exact list is in the configuration contract `bootstrap_subset.allowlist`), failing closed on
every other nested member; never pass unchecked file references to the SDK. Accepted credentials:
`bearer` with an inline `token` and optional `scheme` (`token_path` rejected,
`v1/plugins/rest/auth.go:62-65`); `oauth2` client credentials with inline `token_url`, `client_id`,
`client_secret`, `scopes`, and additional headers or parameters (assertion, signing-key, KMS, and
`client_assertion_path` members rejected, `auth.go:236-254`); `s3_signing` with
`environment_credentials`, `metadata_credentials`, or `assume_role_credentials`, while
`web_identity_credentials`, `profile_credentials`, and `sso_credentials` read files and are
rejected (`aws.go:118-121, 416-431, 707-716, 861-866`); `gcp_metadata` (its `*_token_path` members
are metadata-server URL paths, `gcp.go:49-54`); `azure_managed_identity` (no file members,
`azure.go:49-56`); and `plugin: aib_bearer_token_file`. Service `tls` (`ca_cert` is a file),
`client_tls`, `allow_insecure_tls`, and `type` are rejected (`rest.go:55-70`,
`auth_tls.go:85-88`). `bundles.<name>` accepts `service`, `resource`, `signing` (`keyid`, `scope`,
`exclude_files`), `polling`, `size_limit_bytes`, and `trigger`; `keys.<id>` accepts `algorithm`,
`key`, and `scope` (`private_key` rejected). Services use supported inline credentials or the broker's
contained token-file plugin (R16), and bundle keys are inline. There is no fallback to unrestricted
opening. Only the top-level keys `services`, `bundles`, `keys`, `labels`, and `plugins` are accepted,
and `plugins` may contain only `aib_bearer_token_file`. Every other top-level key or plugin
fails startup, including `decision_logs`, `discovery`, `status`, other `plugins`, `server`,
`persistence_directory`, `caching`, `distributed_tracing`, `nd_builtin_cache`, `storage`,
`default_decision`, and `default_authorization_decision`. `bundles.<name>.persist: true` fails
startup. At least one bundle is required. Diagnostics name the field
`impersonation.opa.policy.config_file` and the offending key name, but never quote values or raw
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
(contradicts the issue's request for an ExtProc-comparable bootstrap). Permit OPA-native file reads
without containment under a documented exception (retains file-based rotation, but lets a replaced
or linked file read unrelated runtime credentials). A contained file resolver for
all SDK credential forms, including mTLS client certificates, is a larger alternative, not part of
this release; R16 covers bearer tokens only.

## R4 — Prohibiting policy-time networking (**Sign-off: DEC-BUILTINS**)

**Decision**: Candidate revisions compile with capabilities derived from
`ast.CapabilitiesForThisVersion()` minus every built-in whose declaration is `Nondeterministic`,
except `time.now_ns` and `io.jwt.decode_verify` (revised 2026-11-26). OPA v1.21.4 marks 11 built-ins
nondeterministic (`v1/ast/builtins.go`); this removes `http.send`, `net.lookup_ip_addr`, `opa.runtime`, `rand.intn`, `uuid.rfc4122`, `io.jwt.encode_sign`, `io.jwt.encode_sign_raw`, `json.verify_schema`, and `json.match_schema`. A module that calls
a removed built-in, including from a helper package that the decision does not reach, fails
compilation, so the candidate is rejected before activation and nothing is called. Print statements
are disabled.

**Rationale**: The nondeterministic flag is OPA's own classification of built-ins with external
effects or inputs. Deriving the allowlist from it fails closed for built-ins a future OPA upgrade
adds. `opa.runtime` exposes the runtime configuration, which can include bundle-service credentials.
`time.now_ns` is needed for time-based governance and has no external effect. `io.jwt.decode_verify`
is marked nondeterministic only because it checks token times against the clock; it has no external
effect and is common in governance policies. `json.verify_schema` and `json.match_schema` can resolve
remote `$ref`s, so they stay removed. Checking whole bundles
rather than only reachable rules needs no dependency-graph walk.

**Alternatives**: Remove only `http.send` and `net.lookup_ip_addr` (fails open for future network
built-ins; leaves `opa.runtime`). Also allow `rand.intn` and `uuid.rfc4122` (no governance use
case; reintroduces nondeterministic decisions). Remove `io.jwt.decode_verify` too (rejected
2026-11-26: it has no external effect and blocks common policies).

## R5 — Binding evidence from the decision's dependency graph (**Sign-off: DEC-BIND**, ADR)

**Withdrawn 2026-11-25 (maintainer decision).** OPA rules have no static binding analysis: no
mandatory `subject_token` or audience-absent role reference, no `with`-modifier analysis, and no
`email` or `sub` field-binding evidence. CEL rules keep their existing startup checks.

**Rationale**: OPA is already a capable policy language. The dependency-graph check was shallow: it
proved only that a policy mentions a value, which shows intent, not that the policy constrains it.
It is not a security control; review of the operator's Rego policy is. The withdrawn design also
needed revision-coupled evidence, `with` voiding, and an analysis of imports and helper rules, which
added complexity without a matching security benefit.

**Consequence**: For an OPA rule, an unverified email is minted when the rule's `email_expression`
yields a nonempty email and the decision allows, and an unsigned subject whose raw `sub` equals the
audience-selected target UUID is agent-category. Principle I needs the constitution amendment
CONST-AMEND (R13).

## R6 — Admission and requirements stay in the domain

**Decision** (revised 2026-11-25): For OPA, `NewService` passes an admission function to the
factory; the adapter calls it for every candidate (initial and update), and it rejects the
candidate unless every OPA rule's decision rule exists (R11). Exists means that
`ast.Compiler.GetRulesExact` on the candidate's broker-owned compiler returns at least one rule for
`data.<package>.<decision>`; a value only in data documents, or only rules nested below the ref, is
`decision_absent` (revised 2026-11-26). A decision rule that exists but yields no value for a
request is not an admission question: it is a terminal runtime failure (R9). CEL rules keep their existing inline
startup `References` checks in `rule.go` unchanged. The service reads one engine revision per request, before the
first rule, and uses it for every OPA rule's decision. The binding requirements of the original
decision are withdrawn with R5.

**Rationale**: Keeping admission in the domain keeps the readiness and all-or-nothing rules out of
the adapter (Principle VI). Reading one revision per request keeps first-match fallthrough across
OPA rules on one policy and data version without locks.

**Alternatives**: Adapter-owned admission (duplicates domain rules). Re-reading the active revision
for each rule (a swap between rules could mix policy versions).

## R7 — Startup readiness bound (**Sign-off: API-CFG**)

**Decision**: Add `impersonation.opa.startup_timeout` (duration, default `30s`, range `1s`–`5m`).
`NewService` starts the shared engine, then waits. The engine is ready when the SDK `Ready` channel
is closed (managed mode) **and** its active admitted revision carries completeness data for all
configured bundles from that revision's own committing snapshot (R2) and passed every OPA rule's
admission. In local mode the one candidate is admitted or rejected immediately. Historical
activations cannot satisfy completeness for a different candidate. If this does not happen within
the bound, the broker stops the engine and startup fails with a raw-credential-excluded diagnostic
naming the field, the last rejection reason class, and the last rejecting rule. In particular,
a usable first-bundle subset followed by a rejected complete two-bundle candidate never becomes
ready; there is no partial admitted serving revision to retain.

**Rationale**: FR-003 requires a bounded interval, and the spec left the setting to planning.
`30s` covers several OPA bundle retry intervals for a slow bundle server; `5m` caps rollout stalls.
One shared bound replaces per-rule bounds; with one engine there is nothing to wait for concurrently.

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
- `version` (`{"ext_authz": "v3", "encoding": "protojson"}`), `parsed_path` (path segments), and
  `parsed_query` (query values as string arrays; unvalidated, ignored by the token endpoint).
- `parsed_body` (revised 2026-11-25): the form body parsed exactly as the Envoy plugin parses
  `application/x-www-form-urlencoded` (`url.ParseQuery`, object of string arrays; `null` for other
  content types), with the credential-bearing OAuth parameters `client_assertion`, `actor_token`, `subject_token`, `client_secret`, `refresh_token`, `code`, and `code_verifier` always removed.
  `truncated_body: false`, because the handler rejects bodies over 256 KiB. No
  `attributes.request.http.body`, `raw_body`, `source`, `destination`, or `id`.
- `context`: `client_assertion`, `actor_token`, `subject_token` (validated claim maps, `{}` when
  empty), `subject_unverified`, and `request` (`resource`, `grant_type`, `scope`, `agent_id`), built
  from the same values as the CEL activation; plus `agent` from the broker-resolved Agent:
  `id` (UUID), and `canonical_id` and literal `external_id`, each omitted when unset. Top-level
  `type: "impersonation"`. Governance examples index bundles by that
  External ID, not UUID. The broker is authoritative for UUID-to-External-ID mapping; callers and
  bundles cannot override it. UUID remains the routing and minted identity.

Field names and shapes follow `envoyauth.RequestToInput`. The adapter converts the input with
`ast.InterfaceToValue` before evaluation; claim maps carry the same values and types as the CEL
activation (arrays and nested objects unchanged), with no extra numeric precision guarantee.

**Rationale**: Envoy-compatible predicates such as `input.attributes.request.http.method == "POST"`
and `input.parsed_path == ["oauth2","token"]` work unchanged (FR-005). The configured public URL is
trusted configuration; the request `Host` header and a TLS-terminating ingress would make `host` and
`scheme` caller- or topology-dependent. Envoy-style body predicates such as
`input.parsed_body.scope[_] == "read"` work, while the raw credentials never enter the input
(SR-003). `parsed_body` duplicates `grant_type` and `scope` from `context.request` as raw arrays;
that is accepted for Envoy fidelity, and documentation points identity and target decisions at
`context`. Shared ExtProc bundles separate by package, not by `parsed_body` shape or `input.type`. The `context` namespace is built only from validated values, so headers cannot overwrite it
(Scenario 2.2).

**Alternatives** (2026-11-25): `parsed_body: null` (the previous proposal; breaks Envoy body
predicates); the full Envoy body including raw JWTs (would require relaxing SR-003); keeping the
credential keys with redacted values (exact key set, but placeholder values policies could misread).

**Alternatives**: Synthesize an ext_authz `CheckRequest` and call `envoyauth.RequestToInput` (adds
the opa-envoy-plugin dependency to the broker binary for a handful of fields, and its body parsing
must then be disabled). Include all headers (exposes `authorization` and cookies). Allowlist headers
(breaks reuse of header predicates such as tenant headers).

**Context consistency with ExtProc's Grant Context** (2026-11-24 revision, after rebasing on
`release/v0.2`): ADR `048-authzen-delegation-pip` gives ExtProc an `input.context` projected from
the broker's AuthZEN permit: `principal`, `agent {id, canonical_id?}`, `resource`, `grant`,
`permission_sets`, and compatibility members, with optional IDs omitted, never `""`. The previous
flat `context.agent.external_id` with `""` for absent would have defined a second `agent` shape.
Decision: reuse ExtProc's `agent` object and its omission rule, and add `external_id` to it. Add
`input.type: "impersonation"` next to ExtProc's discriminator values, so shared bundles can branch.
Keep grant-context members absent: OPA runs before human delegation, so no permit-backed
principal, grant, or permission sets exist, and an unsigned extracted principal must not look like
ExtProc's grant-backed `principal`. Alternatives: (a) keep the flat member with `""` (two `agent`
shapes; `!= ""` guards versus undefined); (b) add `principal` from extraction (same name, weaker
meaning, and it hides that the subject is caller-asserted for unsigned rules); (c) evaluate OPA after delegation
and supply a full Grant Context (moves the rule predicate after delegation and changes first-match
semantics; possible later feature). Follow-up outside this feature: add `external_id` to the
AuthZEN permit agent and the ExtProc `AgentContext` under a revision of ADR
`048-authzen-delegation-pip`.

**Envelope consistency**: Keep the domain builder, but add shared JSON request/output vectors under
`tests/testdata/opa-envoy-input/`, consumed by broker-domain and ExtProc authorization tests without
production imports between the binaries. Compare supported common HTTP/path/query fields against
`envoyauth.RequestToInput`, applying the broker's declared body/header exclusions separately. The
ADR records the deliberate decision divergence: broker invalid/error is terminal `server_error`;
ExtProc invalid/error is deny and supports approval actions. Alternative: adapter-shaped envelope
from an engine-neutral DTO; shared vectors preserve the existing design with less restructuring.

## R9 — Decision contract and failure mapping (**Sign-off: API-DECISION**, **API-DIAG**)

**Decision**: The decision must be exactly one result whose value is an object with a string
`action` of `allow` or `deny`. Other members are ignored and never logged or reflected. `allow`
satisfies the rule's authorization predicate; `deny` is a non-match (`403 access_denied` after
no-match precedence). Undefined, multiple results, non-object, missing or other `action` values
(including `approval_required`) return `500 server_error` with diagnostic
`client_authorization/opa_decision_invalid`. Evaluation errors, built-in errors under strict mode,
timeouts, and cancellation map internally to terminal `server_error` with
`client_authorization/opa_evaluation_failed`; connected callers receive HTTP 500, while
client-disconnect cancellation has no response guarantee and uses correlated telemetry for E2E.
Both abort rule evaluation without fallback. The domain error text stays `authorization evaluation
failed`; the token endpoint returns its fixed `internal server error during token exchange`.

**Rationale**: FR-007 distinguishes deny from failure. CEL failures use
`client_authorization/cel_evaluation_failed`; reusing the CEL detail for OPA would mislead operators,
and separating an invalid decision from an evaluation failure points to policy authoring rather than
runtime problems.

**Alternatives**: One `policy_evaluation_failed` detail for both engines (changes the documented CEL
detail). Treat undefined as deny like ExtProc, or require a `default` decision at admission
(both rejected by the user on 2026-11-26: an undefined decision may hide a policy defect, and
admission cannot prove that a rule is total). Reused policies without a `default` decision
therefore return 500 for every non-matching request; documentation and examples require a
`default` (spec Risks & Mitigations).

## R10 — Configuration schema separation

**Decision**: Add `impersonation.opa` for the shared engine (`policy.config_file` or `policy.path`,
`startup_timeout`), required exactly when a rule selects OPA. Give each impersonation rule a dedicated
`ImpersonationAuthorizationConfig` with `type`, `cel`, and `opa` (`package`, default
`aib.broker.impersonation`; `decision`, default `result`; `evaluation_timeout`). Every other key under
a rule's `opa` block fails startup, including the reserved `policy_url` and `policy_path` that the
shared struct declared, so an impersonation rule has exactly one way to name a policy. `token_exchange.authorization` keeps its existing
`AuthorizationConfig` declarations, including reserved `opa.policy_url` / `opa.policy_path`, but
explicit `type: opa` fails startup instead of being accepted and ignored (**Proposed: API-CFG**).
Its CEL defaults and valid CEL behavior remain unchanged; this feature implements no ordinary
exchange OPA runtime.
Validation for impersonation rules: `type` is `cel` or `opa`; the selected block is required; the
other block must be empty; `cel.evaluation_timeout` and `opa.evaluation_timeout` keep the
`10ms`–`5s` range. `package` must be a dot-separated Rego package path; `decision` a single Rego
identifier. The `package` default mirrors ExtProc's `aib.extproc.authz` default; a bundle that does
not define it fails admission (`decision_absent`), so the default never allows by itself.

**Rationale**: FR-009 keeps OPA out of ordinary third-party exchange. A dedicated type avoids adding
OPA settings to `token_exchange.authorization`. The reserved fields were never documented, so their
absence from impersonation rules changes no documented key. The YAML hierarchy of impersonation rules
is unchanged.

**Alternatives**: Remove the reserved ordinary-exchange OPA struct entirely (unnecessary schema
removal). Keep `package` required as before (more typing, and differs from ExtProc). Put engine
settings on each rule (R11 alternative A). Keep accepting `type: opa` while ignoring it (contradicts explicit engine selection and
FR-009). Extend the shared struct with usable impersonation keys (adds unused keys to a second schema).

## R11 — One shared OPA engine (revised 2026-11-24, maintainer choice "option B")

**Decision**: All OPA rules share one engine configured under `impersonation.opa`: one SDK instance
(or one local bundle), one store, and one active revision. Each rule names only its decision
(`package`, `decision`, `evaluation_timeout`). A candidate revision is admitted only if every OPA
rule's decision rule exists (all-or-nothing). A request
reads the active revision once, before the first rule, and uses it for every OPA rule it evaluates.
The engine stops on broker shutdown and on startup failure.

**Rationale**: With one engine per rule, a request that falls through from one OPA rule to the next
could be judged by two different policy and data versions, because each engine downloads and
admits on its own schedule. Journey 3's ordered-rule behavior would then depend on timing.
All-or-nothing admission plus one read per request removes that. It also removes duplicate bundle
downloads and in-memory data copies for rules that share governance data, shortens the
configuration, and matches ExtProc, which runs one OPA engine.

**Consequence**: Rules are coupled at admission: a candidate that leaves one rule's decision
undefined keeps every rule on the previous revision. The rejection log names the failing rule.
Operators who need independent rollout publish separate packages in the same bundle set and update
them together, or keep a rule on CEL.

**Alternatives**: (A) One runtime per rule, the previous decision (isolation and per-rule timeouts,
but mixed versions within a request and duplicate downloads). (C) Named policies referenced by rules
(more indirection, and it changes working CEL configuration). Shared engine with per-rule partial
admission (brings back mixed versions within a request).

## R12 — Observability (**Sign-off: API-DIAG**)

**Decision**: Scope `tokenexchange` (existing broker token-exchange scope). Span
`tokenexchange.impersonation.opa.evaluate` per OPA evaluation. Counter
`tokenexchange.impersonation.opa.revisions` (`outcome`: `admitted`, `rejected`; `reason`: closed
set) and observable gauge `tokenexchange.impersonation.opa.revision.age` (seconds since the active
revision was admitted), without a `rule` attribute because the engine is shared (R11). In managed
mode a listener registered with OPA's bundle plugin (`bundle.Plugin.Register`, OPA v1.21.4
`v1/plugins/bundle/plugin.go:294`) feeds counter `tokenexchange.impersonation.opa.bundle.downloads`
(`bundle`, `outcome`) and gauge `tokenexchange.impersonation.opa.bundle.download.age` (seconds since
`Status.LastSuccessfulDownload`, `status.go:31`) plus a rate-limited `bundle_download_failed` log,
so a failing bundle service is visible for every credential form (revised 2026-11-26). Lifecycle
log events `oauth2_server.impersonation_policy.revision_admitted` and `...revision_rejected`. The
existing `impersonation.decision.recorded` audit event adds `authorization_engine` (`cel`/`opa`) and, for OPA,
`policy_revision` (bundle manifest revisions) and `policy_admission_id` (process-instance UUID
plus the engine's monotonic admission sequence). The latter identifies the exact deciding snapshot
when manifest revisions are absent or reused; it is never a metric or span label. Details:
[contracts/observability.md](contracts/observability.md).

**Rationale**: Q1 accepts no broker-enforced maximum age, so operators need the age, rejection,
and download signals to manage staleness. The revision-age gauge alone cannot tell "nothing new
published" from "downloads failing", because a failed download produces no candidate. No principal, claim, token, or decision reason is recorded
(OPENTELEMETRY §6, LOGGING §6).

**Alternatives**: Rely on OPA status reporting (disabled by R3, and not authoritative under R2).

## R13 — Superseding ADR (**Sign-off: ADR-OPA**)

**Decision**: One new ADR, drafted as
[adr-20261121-061324-opa-impersonation-authorization.md](adr-20261121-061324-opa-impersonation-authorization.md).
It partly supersedes ADR `009-cel-for-authorization-policies` (OPA is no longer reserved for
impersonation rules) and, for OPA rules only, supersedes ADR `031-unverified-subject-unsigned-jwt`
controls 3–4, ADR `034-audience-absent-impersonation-credentials` control 3, and ADR
`045-agent-subject-impersonation` decision 2a's binding requirement (revised 2026-11-25: no static
binding checks, R5). It does not widen the unsigned-subject exception (subject role,
`verification: none`, local mode). Constitution Principle I requires "a startup-verified
authorization predicate reference to `subject_token`" and "an unverified email minted only when the
predicate binds `subject_token.email`" for unsigned subjects, so **CONST-AMEND** limits
those two controls to CEL-authorized rules. It was applied to the constitution as v4.8.0 on 2026-11-25 (commit `fb84c31`); it takes effect for OPA rules once ADR-OPA is Accepted.
ADR `028-opa-extproc-authorization` is unaffected.

**Alternatives**: Separate ADRs for engine selection and the OPA control changes (two approvals for
one inseparable decision). Amend accepted ADRs in place (forbidden by Principle II). Keep Principle I
unchanged and limit OPA rules to signed roles with an expected audience (narrows Q2).

## R14 — Test strategy

**Decision**: An in-process `httptest` bundle server serves tarballs built with OPA's `bundle.Write`
and controllable ETags, with OPA polling at 1-second delays, so update journeys finish in seconds.
Rego fixtures live under `tests/e2e/fixtures/opa/impersonation/`. Journeys that do not test updates
may load that directory in local mode (R15). Unit tests cover capability rejection, the input builder, decision parsing, the admission
function for both engines, and a race test that swaps revisions during concurrent evaluations. No
container suite is needed.

**Rationale**: Mirrors `tests/e2e/extproc/opa_authorization_test.go` (file and HTTP bundles) and the
existing impersonation E2E fixtures.

## R15 — Local bundle directory (revised 2026-11-24, issue 3)

**Decision**: `impersonation.opa.policy.path` is an alternative to `config_file`. It names a bundle
directory (OPA bundle layout) by absolute path. The adapter walks it through an `os.Root` over that
directory, accepts only `*.rego`, `data.json`, `data.yaml`/`data.yml`, and `.manifest` files, caps it
at 16 MiB and 1,000 files, reads the bytes once, and builds one candidate with OPA's bundle reader.
That candidate goes through the same capability check and admission as a managed
revision. Nothing is polled; a change needs a restart. A `.manifest` revision becomes the
`policy_revision` label.

**Rationale**: Managed-only mode needs a bundle server for local development, demos, and the
E2E fixtures, which ExtProc avoids with its `policy.path`. Reading once at startup keeps Q1's
"no policy-time networking" and adds no runtime file access. The `os.Root` over the bundle directory
stops a symlink inside the bundle from pulling outside files, such as a service-account token, into
policy data.

**Alternatives**: Allow `file://` bundle resources in the bootstrap (OPA reads them itself, outside
`os.Root`). Re-read the directory periodically (adds a second update path to secure and test).
Accept a single `.rego` file like ExtProc (no data files, so governance examples cannot run).

## R16 — Contained bundle token file (revised 2026-11-24, issue 4)

**Decision**: The broker registers one OPA credential plugin, `aib_bearer_token_file`, through
`sdk.Options.Plugins`. A bootstrap service selects it with `credentials.plugin`, and its settings
live under the bootstrap's `plugins.aib_bearer_token_file` (`path`, `scheme`, default `Bearer`). It
implements `rest.HTTPAuthPlugin`: `NewClient` builds the default client with its transport wrapped by
`telemetryhttp.NewTransport`, so bundle downloads through this plugin are traced (services using OPA's
inline credential forms use OPA's own client and stay uninstrumented), and `Prepare` reads the
token through an `os.Root` over the token file's own directory (regular file, at most 64 KiB,
trimmed, non-empty) before every
bundle request and sets `Authorization: <scheme> <token>`. At startup the path is validated and read
once; failure stops startup. Later read failures fail that bundle request, OPA retries, the last
admitted revision stays, and the broker logs `token_unavailable` (rate-limited, without contents).

**Rationale**: Inline credentials need a restart to rotate. OPA's own `bearer.token_path` reads with
`os.ReadFile` outside `os.Root` on every request, which R3 rejects. OPA v1.21.4 resolves
`credentials.plugin` through the plugin manager (`rest.Config.AuthPlugin` and
`plugins.Manager.AuthPlugin`), and `Prepare` runs before every request, so the plugin rotates a
mounted Secret without a restart, and a replaced file cannot link outside its directory.

**Limitation**: One token file per bootstrap, because OPA passes only the request to `Prepare`.
Services that need different tokens use inline credentials. mTLS client certificates stay
unsupported.

**Alternatives**: Keep inline-only credentials (restart to rotate). Accept `token_path` after a
startup containment check (OPA's later reads are not contained; symlink swaps escape). Per-service
token maps keyed by request URL (fragile matching).
