# ADR 20261121-061324: OPA Authorization for Broker Impersonation Rules

**Date**: 2026-11-21
**Status**: Proposed
**Feature**: `20261120-153502-opa-impersonation-authorization` (issue #518, PR #546)
**Final location**: `adrs/20261121-061324-opa-impersonation-authorization.md` (moved by the first implementation task after approval)
**Supersedes (in part)**:
- [`009-cel-for-authorization-policies`](../../adrs/009-cel-for-authorization-policies.md): the reservation of `opa` for broker authorization, for impersonation rules only.
- [`031-unverified-subject-unsigned-jwt`](../../adrs/031-unverified-subject-unsigned-jwt.md) compensating controls 3 and 4, [`034-audience-absent-impersonation-credentials`](../../adrs/034-audience-absent-impersonation-credentials.md) compensating control 3, and [`045-agent-subject-impersonation`](../../adrs/045-agent-subject-impersonation.md) decision 2a's binding requirement: for rules that select OPA only, these controls do not apply (decision 5). CEL rules keep them.

## Context

Impersonation rules (`oauth2_authorization_server.impersonation.rules[]`) authorize with one CEL
predicate (ADR `009-cel-for-authorization-policies`). CEL cannot consult organizational governance
data or reuse existing Rego policies. Issue #518 asks for an OPA option with an ExtProc-like
bootstrap and Envoy-compatible input.

Three accepted ADRs make the CEL predicate a security control:

- ADR `031-unverified-subject-unsigned-jwt` control 3: an unsigned-subject rule's predicate must
  reference `subject_token`, verified at startup. Control 4: an unverified email is minted only when
  the predicate binds `subject_token.email`.
- ADR `034-audience-absent-impersonation-credentials` control 3: the predicate must reference the
  token variable of each audience-absent role, verified at startup.
- ADR `045-agent-subject-impersonation` decision 2a: an unverified exact-target subject is agent
  category only when the predicate binds `subject_token.sub`.

These controls are stated in terms of a compiled CEL expression. The maintainer directed that OPA
support every existing local-mode credential mode, including unsigned subjects (spec Q2). A static
equivalent for OPA, an analysis of which token maps the decision's dependency graph references, was
designed and then withdrawn on 2026-11-25: it proves only that a policy mentions a value, which shows
intent, not that the policy constrains it, so it is not a security control. The accepted
clarification Q1 limits external data to managed policy/data bundles and prohibits policy-time
networking.

ADR `028-opa-extproc-authorization` governs the separate ExtProc service and is unchanged.

## Decision

1. **Engine choice per rule.** `authorization.type` on an impersonation rule is `cel` or `opa`.
   Exactly one engine block is configured. CEL remains the only engine for role principal/email
   extraction, local token claims, third-party token-exchange authorization, and
   `agent_subject_expression`. OPA is not available for third-party exchange or for proxy/hybrid
   mode. Existing valid CEL rules behave as before. Explicit ordinary-exchange
   `token_exchange.authorization.type: opa` fails startup instead of being accepted and ignored;
   reserved OPA declarations are retained but never instantiate a runtime.
2. **One embedded OPA engine, managed or local bundles.** The broker embeds the OPA Go SDK (already
   a module dependency for ExtProc) as one engine shared by every OPA rule, configured once under
   `impersonation.opa`. Rules name only their decision (`package`, default
   `aib.broker.impersonation`; `decision`, default `result`; `evaluation_timeout`); any other rule
   key fails startup, including the reserved `policy_url` and `policy_path`. The engine loads
   either managed bundles through a bootstrap file (`policy.config_file`) or one local bundle
   directory read at startup (`policy.path`). The bootstrap accepts only `services`, `bundles`,
   `keys`, `labels`, and the broker's `aib_bearer_token_file` credential plugin, each restricted to
   a closed member allowlist for OPA v1.21.4 (config contract `bootstrap_subset.allowlist`): inline
   bearer and OAuth2 client-credentials secrets, cloud credentials in forms that read no file
   (`s3_signing` environment, metadata, or assume-role; `gcp_metadata`; `azure_managed_identity`),
   and the token-file plugin. Decision logs, status, discovery, other plugins, persistence,
   `allow_insecure_tls`, and every other key fail startup. Bootstrap,
   local bundle, and token file are configured by absolute path. Each is read through an
   adapter-owned `os.Root` over its own directory (the bundle directory for a local bundle), so
   content written by others, such as a rotated token or a bundle file, cannot link outside it.
   The token file is re-read before every bundle request, so it rotates without a restart. Every
   OPA-native local-file reference (bearer `token_path`, TLS files, `file://` bundles) is rejected
   before SDK startup. The trust directory of `040-trust-anchor-local-key-sets` is not used and
   keeps its key-set scope.
3. **Broker-owned revisions.** After each OPA activation, the broker builds an immutable candidate
   revision from the compiled modules and a copy of the data. A candidate becomes the active revision
   only if its own transaction includes every configured bundle, it compiles with the restricted
   capabilities (decision 4), and every OPA rule's decision ref `data.<package>.<decision>` resolves
   to at least one rule by exact rule lookup in the candidate's compiler (a value only in data, or
   only nested rules, does not count). Otherwise the whole candidate is rejected and the previous
   active revision stays for every rule. Every request reads one revision before the first rule and
   uses it for every OPA rule's decision, so first-match fallthrough never mixes policy versions. Every admission assigns a process-instance
   UUID plus an increasing engine sequence identifier, copied to deciding audits and the admission log;
   empty/reused manifest labels are not identity. Readiness requires SDK readiness and that same
   admitted complete snapshot, never historical bundle activation plus a subset candidate. There
   is no broker-enforced maximum revision age.
4. **No policy-time networking.** Candidate revisions compile without any built-in that OPA marks
   nondeterministic, except `time.now_ns` and `io.jwt.decode_verify`, which only read the clock. In
   OPA v1.21.4 this removes `http.send`, `net.lookup_ip_addr`, `opa.runtime`, `rand.intn`,
   `uuid.rfc4122`, `io.jwt.encode_sign`, `io.jwt.encode_sign_raw`, `json.verify_schema`, and
   `json.match_schema`. A module that uses a removed built-in cannot activate.
5. **No static binding checks for OPA rules.** The broker does not analyze which token maps an OPA
   decision references. For OPA rules only:
   - ADR 031 control 3 and ADR 034 control 3 do not apply: an unsigned-subject or audience-absent
     rule needs no reference to `input.context.subject_token` or the opted-in role's token map.
   - ADR 031 control 4 does not apply: an unverified email is minted when the rule's
     `email_expression` yields a nonempty email and the decision allows.
   - ADR 045 decision 2a's binding requirement does not apply: an unsigned subject whose raw `sub`
     exactly equals the audience-selected target UUID is agent category, as a signed subject is.
     Another registered agent's UUID as raw `sub` or extracted principal stays a rejected
     cross-agent request (decision 2b).
   The operator's policy is then the only control on which subjects, emails, and agents a privileged
   client may assert, and documentation and examples constrain each of them. Explicit per-rule
   opt-in, signature-verified client assertion and actor, audience-absence checks, category
   eligibility, cross-agent rejection, human delegation, fail-closed evaluation and extraction, and
   credential-free audit stay. CEL rules keep their existing startup checks unchanged.
6. **Fail closed.** `{"action":"allow"}` satisfies the rule's predicate; `{"action":"deny"}` is a
   non-match. A decision rule that exists but is undefined for a request, malformed or other
   actions, evaluation errors, timeouts, and cancellation
   map internally to terminal `server_error` and stop rule evaluation; connected callers receive
   HTTP 500, but disconnected callers have no response guarantee. Cancellation acceptance uses
   request-correlated canceled telemetry and no issuance/fallthrough, with domain error-mapping tests. Credential validation, category eligibility,
   cross-agent rejection, human delegation, and local minting are unchanged and remain
   authoritative over any allow.
7. **Raw-credential-excluded sensitive input and credential-free telemetry.** The input carries the Envoy-compatible request envelope
   (Envoy's `version`, `parsed_query`, and form `parsed_body`, which always omits the credential-bearing
   OAuth parameters `client_assertion`, `actor_token`, `subject_token`, `client_secret`, `refresh_token`,
   `code`, and `code_verifier`) without the raw body or the `authorization`,
   `proxy-authorization`, or `cookie` headers, plus the validated
   CEL-equivalent `context`. Claim maps, decision members other than `action`, and bootstrap values
   never appear in logs, spans, metrics, or error responses. The sensitive named input implements
   redacted `LogValue`, `String`, and `GoString` methods.
8. **Broker-authoritative governance identity in ExtProc's vocabulary.** `context.agent` reuses
   the Grant Context agent object of ADR `048-authzen-delegation-pip`: `id` (UUID) and optional
   `canonical_id`, plus `external_id`, the resolved Agent's literal External ID. Unset IDs are
   omitted. Governance examples key by `external_id` with no UUID fallback. Callers and bundles
   cannot override the mapping; routing/minted identity remains UUID. The input carries
   `type: "impersonation"`, and ExtProc grant-context members are reserved and absent, because
   OPA runs before human delegation.
9. **Envelope consistency without binary coupling.** Keep the domain builder; shared request/input
   vectors under `tests/testdata/opa-envoy-input/` are tested by broker and ExtProc for supported
   common Envoy fields, with broker credential/body exclusions asserted separately. Production
   packages remain independent. Parsing deliberately diverges: broker invalid/error is terminal
   `server_error`; ExtProc invalid/error is deny and ExtProc alone supports approval actions.

## Scope of the Unsigned-Subject Exception

The scope is unchanged. The exception still applies only to the subject role of a local-mode
impersonation rule that declares `verification: none`. Client assertions and actors are always
signature-verified. The constitution's Principle I carve-out requires a startup-verified predicate
reference to `subject_token` and email minting only with a `subject_token.email` binding. Decision 5
drops both for OPA rules, so this ADR needs a constitution amendment, **CONST-AMEND**:
Principle I limits those two controls to rules authorized by a CEL predicate and states that
OPA-authorized unsigned-subject rules rely on the operator's policy, retaining every other control.
The text is in the plan's approval item CONST-AMEND. It was applied to the constitution as v4.8.0 on 2026-11-25 (commit `fb84c31`); it takes effect for OPA rules once ADR-OPA is Accepted.
Implementation of the exceptional modes requires approval of ADR-OPA.

## Consequences

### Positive

- Operators can enforce governance-system registration and ownership checks with Rego and bundled
  data, selected rule by rule, without broker releases.
- All OPA rules of a request see one policy and data version, and shared governance data is
  downloaded and held once.
- OPA rules support every existing credential mode without a static analysis whose result proves
  only intent.
- No request waits on a bundle server or any policy-time network destination.

### Negative

- The broker binary now links OPA, which increases its size and dependency-scanning surface.
- The broker holds two copies of bundled data. Large governance datasets cost memory.
- OPA's internal bundle status can report activation of a revision that the broker rejected. The
  broker's own metrics and logs are authoritative.
- For unsigned subjects, unverified emails, unsigned agent subjects, and audience-absent roles, an
  OPA rule's safety depends solely on review of the operator's policy. A policy that does not
  constrain the caller-asserted value lets the client choose it. CEL rules keep a startup check.
- Without a maximum revision age, a long bundle outage leaves an old governance snapshot active.
  Operators monitor the per-bundle download-age gauge and download counter, which come from an OPA
  bundle status listener, together with the revision-age gauge.
- A reused Rego policy without a `default` decision is undefined for every non-matching request,
  which is a terminal 500 that also blocks later rules. Admission cannot detect this; documentation
  and examples require a `default` decision.
- A large bundle publication can exhaust memory on every replica within one polling interval, and
  readiness then needs the same bundle, so replicas crash-loop until it is withdrawn. Mitigation:
  documented sizing and `size_limit_bytes`.
- Bundle transport and signing are operator choices; the broker accepts `http` services and unsigned
  bundles, and documentation recommends `https` and signing. Constitution Principle I ("security
  features MUST be enabled by default") is read as covering broker controls, not the operator's
  trusted bundle distribution channel, consistent with ExtProc's OPA handling under ADR
  `028-opa-extproc-authorization`. Signing cannot be on by default without operator-supplied keys.
- Inline bootstrap credentials require a restart to rotate. The contained token-file plugin covers
  one bearer token per bootstrap; mTLS client certificates and OPA-native file credentials stay
  unsupported rather than reopened outside containment.
- Rules are coupled at admission: a revision that lacks one OPA rule's decision rule keeps every OPA
  rule on the previous revision. A rejected bundle also stays in OPA's store and is not downloaded
  again while its ETag is unchanged, so later updates to other bundles are rejected too until it is
  republished.
- A local bundle directory changes only on restart.

### Risks

- A Rego policy that does not constrain the subject, email, `sub`, or an audience-absent role.
  Mitigation: policy review, documentation, and examples that constrain each value.
- A future OPA built-in with external effects that OPA does not mark nondeterministic. Mitigation:
  OPA upgrades re-run the capability tests, and the review checklist covers new built-ins.

## Alternatives Considered

- **Policy-time HTTP lookups** (spec Q1 alternatives B and C): rejected by the accepted Q1.
- **OPA sidecar or remote decision service**: out of scope; adds an availability dependency per
  request.
- **Evaluate with OPA's own compiler and store**: cannot enforce capabilities or keep one revision
  per request.
- **Signed-only OPA rules**: withdrawn by maintainer direction Q2.
- **Static binding analysis of the decision graph** (withdrawn 2026-11-25): rejected. OPA is
  already a capable policy language, and the check was shallow: it proves only that a policy
  mentions a value, which shows intent, not that the policy constrains it. It is not a security
  control; policy review is. It also added dependency-graph, `with`-modifier, and revision-coupling
  rules.
- **Unsigned exact-target subjects stay human unless the decision opts in** (for example
  `{"action":"allow","subject_category":"agent"}`), and **the decision echoes the approved `sub` or
  email for the broker to compare** (both considered 2026-11-26): rejected by the user, who chose
  policy review as the control. Either option reintroduces a broker-side binding protocol, and it
  proves only that the policy returned a value, not that it constrained the subject sensibly.
- **Undefined decision as deny, or admission requiring a `default` rule** (considered 2026-11-26):
  rejected by the user in favor of the fail-closed terminal error plus documentation; an undefined
  decision may hide a policy defect, and admission cannot prove a rule is total.
- **Require `https` bundle services, or `https` plus bundle signing, by default** (considered
  2026-11-26): rejected. Bundles and the bootstrap are operator-trusted configuration, as in
  ExtProc; transport and signing are operator deployment choices that documentation recommends.
- **Keep Principle I unchanged and allow only signed roles with an expected audience for OPA**:
  avoids the constitution amendment but drops modes that Q2 requires.
- **UUID-only governance lookup**: operators would have to synchronize UUID-to-governance maps.
- **Adapter-shaped engine-neutral input**: sound alternative; shared vectors retain the current
  domain builder without duplicate untested Envoy semantics.
- **Files under `security.trust_material_dir` or a dedicated OPA directory setting**: the admin sets
  both the directory and the paths, so it is no boundary against the admin; the trust directory
  would also widen an accepted ADR and share a mount sized for small key sets.
- **Unrestricted bootstrap/SDK credential reads**: preserves native file rotation but permits
  reading unrelated runtime files; rejected in favor of containment and a smaller bootstrap subset.
- **One OPA runtime per rule**: isolates rules, but a request falling through rules can see two
  policy versions, and shared bundles are downloaded and held twice (research R11).
- **Managed bundles only**: needs a bundle server for development and tests (research R15).
- **Require nonempty changing manifest labels**: would reject publishers that omit/reuse labels;
  broker admission identifiers identify deciding snapshots without restricting publishers.

## References

- Spec: `specs/20261120-153502-opa-impersonation-authorization/spec.md` (FR-001–FR-013, SR-001–SR-006)
- Research: `specs/20261120-153502-opa-impersonation-authorization/research.md` (R2–R4, R6–R11, R13, R15, R16; R5 withdrawn)
- Contracts: `specs/20261120-153502-opa-impersonation-authorization/contracts/`
- ADRs `009-cel-for-authorization-policies`, `011-extproc-standalone-binary`,
  `028-opa-extproc-authorization`, `031-unverified-subject-unsigned-jwt`,
  `032-impersonation-requires-user-delegation`, `034-audience-absent-impersonation-credentials`,
  `045-agent-subject-impersonation`
- Constitution Principles I, II, III, VI
