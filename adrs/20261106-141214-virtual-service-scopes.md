# Virtual Service Scopes for Incremental Authorization

**Status**: Accepted (approved by mjungsbluth on 2026-11-13: https://github.com/zalando-incubator/agentic-identity-broker/pull/323#issuecomment-6857846872; Revision 2 approved by mjungsbluth on 2026-11-25)
**Date**: 2026-10-29
**Scope**: Broker OAuth2 authorization server (local, proxy, hybrid), consent, RFC 8693 errors, ExtProc challenge
**Feature**: [055-incremental-service-authorization](../specs/055-incremental-service-authorization/spec.md)

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
   grammar is in [the scope contract](../specs/055-incremental-service-authorization/contracts/virtual-service-scope.md). Values outside that prefix are ordinary
   scopes.
2. **Meaning**: A virtual scope requires, for this request only, that the authenticated user hold a
   usable session for the service. A selected permission set must cover that session for the
   agent, including the agent's configured required provider scopes. Usability checks the effective
   selected permission-set union after exchange's ceiling rule; all-scopes and agents without explicit
   requirements use the full union even when configured required scopes are empty. Consent combines
   baseline and selected candidates' verified contribution usability for unsaved toggles, without
   exposing raw scope lists. A virtual scope never grants
   provider scopes, never selects permission sets, and never adds a service beyond the agent's
   configuration.
3. **One owner**: A new bounded context, `internal/domain/servicescope/`, owns parsing, formatting,
   requested-set resolution, eligibility, the coverage rule, and usability evaluation. The
   authorization, consent, local-issuance (including local impersonation), and token-exchange
   services all consume it.
4. **Local issuance**:
   - Granted virtual scopes appear in the token-response `scope` and the access-token `scope`
     claim, in their requested form.
   - Refresh cannot widen to a virtual scope that was not granted (`invalid_scope`).
   - Refresh re-checks every original binding, even when `scope` is omitted or narrowed. The check
     uses current grant-derived effective provider scopes and runs before rotation, so a failure
     never consumes the refresh token.
   - Successful narrowing returns and persists the narrowed `Scope`, while inheriting all bindings
     unchanged as assurance-only obligations. Scope-less successor refresh cannot restore removed
     scopes, and retained bindings cannot authorize widening. Validation distinguishes code-granted
     bindings (within `Scope`) from refresh assurance bindings (possibly outside current `Scope`).
   - **Local impersonation** exempts broker-prefixed scopes from the `allowed_scopes` pre-check and,
     after rule authorization and human delegation and before minting, applies parsing, existence,
     eligibility, grant coverage and usability to every requested virtual scope. Malformed, unknown,
     ineligible or unsatisfied scopes return `400 invalid_scope` without consent redirection; a
     lookup failure or transiently unverifiable usability returns `500 server_error`. A failure is
     terminal and does not fall through to later rules. Agent subjects
     ([`045-agent-subject-impersonation`](045-agent-subject-impersonation.md)) receive
     `invalid_scope` for any virtual scope, as `client_credentials` does. Impersonation stores no
     Service-Scope Binding because it issues no authorization code or refresh token.
5. **Identity pinning**: Each granted virtual scope's resolved service UUID is persisted as a
   Service-Scope Binding on the authorization code and refresh-token session. Refresh fails closed
   when the scope no longer resolves to the bound UUID. Only UUID scopes survive canonical-ID
   renames, and no alias history is kept. Successful consent Save returns the sealed original
   authorize URL with the same JWE `session_token`. Authorize re-entry independently validates the
   original request, expiry, user, agent and bindings before code issuance or proxy redirect. The
   token is consumed locally; Save-time validation alone is insufficient.
6. **Proxy**:
   - Proxy authorization checks requested services exactly as local authorization does, then
     removes virtual scopes from the upstream request.
   - Before proxy refresh forwards a request whose `scope` contains a broker-prefixed token, it
     validates scope syntax, resolves the agent through the existing token-endpoint client
     resolution, and checks service existence and agent eligibility. Malformed, unknown and
     ineligible scopes return `400 invalid_scope` (`requested service scope is not permitted`);
     lookup failure returns `500 server_error`, with no upstream call. Ordinary-only requests keep
     their existing path. Valid virtual scopes are then removed and the upstream outcome is
     forwarded unchanged, without provider-session usability checks or a veto of response/rotation.
   - The broker never modifies upstream tokens.
   - Hybrid mode follows the agent's client type.
7. **Discovery**: Virtual scopes are never listed in `scopes_supported`. Clients learn them from
   the token-exchange `scope` error member, which the gateway copies into its
   `insufficient_scope` challenge, or from operator route configuration.
   - **Challenge attributes.** For a non-transient broker exchange error with `error_uri`, ExtProc
     appends to its existing 403 `insufficient_scope` challenge, in this fixed order:
     `scope="<broker scope>"` and then `resource_metadata="<route metadata URL>"`. Status, body
     and URL elicitation do not change. `scope` is included only when it is exactly one absolute
     `http` or `https` URL with a host and only bytes `0x21`–`0x7E` other than `"` and `\`.
     Otherwise it is omitted and the challenge stays valid.
   - **Route metadata field.** `resource_metadata_url` is an optional string member of the
     literal `aib.tokenexchange` metadata namespace, set on each Agentgateway route as a CEL string
     expression beside `resource_uri`. It is advisory challenge input, not a token-exchange
     input. ExtProc validates it only when it builds a challenge: an absolute `https` URL with a
     non-empty host, no userinfo and no fragment, at most 2048 bytes, and only bytes
     `0x21`–`0x7E` other than `"` and `\`. An absent value omits `resource_metadata`. An invalid
     or non-string value also omits it, and ExtProc logs `challenge metadata invalid` with the
     field name and without the value. The field never affects subject-token or resource
     validation, exchange, caching, OPA input or the request outcome. There is no ExtProc YAML
     key, environment variable, CLI flag, broker key or Helm value, and no default.
   - **Grant-context mode.** With the `048-authzen-delegation-pip` resolver enabled, a body-bearing request whose
     delegation is denied keeps `048-authzen-delegation-pip` precedence and its 403
     `{"error":"access_denied","error_description":"delegation denied: <code>","error_uri":"<consent URL>"}`
     body. ExtProc adds the `insufficient_scope` challenge header only when the concurrent
     exchange failed with a non-transient broker `access_denied` whose `error_uri` equals the
     denial's consent URL and whose `scope` is valid. The challenge reuses the default-mode
     builder with `error_description="delegation denied: <code>"`, the consent URL as
     `error_uri`, the exchange `scope` and the route's `resource_metadata`. Any other
     exchange outcome, including success, a transient or non-broker failure, another code or
     another URL, keeps the denial without a challenge. A header-only denial has no exchange
     result, so it keeps the denial without a challenge and makes no extra broker call.
     Resolution failures (non-decisions) keep their 503 response without a challenge.

## Consequences

- One focused consent interaction can repair an optional service. The user's other optional
  selections are not touched.
- The broker's issued tokens reveal which managed services a client was granted access to. The
  issuer is the same broker, and audience handling does not change.
- `authorization_codes` and `refresh_token_sessions` gain a nullable `service_scope_bindings`
  column (migration 050, decision DEC-004).
- Agents with a non-empty `allowed_scopes` can request virtual scopes without listing them. The
  broker checks eligibility separately.
- Proxy refresh gives weaker assurance than local refresh. This is documented.
- Known limitation: a service whose canonical ID is exactly `.` or `..` has a scope, service
  page and recovery link that browsers normalize to another path. No UUID fallback or canonical-ID
  validation change is added here. Issue #446 restricts them; operators can choose another ID or none.
- Scope narrowing durably removes privileges without dropping original continuing-service checks.
- Consent adds per-candidate contribution usability and a protected authorize continuation parameter;
  both are covered by the supplied API-CONSENT approval, alongside the local narrowing contract under API-LOCAL-SCOPES.
- ExtProc reads a third, optional `aib.tokenexchange` member. `036-extproc-metadata-token-exchange-input`'s exchange input boundary,
  validation order and fail-closed responses for `subject_token` and `resource_uri` do not change.
- In grant-context mode, a body-bearing request for a service the grant does not cover offers the
  same scope step-up as the default mode. A header-only request for that service does not, and
  clients must step up from a body-bearing request.

## Alternatives considered

- **Rich Authorization Requests (RFC 9396) `authorization_details`**: This is more expressive, but
  MCP clients and gateways challenge with `scope`, not `authorization_details`. It was rejected for
  interoperability.
- **Opaque scope names such as `aib:service:github`**: These cannot double as the human-readable
  recovery page. They were rejected because the spec requires a scope URL that a person can open.
- **Making requested services permanently mandatory**: This changes agent configuration based on
  client input. It was rejected because it violates FR-007 and SR-001.
- **Check only configured required scopes**: Rejected because all-scopes and no-requirements agents
  can have empty required scopes but nonempty effective provider scopes.
- **Plain post-Save authorize URL**: Rejected because it loses authenticated identity bindings
  between Save and authorization re-entry.
- **Retain full successor Scope after narrowing / drop removed bindings**: Rejected because the
  former restores dropped privileges on scope-less refresh and the latter evades continuing checks.
- **Canonical-ID alias history**: This adds retention and ownership policy. It was rejected for
  this release (DEC-001).
- **Silently strip invalid broker scopes on proxy refresh**: Rejected because it contradicts
  FR-002 and hides an invalid service request from the client.
- **UUID-only emission for `.` and `..`**: Rejected for this feature. Issue #446 owns the shared
  canonical-ID validation defect and its migration/Admin API consequences.
- **Global ExtProc configuration key for the metadata URL**: Rejected. RFC 9728 metadata is
  specific to each protected resource, and `036-extproc-metadata-token-exchange-input` makes route metadata the gateway-to-ExtProc
  input boundary.
- **Full URL-elicitation body for a grant-context denial**: Rejected. It would replace the
  `access_denied` body that `048-authzen-delegation-pip` §4 and Feature 054 define. The added header gives MCP clients
  the step-up, and the body keeps the consent URL.
- **Add the virtual scope to the AuthZEN denial context**: Deferred. It would give header-only
  denials a challenge without an exchange, but it changes the End-User AuthZEN contract and needs
  its own API approval (Principles IV and X).
- **Run an exchange after a header-only denial to obtain the scope**: Rejected. It adds a broker
  call to every denied header-only request and records the `012-extproc-in-memory-token-cache` re-consent cooldown on that
  path, which `048-authzen-delegation-pip` avoids.

## References

- [Approved Revision 1](20261107-143056-virtual-service-scopes-revision-1.md) (proxy pre-forward validation and dot-segment limitation).
- [Approved Revision 2](20261122-103452-virtual-service-scopes-revision-2.md) (ExtProc challenge metadata, grant-context denial challenge and local impersonation; approved by mjungsbluth on 2026-11-25).

- Feature 055 [spec](../specs/055-incremental-service-authorization/spec.md), [research](../specs/055-incremental-service-authorization/research.md) R1–R8, [data model](../specs/055-incremental-service-authorization/data-model.md),
  and [contracts](../specs/055-incremental-service-authorization/contracts/).
- [ADR 014](014-oauth2-server-mode.md), [ADR 016](016-authorization-session-anti-spoofing.md),
  [ADR 036 (ExtProc metadata input)](036-extproc-metadata-token-exchange-input.md),
  [ADR 20261106-141214-service-session-recovery-return](20261106-141214-service-session-recovery-return.md).
