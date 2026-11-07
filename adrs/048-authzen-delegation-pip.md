# ADR 048: AuthZEN Delegation Policy Information Point

**Status**: Accepted (approved by mjungsbluth on 2026-11-06: https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728)

Approval scope: ADR-2 permit-bound result reuse, API-7 terminal conflict and forwarding expiry, and D-8 provider/provenance limitations. Earlier approvals remain in Revision history. ADR-3 canonical policy-input guidance is incorporated with the maintainer-supplied follow-up issue #441.
**Date**: 2026-10-29 (accepted 2026-10-30)
**Scope**: End-user API AuthZEN discovery and Access Evaluation; ExtProc delegation context;
`/oauth2/token` `granted_permission_sets` deprecation
**Supersedes in part**: ADR 028 (enabled-mode OPA input schema and request ordering only)
**Extends**: ADR 033 (deferred-finalization seam), ADR 009 (caller CEL reuse), ADR 012 (separate cache; permit-bound token reuse in §10)
**Feature**: `specs/054-authzen-delegation-pip`

The maintainer approved all eleven approval items (API-1–API-6, ADR-1, D-1–D-4) with a 👍 on
the pipeline approval comment for commit `a5042d5`. Under Principle II this ADR may still be
revised in the same PR before it merges; the final ADR and the code must agree.

## Context

A gateway policy needs to know whether a user's delegation to an agent covers one protected
resource. Today ExtProc learns this only from the `granted_permission_sets` extension of the RFC
8693 exchange response. That snapshot lives as long as the exchanged token (up to an hour in the
ADR 012 cache), so a revocation keeps informing policy until the token expires. The header-only
path evaluates policy before exchange and therefore has no delegation data at all
(`granted_permission_sets_available: false`). Policies also have to key on deployment UUIDs.

Issue #289 asks the broker to act as a delegation Policy Information Point through the AuthZEN
Authorization API 1.0 Access Evaluation endpoint, and ExtProc to consume it with bounded freshness.

## Decision

### 1. Broker: AuthZEN Access Evaluation as an information boundary

- The end-user server exposes `GET /.well-known/authzen-configuration` and
  `POST /access/v1/evaluation` whenever token exchange is configured. There is no new broker
  flag or configuration key, and no admin API.
- `policy_decision_point` is `server.enduser.public_url` without a trailing slash, the same base
  as consent links and RFC 8414 metadata.
- Evaluation and exchange share one `tokenexchange.DelegationResolver` that holds exchange steps
  2–9a unchanged. Evaluation never reaches session retrieval, decryption, or refresh, so it never
  touches provider credentials (FR-013).
- A permit is `decision: true` with a credential-free Delegation Context (UUIDs, canonical IDs
  when configured and omitted otherwise, effective scopes, grant version and expiry). A grant
  denial is HTTP 200 `decision: false` with `reason_admin.code` and `reason_user.consent_url`.
  Request, credential, caller-policy, and dependency failures keep HTTP 400/401/403/500 and are
  never represented as decisions.
- Request members follow the AuthZEN 1.0 HTTPS binding, which requires receivers to ignore
  unknown fields: unknown members, AuthZEN `properties`, and `context` members other than `agent`
  are ignored. The supported values are required exactly (`subject.type` is the JWT token-type
  URN, `action.name` the token-exchange grant-type URN, `resource.type` `protected_resource`),
  and a present `context` must be an object whose `agent`, when present, is an Agent UUID string.
  Ignored members never influence the decision; the delegation is always resolved from the
  verified subject token alone.

### 2. Caller authorization reuses ADR 009 CEL unchanged

Evaluation authenticates the gateway with a bearer client assertion (ADR 029 trust anchor) and
calls the existing caller CEL policy with
`request.grant_type = urn:ietf:params:oauth:grant-type:token-exchange`, the raw resource, and an
empty scope. A policy cannot distinguish evaluation from exchange. This is intentional:
evaluation is never more permissive than exchange, and it issues no token.

### 3. Freshness

- The broker sends `Cache-Control: private, max-age=min(60, floor(seconds until grant expiry))`,
  bounded below by 0. The 60-second cap is fixed. Denials and errors are `no-store`.
- ExtProc caches only permits, per exact `{subject token, resource URI}` pair, in a cache
  separate from the ADR 012 token cache. The token cache's stale-on-error fallback never applies
  to delegation.
- A permit without exactly one positive integer `max-age` is an `invalid_response`: 503, not
  used, not cached.
- ExtProc computes `deadline = requestStart + min(max-age, max_staleness)` once, from the clock
  read just before sending the request. Cache hits and singleflight followers reuse it unchanged.
  ExtProc does no `Age` or transit accounting; the remaining skew is accepted.
- Immediately before every OPA evaluation that receives delegation input, ExtProc requires
  `now < deadline`. Otherwise it returns 503 without policy evaluation, forwarding, or
  re-resolution.

### 4. ExtProc `DelegationContextResolver` (fourth port) and ordering — supersedes ADR 028 ordering when enabled

- `server.DelegationContextResolver` joins `Exchanger`, `Authorizer`, and `ApprovalGate`. Its
  implementation lives in `internal/extproc/delegation` and imports no broker package (ADR 011).
- At startup ExtProc discovers the metadata from the expected PDP identifier (default: the origin
  of `oauth2.token_endpoint`), compares `policy_decision_point` by exact string, and refuses to
  start on any discovery error or mismatch. Discovery sends no credentials.
- Body-bearing path: exchange and resolution run concurrently during request headers; both
  complete before a result is chosen. Precedence: delegation denial (403 `access_denied` with the
  consent URL), resolution error (503), exchange error (existing handling), then proceed.
- Header-only path: resolve, then evaluate policy, then exchange only on allow.
- ADR 036's metadata-only input boundary for the subject token and resource URI is unchanged.

### 5. OPA input — supersedes ADR 028's input schema when enabled

ExtProc decodes the permit into its own typed struct and adds five members directly to
`input.context`: `principal` (from the permit `subject.id`), `agent`, `resource`, `grant`, and
`permission_sets`. Unset optional `canonical_id` and `valid_until` are omitted, never null; unknown
permit members never reach policy. There is no `delegation` wrapper, `subject` member, or
`delegation_available` flag. On both enabled paths `granted_permission_sets` is derived from the
permit and `granted_permission_sets_available` is `true`; both OPA members are retained
in this increment. `input.context.permission_sets` is canonical for new enabled-mode policies;
its typed IDs, optional canonical IDs and included-service contributions do not replace the
effective resource ceiling in `input.context.resource.scopes`. No OPA member is removed or
deprecated here. Future removal requires a separately approved, versioned policy-input contract
and migration. Disabled mode is unchanged byte-for-byte. All other parts of ADR 028 stay in force.

**Future Work**: [Issue #441](https://github.com/zalando-incubator/agentic-identity-broker/issues/441)
tracks both compatibility fields, availability replacement, migration guidance, enabled/disabled
behavior and versioned contract approval. Creating that issue does not approve removal. The
separate exchange-response deprecation in §8 remains unchanged.

### 6. Audit identity — extends ADR 033's deferred-finalization seam

The shared deferred-route matcher in `SecurityContextMiddleware` extends from `POST /oauth2/token`
to `POST /access/v1/evaluation`. The shared resolver finalizes the verified Actor and the distinct
CallingPeer before caller authorization, so permits and caller-policy denials carry both. Early
decode or authentication failures use the existing idempotent request-principal/anonymous
finalization before in-handler audit; the perimeter fallback remains. Ordinary-request and
token-route behavior is unchanged. `ARCHITECTURE.md` §3.1.4.2 is reconciled.

### 7. Opt-in consumer — Security-First reconciliation

`delegation_context.enabled` defaults to `false` and requires `authorization.enabled: true`.
Like ADR 028's reconciliation, the consumer is an additive policy-input surface. With it
disabled, today's controls (exchange, ADR 009 CEL, ExtProc OPA when enabled, the approval gate)
remain in force. With it enabled, every failure is closed. The broker endpoint itself is always
authenticated and authorized; it has no opt-out.

### 8. Exchange-field deprecation

`TokenExchangeResponse.granted_permission_sets` is marked `deprecated: true` in end-user API
`1.2.0` (an additive bump from the current `1.1.0`) and stays populated. `principal` and `agent_id` are unchanged. Removal happens one release
after the ExtProc resolver ships, as a separately approved end-user API `2.0.0` change.

### 9. Standard-defined naming

The permit's `context.resource.service_id` and the enabled OPA context's
`resource.service_id` identify the resolved owning service UUID. `resource.resource_uri`
identifies the protected resource. The AuthZEN request's `resource.id` remains the URI,
unchanged. This separates service identity from protected-resource identity (ADR 030).
The ExtProc port is `server.DelegationContextResolver`, distinct from the broker's
`tokenexchange.DelegationResolver`; the former retrieves fresh permit context, the latter
enforces delegation invariants shared by exchange and evaluation.

The AuthZEN paths (`/access/v1/...`, `/.well-known/authzen-configuration`) and snake_case members
are defined by the standard and are a documented exception to general REST naming conventions.

### 10. The permit governs enabled-mode token reuse

- A **Token Binding** is the sorted, de-duplicated set of permit `resource.scopes` and the
  instant parsed from `grant.updated_at`. Equality compares both sets and both instants.
  It records the permit that first claimed an exchange result, not the grant under which
  the broker actually minted or obtained the credential.
- Every successful exchange result carries an opaque **entry identity** and a shared,
  synchronized binding cell, also for non-cacheable successes. Cache hits and singleflight
  followers carry that same handle and the unchanged exchange identities/extensions.
  Every new cache installation has a fresh identity even when provider token bytes repeat.
  Credential bytes are never an identity, comparison key or exported diagnostic.
- Under synchronization, the first permit may claim an unbound result exactly once; an
  already bound result can only be compared, never rebound. Request ownership, leader
  status or "new exchange" status confers no right to overwrite a binding.
- Before forwarding, validate the delivered result handle against this request's permit.
  A matching binding is usable, including a detached result whose cache entry was replaced.
  An unbound result is usable only after an atomic successful claim. A mismatched result
  is never forwarded, regardless of which request performed the exchange.
- On mismatch, compare-and-delete only the observed entry identity under the cache lock.
  If a concurrent replacement exists, preserve it even if it holds the identical token
  string. Re-read that replacement once: compare/claim its handle as above. If compatible,
  use it without another broker call; otherwise perform at most one additional exchange
  through existing per-key singleflight and circuit-breaker handling. A failed delete is
  not an exchange error and does not authorize deletion of the replacement.
- After that one additional exchange, compare/claim the delivered handle again. If it is
  already bound to another live permit, return the terminal API-7 503
  `service_unavailable` / `delegation token binding unavailable`. Do not rebind, loop,
  delete the competing result, forward either incompatible result or use stale fallback.
  An exchange failure retains existing `exchangeErrorResponse` handling, with no removed
  or mismatched token fallback. Detailed outcomes are in contract §3.1.
- Body path validation follows completion of both concurrent calls and unchanged precedence
  (delegation denial, resolution error, exchange error), before buffering or Authorization
  mutation. Header-only binding uses the permit evaluated by policy after allow. Existing
  deadline checks apply immediately before every policy decision. An expired permit before
  forwarding is rejected with the existing delegation-unavailable 503, not re-resolved.
- Disabled mode, cache key, per-key singleflight key, TTL calculations, `cache.max_ttl`,
  and five-second re-consent cooldown are unchanged. Cooldown entries carry no token binding.
  This revision adds no broker response member, configuration key, schema or dependency.

**Provider limitation**: Narrowing a grant alone does not narrow the provider-session token.
A re-exchange independently checks the broker's then-current grant and session but may return
identical broader token bytes; a new handle can legitimately bind those bytes to the new
permit. Narrowed provider scopes reach upstream only when the session carries them, such as
Journey 5's re-authorization. OPA effective scopes govern local policy, not provider capability.

**Provenance limitation**: On double cache miss, evaluation and exchange can read different
grant versions concurrently; unbound first use can associate an exchange result with a
permit without evidence that exchange read that version. First-claim serialization prevents
conflicting bindings to one handle, not conflicting broker reads. The permit deadline bounds
policy snapshot age and ADR 012 TTL bounds token retention; neither proves exchange provenance
or provides a stronger race bound. This residual limitation is explicitly approved in D-8.


## Alternatives considered

- **Resource Search or batch evaluation**: out of scope; single Access Evaluation answers the
  per-request question.
- **A distinct caller-policy grant type for evaluation**: rejected by the 2026-10-27 maintainer
  guidance; it would let a policy grant evaluation that exchange denies, or require policy edits.
- **Extending `TokenExchanger` instead of a new port**: rejected; its stale fallback must not
  apply and FR-014 requires an independent boundary.
- **A `delegation` wrapper with a verbatim permit and a `delegation_available` flag**: rejected by
  the 2026-10-28 clarification; it leaks transport details into policy and lets broker additions
  become policy input.
- **Using the remaining `max-age` from a cache hit or `Age` accounting**: rejected for simplicity
  (U1); the broker already bounds `max-age` by grant expiry.
- **Enabled by default in ExtProc**: rejected; it would require every deployment to reach the
  broker's discovery endpoint at startup and change header-only policy input without notice.

### Permit-bound reuse alternatives

- **Keep forwarding an older bound result until ADR 012 expiry**: contrary to F1; leaves
  policy context and recorded token-reuse binding divergent.
- **Return verified exchange grant version and effective scopes from `/oauth2/token`**:
  stronger provenance, but a new externally visible exchange contract. Not selected for this
  increment; would require its own API/ADR approval and broker implementation.
- **Broker/provider token downscoping**: stronger upstream capability guarantee, but changes
  provider-session behavior and possibly provider requirements. Not selected; separately
  proposed scope is required before implementation.
- **Compare credential bytes, rebind a shared result, or retry until compatible**: rejected;
  loses same-token concurrent replacements, violates ownership, or permits unbounded work.


## Consequences


- A request with a new permit cannot forward an old, differently bound result handle.
  Compatible concurrent replacements avoid unnecessary exchange; other mismatches cost at
  most one extra exchange per request, not necessarily one per grant change/cache key.
- Concurrent live permits may deterministically receive API-7 503 after a conflicting
  singleflight result; availability is traded for binding-consistent fail-closed forwarding.
- New-generation identical provider tokens and unbound first use remain permitted under the
  explicit provider/provenance limitations. These do not claim upstream downscoping.
- D-5's five-second cooldown remains unchanged; only its broader cache-unchanged statement
  is revised. All new corrections require genuine test-first evidence outside historical D-6.


- Revocation stops informing ExtProc policy within `min(60 s, max_staleness)` of the last permit,
  independent of the exchanged-token lifetime.
- Header-only policies receive delegation data in enabled mode; `granted_permission_sets_available`
  becomes `true` there.
- A broker outage now blocks enabled-mode ExtProc requests with 503 even when a cached exchanged
  token exists.
- Policies can key on canonical IDs instead of deployment UUIDs.
- Delegation denials are never cached, and header-only requests permit as soon as the user
  consents. On the body path, the concurrent exchange during a denied request records the
  existing ADR 012 exchange re-consent cooldown (5 s), so a body-bearing request right after
  consent is answered from that cooldown until it lapses. This ADR does not change that
  cooldown; the maintainer accepted this behavior.
- Gateways other than ExtProc can consume the endpoint.
- The AuthZEN 1.0 discovery member names, well-known path insertion (between host and path),
  request identification and the implementation-defined decision `context` were verified against
  the standard's source text before the OpenAPI was frozen.

## Revision history

- **2026-11-07 (same PR, before merge)**: Incorporated ADR-3 into §5 using the supplied approval by `mjungsbluth` on 2026-11-06 at [the pipeline approval comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728). The maintainer supplied [issue #441](https://github.com/zalando-incubator/agentic-identity-broker/issues/441) in this run, satisfying T104's prerequisite. Canonical guidance changes; current compatibility values, serialization and response deprecation do not.

- **2026-11-06 (same PR, before merge)**: `mjungsbluth` approved ADR-2, API-7, ADR-3, D-7 and D-8 at `2026-11-06T15:30:38Z` on [the pipeline approval comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728). §10 incorporates ADR-2 and its API-7/D-8 constraints. ADR-3 incorporation remains pending the required maintainer-created follow-up issue. D-7 telemetry migration is implemented in run 42297225880/1 after the publication path became available: the broker span/events use `tokenexchange.access_evaluation*`, both required namespaces are registered, span kinds are explicit, and the breaking-telemetry changelog/query migration is published. Test-first and current validation evidence are in quickstart §16.

- **2026-11-06 (same PR, before merge)**: `mjungsbluth` approved P3/F4 on
  [the remediation proposal](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6669627407)
  at `2026-11-06T06:59:39Z`: rename the ExtProc resolver port to
  `DelegationContextResolver`, define Delegation and Access Evaluation in the architecture
  glossary, and use `resource.service_id` in permits and enabled OPA input. The request URI
  member is unchanged. P2/F3's issue-dependent permission-set revision remains unapplied
  because this workflow cannot create its required follow-up issue.

- **2026-11-05 (same PR, before merge)**: Supplied maintainer approval by `mjungsbluth` at
  `2026-11-05T16:38:49Z` on [the pipeline approval comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6661347255)
  covers all thirteen items (API-1–API-6, ADR-1, D-1–D-6), including §1's ignore-unknown
  contract and the unchanged body-path exchange cooldown. The historical VIII/XIII exception
  D-6 is conditional alternative validation, not a claim that original test-first sequencing
  occurred; its disposition is recorded in the feature's `quickstart.md`.

- **2026-10-30 (same PR, before merge)**: API-003 verification found that AuthZEN 1.0 requires
  receivers to ignore unknown request fields, which conflicts with the approved strict rejection
  (research R10). Following the user's direction in the implementation session, §1 now ignores
  unknown members and `properties`. Error bodies keep the approved `ErrorResponse` object; the
  standard's "error message string" is interpreted as a JSON body carrying an error message.
  The maintainer confirmed this revision of API-2 on 2026-10-31 in the implementation session.
- **2026-10-31 (same PR, before merge)**: The maintainer accepted the body-path consequence
  recorded below (consent after a denial takes effect on body-bearing requests once the existing
  ADR 012 re-consent cooldown lapses) and the in-PR Phase 0 resolver extraction.
