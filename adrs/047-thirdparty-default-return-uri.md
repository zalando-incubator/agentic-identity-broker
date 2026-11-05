# ADR 047: Default Third-Party Login Return URI

**Status**: Accepted
**Date**: 2026-11-05
**Scope**: Third-party session GET authorization initiation

## Context

Issue [#212](https://github.com/zalando-incubator/agentic-identity-broker/issues/212),
opened by mjungsbluth, requests opening a bare third-party authorize URL to verify
service setup, returning to `/sessions` after provider login. The existing handler
rejects missing return URIs before initiating a flow.

## Decision

For `GET /api/third-party/{serviceId}/oauth2/authorize`, an omitted or empty
`redirect_uri` selects `<configured public URL>/sessions`. The session domain
chooses this default from its injected callback-base configuration. Request Host
and forwarded headers cannot select the return origin.

Explicit GET return URIs retain the configured-origin validation and reserved
consent-query stripping. The default is sealed in the existing JWE state; the
provider still receives the broker callback URI and PKCE challenge. Callback
validation, token storage, and success/error return parameters do not change.

The consent-selection POST still requires both `redirect_uri` and a valid
`consent_state_id`. This default does not apply to the broker authorization
server's `/oauth2/authorize` endpoint or relax registered-client redirect rules.

This implements the additive input relaxation explicitly requested in #212;
previously valid requests retain their behavior. The End-User API remains at
`1.0.0`. ADR 044's RFC 8693 recovery links retain their explicit `/sessions`
return query, and ADR 037's consent-selection transport remains unchanged.

## Consequences

- Operators can verify a configured provider by opening its bare authorize path.
- Login return state uses the existing encrypted, principal- and service-bound
  mechanism, without adding configuration, persistence, or dependencies.
- Invalid explicit return URIs still fail before provider redirection. Omitting
  the parameter cannot select an arbitrary caller-controlled return page.

## References

- [End-User OpenAPI](../api/enduser/openapi.yaml)
- [Feature 008, US2-S12 and Journey 3](../specs/008-thirdparty-oauth2-sessions/spec.md)
- [ADR 044](044-service-specific-session-recovery-url.md)
- [ADR 037](037-consent-selection-oauth-redirect-state.md)
