# Research: Incremental Service Authorization

**Feature**: [spec.md](./spec.md) | **Plan**: [plan.md](./plan.md) | **Date**: 2026-10-29

Each entry records the decision, why it was chosen, and the main alternatives. Items marked
**Proposed (approval item …)** need the maintainer's sign-off before implementation. They are
registered in [plan.md › Approval Items](./plan.md#approval-items).

## Baseline findings from the current code

| Area | Current behavior | Source |
|---|---|---|
| Recoverable exchange errors | `invalid_grant` + `error_uri` from `OAuth2SessionService.ServiceAuthorizeURL`, which returns to fixed `/sessions`. Grant denials use `access_denied` + `<public URL>/agents/{id}`. No `scope` member. | `internal/domain/tokenexchange/service.go:296-372`, `internal/domain/oauth2session/service.go:350-355`, `internal/adapters/http/enduser/oauth2_token.go:373-390` |
| Rejected provider refresh | Already persisted since #271: under the session row lock, `refresh_token_expires_at` is set to the rejection time. `CanRefresh()`/`IsExpired()` and `ListActiveByPrincipal` then treat the session as unusable once the access token has expired. A reconnect replaces the row through the same lock and clears the marker. | `internal/domain/oauth2session/service.go:1746-1795`, `internal/domain/storage/user_session.go:138-173`, `specs/013-token-exchange/spec.md` Journey 1 |
| ADR 044 text | Still says "Provider refresh rejection leaves the stored session unchanged" and fixes the return target to `/sessions`. The first clause is stale relative to #271; the second blocks FR-006. | `adrs/044-service-specific-session-recovery-url.md:25-27` |
| ExtProc challenge | `reauthChallengeResponse` forwards `error_uri` and description only. Inputs come only from the `aib.tokenexchange` metadata namespace (ADR 036). | `internal/extproc/server/reauth_challenge.go`, `internal/extproc/server/server.go:900-950` |
| Authorization endpoint | Checks `agent.AllowedScopes`, then grant, then `anyDelegatedSessionUnusable` (no refresh attempt), then mandatory requirements. Proxy agents get `req.Scope` copied upstream unchanged. Consent context is a JWE (`AuthorizationSessionClaims`) carrying `original_url`. | `internal/domain/oauth2/service.go:137-480`, `internal/domain/oauth2/sessiontoken/claims.go` |
| Local issuance | fosite. Scope strategy `oauth2.IsScopeAllowed` (empty allow-list = unrestricted). Granted scopes are stored as a string on `authorization_codes.scope` and `refresh_token_sessions.scope`, and become the JWT `scope` claim and token-response `scope`. Refresh rotation happens in `PopulateTokenEndpointResponse`, after request validation. | `internal/domain/oauth2server/provider.go:208-470`, `fosite_storage.go:113-372`, `strategies.go:104` |
| Consent | Detail response has `services[].connectionStatus`, `active_session_service_ids`, and `permission_sets[].permission_set.service_scopes[]` (no scope names). Grant submission already rejects included services without active sessions (`ErrUnconnectedServices`). | `internal/adapters/http/handlers/consent/agent_detail_handler.go`, `grants_handler.go:108-305`, `internal/domain/consent/service.go:290-470` |
| Service identity | Services have a UUID and an optional canonical ID (`^[A-Za-z0-9._-]{1,128}$`, never UUID-shaped). Repositories provide `GetByCanonicalID`. | `internal/domain/canonical/id.go`, `internal/adapters/storage/*/thirdparty_provider.go` |
| Session detail API | `GET /api/third-party/{serviceId}/session` accepts only a UUID and returns `404` when no session exists. | `internal/adapters/http/oauth2_sessions/handler.go:384-485` |

## R1 — Virtual scope grammar and matching

**Decision**: A virtual service scope is the exact string `<server.enduser.public_url without trailing slash>/sessions/<identifier>`.
Recognition is an exact, case-sensitive string-prefix match on `<public URL>/sessions/`, with no URL
normalization. The remainder must be either the lowercase canonical form of a UUID or a value that
`canonical.Validate` accepts. Any other remainder under that prefix is a *malformed broker service
scope*: extra `/`, query, fragment, percent-encoding, a trailing slash, an empty identifier, or an
uppercase UUID. A value outside the prefix is an ordinary scope, including scheme or host case
variants and other origins, and remains subject to ordinary scope rules.

**Rationale**: Exact prefix matching avoids parser differentials and Host-derived origins (FR-002,
SR-004). Canonical IDs can never be UUID-shaped, so the two identifier forms are unambiguous.

**Alternatives**: Parse with `net/url` and compare normalized origins. This was rejected because it
creates more than one accepted spelling per service and needs a normalization policy that must be
re-specified for every consumer.

## R2 — Which scope string is granted, and how identity is pinned (DEC-001, DEC-004)

**Decision**:

- Authorization grants each virtual scope in the form the client requested. The broker does not
  rewrite it. Repeated identical scope tokens collapse to their first occurrence. Canonical and
  UUID tokens for the same service form one Requested Service Set entry, but both distinct raw
  tokens remain granted and bound to that service. This follows FR-020 and the data model's
  `Scopes []VirtualServiceScope`; deduplicating services must not rewrite the granted scope set.
  Challenges always emit the current canonical form, or the UUID form when the service has no
  canonical ID.
- When the authorization endpoint resolves a virtual scope, it records a **service-scope binding**
  (`scope string → service UUID`). The binding is carried by:
  - the consent JWE (`requested_service_ids` plus bindings);
  - the authorization-code record;
  - every refresh-token session record, inherited from the code and then from each predecessor.
- At local refresh, each bound scope is resolved again. If the scope no longer resolves, or it
  resolves to a different UUID, refresh fails with `invalid_scope` and does not rotate the token.
  This happens, for example, after a rename or when a canonical ID is reused.
- **Proposed (DEC-004)**: Migration `046_add_service_scope_bindings` adds a nullable
  `service_scope_bindings JSONB` column to `authorization_codes` and `refresh_token_sessions`.
  `NULL` means "no virtual scopes", so existing rows are unaffected and need no backfill.

**Rationale**: FR-019 requires that canonical-ID reuse cannot redirect an existing grant and that
only UUID scopes are durable. Without a persisted binding, a reused canonical ID would silently
resolve to another service at refresh. Preserving the requested string keeps RFC 6749 refresh
comparisons stable. A client that requested the UUID form keeps working after a rename.

**Alternatives**:

1. *Store only the UUID form in `scope` and render the canonical form in responses.* This was
   rejected. Old-canonical refresh would keep succeeding, which contradicts FR-019 and Story 4
   Scenario 2. Stored and returned scope strings would also diverge.
2. *Resolve again without pinning.* This was rejected because it fails open on canonical-ID reuse.
3. *A separate binding table.* This was rejected because it needs more lifecycle work for data that
   is strictly 1:1 with an existing row.

## R3 — Session usability check (FR-008, DEC-003)

**Decision**: Add `OAuth2SessionService.CheckServiceUsability(ctx, principal, serviceID, effectiveScopes)`.
`effectiveScopes` is the provider-scope set computed by the same selected-grant/ceiling rule as
exchange (R4), not merely the configured `required_scopes`. It reuses `GetValidAccessToken`: a
valid access token, or a single-flight, locked refresh of an expired one. It then checks the stored
session scopes against `effectiveScopes` and the established client identity, and classifies the
outcome:

- `usable`
- `missing`
- `unusable_definitive` (access expired and refresh rejected, unavailable or expired; or effective
  scopes insufficient; or established client identity mismatch). A valid access token with sufficient
  effective scopes and matching identity is `usable` even without a refresh token or with an expired
  refresh token; no refresh is attempted in that case.
- `unverifiable_transient` (provider 5xx, 429, network error, storage error, or cancellation)

The authorization endpoint, consent detail, grant submission, and local refresh recheck all use
this check. Transient results fail closed:

- authorization returns `temporarily_unavailable`, sent by redirect;
- local refresh returns `server_error`;
- consent keeps Save disabled with a retry reason.

Transient results never write an unusability marker.

**Proposed (DEC-003)**: Definitive provider refresh rejection stays persisted through the existing
#271 marker, `refresh_token_expires_at = rejection time`, with no schema change. ADR 045 (draft)
records this behavior and retires ADR 044's stale "leaves the stored session unchanged" clause.
Every consumer reads the same row: consent, the session list and detail, tool exchange, and the
background refresh and sweep. Reconnect precedence and update-only termination stay as ADR 038 and
the `WithLockedSession` contract define them (FR-018).

**Rationale**: This gives the same outcome as a real service call (FR-008). The existing row lock
already orders refresh against reconnect. A rejected-refresh write cannot overwrite a newer
reconnect, because both run under `WithLockedSession`, and the refresh re-reads the current row
after it acquires the lock.

**Alternatives**: A new `unusable_at` column. This was rejected because the existing marker already
expresses the state for every consumer. A new column would duplicate it and need a migration.

## R4 — Requested Service Set and its enforcement point

**Decision**: A new bounded context, `internal/domain/servicescope/`, owns these concepts:

- the Virtual Service Scope value object (`Parse`, `Format`);
- the Requested Service Set (`Resolve` deduplicates and checks agent eligibility);
- `Evaluate`, which returns, for each requested service, the covering permission sets, the
  satisfied required scopes, and the usability.

The context has no I/O of its own. It depends on these interfaces:

- a service lookup over `ports.ThirdpartyOAuth2ProviderRepository`;
- the permission-set resolver;
- a usability checker, which `OAuth2SessionService` satisfies.

The following domain services consume it:

- `oauth2.AuthorizationService`: authorization and proxy stripping;
- `consent.Service`: detail and submission;
- `oauth2server.Provider`: refresh recheck and binding persistence;
- `tokenexchange.TokenExchangeService`: emitting the `scope` member;
- `oauth2session.OAuth2SessionService`: the recovery return URL.

**Eligibility**: An agent can request a service only if that service appears in its
`service_requirements`. When the agent declares no requirements, the service must instead be
covered by one of the agent's declared permission sets. This matches the token-exchange eligibility
rule in `resolveEffectiveScopes`.

**Coverage and effective scopes**: Compute `U`, the union of provider scopes from the agent's
selected permission sets that define and include this service. With an explicit requirement and
`require_all_scopes: false`, `E = U ∩ required_scopes`; otherwise (all-scopes or no explicit service
requirements), `E = U`. Preserve exchange's structural-reference, service-inclusion/definition,
requirement-exclusion and empty-intersection denials. Explicit fixed-scope coverage additionally
requires `required_scopes ⊆ E`; all-scopes/no-requirements coverage requires inclusion and definition,
not a fabricated empty required-scope ceiling. Session usability always requires `E ⊆ session.Scopes`.
Thus empty configured `required_scopes` never erases a nonempty permission-set union. Authorization,
consent submission and local refresh pass `E` to R3, just as exchange does.

For live consent selection, compute each candidate set's contribution `E_i` with this same ceiling.
Check the session once (including any eligible refresh), then classify every `E_i` against that same
verified session snapshot. Return each candidate's `usability`, plus its existing
`supplied_required_scopes`. The service-level usability checks the fixed configured required scopes
(or the empty baseline for all-scopes/no-requirements); it is not proof of an unsaved selection.
Save requires the baseline usable, at least one defining candidate selected, required-scope coverage,
and `usable` for every selected candidate's contribution. Since `E = union(E_i)`, this checks the
live selection without exposing raw permission-set scopes or adding a request on every toggle.
Unselected unusable candidates do not block Save; selecting one immediately does. Submission and
authorize re-entry recompute `E` and verify current usability server-side, never trusting UI flags.

**Rationale**: Principle VI rejects anemic handlers and logic leakage. Four contexts need the same
invariant, and one owner prevents divergence (SR-001).

**Alternatives**: Duplicating the logic in each service. This was rejected because there is already
evidence of drift between `anyDelegatedSessionUnusable`, which does no refresh, and token exchange,
which does.

## R5 — Authorization endpoint flow with requested services (FR-007, FR-013)

**Decision**: `HandleAuthorization` parses virtual scopes after the redirect URI is validated.
Malformed, unknown, or ineligible broker scopes produce an `invalid_scope` redirect. Virtual scopes
are exempt from the `allowed_scopes` check, and ordinary scopes are still checked against it. When
a grant is active:

1. Every requested service must be covered and usable, in addition to the existing checks.
2. If any is not, the user is redirected to consent with `requested_service_ids` and bindings
   sealed in the JWE.
3. If all are satisfied:
   - local and CIMD agents get a code carrying the bindings;
   - proxy agents get the upstream URL built from the request scope with every virtual scope
     removed. The `scope` parameter is omitted when nothing remains.

On grant submission, validate the sealed IDs/bindings against a fresh resolution of `original_url`.
After Save, return a server-built `redirect_url` consisting of that original authorize URL plus
`session_token=<the same JWE>`. The browser follows it as an opaque URL. On authorize re-entry,
authenticate the JWE again (original 10-minute expiry, principal and agent), compare every original
request parameter with the sealed `original_url` after removing only `session_token`, and use the
sealed request and bindings as authority. Fresh resolution must reproduce the bound UUIDs before
local code issuance or proxy redirect. Never discard bindings and treat this continuation as a new
initial resolution. Reassignment after Save but before re-entry therefore returns `invalid_scope`
and issues no code or upstream redirect. Forged, expired or request/user/agent-mismatched context
uses the existing invalid-session error handling without issuing a code (SR-002).
The continuation parameter is consumed locally and never forwarded upstream or to the client callback;
returning to consent retains the same bindings and original expiry rather than renewing the TTL.

**Rationale**: Reuse the existing JWE and `original_url` round trip (ADR 016), including the browser
re-entry, rather than inventing browser-authoritative IDs or another state store.

**Alternative**: Return the plain original URL after validating Save. Rejected because bindings
would be lost before the final authorization decision.

## R6 — Local refresh (FR-012, FR-020, API-LOCAL-SCOPES)

**Decision**: `Provider.HandleRefreshToken` runs two broker checks after fosite validates the
request and before `PopulateTokenEndpointResponse`, which rotates the token:

1. **Widening check**: Any requested virtual scope that is not in the stored granted set causes
   `invalid_scope` with "scope <s> requires interactive authorization".
2. **Recheck**: For every granted virtual scope, the broker resolves the binding again, then checks
   eligibility, coverage, and effective-scope usability through `servicescope.Evaluate`. Iterate all
   immutable binding keys, including ones no longer in the current `Scope` after narrowing. This
   applies whether `scope` is omitted or narrowed.

A definitive failure returns `invalid_scope`, and a transient one returns `server_error`. Neither
rotates or revokes the refresh token. An existing revoked-grant error keeps its current
`invalid_grant` handling.

**Successful narrowing (Proposed, API-LOCAL-SCOPES)**: Request `scope` must be a subset of the
predecessor's current `Scope`. The response `scope` and access-token claim equal that subset, and
the successor refresh row's `Scope` also equals it. A scope-less request instead uses the
predecessor's current `Scope`. The entire `ServiceScopeBindings` map is inherited unchanged as a
continuing-service assurance set, even for scopes removed from `Scope`. It cannot grant a removed
scope again. Every later refresh rechecks all retained bindings; re-requesting a removed scope is
widening and fails without consumption. Code validation keeps bindings within its granted `Scope`;
refresh validation permits assurance-only bindings and successor creation enforces exact map
inheritance and scope subset. No second column is needed.

**Rationale for narrowing**: Durable narrowing preserves least privilege while retaining FR-012's
continuing-service checks. The main alternative, keeping full successor `Scope`, would restore
dropped scopes on the next scope-less refresh. Dropping bindings with scopes would evade the
continuing-service check. Neither is chosen.

`client_credentials` requests that contain a virtual scope return `invalid_scope`, because there is
no user whose sessions could be checked.

**Rationale**: Doing both checks before rotation is the only way to keep the refresh token valid
(FR-012). fosite's scope strategy is extended so that virtual scopes pass the per-client
allow-list. The broker checks above remain authoritative.

## R7 — Proxy refresh (DEC-002, FR-021)

**Decision**: The proxy `/oauth2/token` path removes virtual scopes from the `scope` form value of
`refresh_token` requests and omits `scope` when nothing remains. It forwards everything else and the
upstream response unchanged. It runs no post-response usability check.

**Rationale**: This was clarified. It keeps upstream rotation intact.

**Alternative**: A post-response veto. This was rejected because it would discard the only successor
credential after an upstream success.

## R8 — Gateway challenge (FR-005, API-EXCHANGE-CHALLENGE, API-METADATA-CONFIG)

**Decision**:

- **Broker**: Every token-exchange error that carries an `error_uri` after the service is resolved
  also carries `"scope": "<canonical virtual scope>"`. These are the grant-missing, grant-expired,
  grant-empty, coverage, session, and scope-insufficient errors. Errors raised before the service is
  resolved, and non-recoverable errors, carry no `scope`.
- **ExtProc**: The broker-error decoder reads `scope`. A non-transient error with an `error_uri`
  still produces the existing 403 response. The challenge additionally gets:
  - `scope="<value>"`, only if the value is one absolute `http` or `https` URL that passes
    `validChallengeURI`;
  - `resource_metadata="<value>"`, when the request carries the optional
    `aib.tokenexchange.resource_metadata_url` metadata field and that value is a valid absolute
    `https` URL that is safe in a header.

  An invalid optional value is omitted and logged as a diagnostic. It never changes the exchange
  outcome. The elicitation body is unchanged.
- **Proposed (API-METADATA-CONFIG)**: The metadata URL is configured for each route in the
  Agentgateway `extProc.metadataContext` as a CEL string literal, beside `resource_uri`. It is not
  an ExtProc YAML key and not a broker key.

**Rationale**:

- RFC 9728 metadata is specific to each resource, and the gateway route already owns
  `resource_uri`.
- ADR 036 makes the metadata namespace the gateway-to-ExtProc trust boundary.
- A single ExtProc YAML key could not describe multiple routes.
- Omitting the optional value when it is invalid is safe, because the value is advisory and not a
  security control.

**Alternatives**: `EXTPROC_CHALLENGE_RESOURCE_METADATA_URL` as one global value, which is wrong for
multi-route sidecars. A map keyed by resource in the ExtProc YAML, which duplicates gateway route
configuration.

## R9 — Recovery return URL (FR-006, ADR-RECOVERY)

**Decision**: `ServiceAuthorizeURL(service)` returns
`<public URL>/api/third-party/<uuid>/oauth2/authorize?redirect_uri=<QueryEscape(<public URL>/sessions/<canonical-or-uuid>)>`.
The path keeps the UUID, and the return page uses the emitted scope identifier. The existing
same-origin `redirect_uri` validation already accepts this target. The callback appends its
existing `service_id` and `success` parameters. Grant-denial links remain `<public URL>/agents/<id>`.

**Proposed (ADR-RECOVERY)**: Draft [ADR 045](./adrs/045-service-session-recovery-return.md) replaces
two clauses of ADR 044: the fixed `/sessions` return clause and the "leaves stored session
unchanged" clause. It keeps every other ADR 044 protection. ADR 044 is not edited.

## R10 — Session page API (FR-015, API-SESSION-DETAIL)

**Decision**: Add `GET /api/third-party/sessions/{service}`, where `{service}` is a canonical ID or
a UUID. The response has this shape:

```text
{data: {service: {id, canonical_id, display_name, description, scope_url},
        session: SessionStatus | null,
        dependent_agents: [AffectedAgent]}}
```

- An unknown service returns `404 not_found`.
- A request without a principal returns `401`.
- `dependent_agents` uses the existing grant-owned derivation (`ListByPrincipalAndServiceID`).
- The page's refresh and terminate actions call the existing UUID routes.

**Rationale**: The design is additive, and it represents "service exists, no session" as `200`
with `session: null` (FR-015). The existing UUID route keeps its `404` behavior.

**Alternatives**: Change `GET /api/third-party/{serviceId}/session` to accept canonical IDs and
return `200` with `session: null`. This was rejected because it is a breaking behavior change for
existing clients.

## R11 — Consent API additions (FR-009–FR-011, API-CONSENT)

**Decision**:

- `GET /api/consent/agents/{id}?session_token=…` gains two fields. Both are always present and are
  `[]` when the request has no token or no requested service:
  - `requested_service_ids: [uuid]`;
  - `requested_service_coverage: [{service_id, required_scopes, usability, covering_permission_sets:
    [{permission_set_id, supplied_required_scopes, usability}]}]`.

  The UI decides "selected sets cover the required scopes" from the union of
  `supplied_required_scopes` over the user's live selection. Raw permission-set scopes stay out of
  `permission_sets[].service_scopes`, which the current contract omits on purpose. The coverage
  object exposes only the agent's required scopes, which `services[].requiredScopes` already
  publishes. Each candidate's `usability` verifies its effective contribution against the same
  session snapshot (R4). Live Save gating combines the baseline and selected candidates' usability;
  it cannot infer all-scopes usability from empty `required_scopes`.
- For requested services, `services[].connectionStatus` reflects usability for the current saved
  selection's effective scopes. The existing enum does not change; Save separately evaluates the
  live selection, so an unselected failing candidate does not force an unnecessary login.
  `requested_service_coverage[].usability` (`usable`, `missing`, `unusable`, or `unavailable`)
  lets the UI distinguish "log in" from "temporarily unavailable, retry".
- `POST /api/consent/agents/{id}/grants?session_token=…` rejects a submission with
  `400 {"error":"requested service not satisfied","message":"…<service ids>…"}` when any requested
  service is uncovered, lacks required-scope coverage, or is unusable. It neither changes the grant
  nor returns `redirect_url`. Success returns the original authorize URL plus the same sealed
  `session_token`; the authorization service revalidates it independently after the browser round
  trip, including binding identity and current effective-scope usability (R5).

**Rationale**: The IDs are derived on the server from the sealed context (SR-002). The UI and the
server share the same coverage rule. No raw scope catalog is newly exposed.

**Alternatives**:

1. Add raw `scopes` to `service_scopes`. This was rejected because it reverses that field's
   deliberate omission.
2. Return a single server-computed "satisfied" flag. This was rejected because coverage depends on
   the user's unsaved toggles.

## R12 — Frontend

**Decision**:

- Add a `/sessions/:service` route rendered by a new `ThirdPartySessionDetailPage`. The page reuses
  the `SessionCard` content, the confirmation dialog, and the actions.
- On the consent page, `AgentGrantDetailPage` and `PermissionSetsList` gain:
  - a `RequestedServiceBanner` for each requested service;
  - a "Covers requested service" `Badge` on each candidate set;
  - a "requested" marker on service entries;
  - Save disabled while unsatisfied, with the reason linked through `aria-describedby`.

These are application components built from existing design-system primitives (`Card`, `Badge`,
`Alert`, `Button`, and `Toggle`). No new universal component is planned.

**Rationale**: This follows the spec's Frontend requirements and the design-system review gate (Principle XI).

## R13 — API versioning

**Decision**: End-User API `info.version` changes from `1.1.0` to `1.2.0` for the additive fields,
route, and error. The `error_uri` target change stays within the documented recovery purpose, as
the ARCHITECTURE versioning note and ADR 044 rationale describe. The ExtProc metadata contract
changes additively; the ExtProc has no versioned OpenAPI.

## Deferred findings (outside this feature; not tasks)

- `anyDelegatedSessionUnusable` does not try a refresh for granted services that were not
  requested. Changing it would alter unrelated authorization behavior. The maintainer can decide
  whether to file an issue.
- ADR 044's stale rejected-session clause is corrected by ADR 045 as part of this feature, so no
  separate issue is needed.
