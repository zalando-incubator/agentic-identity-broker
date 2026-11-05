# ADR 047: Client Identity Revisions Invalidate Third-Party Sessions

**Status**: Proposed
**Date**: 2026-11-05
**Feature**: 055-invalidate-client-sessions
**Supersedes**: In part, [ADR 041](../../adrs/041-oauth2-client-secret-file-overrides.md) (scope and timing of client-identity enforcement, and the legacy identity exception) and [ADR 038](../../adrs/038-proactive-token-refresh.md) (unconditional immediate return of a valid access token). All other decisions of both ADRs remain binding. [ADR 046](../../adrs/046-observed-refresh-token-rejection.md) remains binding in full: identity invalidation extends its reauthentication predicate without weakening observed-rejection handling.

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
| Filesystem publication | When any broker operation reads the client-ID file: authorization initiation, code exchange, token acquisition, refresh, session sweep, agent authorization, consent, or session listing. A compare-and-set on the read revision prevents double increments across replicas. | `client_identity`. A null value is set without an increment. |
| Session sweep dry run | Never recorded. A dry run compares without writing, consistent with its no-write contract. | — |
| Update to a filesystem source | Not recorded at update time, because metadata operations never read files (ADR 041). The first read after the update applies the filesystem rule. | — |

Same-ID secret rotation, metadata-only updates, and a source transition that keeps the effective client ID record nothing. Observation never changes the service `version`, ETag, or `updated_at`.

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
- **Callback** compares the sealed identity and revision with the current ones before code exchange. A stale authorization fails with the existing credential-source outcome, and the user can start a new one. The existing callback-ID compare-and-set still decides between competing reconnects.
- **Agent authorization** (`/oauth2/authorize`) extends the existing ADR 046 unusability rule with identity invalidation for each service included in the grant. It redirects to agent consent.
- **Consent and session management** report such a session as not connected and as requiring reauthentication.

### Failure semantics

- A session that needs reauthentication produces the existing recoverable outcome: `400 invalid_grant` with ADR 044's service-specific `error_uri`. Its internal diagnostic is `credential_identity_changed`. The broker uses a new credential-source reason, `identity_superseded`, for a revision mismatch.
- If the client-ID file cannot be read, or the service state cannot be loaded, token release fails closed with the existing infrastructure outcome (`500 server_error`, no `error_uri`), even if the access token is still valid. Agent authorization returns its existing server error. Display surfaces (session listing, session detail, and consent detail) fall back to the persisted revision evidence and log the source failure. They never release a token.
- Each recorded change emits one credential-free `Info` audit event, `session.oauth2.client_identity_changed`, with `service_id`, `client_identity_revision`, and `channel` (`admin_update` or `filesystem_observation`). The event never contains the client ID value. Each denied use logs the existing source-failure event with reason `identity_superseded` or `identity_mismatch`.

### Persistence and migration

Both stores persist the service values and the session revision with the same semantics. Migration `047_add_client_identity_revision` atomically adds these columns with metadata-only defaults and `NOT VALID` CHECK constraints, using the ADR 041 lock discipline (`ACCESS EXCLUSIVE ... NOWAIT`, 1-second statement timeout). Migration `048_validate_client_identity_revision` validates the constraints in a separate transaction under `SHARE UPDATE EXCLUSIVE ... NOWAIT`:

- `thirdparty_oauth2_services.client_identity TEXT NULL`
- `thirdparty_oauth2_services.client_identity_revision BIGINT NOT NULL DEFAULT 0`, with a non-negative CHECK
- `user_sessions.upstream_client_revision BIGINT NOT NULL DEFAULT 0`, with a non-negative CHECK

No backfill runs. DOWN 048 restores the unvalidated constraints. DOWN 047 refuses to run while any service has a revision greater than 0, because dropping the evidence would revive invalidated sessions. Otherwise it drops the three columns. Brokers older than this feature ignore the columns. Before an application rollback, the operator must expect the older enforcement of ADRs 038 and 041.

### Multi-replica observation

Replicas can briefly read different client-ID file values while a publication propagates. Each differing value is recorded as a change, including a stale value after a newer one. A user who reconnects during an incomplete propagation can therefore need to reconnect again. This fails closed. Operators publish the complete new pair coherently, as ADR 041 requires, and let it reach every replica before they announce the change to users.

## Consequences

- Both change channels invalidate every affected session on its next use, without a broker scheduler, sweep, or bulk write. A rollback never revives a session.
- Routine secret rotation through files or the Admin API causes no reconnection.
- Every token exchange for a filesystem service reads the client-ID file, and every token exchange reads the service's identity state. The added cost is one small file read and one indexed read. A file outage now blocks the release of stored tokens for that service. Before this ADR, a valid token was still released.
- Public, CIMD, DCR, and Google-flavor services now enforce identity for new connections. Their identity-less legacy sessions keep the never-transitioned exception until the first recorded change.
- Users see identity invalidation through the existing `requires_reauthentication` field and its equal deprecated `is_expired` alias, with `not_connected` in agent consent. `access_token_expired` retains its distinct meaning. They reconnect through the existing service sign-in flow. The proposed API-001 major 1.2.0 → 2.0.0 cutover and manual-refresh behavior are detailed in [the End-User contract](contracts/enduser-api.md); ADR 046's `409 refresh_rejected` remains unchanged.
- Rollback to an older binary removes enforcement but keeps the evidence. The guarded DOWN migration protects that evidence.

## Alternatives Considered

- **Compare only the client ID at use**: This is simple, but restoring a previous client ID revives invalidated sessions (FR-005), and legacy sessions cannot be judged. Rejected.
- **Delete or bulk-mark sessions on change**: This works for Admin API updates. Filesystem changes are discovered lazily, and a large bulk write inside an Admin API update holds locks. Users would also lose the session record that drives the sign-in prompt. Rejected.
- **Reject Admin API client-ID changes while sessions exist**, as for issuer changes: This blocks a legitimate operator action and does not cover filesystem changes. Rejected.
- **A file watcher or periodic poller**: This adds a scheduler and still needs use-time checks for correctness across replicas. Rejected.
- **Time-based evidence** (`changed_at` compared with the session establishment time): This depends on clock agreement across replicas and the database. Rejected in favor of a counter.

## References

- [Feature specification](spec.md), [plan](plan.md), [research](research.md), [data model](data-model.md)
- [ADR 041](../../adrs/041-oauth2-client-secret-file-overrides.md), [ADR 038](../../adrs/038-proactive-token-refresh.md), [ADR 044](../../adrs/044-service-specific-session-recovery-url.md)
