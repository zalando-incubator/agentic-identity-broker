# ADR 032: User Delegation Required for OAuth2 Impersonation

## Status

Accepted

> **Superseded in part by [ADR 045](./045-agent-subject-impersonation.md)**: decisions 1 and 2 do not apply to eligible, signed agent-category requests. Human-category requests are unchanged.

## Context

Feature 037 lets a privileged client request a local broker token whose `sub` is supplied by a signed or explicitly permitted unverified subject credential. The feature originally authenticated the client, validated every credential, and applied a rule-level CEL predicate, but minted the token without checking that the subject had delegated the resolved target agent.

The normal authorization-code flow and third-party RFC 8693 exchange both enforce an active `UserGrant` before granting an agent access. Omitting the same check in impersonation lets a privileged client mint a target-agent token for an arbitrary subject, including an unverified subject, even when that user never consented to the agent.

`internal/domain/impersonation` is an independent bounded context. It must not import the concrete consent domain service or classify that service's sentinel errors directly.

## Decision

1. An impersonation request MUST have an active user delegation for `(extracted subject principal, resolved target agent)` after its first rule matches and before token minting. The check is mandatory for signed and unverified subjects and has no configuration opt-out.
2. The extracted subject identity is used byte-for-byte as `id.Principal`; it is both the minted `sub` and the delegation principal. No fallback or inferred principal is permitted.
3. `internal/ports/oauth2.go` defines the narrow `UserDelegationVerifier` port and a status enum (`active`, `missing`, `expired`). A verifier error means the state is unknown and fails closed.
4. An app-layer adapter wraps `consent.Service.VerifyAgentAccess` and translates consent sentinel errors into the port status. This keeps the impersonation domain independent from the consent domain.
5. Missing or expired delegation is terminal: return `access_denied` with the existing RFC 6749 §5.2 `error_uri` response member pointing to `<end-user-public-url>/agents/<target-agent-id>`. The response does not distinguish missing from expired; structured audit categories do. A verifier error returns `server_error` without `error_uri`.
6. Do not use `WWW-Authenticate`: it is a resource-server challenge header. The token endpoint already serializes `TokenExchangeError.ErrorURI`, and ExtProc consumes it to initiate MCP URL elicitation.

## Consequences

### Positive

- User consent becomes a mandatory security boundary for every impersonated token.
- Existing token-endpoint and ExtProc clients receive an actionable, established consent signal without a new response format.
- The direct consent-management URL works for first-time delegation and requires no authorization-resumption session token or new frontend work.
- The port isolates bounded contexts and makes impersonation tests deterministic.

### Negative

- Privileged clients must surface the returned consent URL and retry after the user grants access.
- Unverified-subject integrations can no longer mint for identities that do not map to a broker principal with an active delegation.
- The additional storage lookup is on the successful minting path.

### Non-goals

- No permission-set completeness validation or scope-to-grant intersection is added to impersonation; this is an agent-level delegation check.
- No new consent UI, consent API endpoint, or configuration flag is introduced.

## References

- `specs/037-oauth2-user-impersonation/spec.md` — FR-017 through FR-019, CR-010, SC-006
- `internal/domain/consent/service.go` — `VerifyAgentAccess`
- `internal/domain/tokenexchange/errors.go` — `TokenExchangeError.WithErrorURI`
- `adrs/014-oauth2-server-mode.md`
- `adrs/016-authorization-session-anti-spoofing.md`
