# ADR 045: Agent-Subject Impersonation Without User Delegation

**Date**: 2026-10-27
**Status**: Accepted (approved by mjungsbluth on 2026-10-29: https://github.com/zalando-incubator/agentic-identity-broker/pull/293#issuecomment-6495558326); revised in this PR by architecture-review items A11 and A12, approved by mjungsbluth on 2026-10-31 (https://github.com/zalando-incubator/agentic-identity-broker/pull/293#issuecomment-6546623321); decision 2a revised by item A13, approved by mjungsbluth on 2026-11-03 (https://github.com/zalando-incubator/agentic-identity-broker/pull/293#issuecomment-6601909328)
**Feature**: 054-agent-subject-impersonation
**Supersedes (in part)**: [ADR 032](032-impersonation-requires-user-delegation.md), decisions 1 and 2, for agent-category requests only

## Context

[ADR 032](032-impersonation-requires-user-delegation.md) requires an active user delegation for every impersonation request, signed or unverified, with no opt-out. Feature 054 (issue #222) adds a second class of request. A privileged client with an external machine identity obtains a broker token whose subject is a registered agent acting for itself. That subject has no human to consent. To satisfy ADR 032, an operator would have to invent a human principal and a delegation for a machine. That defeats the purpose of the delegation check and pollutes consent state.

The clarified specification limits the extension:

- Agent issuance requires explicit per-rule enablement. Existing rules stay human-only by default.
- The subject `sub` must be byte-for-byte equal to the UUID of the registered target agent selected by the audience. A signed subject must verify; an unverified subject additionally needs a predicate that binds `subject_token.sub` (decision 2a). The request has no category input, and no aliases or cross-agent representation are allowed.
- Credential validation and the rule's CEL authorization remain mandatory, and `act` is kept.
- With the default detector, or a configured detector that still matches them, tokens issued to agent subjects are rejected for third-party service-token exchange before any human grant, session, or provider access. Detection failures fail closed. A configured detector replaces the default (decision 6); a non-matching result continues the existing exchange checks, which may consult matching human grants and sessions and return existing consent errors.
- Unsigned subjects (ADR 031) are agent-category only under decision 2a; ADR 031's compensating controls are otherwise unchanged.

## Decision

1. **Classification and narrow supersession.** An **agent-category** request is one where the rule's subject role is signed (`verification: jwks`), the subject credential verified, and its raw verified `sub` claim is a string exactly equal to the target agent's canonical UUID. Classification is independent of `allowed_subject_categories`; that list is a separate eligibility gate (decision 3).

   For an eligible, CEL-authorized agent-category request, this ADR supersedes ADR 032 decision 1's mandatory delegation check and decision 2's extracted-principal-as-minted-`sub` invariant. All configured principal extractions must still succeed, but the minted `sub` is the target UUID (equal to the raw verified `sub`), not necessarily the extracted subject principal. No additional equality restriction is imposed on `principal_expression`. The broker does not call `UserDelegationVerifier` and creates no grant or session.
2. **Human invariants are preserved.** Every signed subject whose raw verified `sub` does not exactly equal the target UUID, and every unsigned subject not classified as agent under decision 2a, is human-category. For eligible human requests, ADR 032 decisions 1–6 continue to apply in full: the extracted principal remains both the byte-for-byte minted `sub` and the delegation principal. Signed exact equality is never human-category, including on human-only rules.
3. **Category gating precedes delegation.** A rule that does not allow the classified category falls through, like a policy denial. A human delegation failure stays terminal and is never retried as an agent request. Signed exact equality on a human-only rule (including the omitted setting) is ineligible even when an identically named human has active delegation, and is never retried as human on that rule. Ordered evaluation may consider later rules only under their own eligibility and authorization constraints.
2a. **Unverified agent subjects (approved A13).** An unverified subject is agent-category when its raw `sub` exactly equals the UUID of the registered, audience-selected target and the rule's authorization predicate binds `subject_token.sub` (the same field-binding check as the unverified-email control). Target resolution already requires a stored agent record, so the UUID always identifies an existing agent. Without that binding the subject stays human-category. The predicate is the only control on which agent a privileged client may represent, so it must constrain `subject_token.sub` meaningfully, for example `subject_token.sub == request.agent_id`. An unverified exact-target subject bound by the predicate is ineligible on human-only rules, like signed exact equality.
2b. **Cross-agent subjects are rejected (A15, approved by mjungsbluth on 2026-11-04 in response to review comment r4466927172).** A signed subject, or an unverified subject whose predicate binds `subject_token.sub`, whose raw `sub` is the canonical UUID of a registered agent other than the audience-selected target is agent-category and ineligible on every rule. It never falls back to human delegation, even when a same-text human grant exists, and the denial carries no consent link. The broker looks the agent up only when `sub` is a canonical UUID; a lookup failure is a terminal `server_error`.
4. **Default is human only.** When `allowed_subject_categories` is omitted, the rule allows `[human]`. Empty, unknown, duplicate, or non-list values fail startup. A rule with an unsigned subject may list `agent`; startup accepts it, and agent issuance then follows decision 2a.
5. **Distinguishable tokens without a category claim.** An agent-subject token has no additional claim. Its `sub` and `agent_id` both equal the target UUID, and both are reserved base claims that local token-claims policy cannot change. A human token's `sub` is the human principal.
6. **Third-party exchange rejection.** RFC 8693 third-party exchange evaluates the CEL boolean `token_exchange.claim_extraction.agent_subject_expression` on the validated subject token. The default is the guarded form of `sub == agent_id`. A configured expression replaces the default; there is no additional built-in check. A configured expression must reference `subject_token.sub`; otherwise startup fails with a CEL configuration error (approved A12). The operator must keep a configured expression matching broker agent-subject tokens. A `true` result is rejected with `403 access_denied`. An evaluation failure is a terminal `server_error`, never "not an agent". The check runs before any grant or session lookup, so for a `true` result or an evaluation failure a human principal with the same identifier text cannot lend credentials to the request. A `false` result continues the existing exchange checks unchanged.
6a. **Approval authentication rejection (approved A11).** Approval endpoints that authenticate a subject token (`POST /api/approvals` and `POST /api/approvals/{id}/consume`, ADR 018) resolve it through the same domain entry point as third-party exchange: principal extraction, then `agent_subject_expression`, then agent extraction. A detected agent-subject token returns `403 forbidden` before any approval is read or written, so it never acts as a human approval principal. A detection failure returns `503 service_unavailable`, never "not an agent". The client-assertion-only sync endpoint is unaffected.
7. **Auditability.** Every impersonation decision records the subject category together with the represented identity, actor, privileged client, rule, and target. No credentials are recorded.
8. **Contract version (A14, replaces A10; approved by mjungsbluth on 2026-11-03, https://github.com/zalando-incubator/agentic-identity-broker/pull/293#issuecomment-6603568011).** The End-User OpenAPI contract stays at `1.1.0`. Request fields, response envelopes and routes are unchanged. The new denials (A8, A9, decisions 2a and 6a) only affect identities that collide with an agent UUID, so they are fail-closed security behavior, not a contract break. No migration guide or version bump is published.

## Consequences

### Positive

- Unattended agent workloads obtain attributable broker tokens without fabricated human identities or consent records.
- Existing human delegation and response invariants are preserved, except for the intentional human-only signed exact-equality rejection described below. Agent issuance is explicit and limited to a single target.
- With the default detector or a matching override, agent-subject tokens are rejected before human-state access, so they cannot reach third-party credentials through an identifier collision. Detection failures fail closed. A non-matching override removes this protection (see Negative).

### Negative

- A request whose verified `sub` exactly equals the target UUID is now rejected on a human-only rule, even when a human with that identifier text holds an active delegation (spec Q4). Such collisions are not expected in practice.
- With the default expression, a trusted external issuer whose subject tokens carry equal `sub` and `agent_id` claims has those tokens rejected for third-party exchange. So does a broker human token whose minted principal is the exact target UUID text, which is possible only through an unsigned subject or a `principal_expression` that maps another claim.
- An operator who overrides `agent_subject_expression` owns its correctness (accepted replacement semantics). Startup rejects an override that does not reference `subject_token.sub` (for example `false`, `true`, or a comparison on `azp`). This is a necessary, not a sufficient, condition: an override that references `sub` but still stops matching broker agent-subject tokens lets those tokens reach the existing third-party exchange flow. The documentation therefore requires an override to keep matching them, for example by keeping the default as a disjunct.
- On an unsigned-subject rule that lists `agent`, any privileged client the predicate admits can mint a consent-free token for every registered agent whose UUID the predicate accepts in `subject_token.sub`. Operators own that predicate. A predicate that does not bind `sub` keeps such subjects human.

### Non-goals

- Third-party service identities for agents, cross-agent representation, a new grant type, and proxy or hybrid support are all out of scope.

## References

- `specs/054-agent-subject-impersonation/spec.md` (FR-001..FR-012, CFG-001, SR-001..SR-006)
- `specs/054-agent-subject-impersonation/research.md` (R1–R11)
- `specs/054-agent-subject-impersonation/contracts/token-exchange-agent-subject-config.yaml`
- [ADR 009](009-cel-for-authorization-policies.md), [ADR 014](014-oauth2-server-mode.md), [ADR 031](031-unverified-subject-unsigned-jwt.md), [ADR 032](032-impersonation-requires-user-delegation.md), [ADR 033](033-request-security-context-propagation.md), [ADR 034](034-audience-absent-impersonation-credentials.md)
