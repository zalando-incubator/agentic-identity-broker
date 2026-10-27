# ADR 044: Service-Specific Session Recovery URL

**Status**: Accepted
**Date**: 2026-10-26
**Scope**: RFC 8693 recoverable third-party session errors on the End-User API

## Context

The broker returned `<public URL>/sessions` as `error_uri` for recoverable provider-session failures. This landing page does not start the required service's authorization flow. Feature 013's October 8 landing-page decision reached merged PR #186. Feature 023 FR-001 instead described a service-specific authorization URL.

**Approval record**: On 2026-10-26, the user approved the service-specific authorization URL with a fixed `/sessions` return URI. In the 2026-10-27 review of PR #294, the user clarified that the broker chooses the `error_uri` target. This change does not require an End-User API version increase. This decision supersedes the October 8 landing-page target from Feature 013 and reconciles Feature 023 FR-001. It does not alter their unrelated requirements.

## Decision

For a recoverable RFC 8693 provider-session failure, the broker returns `400 invalid_grant` with:

```text
<public URL>/api/third-party/{serviceId}/oauth2/authorize?redirect_uri=<url.QueryEscape(<public URL>/sessions)>
```

For `PublicURL=https://broker.example.com` and resolved service UUID `550e8400-e29b-41d4-a716-446655440000`, the exact `error_uri` is:

```text
https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions
```

The broker uses its configured `PublicURL` for the authorization origin and fixed `/sessions` return URI. It uses the service UUID resolved from the requested protected resource, not a caller-supplied identifier. It never uses a request Host, query value, or raw provider URL to build the link. The authenticated browser GET starts third-party authorization with PKCE. The handler validates that `redirect_uri` has the configured public origin. After the provider callback, the broker redirects the browser to `/sessions`. This is not the broker authorization server's `/oauth2/authorize` endpoint.

The link applies to missing sessions, expired sessions without usable refresh, insufficient stored scopes, provider refresh `invalid_grant`, and missing or mismatched established client identity. Provider refresh rejection leaves the stored session unchanged. Grant denials still point to `<public URL>/agents/{agent-id}`. Non-recoverable exchange errors still have no `error_uri`. The broker's `/oauth2/token` response still has no `WWW-Authenticate` header. ExtProc forwards the broker URI unchanged with its existing HTTP 403 `insufficient_scope` challenge and URL-elicitation body. Approval responses do not change.

The End-User API stays at `1.0.0`. The broker selects a service-specific target for the existing optional `error_uri` action link; the response shape, route, and status do not change. No compatibility alias or new endpoint is added. The OpenAPI version remains separate from the broker release tag.

## Consequences

- Clients that open `error_uri` enter the correct service's provider login and return to session management after the callback.
- Clients that treated `error_uri` as a fixed sessions-page path must follow the broker-provided action URI instead.
- A fixed same-origin return target avoids caller-controlled redirects. The service UUID comes from broker resource resolution.
- Other token exchange errors and ExtProc response status, headers, and body rules remain unchanged.

## References

- [Feature 013 User Story 5 Amendment](../specs/013-token-exchange/spec.md#user-story-5-amendment---service-specific-session-recovery-2026-10-26)
- [Feature 023 FR-001](../specs/023-extproc-mcp-elicitation/spec.md#functional-requirements)
- [End-User API 1.0.0](../api/enduser/openapi.yaml)
- [Token exchange recovery guide](../docs/reference/token-exchange.md#400-invalid_grant)
- [ADR 032](032-impersonation-requires-user-delegation.md)
