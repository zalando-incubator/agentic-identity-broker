# ADR: Human Impersonation Checks Consent Status

**Date**: 2026-11-20
**Status**: Accepted (the stakeholder requested consent parity in the [issue #367 body](https://github.com/zalando-incubator/agentic-identity-broker/issues/367) and the shared abstraction in [comment 6615878913](https://github.com/zalando-incubator/agentic-identity-broker/issues/367#issuecomment-6615878913); the error signal was revised in review to follow RFC 6749 §5.2)
**Supersedes in part**: [032-impersonation-requires-user-delegation](032-impersonation-requires-user-delegation.md) decisions 3–5 and its agent-level-only non-goal, only for human-category local impersonation

## Context

[032-impersonation-requires-user-delegation](032-impersonation-requires-user-delegation.md) made an active agent-level `UserGrant` mandatory for human impersonation. It excluded permission-set checks and a `WWW-Authenticate` header. Browser `/oauth2/authorize` also checks current grant definitions, included-service sessions, and mandatory service sessions and scopes. A human token minted through impersonation could pass the old check when the browser required renewed consent.

The [issue #367 body](https://github.com/zalando-incubator/agentic-identity-broker/issues/367) requests a token-endpoint signal when browser authorization would show consent, and proposes `403` with `WWW-Authenticate`. The stakeholder's [follow-up comment](https://github.com/zalando-incubator/agentic-identity-broker/issues/367#issuecomment-6615878913) requests one code-level abstraction for both paths and updates to existing specs. The consent page at `/agents/{agent-id}` remains the recovery destination.

RFC 8693 §2.2.2 uses the RFC 6749 §5.2 error response. That section answers `400` for every error except `invalid_client` (`401`) and defines no `access_denied` code; `access_denied` belongs to the authorization endpoint. A `Bearer` challenge (RFC 6750 §3) is issued by a protected resource for the bearer token it rejected. The token endpoint is not that resource: the privileged client authenticates with a client assertion and sends the subject token in the request body. 032-impersonation-requires-user-delegation decision 6 already excluded `WWW-Authenticate` for this reason.

## Decision

1. After the first impersonation rule authorizes a **human-category** subject, the broker checks consent with the same criteria as browser `/oauth2/authorize`. A shared consent-domain evaluator receives the extracted human principal and already-resolved target agent. The app adapter implements `ports.ConsentChecker` for impersonation and receives `ports.ConsentStatus`. The evaluator requires an active grant with current declared permission sets, usable sessions for all included services, and mandatory service sessions with required scopes. A denial is terminal; the broker does not try later rules or mint a token.
2. If the consent check denies a human request, local `POST /oauth2/token` returns HTTP `400` and OAuth `invalid_grant`: the subject's authorization grant for the target agent is missing, expired, or no longer sufficient. Its generic body includes the RFC 6749 `error_uri` at `<publicURL>/agents/<target UUID>`. The response has no `WWW-Authenticate` header. The broker never copies caller input into the recovery URI.
3. A grant, permission-set, or session repository failure returns HTTP `500 server_error` without `error_uri`. A missing or unusable consent state is not a repository failure. Neither response exposes grant details, credentials, or a service identifier.
4. The browser retains its sealed-JWE redirect to the consent UI. Eligible self-agent issuance stays consent-free under [045-agent-subject-impersonation](045-agent-subject-impersonation.md). A category-ineligible or policy-denied request keeps `403 access_denied` without `error_uri`. Third-party token-vault exchange and its service-specific session recovery stay unchanged under [046-observed-refresh-token-rejection](046-observed-refresh-token-rejection.md) and [044-service-specific-session-recovery-url](044-service-specific-session-recovery-url.md).
5. End-User OpenAPI `info.version` becomes `2.0.0` on the existing route. The published `1.2.0` API answered a missing or expired human delegation with `403 access_denied` and minted some human impersonation tokens with incomplete browser consent; both cases now receive a terminal `400 invalid_grant` with the consent `error_uri`. This is a breaking change. Integrations must open the body recovery URI, complete consent, and retry. No alternate endpoint, compatibility mode, or response alias is added.

The token-endpoint response carries the recovery URI only in the body:

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json
Cache-Control: no-store

{"error":"invalid_grant","error_description":"User authorization is insufficient. Please re-consent.","error_uri":"https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"}
```

## Consequences

- A privileged client can send the user to the existing agent consent page. It can retry only after the user repairs consent.
- Human impersonation can no longer issue a token when browser authorization requires consent. A usable agent-level grant alone is insufficient.
- Consent denials follow RFC 6749 §5.2 and RFC 8693 §2.2.2. A generic RFC 8693 client receives a standard error code and `error_uri`.
- No token-endpoint response gains `WWW-Authenticate`, consistent with 032-impersonation-requires-user-delegation decision 6 and 044-service-specific-session-recovery-url. A gateway that fronts a protected resource can still build its own RFC 6750 challenge from `error_uri`, as ExtProc does for third-party exchange.
- Human impersonation consent denials use `400 invalid_grant`, while third-party grant denials keep `403 access_denied`. Clients detect consent recovery by the presence of `error_uri`, not by status code.
- Agent-category classification and third-party agent-token detection remain governed by 045-agent-subject-impersonation. Recorded refresh-token rejection still makes an included service session unusable under 046-observed-refresh-token-rejection.

## References

- [Issue #367 request](https://github.com/zalando-incubator/agentic-identity-broker/issues/367) and [shared-abstraction request](https://github.com/zalando-incubator/agentic-identity-broker/issues/367#issuecomment-6615878913)
- [RFC 6749 §5.2](https://www.rfc-editor.org/rfc/rfc6749#section-5.2), [RFC 6750 §3](https://www.rfc-editor.org/rfc/rfc6750#section-3), [RFC 8693 §2.2.2](https://www.rfc-editor.org/rfc/rfc8693#section-2.2.2)
- [032-impersonation-requires-user-delegation](032-impersonation-requires-user-delegation.md), [044-service-specific-session-recovery-url](044-service-specific-session-recovery-url.md), [045-agent-subject-impersonation](045-agent-subject-impersonation.md), [046-observed-refresh-token-rejection](046-observed-refresh-token-rejection.md)
- `api/enduser/openapi.yaml`
- `specs/037-oauth2-user-impersonation/spec.md`
