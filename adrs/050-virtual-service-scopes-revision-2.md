# ADR 050 Revision 2 — ExtProc Challenge Metadata, Grant-Context Denial Challenge and Local Impersonation

**Status**: Proposed
**Date**: 2026-11-22
**Revises**: [`050-virtual-service-scopes`](050-virtual-service-scopes.md), introduced and accepted in this pull request.
Under Principle II, the same pull request may revise it before merge. After approval, apply the
changes below to `adrs/050-virtual-service-scopes.md` and record the approval there, as for
[`050-virtual-service-scopes-revision-1`](050-virtual-service-scopes-revision-1.md).

## Context

`050-virtual-service-scopes` Decision 7 says that clients learn virtual scopes from the token-exchange `scope` error
member, "which the gateway copies into its `insufficient_scope` challenge, or from operator route
configuration". Feature 055 implemented this in ExtProc with two decisions that no accepted ADR
records:

- **Route metadata field.** ExtProc reads a third member of the `aib.tokenexchange` metadata
  namespace, `resource_metadata_url`, and emits it as the RFC 9728 `resource_metadata` challenge
  attribute. [`036-extproc-metadata-token-exchange-input`](036-extproc-metadata-token-exchange-input.md) lists only `subject_token`
  and `resource_uri` in that namespace. [`044-service-specific-session-recovery-url`](044-service-specific-session-recovery-url.md)
  keeps "ExtProc response status, headers, and body rules" unchanged, and
  [`049-service-session-recovery-return`](049-service-session-recovery-return.md) leaves that clause binding.
- **Grant-context mode.** With the [`048-authzen-delegation-pip`](048-authzen-delegation-pip.md) grant-context resolver
  enabled, a delegation denial takes precedence over the exchange error (`048-authzen-delegation-pip` §4). ExtProc
  returned the plain 403 `access_denied` JSON with the consent URL and no `WWW-Authenticate`
  header. A service that the grant does not cover therefore produced a challenge in the default
  mode but none in grant-context mode, although `050-virtual-service-scopes`'s Context and Decision 7 name "the grant
  does not cover it" as a case the challenge must repair.

The AuthZEN denial carries only an allowlisted grant detail (`reason_admin.code`) and the agent
consent URL (`reason_user.consent_url`). It names no service, so ExtProc cannot derive the virtual
scope from the denial alone. On the body-bearing path, ExtProc runs the RFC 8693 exchange
concurrently with the evaluation and waits for both (`048-authzen-delegation-pip` §4). For the same grant denial, the
broker's exchange error carries `access_denied`, the same consent URL as `error_uri`, and the
resolved service's virtual `scope`. On the header-only path, ExtProc resolves before any exchange
and calls exchange only after policy allows.

- **Local impersonation.** `050-virtual-service-scopes` Decision 3 names authorization, consent,
  local issuance and token exchange as consumers of `internal/domain/servicescope/`, but local-mode
  impersonation was not one of them. It checked each requested scope only against the target
  Agent's `allowed_scopes` and copied the scopes into the minted token. With an unrestricted Agent
  it minted malformed, unknown and grant-uncovered virtual scopes; with a restricted Agent it
  rejected valid ones, contrary to this ADR's Consequences, which exempt virtual scopes from the
  allow list.

## Changes to `050-virtual-service-scopes`

### Decisions 3 and 4 (Local impersonation)

In Decision 3, replace "The authorization, consent, local-issuance, and token-exchange services all
consume it." with "The authorization, consent, local-issuance (including local impersonation), and
token-exchange services all consume it."

Add to Decision 4:

> - **Local impersonation** exempts broker-prefixed scopes from the `allowed_scopes` pre-check and,
>   after rule authorization and human delegation and before minting, applies parsing, existence,
>   eligibility, grant coverage and usability to every requested virtual scope. Malformed, unknown,
>   ineligible or unsatisfied scopes return `400 invalid_scope` without consent redirection; a
>   lookup failure or transiently unverifiable usability returns `500 server_error`. A failure is
>   terminal and does not fall through to later rules. Agent subjects
>   ([`045-agent-subject-impersonation`](045-agent-subject-impersonation.md)) receive
>   `invalid_scope` for any virtual scope, as `client_credentials` does. Impersonation stores no
>   Service-Scope Binding because it issues no authorization code or refresh token.


### Decision 7 (Discovery)

Replace:

> 7. **Discovery**: Virtual scopes are never listed in `scopes_supported`. Clients learn them from
>    the token-exchange `scope` error member, which the gateway copies into its
>    `insufficient_scope` challenge, or from operator route configuration.

with:

> 7. **Discovery**: Virtual scopes are never listed in `scopes_supported`. Clients learn them from
>    the token-exchange `scope` error member, which the gateway copies into its
>    `insufficient_scope` challenge, or from operator route configuration.
>    - **Challenge attributes.** For a non-transient broker exchange error with `error_uri`, ExtProc
>      appends to its existing 403 `insufficient_scope` challenge, in this fixed order:
>      `scope="<broker scope>"` and then `resource_metadata="<route metadata URL>"`. Status, body
>      and URL elicitation do not change. `scope` is included only when it is exactly one absolute
>      `http` or `https` URL with a host and only bytes `0x21`–`0x7E` other than `"` and `\`.
>      Otherwise it is omitted and the challenge stays valid.
>    - **Route metadata field.** `resource_metadata_url` is an optional string member of the
>      literal `aib.tokenexchange` metadata namespace, set on each Agentgateway route as a CEL string
>      expression beside `resource_uri`. It is advisory challenge input, not a token-exchange
>      input. ExtProc validates it only when it builds a challenge: an absolute `https` URL with a
>      non-empty host, no userinfo and no fragment, at most 2048 bytes, and only bytes
>      `0x21`–`0x7E` other than `"` and `\`. An absent value omits `resource_metadata`. An invalid
>      or non-string value also omits it, and ExtProc logs `challenge metadata invalid` with the
>      field name and without the value. The field never affects subject-token or resource
>      validation, exchange, caching, OPA input or the request outcome. There is no ExtProc YAML
>      key, environment variable, CLI flag, broker key or Helm value, and no default.
>    - **Grant-context mode.** With the `048-authzen-delegation-pip` resolver enabled, a body-bearing request whose
>      delegation is denied keeps `048-authzen-delegation-pip` precedence and its 403
>      `{"error":"access_denied","error_description":"delegation denied: <code>","error_uri":"<consent URL>"}`
>      body. ExtProc adds the `insufficient_scope` challenge header only when the concurrent
>      exchange failed with a non-transient broker `access_denied` whose `error_uri` equals the
>      denial's consent URL and whose `scope` is valid. The challenge reuses the default-mode
>      builder with `error_description="delegation denied: <code>"`, the consent URL as
>      `error_uri`, the exchange `scope` and the route's `resource_metadata`. Any other
>      exchange outcome, including success, a transient or non-broker failure, another code or
>      another URL, keeps the denial without a challenge. A header-only denial has no exchange
>      result, so it keeps the denial without a challenge and makes no extra broker call.
>      Resolution failures (non-decisions) keep their 503 response without a challenge.

### Consequences, new bullets

> - ExtProc reads a third, optional `aib.tokenexchange` member. `036-extproc-metadata-token-exchange-input`'s exchange input boundary,
>   validation order and fail-closed responses for `subject_token` and `resource_uri` do not change.
> - In grant-context mode, a body-bearing request for a service the grant does not cover offers the
>   same scope step-up as the default mode. A header-only request for that service does not, and
>   clients must step up from a body-bearing request.

### Alternatives considered, new bullets

> - **Global ExtProc configuration key for the metadata URL**: Rejected. RFC 9728 metadata is
>   specific to each protected resource, and `036-extproc-metadata-token-exchange-input` makes route metadata the gateway-to-ExtProc
>   input boundary.
> - **Full URL-elicitation body for a grant-context denial**: Rejected. It would replace the
>   `access_denied` body that `048-authzen-delegation-pip` §4 and Feature 054 define. The added header gives MCP clients
>   the step-up, and the body keeps the consent URL.
> - **Add the virtual scope to the AuthZEN denial context**: Deferred. It would give header-only
>   denials a challenge without an exchange, but it changes the End-User AuthZEN contract and needs
>   its own API approval (Principles IV and X).
> - **Run an exchange after a header-only denial to obtain the scope**: Rejected. It adds a broker
>   call to every denied header-only request and records the `012-extproc-in-memory-token-cache` re-consent cooldown on that
>   path, which `048-authzen-delegation-pip` avoids.

## Relationship to other ADRs

- [`032-impersonation-requires-user-delegation`](032-impersonation-requires-user-delegation.md): its non-goal "no scope-to-grant intersection is added to impersonation" is narrowed for Virtual Service Scopes only. Provider and other scopes keep the merged behavior.
- [`036-extproc-metadata-token-exchange-input`](036-extproc-metadata-token-exchange-input.md): extended by one optional advisory member
  of the same namespace. Its two required inputs and their handling do not change.
- [`044-service-specific-session-recovery-url`](044-service-specific-session-recovery-url.md): its clause that ExtProc response
  headers remain unchanged is superseded only for the `scope` and `resource_metadata` challenge
  attributes and for the grant-context denial challenge above. `049-service-session-recovery-return`'s statement that every other
  `044-service-specific-session-recovery-url` decision remains binding is read with this exception. Status, bodies and URL
  elicitation of existing responses do not change, and ExtProc still forwards the broker
  `error_uri` unchanged.
- [`048-authzen-delegation-pip`](048-authzen-delegation-pip.md) §4: precedence, the denial status and the denial body do
  not change. Only the body-path denial may additionally carry the challenge header above.

## Consequences of this revision

- Operators configure the protected-resource metadata URL per route, and clients can discover it
  from every service challenge.
- Grant-context deployments no longer lose the scope step-up for body-bearing requests whose grant
  does not cover the service.
- Feature 054 clients that read the denial body see no change.

## References

- Feature 055 [spec](../specs/055-incremental-service-authorization/spec.md) FR-005 and Story 4
  Scenario 1, and the
  [ExtProc challenge contract](../specs/055-incremental-service-authorization/contracts/extproc-challenge.md)
- Feature 054 [ExtProc grant-context contract](../specs/054-authzen-delegation-pip/contracts/extproc-grant-context.md)
- [RFC 6750 §3](https://www.rfc-editor.org/rfc/rfc6750#section-3) and
  [RFC 9728 §5.1](https://www.rfc-editor.org/rfc/rfc9728#section-5.1)
