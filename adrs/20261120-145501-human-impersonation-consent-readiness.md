# ADR: Human Impersonation Uses Browser Consent Readiness

**Date**: 2026-11-20
**Status**: Accepted (the stakeholder requested the 403 challenge in the [issue #367 body](https://github.com/zalando-incubator/agentic-identity-broker/issues/367) and the shared abstraction in [comment 6615878913](https://github.com/zalando-incubator/agentic-identity-broker/issues/367#issuecomment-6615878913))
**Supersedes in part**: [032-impersonation-requires-user-delegation](032-impersonation-requires-user-delegation.md) decisions 3–6 and its agent-level-only non-goal, plus [044-service-specific-session-recovery-url](044-service-specific-session-recovery-url.md)'s general no-token-endpoint-challenge statement, only for human-category local impersonation

## Context

[032-impersonation-requires-user-delegation](032-impersonation-requires-user-delegation.md) made an active agent-level `UserGrant` mandatory for human impersonation. It excluded permission-set checks and a `WWW-Authenticate` header. Browser `/oauth2/authorize` also checks current grant definitions, included-service sessions, and mandatory service sessions and scopes. A human token minted through impersonation could pass the old check when the browser required renewed consent.

The [issue #367 body](https://github.com/zalando-incubator/agentic-identity-broker/issues/367) requests `403` with `WWW-Authenticate` when browser authorization would show consent. The stakeholder's [follow-up comment](https://github.com/zalando-incubator/agentic-identity-broker/issues/367#issuecomment-6615878913) requests one code-level abstraction for both paths and updates to existing specs. The consent page at `/agents/{agent-id}` remains the recovery destination.

## Decision

1. After the first impersonation rule authorizes a **human-category** subject, the broker applies the same consent-readiness evaluation as browser `/oauth2/authorize`. A shared consent-domain evaluator receives the extracted human principal and already-resolved target agent. The app adapter implements `ports.AuthorizationReadinessVerifier` for impersonation. The evaluator requires an active grant with current declared permission sets, usable sessions for all included services, and mandatory service sessions with required scopes. A denial is terminal; the broker does not try later rules or mint a token.
2. On a human consent-readiness denial, local `POST /oauth2/token` returns HTTP `403` and OAuth `access_denied`. Its generic body includes the existing RFC 6749 `error_uri` at `<publicURL>/agents/<target UUID>`. It also returns `WWW-Authenticate: Bearer error="insufficient_scope", error_uri="<same URI>"` when the URI is safe as an RFC 6750 challenge value. If the URI contains a disallowed challenge-value character, the header omits `error_uri`; the body keeps it. The broker never copies caller input into the recovery URI.
3. A grant, permission-set, or session repository failure returns HTTP `500 server_error` without `error_uri` or `WWW-Authenticate`. A missing or unusable consent state is not a repository failure. Neither response exposes grant details, credentials, or a service identifier.
4. The browser retains its sealed-JWE redirect to the consent UI. It does not return the token-endpoint challenge. Eligible self-agent issuance stays consent-free under [045-agent-subject-impersonation](045-agent-subject-impersonation.md). Third-party token-vault exchange and its service-specific session recovery stay unchanged under [046-observed-refresh-token-rejection](046-observed-refresh-token-rejection.md) and [044-service-specific-session-recovery-url](044-service-specific-session-recovery-url.md).
5. End-User OpenAPI `info.version` becomes `2.0.0` on the existing route. The published `1.2.0` API minted some human impersonation tokens with incomplete browser consent; those requests now receive a terminal `403`. This is a breaking change. Integrations must open the body recovery URI, complete consent, and retry. No alternate endpoint, compatibility mode, or response alias is added.

For a safe public URL, the token-endpoint response includes both recovery signals:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
WWW-Authenticate: Bearer error="insufficient_scope", error_uri="https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"

{"error":"access_denied","error_description":"User authorization is insufficient. Please re-consent.","error_uri":"https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"}
```

## Consequences

- A privileged client can send the user to the existing agent consent page. It can retry only after the user repairs consent.
- Human impersonation can no longer issue a token when browser authorization requires consent. A usable agent-level grant alone is insufficient.
- The token-endpoint challenge is an approved, narrow exception to the no-header decisions in 032-impersonation-requires-user-delegation and 044-service-specific-session-recovery-url. Other token-endpoint errors do not gain a challenge.
- Agent-category classification and third-party agent-token detection remain governed by 045-agent-subject-impersonation. Recorded refresh-token rejection still makes an included service session unusable under 046-observed-refresh-token-rejection.

## References

- [Issue #367 wire request](https://github.com/zalando-incubator/agentic-identity-broker/issues/367) and [shared-abstraction request](https://github.com/zalando-incubator/agentic-identity-broker/issues/367#issuecomment-6615878913)
- [032-impersonation-requires-user-delegation](032-impersonation-requires-user-delegation.md), [044-service-specific-session-recovery-url](044-service-specific-session-recovery-url.md), [045-agent-subject-impersonation](045-agent-subject-impersonation.md), [046-observed-refresh-token-rejection](046-observed-refresh-token-rejection.md)
- `api/enduser/openapi.yaml`
- `specs/037-oauth2-user-impersonation/spec.md`
