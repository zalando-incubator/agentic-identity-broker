# ADR 047: Client Identity Revisions Invalidate Third-Party Sessions

**Status**: Proposed
**Date**: 2026-11-05
**Feature**: 055-invalidate-client-sessions
**Supersedes**: In part, [ADR 041](../../adrs/041-oauth2-client-secret-file-overrides.md) (scope and timing of client-identity enforcement, the legacy identity exception, the no-file-read rule for consent/session usability displays, and rollout/rollback procedure) and [ADR 038](../../adrs/038-proactive-token-refresh.md) (unconditional immediate return of a valid access token). All other decisions of both ADRs remain binding. [ADR 046](../../adrs/046-observed-refresh-token-rejection.md) remains binding in full: identity invalidation extends its reauthentication predicate without weakening observed-rejection handling.

> Feature-local draft. Implementation records the maintainer's approval and moves it to `adrs/047-client-identity-revisions.md` only after sign-off. `main` already owns ADRs 045 (agent-subject impersonation) and 046 (observed refresh-token rejection), so this draft uses the next unused number, 047; if `main` assigns 047 before the move, use the next unused number and update every reference.

## Context

A third-party service's provider-facing client ID can change in two ways. An operator replaces a stored service's `client_id` through the Admin API (`PUT /api/services/{service-id}`), or publishes a new filesystem credential pair whose client-ID file holds a different value. Both are legitimate. Secret rotation under the same client ID is routine and frequent.

ADR 041 records the initiating client identity (`UpstreamClientID`) in the authorization state and the session. It compares that identity before code exchange and refresh. Three gaps remain:

1. ADR 038 returns a still-valid access token immediately unless ADR 046's observed-rejection rule blocks it, but token exchange does not check client identity. A previous-identity token stays available until it expires.
2. Only stored or filesystem shared-secret services are eligible. Public, CIMD, DCR, and Google-flavor services record no identity, so a change of their client ID is not detected.
3. A session compares only its recorded client ID with the current one. When an operator restores the previous client ID, an invalidated session becomes usable again. Identity-less legacy sessions of never-transitioned stored services stay usable after any later change.

Feature 055 requires invalidation on the next use for every affected principal, survival across restarts and rollbacks, and no invalidation from secret rotation (spec FR-001–FR-005, FR-012, FR-013).

## Decision

### Client Identity Revision

Each service owns two internal, non-secret values. Neither is ever an administrative input or output.

- `client_identity` is the last effective client ID that the broker recorded for the service. It is null until the first record.
- `client_identity_revision` is a non-negative counter that starts at 0. It only increases.

The effective client ID is the stored `client_id` for `credential_source: stored`, including static, public, CIMD, DCR, and Google-flavor services. It is the trimmed client-ID file value for `credential_source: filesystem`. A CIMD identifier is derived from the configured public URL and the service ID. It changes only when the stored value changes, so no mutable identity is invented for it.

A **client identity change** is recorded when a newly effective client ID differs from the previous effective one. Recording atomically sets `client_identity` to the new value and increments the revision by exactly 1.

| Channel | When it is recorded | Previous effective identity |
|---|---|---|
| Admin API update to a stored source | In the same atomic service update that commits the new `client_id`. A rejected, invalid, or version-conflicting update records nothing. | `client_identity`, or, while it is null, the persisted stored `client_id` |
| Filesystem publication | When any broker operation reads the client-ID file: authorization initiation, code exchange, token acquisition, refresh, session sweep, agent authorization, consent, or session listing. A compare-and-set atomically requires filesystem source, the read administrative version and canonical ID, and the read identity revision, including first and same-ID observations. This prevents double increments and obsolete observations across administrative changes. | `client_identity`. A null value is set without an increment. |
| Session sweep dry run | Never recorded. A dry run compares without writing, consistent with its no-write contract. | — |
| Update to a filesystem source | Not recorded at update time, because administrative service-metadata operations never read files (ADR 041). The first read after the update applies the filesystem rule. | — |

Same-ID secret rotation, metadata-only updates, and a source transition that keeps the effective client ID record nothing. Observation never changes the service `version`, ETag, or `updated_at`. The existing administrative version fences source/canonical-ID changes and round trips. On a lost observation precondition the domain discards the old value, reloads the whole service/source/version/canonical ID/revision, reselects its binding from the immutable injected configuration, and reads again if still filesystem. It never replays an obsolete file value against a stored source or a new binding. A version change alone never invalidates a session.

### Session establishment and usability

Every new authorization flow, for every service variant, seals the effective client ID and the current revision in its existing JWE state (`upstream_client_id`, `upstream_client_revision`). A successful code exchange copies both into the session. Refresh and source transitions never rewrite them.

A session is **usable for its identity** only when the broker has resolved and recorded the current effective identity `E` at revision `R`, and one of these conditions is true:

- The session has a recorded identity equal to `E` and a recorded revision equal to `R`.
- The session has no recorded identity (legacy), the service is stored, has never transitioned its credential source (ADR 041), and `R` is 0, so no change has been recorded since rollout.

Otherwise the session needs reauthentication. Identity usability does not override ADR 046: a recorded rejection still blocks issuance with an unexpired access token, while access expiry with a stored, not-rejected refresh token remains refreshable if identity is usable. The complete predicate is ADR 046's `RequiresReauthentication` OR identity unusability. A session established before rollout has revision 0. A session that ADR 041 already accepts therefore stays usable until the first change after rollout. No historical identity is inferred or backfilled.

Because usability is evaluated against the persisted revision on each use, no bulk session write is needed. Restoring a previous client ID records a new revision. It cannot revive an older session.

### Enforcement points

- **Token acquisition** (`GetValidAccessToken`, used by RFC 8693 exchange) checks identity before it returns any stored token, including a still-valid access token and a token with unknown expiry. This narrows ADR 038's immediate return. A valid token still returns immediately after the identity check, without waiting for a refresh.
- **Refresh** (on-demand, background, manual, and sweep) preserves ADR 046's exclusion of rejected sessions and checks identity and revision under the existing session lock before provider authentication. A failed check never submits the refresh token and never changes the stored session.
- **Callback** compares the sealed identity and revision with the current ones before code exchange. A stale authorization fails with the existing credential-source outcome, and the user can start a new one. Both sealed claims must be present: a missing revision is not revision 0. Pre-upgrade callbacks are rejected with `identity_missing` before provider exchange and must restart, including service variants that previously sealed no identity. The legacy session exception does not apply to callback state. The existing callback-ID compare-and-set still decides between competing reconnects.
- **Agent authorization** (`/oauth2/authorize`) extends the existing ADR 046 unusability rule with identity invalidation for each service included in the grant. It redirects to agent consent.
- **Consent and session management** report such a session as not connected and as requiring reauthentication. Consent detail and session list/detail/refresh summaries resolve the live client-ID file, never the secret file, before falling back to persisted evidence on source failure. This partially supersedes ADR 041's no-file-read boundary for these usability displays and the corresponding boundary in `ARCHITECTURE.md`. Administrative service create/update/get/list metadata operations still never read files. Agent authorization and grant validation are gating operations and fail closed, not display fallbacks. An explicit session sweep is not a service-metadata operation.

### Failure semantics

- A session that needs reauthentication produces the existing recoverable outcome: `400 invalid_grant` with ADR 044's service-specific `error_uri`. Its internal diagnostic is `credential_identity_changed`. The broker uses a new credential-source reason, `identity_superseded`, for a revision mismatch.
- If the client-ID file cannot be read, or the service state cannot be loaded, token release fails closed with the existing infrastructure outcome (`500 server_error`, no `error_uri`), even if the access token is still valid. Agent authorization returns its existing server error. Display surfaces (session listing, session detail, and consent detail) fall back to the persisted revision evidence and log the source failure. They never release a token.
- Each recorded change emits one credential-free `Info` audit event, `session.oauth2.client_identity_changed`, with `service_id`, `client_identity_revision`, and `channel` (`admin_update` or `filesystem_observation`). The event never contains the client ID value. Each denied use logs the existing source-failure event with reason `identity_superseded` or `identity_mismatch`.

### Persistence and migration

Both stores persist the service values and the session revision with the same semantics. Migration `047_add_client_identity_revision` atomically adds these columns with metadata-only defaults and `NOT VALID` CHECK constraints, using the ADR 041 lock discipline (`ACCESS EXCLUSIVE ... NOWAIT`, 1-second statement timeout). Migration `048_validate_client_identity_revision` validates the constraints in a separate transaction under `SHARE UPDATE EXCLUSIVE ... NOWAIT`:

- `thirdparty_oauth2_services.client_identity TEXT NULL`
- `thirdparty_oauth2_services.client_identity_revision BIGINT NOT NULL DEFAULT 0`, with a non-negative CHECK
- `user_sessions.upstream_client_revision BIGINT NOT NULL DEFAULT 0`, with a non-negative CHECK

No backfill runs. DOWN 048 restores the unvalidated constraints. DOWN 047 refuses to run while any service has a revision greater than 0, because dropping the evidence would revive invalidated sessions. Otherwise it drops the three columns. Brokers older than this feature ignore the columns, so retaining the schema alone cannot enforce the guarantee. The following staged rolling procedure is mandatory. It retains ADR 041's online additive-migration pattern but constrains identity mutations during overlap and replaces its older-binary post-activation rollback procedure.

### Guarded rollout and rollback (Proposed DEC-005)

Use a continuous rolling upgrade with fixed identities during mixed-version serving. No maintenance outage or global ingress closure is required. Activation means the operator has verified a fully revision-aware fleet and may resume identity mutations; it is not a runtime flag. Every new replica enforces revisions from startup.

1. Before migration, record and hold effective client IDs, credential sources, canonical IDs, and immutable binding configuration fixed across replicas. Use existing operator publication/change controls to pause identity-mutating Admin automation, discovery/DCR re-registration, and direct service writers; defer new registrations that assign identities. Keep end-user/Admin ingress, existing token acquisition, refresh, browser authorization, and session access open. Same-ID secret rotation and metadata-only writes may continue only when they cannot mutate the frozen fields. Ensure old-image restart controllers remain under operator control through activation.
2. Apply and validate additive migrations 047/048 online before starting new replicas. Verify that every supported prior binary uses explicit query columns, preserves unknown columns on updates, and creates legacy rows with revision default 0. Existing repositories have those shapes; prove them against the expanded schema rather than relying on column defaults alone. Lock contention aborts migration/deployment instead of queuing an exclusive lock behind traffic. Leave healthy replicas serving and the freeze intact on failure; never force past failed validation or automate dirty-state recovery.
3. Replace replicas incrementally with revision-aware images using the same effective IDs and bindings. Old and new processes may share traffic while identity is fixed. Initialization records the unchanged identity without incrementing revision. Connections created by old binaries retain revision 0; new binaries apply the existing legacy exception only where allowed. New binaries reject callback state lacking explicit identity/revision before exchange, so a browser crossing versions may need to restart sign-in; state claims are additive and an old callback handler can consume new state only during this unchanged-identity interval. Never use that compatibility to permit an unaware handler after activation.
4. Keep healthy new replicas serving while withdrawing each old replica from end-user/Admin/direct traffic and draining its admitted requests, refreshes, sweeps, callbacks, and service writes under the existing shutdown discipline. Verify the full serving/writing replica, worker, and job inventory, including direct callers and DCR writers, is revision-aware, old admitted work has finished, and no controller/rollback target can restart an unaware image. Only then resume identity changes. A failed drain or inventory check leaves traffic on healthy replicas and the freeze in force; it does not authorize activation.
5. Before activation, an abort to the prior binary is allowed only after verifying the frozen fields never changed and every service revision remains 0, draining new admitted work while healthy replicas serve, and retaining schema/evidence. Zero revisions alone are insufficient, since an old writer or file change could have escaped recording. After activation, roll only between revision-aware releases with schema/evidence retained, even if revisions happen to remain 0. DOWN's guard is not a rollback authorization. If freeze integrity is lost, do not claim a safe old-binary abort: isolate unaware readers/writers and restore a revision-aware fleet before changes resume. An unsafe deployment can require incident containment; this does not make an outage the normal rollout procedure.

**Rationale**: An old reader can release tokens and an old writer can change `client_id` without revision recording, so unrestricted identity changes during overlap are unsafe. Keeping identity fixed allows existing traffic to continue while the additive schema and revision-aware fleet are introduced. A verified drain of only old work, with new replicas already serving, establishes the boundary before the first permitted identity mutation. This trades a temporary restriction on identity mutations/new registrations and constrained post-activation rollback for availability, without an enforcement opt-out, new persisted coordination state, dependency, or Helm setting. **Main alternative**: a preparatory compatibility release with automated serving admission and database write fencing; it adds coordination machinery and still needs equivalent filesystem publication controls. A stop-the-world maintenance cutover is rejected by maintainer direction. DEC-005 and revised ADR-001 remain Proposed; this procedure grants no security exception.

Regression coverage under AS-1.6 verifies online schema expansion, simultaneous old/new traffic at unchanged identity, legacy and newly established sessions, refreshes and same-ID rotation, incremental draining, failed-upgrade/pre-activation abort, and invalidation only after all unaware work is removed. Use the actual supported prior release for the PostgreSQL mixed-version variant; do not simulate compatibility by disabling enforcement in new code. AS-1.4 rejects pre-upgrade callback fixtures, and AS-1.5 proves revision-aware restart/rollback never revives an invalidated session. Deployment review must verify the mutation freeze, completed old-work drains, pinned image inventory, and prevention of old-image restart before activation. Tests prove the sequence under those controls, not safety of arbitrary mixed-version identity changes. Document the controls in the chart/operator runbook, not a new workflow or hook.

### Multi-replica observation

Replicas can briefly read different client-ID file values while a publication propagates. Each differing value is recorded as a change, including a stale value after a newer one. A user who reconnects during an incomplete propagation can therefore need to reconnect again. This fails closed. Operators publish the complete new pair coherently, as ADR 041 requires, and let it reach every replica before they announce the change to users.

## Consequences

- Both change channels invalidate every affected session on its next use, without a broker scheduler, sweep, or bulk write. A rollback never revives a session.
- Routine secret rotation through files or the Admin API causes no reconnection.
- Every token exchange for a filesystem service reads the client-ID file, and every token exchange reads the service's identity state. The added cost is one small file read and one indexed read. A file outage now blocks the release of stored tokens for that service. Before this ADR, a valid token was still released.
- Public, CIMD, DCR, and Google-flavor services now enforce identity for new connections. Their identity-less legacy sessions keep the never-transitioned exception until the first recorded change.
- Users see identity invalidation through the existing `requires_reauthentication` field and its equal deprecated `is_expired` alias, with `not_connected` in agent consent. `access_token_expired` retains its distinct meaning. They reconnect through the existing service sign-in flow. The proposed API-001 major 1.2.0 → 2.0.0 cutover and manual-refresh behavior are detailed in [the End-User contract](contracts/enduser-api.md); ADR 046's `409 refresh_rejected` remains unchanged.
- First deployment keeps existing traffic flowing during mixed-version serving, but freezes identity/source/canonical-ID/binding mutations and defers identity-assigning registrations until all old processes and admitted work are drained. Same-ID rotation remains available. Legacy callback state reaching a new replica must restart sign-in. A verified unchanged, zero-revision pre-activation abort can use the prior binary; post-activation rollback is revision-aware only. The guarded DOWN protects evidence but does not replace the operator's freeze and inventory controls.
- Consent and session usability displays now read live client-ID files and can observe changes; administrative service metadata retains its no-file-read boundary. No secret file is read to render usability.

## Alternatives Considered

- **Compare only the client ID at use**: This is simple, but restoring a previous client ID revives invalidated sessions (FR-005), and legacy sessions cannot be judged. Rejected.
- **Delete or bulk-mark sessions on change**: This works for Admin API updates. Filesystem changes are discovered lazily, and a large bulk write inside an Admin API update holds locks. Users would also lose the session record that drives the sign-in prompt. Rejected.
- **Reject Admin API client-ID changes while sessions exist**, as for issuer changes: This blocks a legitimate operator action and does not cover filesystem changes. Rejected.
- **A file watcher or periodic poller**: This adds a scheduler and still needs use-time checks for correctness across replicas. Rejected.
- **Time-based evidence** (`changed_at` compared with the session establishment time): This depends on clock agreement across replicas and the database. Rejected in favor of a counter.

## References

- [Feature specification](spec.md), [plan](plan.md), [research](research.md), [data model](data-model.md)
- [ADR 041](../../adrs/041-oauth2-client-secret-file-overrides.md), [ADR 038](../../adrs/038-proactive-token-refresh.md), [ADR 044](../../adrs/044-service-specific-session-recovery-url.md)
