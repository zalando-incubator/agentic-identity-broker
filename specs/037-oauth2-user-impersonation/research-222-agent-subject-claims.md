# Research: Token Claims for an Agent Subject (Issue 222)

**Issue**: [zalando-incubator/agentic-identity-broker#222](https://github.com/zalando-incubator/agentic-identity-broker/issues/222)
**Date**: 2026-10-23
**Status**: Discussion input, no decision taken
**Extends**: [spec 037](spec.md), [ADR 032](../../adrs/032-impersonation-requires-user-delegation.md)

## Purpose

Issue 222 asks the impersonation flow to mint a token whose `sub` is an agent instead of a user.
This document compares the claim sets such a token could carry against three requirements:

1. A consumer can tell whether the token represents a human or a machine.
2. The token is equivalent to one issued through the `client_credentials` grant.
3. The claims stay as close to the OAuth 2.0 RFC family as possible.

Request-side design (how the privileged client names the agent, what `subject_token` carries) is
touched on only where it changes the minted claims.

## Baseline: What the Broker Mints Today

All three local token shapes come from the same minting path
(`internal/domain/oauth2server/strategies.go`): same issuer, lifetime, signing key, `jti`, and the
same `token_claims_expression` policy. Only `sub`, `agent_id`, `act`, and the CEL inputs differ.
None of the shapes carry `typ: at+jwt`, `client_id`, or a mandatory `aud` (RFC 9068 §2.1, §2.2).
`aud` is emitted only if the operator's `token_claims_expression` returns it.

### B1. `client_credentials` (machine, today)

```json
{
  "iss": "https://broker.example.com",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "scope": "read write",
  "iat": 1761200000,
  "exp": 1761203600,
  "jti": "a9b5a7a1-5d0a-4e35-9b7a-9f7c5f3f5e2d"
}
```

`sub` is the agent UUID from storage. `agent_id` echoes the `client_id` the caller presented, which
the resolver only accepts as that same UUID. CEL sees `principal.id == agent.id`,
`principal.email == ""`, and `request.grant_type == "client_credentials"`. No `act`.

### B2. `authorization_code` (human, today)

```json
{
  "iss": "https://broker.example.com",
  "sub": "jdoe",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "scope": "read",
  "iat": 1761200000,
  "exp": 1761203600,
  "jti": "..."
}
```

`sub` is the `X-Remote-User` principal. CEL sees `principal.email` from the session profile and
`request.grant_type == "authorization_code"`. There is no `auth_time`, `amr`, or `acr`.

### B3. Impersonation with a user subject (human, today, spec 037)

```json
{
  "iss": "https://broker.example.com",
  "sub": "jdoe",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "act": {
    "iss": "https://idp.internal.example.com",
    "sub": "gateway-prod"
  },
  "scope": "read",
  "iat": 1761200000,
  "exp": 1761203600,
  "jti": "..."
}
```

`act` is the RFC 8693 §4.1 delegation claim. CEL sees
`request.grant_type == "urn:ietf:params:oauth:grant-type:token-exchange"`. A `UserGrant` for
`(sub, agent_id)` is mandatory (ADR 032).

## How Consumers Read These Tokens Today

| Consumer | Claim it reads | Effect for an agent subject |
|---|---|---|
| Third-party token exchange (`claim_extraction.principal_expression`, default `claims.sub`) | `sub` as the user principal | Looks up a `UserGrant` and `UserSession` for principal = agent UUID. Neither exists, so the exchange ends in `access_denied` with a consent `error_uri`. Issue 222 accepts that exchange is out of scope, but the error would be misleading. |
| ExtProc approval client | `agent_id` | Unchanged. |
| OPA policies in ExtProc, downstream resource servers | Whatever the operator wrote | Today a policy can detect a machine token only by `sub == agent_id`. |
| Audit events | subject identity, target agent | Need a subject-kind field to tell the two cases apart. |

## Claim Variants for an Agent Subject

The examples assume the privileged client `gateway-prod` presents an actor token for the external
machine identity `svc-order-pipeline`, issued by `https://idp.internal.example.com`, and targets
agent `550e8400-...`. The `act` claim is kept in every variant, as the issue requires.

### V1. Mirror `client_credentials`, add `act`

```json
{
  "iss": "https://broker.example.com",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "act": {
    "iss": "https://idp.internal.example.com",
    "sub": "svc-order-pipeline"
  },
  "scope": "read write",
  "iat": 1761200000,
  "exp": 1761203600,
  "jti": "..."
}
```

- **Human vs machine**: structural, `sub == agent_id`. This is exactly how a B1 token is already
  recognisable, so no consumer learns a new rule.
- **Equivalence to B1**: identical except for `act`. CEL sees the same `principal.id == agent.id`
  and an empty `principal.email`.
- **RFC posture**: pure RFC 8693 delegation on top of the existing claim set. No new claim names.

### V2. V1 plus RFC 9068 `client_id`

```json
{
  "iss": "https://broker.example.com",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "client_id": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "act": { "iss": "https://idp.internal.example.com", "sub": "svc-order-pipeline" },
  "scope": "read write",
  "iat": 1761200000,
  "exp": 1761203600,
  "jti": "..."
}
```

- RFC 9068 §2.2 makes `client_id` mandatory and defines it as the client that requested the token.
  For impersonation that client is `gateway-prod`, not the agent. Setting `client_id` to the agent
  keeps B1 equivalence but bends the RFC 9068 definition. Setting it to `gateway-prod` follows the
  RFC but breaks equivalence, and duplicates what `act` already says.
- Adding `client_id` only to the new token shape makes it the odd one out. Adding it to B1 and B2
  as well is a separate, additive change that moves all local tokens closer to RFC 9068.
- `agent_id` and `client_id` would carry the same value in B1 and V2, which invites consumers to
  treat them as synonyms. They are not: `agent_id` is the broker's target identity and survives
  into impersonated tokens, while `client_id` is the requesting client.

### V3. V1 plus an explicit subject-kind claim

```json
{
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "sub_kind": "agent",
  "act": { "iss": "https://idp.internal.example.com", "sub": "svc-order-pipeline" },
  "...": "..."
}
```

Two ways to produce it:

- **V3a, broker-minted base claim**: the broker always writes `sub_kind` (`user` or `agent`) on every
  local token, including B1 and B2, and adds it to the reserved `baseClaims` set so CEL cannot
  override it. Deterministic and fail-closed.
- **V3b, policy-minted via CEL**: the broker exposes a new `principal.kind` variable (and keeps
  `request.grant_type` truthful) so the operator writes
  `{"sub_kind": principal.kind}` in `token_claims_expression`. No fixed claim name, but the
  discriminator exists only if the operator configures it.

`sub_kind` is a private claim under RFC 7519 §4.3. There is no IANA-registered claim for "the
subject is a workload", so any explicit marker is non-standard. The name should be chosen once and
documented in the glossary; `subject_type` and `sub_type` are equally plausible.

### V4. Human-authentication claims on human tokens only

RFC 9068 §2.2.1 reserves `auth_time`, `acr`, and `amr` for tokens that result from end-user
authentication. Machine tokens carry none of them. Under this variant, B2 and B3 would gain at least
`auth_time`, and the absence of those claims would mark a token as machine-issued.

- Standards-aligned and needs no new claim on the machine side.
- The broker does not know how or when the user authenticated. It trusts `X-Remote-User` from a
  reverse proxy, so `auth_time` would be the authorization-code or impersonation request time, and
  `amr` would be a configured constant. That is weaker than the claim name suggests.
- Detection by absence is fragile: a consumer that forgets the check treats every token as machine.

### V5. Self-describing `sub` (URI or URN)

```json
{
  "sub": "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "act": { "...": "..." }
}
```

- Readable without any extra claim, and the format itself separates agents from opaque user
  principals.
- Breaks equivalence with B1, where `sub` is the bare UUID, unless B1 changes too. Every consumer
  that compares `sub` to `agent_id` or to an admin-visible agent ID would break.
- Mixing identifier formats inside one `sub` namespace is exactly the kind of parsing the
  resolver avoids today, where `client_id` is either a CIMD URL or a UUID.

### V6. RFC 9493 `sub_id`

RFC 9493 (Subject Identifiers for Security Event Tokens) registers a structured `sub_id` claim with
formats such as `email`, `opaque`, `uri`, and `iss_sub`. An agent token could carry
`{"format": "uri", "uri": "https://broker.example.com/agents/<uuid>"}` while a user token carries
`{"format": "opaque", "id": "jdoe"}`. It is a registered claim but was designed for SET events, has
little library support in access-token validators, and would be the only structured identity claim
in the token. Listed for completeness only.

### Rejected: `act` as the discriminator

`act` is present on B3 and on every variant above, so it does not separate human from machine
subjects. `act.sub == sub` is also not a reliable marker: spec 037 permits it for user subjects,
and in the issue's scenario the actor is the external machine identity, not the agent itself.

## Comparison

| Variant | Human vs machine | Equivalent to B1 | RFC posture | Change to existing tokens | Fail-closed |
|---|---|---|---|---|---|
| V1 mirror + `act` | `sub == agent_id` | Yes, plus `act` | RFC 8693 only | None | Yes (structural) |
| V2 + `client_id` | `sub == agent_id` | Only if B1 also gets `client_id` | Closer to RFC 9068, semantics bent if value is the agent | B1, B2 gain a claim | Yes |
| V3a broker `sub_kind` | Explicit | Yes if B1 also gets it | Private claim | B1, B2 gain a claim | Yes |
| V3b CEL `principal.kind` | Explicit if configured | Yes | Private claim | None by default | No |
| V4 `auth_time`/`amr` on human tokens | By absence | Yes | RFC 9068 §2.2.1 | B2, B3 gain claims | No |
| V5 URI `sub` | By format | No | None against, none for | B1 would have to change | Yes |
| V6 `sub_id` | By `format` | Yes if B1 also gets it | RFC 9493 (SET) | B1, B2 gain a claim | Yes |

## Trade-offs Across All Variants

- **Equality check versus explicit marker.** `sub == agent_id` costs nothing and already holds for
  B1. It silently misfires only if a human principal string equals an agent UUID. Principals come
  from the operator's `X-Remote-User` namespace, so the broker could reject minting a user token
  whose principal parses as a registered agent UUID, which closes the gap without a new claim.
- **Mandatory consent.** ADR 032 and spec 037 FR-017 state that the delegation check has no
  configuration opt-out. A `subject.kind: agent` rule skips it by design, so the change needs an
  Accepted superseding ADR before merge (constitution Principle II). The ADR should state that the
  check is skipped only when the subject is the target agent itself, never for arbitrary machine
  identities.
- **Meaning of `act`.** For a user subject, `act` says who is acting for the user. For an agent
  subject, `act` says which external workload is running as the agent. Consumers that render
  `act.sub` as a person need to know this. The RFC 8693 framing stays delegation either way.
- **`request.grant_type` in CEL.** Impersonation reports the token-exchange grant type, B1 reports
  `client_credentials`. Policies that branch on grant type will diverge between the two even if
  the minted claims match. Variant V3b addresses this by giving policies a `principal.kind` to
  branch on instead.
- **Scope semantics are not identical to B1.** fosite's client-credentials handler validates each
  requested scope against the agent's `allowed_scopes` and rejects every scope when that list is
  empty. Impersonation treats an empty list as unrestricted (spec 037). A consumer cannot rely on
  the `scope` claim meaning the same policy outcome unless this is aligned.
- **Third-party exchange of an agent token.** With `sub` = agent UUID, the default principal
  expression resolves no `UserGrant` and the exchange returns `access_denied` with a consent
  `error_uri` that no human can act on. Either reject agent-subject tokens at the exchange with a
  clearer error, or document the behaviour, until the future work in the issue lands.
- **RFC 9068 gaps are shared.** No local token carries `typ: at+jwt`, `client_id`, or a required
  `aud`. Picking V1 does not worsen that; picking V2 fixes one gap for one token shape only, which
  is arguably worse than leaving all shapes consistent.
- **Request shape for an agent subject.** RFC 8693 §2.1 requires `subject_token`. For an agent
  subject the broker already knows the target from the audience, so the request either carries the
  actor token twice, an unverified subject JWT naming the agent (ADR 031 profile), or a new rule
  kind that derives `sub` from the audience and ignores `subject_token`. The first two keep the
  request RFC-shaped; the third needs an explicit profile note in the API contract.
- **Audit.** Whatever the token carries, the existing credential-free audit event should record
  the subject kind so that user and agent impersonation can be told apart in logs.

## Suggested Direction

V1 meets all three requirements with no new claim and no change to existing tokens: the agent token
is a `client_credentials` token with `act` attached, and the human-or-machine question is answered
the same way it is answered for `client_credentials` today. Add V3b's `principal.kind` CEL variable
so operators who want an explicit marker can emit one, and reject minting a user token whose
principal equals a registered agent UUID so the equality check cannot be spoofed. Revisit V2 and
`typ: at+jwt` as a separate, all-token RFC 9068 alignment rather than inside issue 222.
