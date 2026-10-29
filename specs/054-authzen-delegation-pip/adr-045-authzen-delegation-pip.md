# ADR 045: AuthZEN Delegation Policy Information Point

**Status**: Proposed (draft; awaiting maintainer sign-off on the pipeline's approval comment)
**Date**: 2026-10-29
**Scope**: End-user API AuthZEN discovery and Access Evaluation; ExtProc delegation context;
`/oauth2/token` `granted_permission_sets` deprecation
**Supersedes in part**: ADR 028 (enabled-mode OPA input schema and request ordering only)
**Extends**: ADR 033 (deferred-finalization seam), ADR 009 (caller CEL reuse), ADR 012 (separate cache)
**Feature**: `specs/054-authzen-delegation-pip`

This file is the feature-local draft. The implementation phase records the maintainer's approval
in the Status line and moves the file to `adrs/045-authzen-delegation-pip.md`, after rechecking
that 045 is still the next unused number. Under Principle II, the draft may be revised in this PR
until it merges; the final ADR and the code must agree.

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
- Request members are strict: unknown members, `properties`, and `context.actor` return 400.

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

### 4. ExtProc `DelegationResolver` (fourth port) and ordering — supersedes ADR 028 ordering when enabled

- `server.DelegationResolver` joins `Exchanger`, `Authorizer`, and `ApprovalGate`. Its
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
permanently. Disabled mode is unchanged byte-for-byte. All other parts of ADR 028 stay in force.

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

The AuthZEN paths (`/access/v1/...`, `/.well-known/authzen-configuration`) and snake_case members
are defined by the standard and are a documented exception to general REST naming conventions.

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

## Consequences

- Revocation stops informing ExtProc policy within `min(60 s, max_staleness)` of the last permit,
  independent of the exchanged-token lifetime.
- Header-only policies receive delegation data in enabled mode; `granted_permission_sets_available`
  becomes `true` there.
- A broker outage now blocks enabled-mode ExtProc requests with 503 even when a cached exchanged
  token exists.
- Policies can key on canonical IDs instead of deployment UUIDs.
- Gateways other than ExtProc can consume the endpoint.
- API-003 verification against the final AuthZEN 1.0 text must happen before the OpenAPI is
  frozen; a conflict with the decisions above returns to the maintainer.
