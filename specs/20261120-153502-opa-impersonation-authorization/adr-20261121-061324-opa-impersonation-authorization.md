# ADR 20261121-061324: OPA Authorization for Broker Impersonation Rules

**Date**: 2026-11-21
**Status**: Proposed
**Feature**: `20261120-153502-opa-impersonation-authorization` (issue #518, PR #546)
**Final location**: `adrs/20261121-061324-opa-impersonation-authorization.md` (moved by the first implementation task after approval)
**Supersedes (in part)**:
- [`009-cel-for-authorization-policies`](../../adrs/009-cel-for-authorization-policies.md): the reservation of `opa` for broker authorization, for impersonation rules only.
- [`031-unverified-subject-unsigned-jwt`](../../adrs/031-unverified-subject-unsigned-jwt.md) compensating controls 3 and 4, [`034-audience-absent-impersonation-credentials`](../../adrs/034-audience-absent-impersonation-credentials.md) compensating control 3, and [`045-agent-subject-impersonation`](../../adrs/045-agent-subject-impersonation.md) decision 2a: only their CEL-specific enforcement mechanism, and only for rules that select OPA.

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

These controls are stated in terms of a compiled CEL expression. A managed OPA policy changes while
the broker runs, so a startup-only check is not equivalent. The maintainer directed that OPA support
every existing local-mode credential mode, including unsigned subjects (spec Q2). The accepted
clarification Q1 limits external data to managed policy/data bundles and prohibits policy-time
networking.

ADR `028-opa-extproc-authorization` governs the separate ExtProc service and is unchanged.

## Decision

1. **Engine choice per rule.** `authorization.type` on an impersonation rule is `cel` or `opa`.
   Exactly one engine block is configured. CEL remains the only engine for role principal/email
   extraction, local token claims, third-party token-exchange authorization, and
   `agent_subject_expression`. OPA is not available for third-party exchange or for proxy/hybrid
   mode. Existing CEL rules behave as before.
2. **Embedded OPA with managed bundles only.** The broker embeds the OPA Go SDK (already a module
   dependency for ExtProc) with one runtime per OPA rule. The bootstrap accepts only the OPA
   configuration keys `services`, `bundles`, `keys`, and `labels`; decision logs, status, discovery,
   plugins, persistence, and every other key fail startup.
3. **Broker-owned revisions.** After each OPA activation, the broker builds an immutable candidate
   revision from the compiled modules and a copy of the data. A candidate becomes the active revision
   only if it compiles with the restricted capabilities (decision 4), defines the decision rule, and
   passes binding admission (decision 5). Otherwise the previous active revision stays. Every
   request reads one revision per rule evaluation and uses it for the decision, subject
   classification, and email handling. There is no broker-enforced maximum revision age.
4. **No policy-time networking.** Candidate revisions compile without any built-in that OPA marks
   nondeterministic, except `time.now_ns`. This removes `http.send`, `net.lookup_ip_addr`, and
   `opa.runtime`, among others. A module that uses a removed built-in cannot activate.
5. **Equivalent binding controls, enforced on every revision.** For an OPA rule, references and
   field bindings are computed from the rules reachable from `data.<package>.<decision>` in the
   compiled dependency graph, resolving imports and rule-local aliases. A reachable `with input`
   modifier voids all evidence. The equivalents of the superseded controls are:
   - ADR 031 control 3 and ADR 034 control 3: the reachable decision references
     `input.context.subject_token` (unsigned-subject rule) and `input.context.<role>` for each
     audience-absent role. A candidate without them is not admitted. Without an admitted revision
     within `startup_timeout`, the broker does not start.
   - ADR 031 control 4: an unverified email is minted only when the evaluated revision binds
     `input.context.subject_token.email`.
   - ADR 045 decision 2a: an unverified exact-target subject is agent category only when the
     evaluated revision binds `input.context.subject_token.sub`; otherwise it stays human.
   These checks are necessary guards, not proof of a restrictive policy. Operators own policy
   quality, as with CEL. CEL rules keep their existing startup checks unchanged.
6. **Fail closed.** `{"action":"allow"}` satisfies the rule's predicate; `{"action":"deny"}` is a
   non-match. Undefined, malformed, or other actions, evaluation errors, timeouts, and cancellation
   return `500 server_error` and stop rule evaluation. Credential validation, category eligibility,
   cross-agent rejection, human delegation, and local minting are unchanged and remain
   authoritative over any allow.
7. **Credential-free input and telemetry.** The input carries the Envoy-compatible request envelope
   without the body, `authorization`, `proxy-authorization`, or `cookie` headers, plus the validated
   CEL-equivalent `context`. Claim maps, decision members other than `action`, and bootstrap values
   never appear in logs, spans, metrics, or error responses.

## Scope of the Unsigned-Subject Exception

Unchanged. The exception still applies only to the subject role of a local-mode impersonation rule
that declares `verification: none`. Client assertions and actors are always signature-verified.
The constitution's Principle I carve-out requires a startup-verified predicate reference to
`subject_token` and email minting only with a `subject_token.email` binding. Decision 5 meets both
at startup and additionally at every activation, so no constitution amendment is needed.

## Consequences

### Positive

- Operators can enforce governance-system registration and ownership checks with Rego and bundled
  data, one rule at a time, without broker releases.
- Binding controls hold across live policy changes, and evidence cannot outlive its revision.
- No request waits on a bundle server or any policy-time network destination.

### Negative

- The broker binary now links OPA, which increases its size and dependency-scanning surface.
- The broker holds two copies of bundled data. Large governance datasets cost memory.
- OPA's internal bundle status can report activation of a revision that the broker rejected. The
  broker's own metrics and logs are authoritative.
- The static analysis does not follow claim maps through function arguments. Such policies provide
  no field-binding evidence: their unverified emails are not minted and their unverified
  exact-target subjects stay human.
- Without a maximum revision age, a long bundle outage leaves an old governance snapshot active.
  Operators monitor the revision-age gauge.

### Risks

- A weak Rego policy that references a token map without constraining it. Mitigation: documentation
  and examples, same as for CEL.
- A future OPA built-in with external effects that OPA does not mark nondeterministic. Mitigation:
  OPA upgrades re-run the capability tests, and the review checklist covers new built-ins.

## Alternatives Considered

- **Policy-time HTTP lookups** (spec Q1 alternatives B and C): rejected by the accepted Q1.
- **OPA sidecar or remote decision service**: out of scope; adds an availability dependency per
  request.
- **Evaluate with OPA's own compiler and store**: cannot enforce capabilities or couple evidence to
  the evaluated revision.
- **Signed-only OPA rules**: withdrawn by maintainer direction Q2.
- **Operator assertion of binding**: forbidden by spec SR-005.

## References

- Spec: `specs/20261120-153502-opa-impersonation-authorization/spec.md` (FR-001–FR-013, SR-001–SR-006)
- Research: `specs/20261120-153502-opa-impersonation-authorization/research.md` (R2–R9, R13)
- Contracts: `specs/20261120-153502-opa-impersonation-authorization/contracts/`
- ADRs `009-cel-for-authorization-policies`, `011-extproc-standalone-binary`,
  `028-opa-extproc-authorization`, `031-unverified-subject-unsigned-jwt`,
  `032-impersonation-requires-user-delegation`, `034-audience-absent-impersonation-credentials`,
  `045-agent-subject-impersonation`
- Constitution Principles I, II, III, VI
