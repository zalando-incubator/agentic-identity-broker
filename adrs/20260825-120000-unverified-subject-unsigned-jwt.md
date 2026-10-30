# ADR 20260825-120000: Bounded Unsigned Unverified Subject JWT for Impersonation

**Status**: Accepted
**Date**: 2026-08-25

---


## Context

Feature 20260825-120000 (OAuth2 User Impersonation) lets a privileged client mint a broker-issued access
token that represents a user (`sub`) while attributing the acting party via the standard `act`
claim. The privileged client authenticates with a **signed** client assertion and supplies a
**signed** actor token. For the subject, two paths exist:

1. **Signed subject token** — a JWT under the RFC 8693 JWT token type
   (`urn:ietf:params:oauth:token-type:jwt`), fully signature-verified against a trusted issuer.
2. **Unverified subject** — for privileged clients that hold no signed subject token (for
   example a Slack or Google Chat bridge whose subject is a chat user with no upstream OIDC
   token). This path carries an **unsigned (`alg:none`) subject JWT** under the RFC 8693 JWT
   token type (`urn:ietf:params:oauth:token-type:jwt`), used purely as a structured carrier for
   multiple caller-asserted attributes (principal ID + optional email).

Constitution **Principle I** states signature validation is NEVER optional and no code path
shall skip cryptographic verification. Accepting an unsigned (`alg:none`) JWT — even only in the
subject role — is a literal deviation from that binding rule. Constitution **Principle II**
requires a superseding ADR or constitution amendment to deviate from a binding principle. This
ADR and the paired Constitution Principle I carve-out are the superseding decision and merge
together with the feature, approved by a reviewing maintainer.

The tension is genuine but narrow: **signature verification is only a trust mechanism when a
signature is meant to convey issuer trust.** On the unverified path there is no issuer whose
signature would mean anything — the attributes are, by design, asserted by the already-verified
privileged client. Verifying a signature on caller-supplied data that carries no issuer trust
would be security theater, not a control. The real trust boundary is (a) the signed client
assertion and (b) an authorization predicate that must explicitly bind the asserted subject.

## Decision

Approve a **bounded, single-role exception** to Constitution Principle I: the broker MAY accept
an **unsigned (`alg:none`) JWT for the unverified subject role only**, under the following
non-negotiable constraints. All other credential roles remain fully governed by Principle I.

### Scope of the exception (exhaustive)

- Applies **only** to the unverified subject role, selected only when its matching rule declares
  `verification: none`. The subject is an unsigned (`alg:none`) JWT under the RFC 8693 JWT token
  type (`urn:ietf:params:oauth:token-type:jwt`).
- **Never** applies to the client assertion, the actor token, or a signed subject (a rule with
  `verification: jwks`). Those roles MUST always be signature-verified; no configuration setting
  may disable that verification. A signed JWS submitted to an unverified rule is ALWAYS rejected
  by the `alg:none` guard, and an unsigned (`alg:none`) subject submitted to a signed rule is
  ALWAYS rejected.
- Available in **local mode only** (impersonation itself is local-mode only).

### Compensating controls (all REQUIRED)

1. **Trust derives from the signed client assertion, not the subject.** The privileged client is
   authenticated by a signature-verified client assertion against a trusted external issuer. The
   unsigned subject JWT is treated strictly as caller-asserted data; the broker MUST NOT verify or
   require a signature on it and MUST NOT resolve it against any trusted token issuer.
2. **Explicit opt-in per use case.** The unverified subject mode is OFF unless a use case
   explicitly declares it. There is no global default-on path.
3. **Mandatory subject binding in authorization.** A use case that accepts the unverified
   subject mode MUST have exactly one authorization CEL predicate that references `subject_token`.
   This is verified at startup from the compiled expression's variable references; startup fails
   otherwise. A predicate that constrains only the client assertion cannot implicitly authorize an
   arbitrary unverified subject.
4. **Caller-controlled email is not mintable unless bound.** Because an unverified subject is
   caller-controlled and the broker signs its output, a minted `email` claim MUST NOT be emitted
   unless the matching use case's authorization predicate binds `subject_token.email`. A
   signed-subject email originates from a verified issuer and requires no such binding.
5. **Fail closed.** Authorization evaluation errors or timeouts, extraction failures, or an empty
   subject identity reject the request with an OAuth2 error and issue no token.
6. **Auditability.** Every impersonation decision (success or failure) emits a structured audit
   event recording outcome, selected use case, trusted issuer identifiers and roles, and available
   validated identities, and never contains credential values.

### Governance instrument

This ADR is the superseding decision of record. It is paired with a carve-out note in Constitution
Principle I that names this ADR and its bounded scope, so the constitution and the ADR set remain
internally consistent. The deviation is limited to exactly the scope above; any future widening
(additional roles, other modes, removal of a compensating control) requires a new superseding ADR.

## Consequences

### Positive

- Privileged clients without an upstream signed subject token (chat bridges, gateways) can convey
  a multi-attribute subject (principal ID + optional email) through impersonation.
- The trust boundary is explicit and reviewable: a verified client assertion plus a
  subject-binding authorization predicate, not an unverifiable signature.
- Principle I remains fully intact for every credential role where a signature actually conveys
  issuer trust; the exception is surgically bounded and startup-enforced.
- Startup-time enforcement (mandatory `subject_token` reference; email-binding requirement) makes
  the guardrails non-optional and auditable rather than reliant on operator discipline.

### Negative

- The constitution's otherwise-absolute "signature validation is never optional" rule now has one
  named, bounded exception, adding governance surface that reviewers must understand.
- Operators authoring an unverified use case carry real responsibility: a weak authorization
  predicate is the only thing standing between a privileged client and an arbitrary asserted `sub`.

### Risks

- A privileged client whose signing key is compromised could assert arbitrary subjects within the
  bounds its use-case predicate allows. Mitigation: the client-assertion issuer is a
  security-sensitive deployment input (see ADR 20260811-120000) and the predicate constrains the subject space.
- Misreading the exception as broader than the subject role. Mitigation: an unverified rule
  rejects signed JWSs through its `alg:none` guard, and signed roles reject unsigned or
  `none`-algorithm JWTs; the exception is keyed strictly to `verification: none`.
- An operator could author an unverified use case whose predicate technically references
  `subject_token` but constrains it weakly. Mitigation: this is a policy-review responsibility;
  the configuration example (FR-014) demonstrates a properly bound predicate.

---

## References

- Feature spec: `specs/20260825-120000-oauth2-user-impersonation/spec.md` — FR-003d, FR-005, FR-006a,
  FR-007, FR-007a, CR-005a, CR-007
- Constitution: `.specify/memory/constitution.md` — Principle I (Security-First),
  Principle II (ADRs are Binding)
- [ADR 20260116-120100: CEL for Authorization Policies](20260116-120100-cel-for-authorization-policies.md)
- [ADR 20260328-120100: OAuth2 Authorization Server Mode](20260328-120100-oauth2-server-mode.md)
- [ADR 20260811-120000: Token Exchange Client-Assertion Trust Anchor](20260811-120000-token-exchange-client-assertion-trust-anchor.md)
- [RFC 8693: OAuth 2.0 Token Exchange](https://www.rfc-editor.org/rfc/rfc8693)
