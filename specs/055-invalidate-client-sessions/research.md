# Research: Invalidate Sessions After Service Client-ID Changes

**Feature**: [spec.md](spec.md) | **Plan**: [plan.md](plan.md) | **Date**: 2026-11-05

Every unknown in the plan's Technical Context is resolved here. Each entry gives the decision, the rationale, and the alternatives considered. Decisions marked **Proposed** need maintainer sign-off (`plan.md` Approval Items).

## Current behavior (evidence)

| Area | Source | Finding |
|---|---|---|
| Token acquisition | `internal/domain/oauth2session/service.go` `GetValidAccessToken` | ADR 046 already blocks issuance after recorded refresh rejection. Otherwise the valid-access-token path does not compare current service identity (ADR 038); that is the remaining FR-002 gap. |
| Credential selection | `internal/domain/oauth2session/credentials.go` `selectCredentials` | Exchange and refresh compare the established `UpstreamClientID` with the selected client ID. They do so only when `credentialSourceEligible` (not public, not CIMD confidential, not Google). Missing identity passes only for stored services that never transitioned. |
| Initiation | `service.go` `initiateOAuth2Flow` | Seals `UpstreamClientID` in the JWE state only for eligible services. |
| Session write | `service.go` `createSession` | A locked compare-and-set on session ID and `CallbackID` already prevents an older callback from overwriting a newer reconnect (FR-004). |
| Refresh | `refreshLockedSession`, `refreshSessionTokens` | Runs under `WithLockedSession` and calls `RefreshAccessToken` with the established identity. This is shared by on-demand, background, manual, and sweep refreshes. |
| Agent authorization | `internal/domain/oauth2/service.go` `anyDelegatedSessionUnusable` | Already redirects to consent for a missing session or ADR 046 reauthentication condition on an included service. Add identity unusability without replacing rejection or refreshability handling (FR-007, FR-015). |
| Consent detail | `internal/domain/consent/service.go` | `IsConnected` comes from `ListActiveServiceIDsByPrincipal`; preserve ADR 046's existing rejection and refreshability predicates while adding identity filtering. |
| Consent submission | `consent/service.go` `ListActiveByPrincipal` | Preserve ADR 046's existing session-usability rule while adding identity to mandatory-service validation. |
| Session listing | `ListUserSessions`, `storage.NewUserSessionSummary` | `requires_reauthentication` already exists. `is_expired` is its equal deprecated alias, while `access_token_expired` is an independent expiry signal (ADR 046). |
| Admin update | `internal/domain/thirdparty/service.go` `prepareUpdate` and the postgres `UPDATE thirdparty_oauth2_services` | One update path writes `client_id`. `credential_source_transitioned` is computed in SQL atomically. Issuer and resource changes are refused while sessions exist. Client-ID changes are allowed. |
| Token-exchange mapping | `internal/domain/tokenexchange/session_diagnostic.go` | `identity_missing` and `identity_mismatch` map to `credential_identity_changed`, which gives a recoverable `invalid_grant` with ADR 044's `error_uri`. |
| Binding baseline | Accepted `adrs/046-observed-refresh-token-rejection.md`, `api/enduser/openapi.yaml` 1.2.0 | Observed rejection, response fields, deprecated alias, and `409 refresh_rejected` are already accepted, not optional parallel work. |

## R1 — How to detect a client-ID change on both channels

**Decision (Proposed, ADR-001)**: Each service has a persisted `client_identity` and a monotonic `client_identity_revision`. An Admin API update that changes the stored effective client ID increments the revision in the same SQL `UPDATE`, alongside the existing `credential_source_transitioned` expression. A filesystem change is recorded by compare-and-set when any broker operation reads a client-ID file value that differs from `client_identity`.

**Rationale**: The Admin API channel is synchronous, so the update can record the change exactly when it commits (AS-1.7). Rejected or conflicting updates record nothing, because the increment is part of the same conditional statement. Administrative service-metadata operations must not read files (ADR 041); consent/session usability displays are the explicitly proposed exception in R4, so the filesystem channel can only be discovered when a file is read. Every token release reads the file first, so the first relevant use records the change (AS-1.1, FR-002). A counter makes rollback a new revision (FR-005). It also gives legacy sessions a "change since rollout" signal without inferring identity (FR-013).

**Alternatives considered**: comparing only the client ID (fails FR-005 and FR-013); a bulk session invalidation write (does not see lazy filesystem changes, and holds locks in the admin transaction); a file watcher or poller (adds a scheduler, and replicas still need use-time checks); timestamps (depend on clock agreement).

## R2 — Which service variants have a mutable identity (FR-003)

**Decision**: The effective identity is the stored `client_id` for every stored variant and the client-ID file value for filesystem services. The identity association and revision are sealed for every new flow, not only for `credentialSourceEligible` services.

| Variant | Identity source | Can it change? |
|---|---|---|
| Static confidential, stored | Admin `client_id` | Yes, by PUT |
| Static public (`none`), stored | Admin `client_id` | Yes, by PUT |
| Google flavor | `client_id` extracted from the supplied credential | Yes, by PUT |
| CIMD (`private_key_jwt` or `none`) | Derived from `server.enduser.public_url` and the service ID, then stored | Only when the stored value changes, for example a `static`↔`cimd` switch. No mutable identity is invented. |
| DCR (discovery `client_method: dcr`) | Stored registration `client_id` | Yes, when a re-registration stores a different value through the same update path |
| Filesystem | Client-ID file | Yes, by publication |

**Rationale**: FR-003 forbids limiting the rule to the methods that ADR 041 covered. All stored variants share the single `UPDATE` path, so one SQL expression covers them.

**Alternatives considered**: keeping ADR 041's eligibility set (violates FR-003).

## R3 — Usability rule and legacy sessions

**Decision (Proposed, DEC-001 via ADR-001)**: A session is identity-usable when (a) its recorded identity equals the current effective identity and its recorded revision equals the current revision, or (b) it has no recorded identity, the service is stored, the service never transitioned, and the current revision is 0. Sessions that existed before rollout get revision 0 through a column default. No identity is backfilled. Identity usability is only one part of usability: preserve ADR 046's existing `RequiresReauthentication` predicate and OR it with failed identity usability. Rejected sessions remain rejected even with unexpired tokens; expired tokens with stored, not-rejected refresh tokens remain refreshable if identity is usable.

**Rationale**: (a) gives rollback safety. (b) keeps ADR 041's exception exactly until the first recorded change after rollout (FR-013, AS-1.6). It applies per service, so every principal is affected together (AS-1.2).

**Alternatives considered**: invalidating all identity-less sessions at rollout (rejected in the 2026-11-04 clarification); inferring identity from current values (forbidden by FR-013 and SR-001).

## R4 — Where the check runs and how much it costs

**Decision**: Add a single domain operation in `oauth2session`, `resolveClientIdentity(ctx, service) (ClientIdentityState, error)`. It resolves the effective identity: a stored read, or `CredentialFileReader.ReadClientID` for filesystem services. It records a filesystem observation through the focused repository defined in `internal/ports/storage.go`, then returns `{Identity, Revision}`. The observation atomically fences filesystem source, administrative version, canonical ID, and identity revision, including initialization and no-ops. On a lost precondition, discard the observed value, reload the service/source/version/canonical ID/revision, reselect the immutable configuration binding, and perform a fresh file read if still filesystem; never replay an obsolete file value. A pure function, `sessionIdentityStatus(session, service, state)`, applies R3. Callers:

- Token acquisition, before any decrypt (FR-002).
- `refreshLockedSession`, inside the lock before `refreshSessionTokens` (FR-004, AS-1.3).
- `HandleCallback`, before code exchange (AS-1.4).
- `initiateOAuth2Flow`, to seal the identity and revision (it already reads the file).
- Agent authorization, consent detail, consent submission, and session listing and detail, through a narrow port that `oauth2session` implements (FR-007, FR-008).

**Proposed ADR-001 boundary change**: ADR 041 and `ARCHITECTURE.md` currently forbid file resolution on administrative, consent, and session metadata surfaces. Preserve that rule for administrative service create/update/get/list only. Partially supersede it for consent detail and session usability list/detail/refresh summaries: these read the live client-ID file, never the secret file, with R5's persisted fallback. Agent authorization and grant submission are gates and fail closed. Storage summary projections still omit encrypted token blobs; metadata-only storage reads do not mean no client-ID file read in the domain.

Token acquisition reads the service through a projection that needs no secret decryption. This avoids a KMS call on every exchange.

**Rationale**: One rule, used everywhere, keeps the surfaces consistent (SC-002) and keeps conditional logic in the domain (Principle VI). For filesystem services, the cost is one bounded `ReadClientID` call per token exchange. This is the same adapter and limits that initiation already uses.

**Alternatives considered**: putting the check in the SQL predicate (cannot read files); caching the file value in memory (a stale cache would release previous-identity tokens).

## R5 — Failure behavior when identity cannot be resolved

**Decision (Proposed, DEC-003)**: Token release and refresh fail closed with the existing `credential_source_unavailable` infrastructure outcome (`500 server_error`, no `error_uri`). Agent authorization returns its existing server error. Read-only display surfaces use the persisted `client_identity` and revision instead, and log the source failure.

**Rationale**: SR-001 forbids treating unreadable identity as proof of usability, and forbids disguising it as a sign-in problem. Display surfaces do not release credentials. Failing them would hide every session during a file outage without a security benefit.

**Alternatives considered**: returning the stored token when the file is unreadable (violates SR-001); failing the whole session list (worse availability, no security gain).

## R6 — Observation across replicas

**Decision (Proposed, DEC-004)**: The observation compare-and-set is conditional on filesystem source, the administrative version and canonical ID that selected the binding, and the identity revision that was read. Only an observation whose configuration preconditions still hold may record a differing value as a new revision, even if it is a stale or reverted file on that replica. After any lost precondition, reload the service and binding and read again rather than retrying the obsolete observation (see data-model recording rules). The existing administrative version supplies the generation fence without a new column. No version change alone invalidates a session. Immutable binding configuration changes require replica replacement; coherent publication remains an operator prerequisite.

**Rationale**: The broker cannot tell a stale replica from an operator rollback, and FR-005 requires rollback safety. The failure mode is an extra reconnection, never a token release.

**Alternatives considered**: a quorum or grace window (needs coordination infrastructure that ADR 038 rejected); ignoring values seen earlier (fails AS-1.5).

## R7 — API representation

**Decision (Proposed, API-001 and API-002)**:

- End-User API `1.2.0` → proposed `2.0.0` on existing routes. Extend the existing required `requires_reauthentication` field with identity unusability, preserving ADR 046's rejection and refreshability causes. `SessionStatus` already inherits it from `UserSessionSummary`. `is_expired` remains an equal deprecated alias; `access_token_expired` remains independent.
- `connectionStatus` keeps its closed two-value enum and reports `not_connected` for a session that requires reauthentication.
- Document the changed behavior of `/oauth2/authorize` (consent redirect), `/oauth2/token` (no token, `invalid_grant` with `error_uri` despite an unexpired token), the third-party callback (a stale authorization is rejected), and grant submission (a superseded session does not satisfy a mandatory service).
- Admin API `2.1.1` → proposed major `3.0.0` cutover on existing routes. Sweep response classification changes from `refreshed` to `failed` for identity-invalid candidates, including dry runs; this is observable behavior, not description-only, even though schemas and status codes stay unchanged. `PUT /api/services/{service-id}` documents that a client-ID change invalidates sessions and that secret rotation does not. The sweep's `failed` count includes sessions with a superseded identity; a dry run classifies them without recording an observation. The internal values `client_identity`, `client_identity_revision`, and `upstream_client_revision` are not bound by any request type and are never returned. Request validation is unchanged.

**Rationale**: ADR 046 and OpenAPI 1.2.0 already define the fields. Identity invalidation and manual-refresh status changes alter existing behavior, not merely a broker-selected recovery-link target as in ADR 044. Major cutovers for both affected APIs follow Constitution Principles IV and X; no release evidence establishes an exemption for the current Admin baseline. Preserve the accepted alias instead of inventing a second expiry meaning.

**Alternatives considered**: new versioned endpoints (more surface area than a major cutover); a minor bump or unchanged version (does not meet the changed-behavior rule); a new `connectionStatus` value (unneeded enum change); a new sweep endpoint (duplicates the operation to preserve old classification); an Admin patch bump (fails IV/X because response totals change); explicitly rejecting the internal identity values as Admin request fields, as ADR 041 does for `credential_source_transitioned` (turns previously ignored input into a `400` without a security benefit, because no request type binds them).

## R8 — Persistence and migration

**Decision (Proposed, DEC-002)**: Use two atomic migrations, following the 040/041 and 044/045 pairs. `047_add_client_identity_revision` adds the columns with metadata-only defaults and `NOT VALID` non-negative CHECK constraints, under `ACCESS EXCLUSIVE ... NOWAIT` locks and a 1-second statement timeout. `048_validate_client_identity_revision` validates the CHECK constraints under `SHARE UPDATE EXCLUSIVE ... NOWAIT`. DOWN 047 refuses to run while any revision is greater than 0. Both stores implement the same semantics. A new focused port method records observations.

**Rationale**: Principle IX and ADR 004 apply. The guarded DOWN mirrors ADR 041's refusal to discard protective evidence.

**Alternatives considered**: a separate history table (more storage, no added guarantee); storing the revision only in memory (fails restart survival).

## R9 — Frontend

**Decision**: The sessions page treats `requires_reauthentication` as "Sign-in required" with the existing Login action (`sessionNeedsLogin`). Consent needs no new state because `not_connected` already renders the sign-in action. Both use existing design-system status variants and tokens. Wireframes in the spec remain behavioral sketches.

**Alternatives considered**: a new consent card state for "connection changed" (needs the enum change rejected in R7).

## R10 — Observability

**Decision**: Add the `Info` audit event `session.oauth2.client_identity_changed` with `service_id`, `client_identity_revision`, and `channel`. Denied uses reuse `session.oauth2.credential_source_failed` with reason `identity_superseded` or `identity_mismatch`. No client ID value, path, token, or principal appears in any metric. No new metric is required. The existing refresh and sweep failure counters already classify failures.

**Rationale**: SR-004 and `coding-standards/LOGGING.md` §2.2 and §6 apply.

## R11 — Rollout and rollback enforcement

**Decision (Proposed, DEC-005)**: Use ADR 047's continuous rolling procedure. Freeze effective client-ID/source/canonical-ID/binding mutations, including identity-mutating automation and DCR registrations, while existing traffic continues. Same-ID secret rotation and metadata-only writes remain available. Apply additive migrations online, then allow old and new replicas to serve simultaneously under unchanged identities; every new replica enforces revisions immediately. Drain each old process and its admitted work while healthy new replicas serve. Resume identity mutations only after verifying all serving/writing replicas/jobs are revision-aware and no old image can restart (operational activation, not a feature switch). New replicas reject legacy callback state before exchange; users may need to restart in-flight sign-in, not reconnect unchanged stored sessions. A failed upgrade leaves healthy traffic and the freeze intact. A pre-activation prior-binary abort requires verified unchanged identities, zero revision increments, drained new work, and retained schema/evidence. Post-activation rollback is revision-aware only, even at revision 0.

**Rationale**: Unrestricted identity changes during mixed-version serving are unsafe, but overlap under fixed identities does not require an outage. Existing explicit-column reads and updates that leave unknown columns untouched, plus revision-0 defaults, support online expansion; prove compatibility with the actual prior release in the AS-1.6 PostgreSQL variant. The operator's freeze covers filesystem publication and old/direct writers that a new binary cannot fence. The first permitted identity mutation occurs only after unaware readers and admitted work are gone. Failed freeze verification is incident containment, not permission to activate. DEC-005 registers the temporary identity-mutation/new-registration restriction and constrained rollback, not a security exception.

**Main alternative**: a preparatory compatibility release with database write fencing and serving admission gates. It adds coordination machinery and still needs filesystem controls. A stop-the-world cutover is rejected by the maintainer; neither schema retention nor revision 0 alone authorizes unaware binaries after activation.

## Resolved Technical Context unknowns

None remain open. Performance: one additional indexed service read per token exchange, plus one bounded file read for filesystem services. Scale: invalidation is O(1) per change, with no bulk write.
