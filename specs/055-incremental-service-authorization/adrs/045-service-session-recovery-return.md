# ADR 045: Service-Specific Session Recovery Return and Persisted Rejection

**Status**: Proposed (feature-local draft; requires maintainer approval, approval item ADR-RECOVERY)
**Date**: 2026-10-29
**Scope**: End-User API RFC 8693 recoverable session errors, third-party session recovery, stored session usability
**Feature**: [055-incremental-service-authorization](../spec.md)

## Supersedes

This ADR supersedes exactly two clauses of
[ADR 044: Service-Specific Session Recovery URL](../../../adrs/044-service-specific-session-recovery-url.md):

1. The fixed `/sessions` return URI in the `error_uri` that ADR 044 defines.
2. The sentence "Provider refresh rejection leaves the stored session unchanged."

Every other ADR 044 decision remains binding:

- The authorization origin is the configured `PublicURL`, and the service UUID comes from resource
  resolution.
- The broker never uses the request Host, a query value, or a provider URL to build the link.
- The flow is a PKCE-protected browser GET, and same-origin `redirect_uri` validation still applies.
- Grant denials point to `/agents/{agent-id}`.
- Non-recoverable errors have no `error_uri`.
- The broker's `/oauth2/token` sends no `WWW-Authenticate` header.
- ExtProc forwards the URI unchanged.

ADR 044 is not edited in place.

## Context

Feature 055 lets users and MCP clients recover one broker-managed service. Each service has a
session page, `/sessions/{canonical-id-or-UUID}`. That page URL is also the service's Virtual
Service Scope. ADR 044 returns the browser to the generic `/sessions` list after provider sign-in,
so the user lands away from the service they are repairing (spec FR-006, SC-003).

ADR 044 also states that provider refresh rejection leaves the stored session unchanged. Since #271,
the broker has persisted a definitive `invalid_grant` refresh rejection. The broker sets
`refresh_token_expires_at` to the rejection time under the session row lock, and stops contacting
the provider until the user reconnects (`specs/013-token-exchange/spec.md` User Story 5, Scenario 4,
and Journey 1). The End-User OpenAPI already documents this behavior. Consent, session pages, tool
exchange, and background maintenance must all agree on this state (spec FR-008, FR-018), so the
ADR must describe it.

## Decision

### 1. Recovery return target

For a recoverable RFC 8693 provider-session failure, the broker returns `400 invalid_grant` with:

```text
<public URL>/api/third-party/{serviceId}/oauth2/authorize?redirect_uri=<url.QueryEscape(<public URL>/sessions/{identifier})>
```

`{serviceId}` is the resolved service UUID. `{identifier}` is the service's current canonical ID,
or its UUID when it has none. For `PublicURL=https://broker.example.com`, service UUID
`880e8400-e29b-41d4-a716-446655440010`, and canonical ID `github`, the link is:

```text
https://broker.example.com/api/third-party/880e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions%2Fgithub
```

The authorize handler's same-origin `redirect_uri` validation does not change, and already accepts
this target. After the provider callback, the browser returns to the service page. The existing
callback result parameters are appended. Visiting the page or completing provider sign-in never
creates or changes an agent grant.

### 2. Persisted definitive rejection

Only a definitive provider rejection of the stored refresh token (`invalid_grant`) makes a session
unusable in storage. The broker records it under `WithLockedSession` by setting
`refresh_token_expires_at` to the rejection time. Stored ciphertext and identifiers are unchanged.
The following rules apply:

- Transient failures (provider 5xx, 429, network errors, timeouts, storage errors, cancellation)
  never write this marker.
- A deliberate reconnect replaces the session tokens under the same row lock and clears the marker.
  A rejection that was computed for an older token cannot overwrite a newer reconnect, because the
  refresh re-reads the locked row.
- Termination deletes the row. A late refresh or callback cannot recreate it, as ADR 038 already
  guarantees.

No schema change is required. The marker is the single persisted unusability state for every
consumer.

## Consequences

- Users who follow an exchange recovery link, or Login on a service page, return to that service's
  page.
- Clients that treated the encoded return target as exactly `/sessions` must accept the
  service-specific path. Both are broker-selected values of the existing optional `error_uri`, so
  End-User API versioning follows the rule recorded in ADR 044.
- A canonical-ID rename changes the return path of newly issued links. Links that were already
  issued with the old canonical path land on the not-found page after a rename, and the user can
  navigate from the session list.
- The ADR set now matches the implemented rejected-refresh behavior.

## Alternatives considered

- **Keep `/sessions`**: Rejected. The user must find the service again in a list, and SC-003
  requires reaching the same service page directly.
- **Always use the UUID in the return path**: Rejected. The page URL would differ from the emitted
  Virtual Service Scope and from the identifier users see.
- **Accept a caller-supplied return path**: Rejected. It reopens open-redirect and parameter
  injection risks that ADR 044 closed.
- **Add a new `unusable_at` column**: Rejected. The existing marker already expresses the state
  consistently, and a second field would need reconciliation.

## References

- [ADR 044](../../../adrs/044-service-specific-session-recovery-url.md) (partially superseded)
- [ADR 038](../../../adrs/038-proactive-token-refresh.md) (refresh concurrency and update-only termination)
- [ADR 016](../../../adrs/016-authorization-session-anti-spoofing.md)
- Feature 055 [spec](../spec.md) FR-006, FR-008, FR-016, FR-018. [research.md](../research.md) R3 and R9.
