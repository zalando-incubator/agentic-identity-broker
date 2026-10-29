# ADR 046: Virtual Service Scopes for Incremental Authorization

**Status**: Proposed (feature-local draft; requires maintainer approval, approval item ADR-SCOPES)
**Date**: 2026-10-29
**Scope**: Broker OAuth2 authorization server (local, proxy, hybrid), consent, RFC 8693 errors, ExtProc challenge
**Feature**: [055-incremental-service-authorization](../spec.md)

## Context

An MCP client cannot tell the broker which optional third-party service its next tool call needs.
When an optional service has no usable session, or the grant does not cover it, a silent refresh
or authorization succeeds anyway, and the next tool call fails again. This is the reauthorization
loop in issue #299. MCP's step-up model (RFC 6750 `insufficient_scope` with `scope`) lets the
client request more scope interactively, but the broker has no scope value that names one of its
services.

## Decision

1. **Scope value**: Each configured ThirdpartyOAuth2Service has a broker-only Virtual Service Scope,
   `<server.enduser.public_url>/sessions/<identifier>`. The identifier is the current canonical ID
   or the UUID. Recognition is an exact string-prefix match on the configured public URL. The
   grammar is in `contracts/virtual-service-scope.md`. Values outside that prefix are ordinary
   scopes.
2. **Meaning**: A virtual scope requires, for this request only, that the authenticated user hold a
   usable session for the service. A selected permission set must cover that session for the
   agent, including the agent's configured required provider scopes. A virtual scope never grants
   provider scopes, never selects permission sets, and never adds a service beyond the agent's
   configuration.
3. **One owner**: A new bounded context, `internal/domain/servicescope/`, owns parsing, formatting,
   requested-set resolution, eligibility, the coverage rule, and usability evaluation. The
   authorization, consent, local-issuance, and token-exchange services all consume it.
4. **Local issuance**:
   - Granted virtual scopes appear in the token-response `scope` and the access-token `scope`
     claim, in their requested form.
   - Refresh cannot widen to a virtual scope that was not granted (`invalid_scope`).
   - Refresh re-checks every granted virtual scope, even when `scope` is omitted. The check runs
     before rotation, so a failure never consumes the refresh token.
5. **Identity pinning**: Each granted virtual scope's resolved service UUID is persisted as a
   Service-Scope Binding on the authorization code and refresh-token session. Refresh fails closed
   when the scope no longer resolves to the bound UUID. Only UUID scopes survive canonical-ID
   renames, and no alias history is kept.
6. **Proxy**:
   - Proxy authorization checks requested services exactly as local authorization does, then
     removes virtual scopes from the upstream request.
   - Proxy refresh removes virtual scopes and forwards the upstream outcome unchanged. It does not
     veto the response afterwards.
   - The broker never modifies upstream tokens.
   - Hybrid mode follows the agent's client type.
7. **Discovery**: Virtual scopes are never listed in `scopes_supported`. Clients learn them from the
   token-exchange `scope` error member, which the gateway copies into its `insufficient_scope`
   challenge, or from operator route configuration.

## Consequences

- One focused consent interaction can repair an optional service. The user's other optional
  selections are not touched.
- The broker's issued tokens reveal which managed services a client was granted access to. The
  issuer is the same broker, and audience handling does not change.
- `authorization_codes` and `refresh_token_sessions` gain a nullable `service_scope_bindings`
  column (migration 046, decision DEC-004).
- Agents with a non-empty `allowed_scopes` can request virtual scopes without listing them. The
  broker checks eligibility separately.
- Proxy refresh gives weaker assurance than local refresh. This is documented.

## Alternatives considered

- **Rich Authorization Requests (RFC 9396) `authorization_details`**: This is more expressive, but
  MCP clients and gateways challenge with `scope`, not `authorization_details`. It was rejected for
  interoperability.
- **Opaque scope names such as `aib:service:github`**: These cannot double as the human-readable
  recovery page. They were rejected because the spec requires a scope URL that a person can open.
- **Making requested services permanently mandatory**: This changes agent configuration based on
  client input. It was rejected because it violates FR-007 and SR-001.
- **Canonical-ID alias history**: This adds retention and ownership policy. It was rejected for
  this release (DEC-001).

## References

- Feature 055 [spec](../spec.md), [research](../research.md) R1–R8, [data model](../data-model.md),
  and [contracts](../contracts/).
- [ADR 014](../../../adrs/014-oauth2-server-mode.md), [ADR 016](../../../adrs/016-authorization-session-anti-spoofing.md),
  [ADR 036 (ExtProc metadata input)](../../../adrs/036-extproc-metadata-token-exchange-input.md),
  [ADR 045 draft](./045-service-session-recovery-return.md).
