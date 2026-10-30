# ADR 20260903-120000: Audience-Absent Impersonation Credentials

**Status**: Accepted
**Date**: 2026-09-03

---

## Context

Impersonation signed credentials use `expected_audience` for the CR-003 audience binding.
Some identity providers mint legitimate signed credentials without an `aud` claim. Those roles cannot be configured today.

The `aud` claim prevents a confused-deputy attack. Removing this check would allow replay of a token that the issuer minted for another relying party.

## Decision

Add opt-in `audience_requirement: absent` for each signed role. The role can be a client assertion, actor token, or signed subject token.

The setting uses require-absent semantics. The broker accepts a credential only when it has no `aud` claim. The broker rejects a credential that has an `aud` claim.

This setting inverts the confused-deputy guard rather than removing it. A token minted for another audience has an audience and is rejected.

The broker continues to validate the signature, each issuer's asymmetric algorithm allow-list, issuer, expiry, and not-before claim. This setting does not apply to the unverified subject, which has no audience concept.

## Compensating Controls

The following controls are required:

1. The setting is opt-in for each role and is off by default.
2. The setting and `expected_audience` are mutually exclusive. The loader and the domain enforce this rule at startup.
3. The authorization predicate must reference the token variable of each relaxed role. The domain validates this requirement from the compiled expression at startup.
4. The broker fails closed after a validation or extraction error.
5. The existing credential-free impersonation decision event audits this configuration path.

## Consequences

### Positive

Identity providers that omit `aud` are supported without weakening the cross-audience replay guard.

### Negative

This adds a security-relevant configuration setting that reviewers must understand. A weak predicate is the main remaining constraint for an audience-less role.

### Risks

An operator can configure an over-broad predicate. Policy review is responsible for this risk. The example configuration shows a predicate that binds the relaxed role.

---

## References

- Feature specification: `specs/20260825-120000-oauth2-user-impersonation/spec.md` — FR-004, FR-016, CR-003, CR-009
- Constitution: `.specify/memory/constitution.md` — Principles I and II
- [ADR 20260811-120000: Token Exchange Client-Assertion Trust Anchor](20260811-120000-token-exchange-client-assertion-trust-anchor.md)
- [ADR 20260825-120000: Bounded Unsigned Unverified Subject JWT for Impersonation](20260825-120000-unverified-subject-unsigned-jwt.md)
- [RFC 8693: OAuth 2.0 Token Exchange](https://www.rfc-editor.org/rfc/rfc8693)
