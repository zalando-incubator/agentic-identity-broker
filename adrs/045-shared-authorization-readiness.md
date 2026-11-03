# ADR 045: Shared Authorization Readiness for Browser and Inbound Token Exchange

## Status

Accepted — supersedes ADR 032 decisions 3–6 and its agent-level-only non-goal. The remaining credential, principal-binding, and terminal-decision requirements remain binding.

## Context

[Issue #367](https://github.com/zalando-incubator/agentic-identity-broker/issues/367) requests that inbound broker-token minting enforce the same consent prerequisites as `/oauth2/authorize`, through a shared code abstraction. It explicitly requests HTTP 403 with `WWW-Authenticate` when browser consent is needed. ADR 032 limited impersonation to checking an active grant and prohibited this header.

## Decision

- The consent domain owns `EvaluateAuthorizationReadiness`. Both browser authorization and the app-layer `UserDelegationVerifier` use it with the existing grant and session repositories.
- Readiness requires an active grant, no granted permission set absent from the agent's declarations, usable sessions for all services included in the grant, and usable sessions covering every mandatory service requirement's scopes. Unselected optional services do not block. Usability retains `UserSession.IsExpired` semantics, including refreshable sessions. No token is decrypted or refreshed during readiness evaluation.
- The existing delegation port gains `consent_required` for an active but no-longer-ready grant. Impersonation remains independent of the consent implementation. It checks readiness only after credential validation and rule authorization; denial is terminal and cannot try another rule.
- Browser authorization builds its existing sealed consent-resumption URL. Inbound impersonation returns generic `403 access_denied` and the existing `<public-url>/agents/<target-id>` `error_uri` for all readiness denials.
- That impersonation consent response additionally carries `WWW-Authenticate: Bearer error="insufficient_scope"`. Its `error_uri` attribute is included only when it meets RFC 6750's attribute character requirements. The JSON recovery URL remains available regardless. This is an explicitly requested broker recovery signal, not a change to credential authentication.
- Unknown readiness caused by repository failure remains `500 server_error`, without recovery URL or challenge. Other exchange kinds and policy/authentication failures retain their existing contract.

## Consequences

- Changes to browser consent prerequisites and inbound minting now share one domain decision.
- Minting performs additional session metadata reads and can deny previously permitted requests. Clients must complete browser consent/reconnection and retry.
- No new dependency, configuration, persistence, UI, specification, or scope-to-grant intersection policy is introduced.
- Existing specifications and API documentation are updated in this PR. The maintainer command in #367 is stakeholder confirmation; acceptance is reviewed with this PR.
