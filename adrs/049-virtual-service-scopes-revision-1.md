# ADR 049 Revision 1 — Proxy-Refresh Validation and Dot-Segment Canonical IDs

**Status**: Accepted (approved by mjungsbluth on 2026-11-13: https://github.com/zalando-incubator/agentic-identity-broker/pull/323#issuecomment-6857846872)
**Date**: 2026-11-07
**Revises**: [ADR 049](049-virtual-service-scopes.md), introduced and accepted in
this pull request. Under Principle II, the same pull request may revise it before merge.
The supplied approval covers this revision; implementation applied the changes below to
`adrs/049-virtual-service-scopes.md` and recorded the approval there.

## Context

Reanalysis of Feature 055 found two gaps in ADR 049:

- **I2**: Decision 6 says that proxy refresh removes virtual scopes and forwards the upstream
  outcome. FR-002 also requires the broker to reject malformed, unknown and ineligible broker
  service scopes. ADR 049 does not say how proxy refresh rejects them.
- **U4**: Canonical-ID validation accepts the complete identifiers `.` and `..`. Browsers normalize
  these path segments, so the emitted scope, service page and recovery link of such a service do
  not reliably name it.

The maintainer directed the resolution of both gaps in the plan run on PR #323.

## Changes to ADR 049

### Decision 6 (Proxy), second bullet

Replace:

> Proxy refresh removes virtual scopes and forwards the upstream outcome unchanged. It does not
> veto the response afterwards.

with:

> Before proxy refresh forwards a request whose `scope` contains a broker-prefixed token, it
> validates the scope syntax, resolves the agent through the existing token-endpoint client
> resolution, and checks service existence and agent eligibility. Malformed, unknown and
> ineligible scopes return `400 invalid_scope` (`requested service scope is not permitted`), and a
> lookup failure returns `500 server_error`, with no upstream call. Requests with only ordinary
> scopes keep their existing path. Proxy refresh then removes the valid virtual scopes and forwards
> the upstream outcome unchanged. It checks no provider-session usability and does not veto any
> upstream response or rotation.

### Consequences, new bullet

> - Known limitation: a service whose canonical ID is exactly `.` or `..` has a scope, service
>   page and recovery link that browsers normalize to another path. The broker adds no UUID
>   fallback for these identifiers and does not change canonical-ID validation. Issue #446
>   restricts them. Operators can give such a service another canonical ID or none.

### Alternatives considered, new bullets

> - **Silently strip invalid broker scopes on proxy refresh**: Rejected because it contradicts
>   FR-002 and hides an invalid service request from the client.
> - **UUID-only emission for the canonical IDs `.` and `..`**: Rejected for this feature. The
>   root cause is canonical-ID validation, which issue #446 fixes for services and agents.

## Consequences of this revision

- Proxy refresh with an invalid broker service scope fails before any upstream call. The upstream
  refresh token stays valid.
- Proxy refresh adds a service lookup only when the request carries a broker-prefixed scope.
- The dot-segment defect stays visible in ADR 049 until issue #446 removes it.

## References

- Feature 055 [spec](../specs/055-incremental-service-authorization/spec.md) FR-001, FR-002, FR-021 and Clarifications Session 2026-11-07
- [research.md](../specs/055-incremental-service-authorization/research.md) R7 and R14
- [End-User API contract §4](../specs/055-incremental-service-authorization/contracts/enduser-api.md#4-post-oauth2token-proxy-refresh_token-dec-002-api-proxy-refresh-validation)
- [Virtual Service Scope contract](../specs/055-incremental-service-authorization/contracts/virtual-service-scope.md)
- [Issue #446](https://github.com/zalando-incubator/agentic-identity-broker/issues/446)
