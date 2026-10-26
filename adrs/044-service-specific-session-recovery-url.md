# ADR 044: Service-Specific Session Recovery URL

**Status**: Accepted
**Date**: 2026-10-26
**Scope**: RFC 8693 recoverable third-party session errors on the End-User API

## Context

The broker returned `<public URL>/sessions` as `error_uri` for recoverable provider-session failures. This landing page does not start the required service's authorization flow. Feature 013's October 8 landing-page decision reached merged PR #186. Feature 023 FR-001 instead described a service-specific authorization URL.

**Approval record**: On 2026-10-26, the user explicitly approved restoring the service-specific authorization URL with a fixed `/sessions` return URI. End-User API `2.0.0` records the breaking contract change under constitutional versioning rules. This decision supersedes the October 8 landing-page target from Feature 013 and reconciles Feature 023 FR-001. It does not alter their unrelated requirements.

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

The End-User API OpenAPI contract changes from `1.0.0` to `2.0.0` on existing routes. No compatibility alias or new endpoint is added. The contract version is independent of the broker release tag.

## Consequences

- Clients that open `error_uri` enter the correct service's provider login and return to session management after the callback.
- Clients that treated `error_uri` as the sessions page must adopt the End-User API `2.0.0` contract before deployment.
- A fixed same-origin return target avoids caller-controlled redirects. The service UUID comes from broker resource resolution.
- Other token exchange errors and ExtProc response status, headers, and body rules remain unchanged.

## References

- [Feature 013 User Story 5 Amendment](../specs/013-token-exchange/spec.md#user-story-5-amendment---service-specific-session-recovery-2026-10-26)
- [Feature 023 FR-001](../specs/023-extproc-mcp-elicitation/spec.md#functional-requirements)
- [End-User API 2.0.0](../api/enduser/openapi.yaml)
- [End-User API migration guide](../docs/api/enduser-api.md#end-user-api-200-reconnect-a-third-party-session)
- [ADR 032](032-impersonation-requires-user-delegation.md)
