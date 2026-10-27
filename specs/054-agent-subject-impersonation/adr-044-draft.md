# ADR 044: Agent-Subject Impersonation Without User Delegation

**Date**: 2026-10-27
**Status**: Proposed
**Feature**: 054-agent-subject-impersonation
**Location**: Draft kept in the feature directory during planning. On acceptance it moves to `adrs/044-agent-subject-impersonation.md`.
**Supersedes (in part)**: [ADR 032](../../adrs/032-impersonation-requires-user-delegation.md), decisions 1 and 2, for agent-category requests only

## Context

[ADR 032](../../adrs/032-impersonation-requires-user-delegation.md) requires an active user delegation for every impersonation request, signed or unverified, with no opt-out. Feature 054 (issue #222) adds a second class of request. A privileged client with an external machine identity obtains a broker token whose subject is a registered agent acting for itself. That subject has no human to consent. To satisfy ADR 032, an operator would have to invent a human principal and a delegation for a machine. That defeats the purpose of the delegation check and pollutes consent state.

The clarified specification limits the extension:

- Agent issuance requires explicit per-rule enablement. Existing rules stay human-only by default.
- The verified signed subject `sub` must be byte-for-byte equal to the UUID of the target agent selected by the audience. The request has no category input, and no aliases or cross-agent representation are allowed.
- Credential validation and the rule's CEL authorization remain mandatory, and `act` is kept.
- A token issued to an agent subject can never be exchanged for a third-party service token.
- Unsigned subjects (ADR 031) remain human-only.

## Decision

1. **Classification and narrow supersession.** An **agent-category** request is one where the rule's subject role is signed (`verification: jwks`), the subject credential verified, and its raw verified `sub` claim is a string exactly equal to the target agent's canonical UUID. Classification is independent of `allowed_subject_categories`; that list is a separate eligibility gate (decision 3).

   For an eligible, CEL-authorized agent-category request, this ADR supersedes ADR 032 decision 1's mandatory delegation check and decision 2's extracted-principal-as-minted-`sub` invariant. All configured principal extractions must still succeed, but the minted `sub` is the target UUID (equal to the raw verified `sub`), not necessarily the extracted subject principal. No additional equality restriction is imposed on `principal_expression`. The broker does not call `UserDelegationVerifier` and creates no grant or session.
2. **Human invariants are preserved.** Every unsigned subject and every signed subject whose raw verified `sub` does not exactly equal the target UUID is human-category. For eligible human requests, ADR 032 decisions 1–6 continue to apply in full: the extracted principal remains both the byte-for-byte minted `sub` and the delegation principal. Signed exact equality is never human-category, including on human-only rules.
3. **Category gating precedes delegation.** A rule that does not allow the classified category falls through, like a policy denial. A human delegation failure stays terminal and is never retried as an agent request. Signed exact equality on a human-only rule (including the omitted setting) is ineligible even when an identically named human has active delegation, and is never retried as human on that rule. Ordered evaluation may consider later rules only under their own eligibility and authorization constraints.
4. **Default is human only.** When `allowed_subject_categories` is omitted, the rule allows `[human]`. Empty, unknown, duplicate, or non-list values fail startup. A rule with an unsigned subject cannot allow `agent`.
5. **Distinguishable tokens.** An agent-subject token carries the reserved claim `subject_category: "agent"`. Local token-claims policy cannot set or remove this claim. Human tokens do not carry it.
6. **No third-party exchange.** RFC 8693 third-party exchange rejects any validated subject token whose `subject_category` is `agent`, with `403 access_denied`. The check runs before any grant or session lookup, so a human principal with the same identifier text cannot lend credentials to the request.
7. **Auditability.** Every impersonation decision records the subject category together with the represented identity, actor, privileged client, rule, and target. No credentials are recorded.

## Consequences

### Positive

- Unattended agent workloads obtain attributable broker tokens without fabricated human identities or consent records.
- Existing human delegation and response invariants are preserved, except for the intentional human-only signed exact-equality rejection described below. Agent issuance is explicit and limited to a single target.
- Third-party credentials remain bound to human delegation. Agent tokens cannot reach them through an identifier collision.

### Negative

- A request whose verified `sub` exactly equals the target UUID is now rejected on a human-only rule, even when a human with that identifier text holds an active delegation (spec Q4). Such collisions are not expected in practice.
- `subject_category` becomes a reserved claim. A deployment whose `token_claims_expression` already emits it loses the claim, and the broker logs a warning.
- A trusted external issuer that emits `subject_category: "agent"` has its tokens rejected for third-party exchange.

### Non-goals

- Third-party service identities for agents, cross-agent representation, unsigned agent subjects, a new grant type, and proxy or hybrid support are all out of scope.

## References

- `specs/054-agent-subject-impersonation/spec.md` (FR-001..FR-012, CFG-001, SR-001..SR-006)
- `specs/054-agent-subject-impersonation/research.md` (R1–R8)
- [ADR 009](../../adrs/009-cel-for-authorization-policies.md), [ADR 014](../../adrs/014-oauth2-server-mode.md), [ADR 031](../../adrs/031-unverified-subject-unsigned-jwt.md), [ADR 032](../../adrs/032-impersonation-requires-user-delegation.md), [ADR 033](../../adrs/033-request-security-context-propagation.md), [ADR 034](../../adrs/034-audience-absent-impersonation-credentials.md)
