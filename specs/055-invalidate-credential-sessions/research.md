# Research: Invalidate Sessions After Third-Party Credential Changes

**Feature**: 055-invalidate-credential-sessions | **Date**: 2026-11-03 | **Plan**: [plan.md](plan.md)

Each entry records the decision, why it was chosen, and the main alternatives. A decision that needs maintainer sign-off has an approval-item ID that refers to `## Approval Items` in [plan.md](plan.md). Nothing in this document is approved.

## R1. Invalidation mechanism: expire retained session rows

**Decision**: An invalidation updates every `user_sessions` row of the service in the transaction that commits the credential change. It sets `access_token_expires_at` to the change instant and sets `encrypted_refresh_token` and `refresh_token_expires_at` to NULL. It also replaces `callback_id` with a new random UUID. Rows are not deleted. (Approval item D-INVALIDATE.)

**Rationale**:

- Every existing reader already decides usability from these columns:
  - `UserSession.HasValidAccessToken`, `CanRefresh`, and `IsExpired`
  - `storage.SessionIsUsable`
  - `ListActiveByPrincipal` and `ListActiveServiceIDsByPrincipal`
  - The consent gates in `internal/domain/oauth2/service.go`: `anyDelegatedSessionUnusable` and `validateMandatoryRequirements`
  - The ADR 038 sweep and background refresh
  - `CountSessionsByExpiry`
  - The session listing (`is_expired`)

  No reader needs new logic, so FR-003's requirement that all paths agree is met by construction.
- The pattern already exists. A provider `invalid_grant` refresh records the session as expired instead of deleting it (`refreshLockedSession`, ADR 044).
- The sessions page keeps showing the service with its existing "Expired / Log in to reconnect" action. That action is the deliberate retry that FR-009 and Journey 2 State C require after a cancelled login.
- Discarding the refresh-token ciphertext removes the only credential that could renew the old connection. The access-token ciphertext stays because `UserSession.Validate` requires it. It is never decrypted after its expiry.
- Rotating `callback_id` fences every in-flight reconnection that sealed the old value (see R5).

**Alternatives considered**:

- *Deleting sessions*. Consent gating would still work. The sessions page would forget the service, so a cancelled login would have no retry action. Affected-agent counts would also disappear. Rejected.
- *A new `credential_generation` column on each session, compared at every read*. Every reader would need a join or a second read. Missing one path would violate FR-003. Rejected.

## R2. Credential generation in a separate internal table

**Decision**: Add a new table, `thirdparty_credential_states`, with these columns. Rows are created lazily. A missing row means generation 1 with no fingerprint/revision; existing unbaselined filesystem sessions are conservatively invalidated on initialization (D-INITIALIZE). (Approval item D-SCHEMA.)

| Column | Type | Constraints |
|---|---|---|
| `service_id` | `UUID` | Primary key; foreign key to `thirdparty_oauth2_services` with `ON DELETE CASCADE` |
| `credential_generation` | `BIGINT NOT NULL` | `CHECK > 0` |
| `credential_fingerprint` | `TEXT NULL` | Argon2id PHC prefix check |
| `publication_revision` | `BIGINT NULL` | Positive revision high-water mark, retained across source transitions |
| `recognized_at` | `TIMESTAMPTZ NULL` | — |
| `updated_at` | `TIMESTAMPTZ NOT NULL` | — |

**Rationale**:

- The administrative ETag is `strongETag(entity.Version)`. ADR 041 requires metadata operations and service records to be independent of files. If the generation were a service column, a filesystem recognition would change `version` and `updated_at`. That would break operators' `If-Match` and the Feature 051 "service unchanged" assertions in `tests/e2e/oauth2_secret_file_rotation_test.go`. A separate row changes neither.
- Lazy rows avoid a backfill and any lock on `thirdparty_oauth2_services` beyond the foreign-key `SHARE ROW EXCLUSIVE` lock that `CREATE TABLE ... REFERENCES` takes. That lock is brief because the table is new and empty. It follows ADR 041's `NOWAIT` and one-second `statement_timeout` pattern. The migration is atomic, and no `CONCURRENTLY` exception is needed.
- The table has its own ISP repository interface, `ThirdpartyCredentialStateRepository`, which follows ADR 004. Both stores implement it.

**Alternatives considered**: Columns on `thirdparty_oauth2_services` (rejected; ETag churn). An eager backfill (rejected; it is not needed, and it would lock the table).

## R3. Change detection without persisting credentials

**Decision**:

- **Stored → stored**: The update path compares the candidate client ID with the persisted client ID. It decrypts the persisted secret in memory through the existing `decryptSecret` path and compares it with the candidate plaintext using `crypto/subtle.ConstantTimeCompare`. If decryption fails, the update counts as a change. Public and CIMD services compare only the client ID and the presence of a secret.
- **Filesystem**: The comparison uses a durable **credential fingerprint**. A fingerprint is an Argon2id PHC string over `len(client_id) || client_id || secret`. A process-local HMAC-SHA256 cache under a random per-process key avoids an Argon2id computation on every request. (Approval item D-FINGERPRINT.)
- **Transitions**: A stored-to-filesystem update writes the fingerprint of the decrypted persisted pair and does not increment the generation. A filesystem-to-stored update calls Argon2id `Compare` on the supplied pair against the fingerprint. If the pair differs or no fingerprint exists, the update increments the generation. The fingerprint is then cleared but publication revision high-water is retained; re-entering filesystem requires a higher revision.

**Rationale**:

- ADR 041 forbids persisting file secrets or last-known credentials. A salted, memory-hard one-way hash is not recoverable. The repository already accepts this for broker-issued client secrets (`oauth2server.Argon2Hasher`, `client_credentials.secret_hash`).
- Without durable comparison evidence, a replica or a restarted broker cannot tell a secret-only rotation from an unchanged pair. That would violate FR-005 and Scenario 1.6.
- Argon2id is expensive (64 MiB, t=3) on purpose. The per-process HMAC cache keeps steady-state cost to three coherent file reads and one HMAC computation. The cache entry is keyed by (service, generation, publication revision), so another replica's recognition makes it miss.
- The library-first rule (Principle III) is met by `golang.org/x/crypto/argon2` (through the existing hasher), `crypto/hmac`, `crypto/sha256`, `crypto/rand`, and `crypto/subtle`.

**Alternatives considered**:

- *Persisting the encrypted pair*. This keeps a recoverable credential. Rejected.
- *A plain SHA-256 hash*. Low-entropy secrets would be vulnerable to offline guessing. Rejected.
- *An HMAC under a configured broker key*. This needs new key material and configuration, and rotating that key would cause mass invalidation. Rejected.
- *A symlink name or modification time*. Neither proves cross-replica freshness. Rejected. The Proposed explicit operator revision narrowly supersedes ADR 041's generation-identifier prohibition.

## R4. Currency and publication freshness (C3)

**Proposed D-CURRENCY, D-PUBLICATION, API-CONFIG**: Before initiation, callback exchange, every token/refresh decision and included/mandatory browser authorization admission, establish filesystem currency using one coherent revision/client-ID/secret publication. [ADR section 3](adr-draft-credential-change-session-invalidation.md#3-filesystem-currency-c3) is authoritative for revision format, high-water mark, source transitions and bounded CAS handling. The configuration contract is [contracts/configuration.md](contracts/configuration.md).

Read a consistent provider/service-state snapshot before acquisition and state again afterward. Changed generation or revision aborts the operation; never relabel acquired material. Lower revisions and equal-revision pair mismatches fail as source errors, with no mutation, recovery URI or provider call. CAS uses both expected generation and publication revision; a loser aborts after one attempt. A later request reacquires. Cache keys include service, generation and publication revision; freshness precedes any cache hit. A higher revision with unchanged pair advances evidence only.

**Proposed D-INITIALIZE**: For an unbaselined filesystem service, initialize under service write/session locks. If any session exists, advance generation and expire all rows atomically. With no sessions, record evidence without incrementing generation. A-established legacy sessions followed by first acquisition of B therefore never supply A tokens. Source errors write nothing. A stored-to-filesystem transition fingerprint is known evidence, not this legacy case.

Stored services also bind provider/source/pair/generation from a consistent service/state snapshot (service share-lock snapshot). This adds no file read but does add state/guard work. All file/hash work occurs before session locking. Metadata-only reads stay independent of files.

**Rationale and alternatives**: Revisions distinguish delayed A/41 from intentional A/43 restoration after B/42. CAS alone cannot. Coordinated all-replica publication/drain was rejected in favor of independent stale-replica rejection, at the cost of a required file. Legacy grandfathering was rejected because it cannot prove the establishing pair and could leak an unexpired A token after first observing B. Conservative one-time reconnect needs no migration backfill.

**Cost**: Three file opens, state reads before/after acquisition, HMAC, and a final guarded read. Cache misses use existing Argon2id parameters with at most two computations. File/revision unavailability fails even unexpired-token supply. No polling or new dependency.

## R5. Fencing callbacks, refresh admission and token release

**Proposed design under ADR-045/D-CURRENCY/D-INVALIDATE**:

- Recognition and legacy initialization take service FOR UPDATE, CAS expected generation/revision, then invalidate session rows atomically. Stored Update takes the same service write lock. State-only publication advances also take it. Memory uses issuerSessionGate for writing before per-session locks.
- First inserts keep service FOR SHARE with an expected-generation condition. Callback exchange compares sealed generation before provider authentication. Staleness found then makes zero provider requests. Rotation during an admitted exchange cannot recall that call, but insert/reconnect persistence is generation-fenced and returns callback_failed; no usable obsolete connection commits. Reconnect retains CallbackID competing-callback rules.
- Add WithLockedSessionAtCredentialGeneration to the existing focused session refresh repository (contracts/ports.md). Acquire service FOR SHARE before session FOR UPDATE, read current state, compare the acquired CredentialObservation (generation, source and optional revision), then invoke the existing update-only callback. No service lock is acquired after a session lock. Memory takes issuerSessionGate.RLock before the per-session lock; map mutexes are held only for snapshots/writes, never during provider calls.
- Refresh for every source uses that guard. Old acquisition → rotation commit → fresh B reconnection → delayed refresh admission rejects the generation before provider authentication against B tokens. An already admitted A refresh finishes while holding service/session locks; invalidation then commits and expires its result. Preserve ADR 038 bounded contexts, retries and singleflight.
- **Authoritative token-release linearization point**: each caller enters the guard with its acquired CredentialObservation. Reject changed generation/source/revision, including evidence-only revision advances and same-pair source transitions. Under both locks it validates the latest row's access validity, required scopes and established identity, decrypts that row's token, and completes a successful release decision before unlocking. That protected decision authorizes one response snapshot. Copies obtained beforehand and shared refresh results are not authorizations. Obsolete-generation callers fail recoverably without refreshing; a later fresh request may supply the new connection.
- Each singleflight joiner independently executes that decision with its own acquisition generation; never return the leader's token snapshot directly. Final validation applies to unexpired, no-expiry, forced/on-demand-refreshed and background-related supply alike. Pure background/sweep refresh has admission/persistence fencing, not a token response.
- The service write lock orders invalidation with admission/release guards. Commit first means old release is rejected. Release authorization first permits later network delivery of that snapshot, not revocation of authorized/delivered tokens. Stored token decisions now also have guard/state overhead.

**Deterministic tests (both stores; no sleeps)**:

1. Gate A/41 acquisition, recognize B/42 on another replica, establish a B session, resume A: state revalidation or CAS rejects it; generation/fingerprint/revision and B session remain unchanged. A cold stale replica serving A/41 also fails against B/42. Intentional A/43 produces one new change without reviving retired sessions.
2. Two B/42 observations racing CAS: one invalidation/increment, loser fails closed, fresh retry converges without increment. Same-revision/different-pair and lower revision fail without writes.
3. Acquire old stored/filesystem provider, commit rotation, complete fresh login, release delayed refresh-admission gate: zero obsolete provider calls and no fresh-session write. Also cover admitted refresh → invalidation waits → result expired.
4. Gate before final release guard, commit invalidation, resume: no old token. Reverse order: authorize/decrypt under guard, pause network delivery, commit invalidation, then deliver only the preauthorized snapshot. Test every singleflight joiner independently with both orderings.
5. Gate callback before exchange versus inside exchange: zero provider calls in the first, admitted call allowed but stale persistence rejected in the second; both callback_failed. Cover insert and reconnect.
6. A higher revision with identical pair or same-pair source transition between acquisition and final guard rejects the old observation without invalidating fresh sessions; a new request reacquires the correct source/revision.
7. Existing A sessions without evidence → first B acquisition: expire all atomically and require fresh login. Empty service initializes without increment. Restart cannot restore expired rows.

**Alternatives**: Unguarded rereads lack atomic ordering; session-only locking cannot bind the acquired provider to service generation. Serializable retries are unnecessary with service→session ordering. No new public error is needed.

## R6. Consent and recovery entry points (C2)

**Decision**: The browser authorization flow needs no code change beyond the currency check in R4. Step 4 (`anyDelegatedSessionUnusable`) and Step 5 (`validateMandatoryRequirements`) of `HandleAuthorization` already redirect a user with an active grant to `/agents/{agent-id}?session_token=...` when an included or mandatory session is missing or expired. This happens in every mode, because the decision precedes `ProceedHandler`. The consent page (`AgentGrantDetailPage`) lists such services under "Connect Your Accounts" with **Login** and keeps **Save** disabled until they are connected. `GrantConsent` returns the existing grant unchanged when the submitted selection matches it, so reconnecting does not extend or broaden the delegation. Protocol recovery remains the ADR 044 `error_uri`. (Approval item D-CONSENT.)

**Rationale**: This reuses existing, tested flows (`TestService_HandleAuthorization_SessionExpiry` in `internal/domain/oauth2/service_test.go`, `tests/e2e/frontend/selection_preservation_test.go`) without changing the UI or the API shape.

**Spec reconciliation**: The State A/B/C wireframes now show the existing consent and sessions UI: "Connect Your Accounts", **Login**, **Save**, and the sessions-page "Expired" card with "Log in to reconnect". They no longer show a new reconnect banner. This is recorded in the spec's Clarifications section.

**Alternatives considered**: A new "connection changed" banner. It would need a retained invalidation reason in the API, which is an extra API change that the issue does not require. Rejected.

## R7. API contracts and versions

**Decision**: Response shapes and routes are unchanged. Only documented behavior changes.

- **Admin API 2.1.0 → 3.0.0** (approval item API-ADMIN): `PUT /api/services/{service-id}` documents the invalidation of all user sessions on an effective credential change, its atomicity, and the rule that unchanged values do not invalidate.
- **End-User API 1.1.0 → 2.0.0** (approval item API-ENDUSER): The following behavior is documented:
  - `/oauth2/authorize` redirects to consent for an unusable included or mandatory connection, including an invalidated one, and returns a filesystem source failure as `server_error`.
  - Third-party initiation requires both credential files for a filesystem service.
  - A callback from an authorization that began before a credential change returns `callback_failed`.
  - RFC 8693 `/oauth2/token` returns `400 invalid_grant` with the ADR 044 `error_uri` for invalidated sessions. For filesystem services, an unexpired-token supply can return the existing non-recoverable infrastructure failure.
  - The session list reports invalidated sessions with `is_expired: true` and `has_refresh_token: false`.
  - The session refresh endpoint returns its existing `409` for them.

**Rationale**: Release `v0.1.133` contains Admin API `2.1.0` and End-User API `1.1.0`. Invalidation and filesystem availability coupling change their released behavior even though response shapes remain unchanged. Constitution Principles IV and X require a major version bump or a new endpoint for released behavior changes. Propose Admin API `3.0.0` and End-User API `2.0.0` on the existing routes, with cutover documentation and no compatibility shim. The maintainer must approve API-ADMIN and API-ENDUSER before implementation.

**Alternatives considered**:

- *No version change or a minor bump*. Rejected: neither satisfies the released-behavior rule. ADR 044's approved action-target treatment does not authorize this lifecycle change.
- *New versioned endpoints*. Rejected: duplicating the existing service, authorization, and session flows would add unnecessary contract surface.

## R8. Logging, telemetry, and errors

**Decision**: Four credential-free audit events (see the [ADR draft](adr-draft-credential-change-session-invalidation.md#6-audit)). Recognition and stored updates run inside existing spans (`thirdparty` update, session operations). This plan adds no new metric or span name. No new `oauth2session` error detail is exposed in token-exchange diagnostics. After a recognition, the triggering request reads an expired session and reports the existing `session_missing`, `refresh_unavailable`, or `access_token_expired` recoverable details. A failed recognition commit reports the existing `persistence_failed` or `repository_unavailable` details.

**Rationale**: `coding-standards/LOGGING.md` §2.2 treats event identifiers as contracts. Adding events is additive. Reusing the diagnostic details avoids a dashboard contract change.

## R9. Feature 051 acceptance scenarios affected

**Decision**: The ADR supersession amends these Feature 051 E2E scenarios in `tests/e2e/`. They are amended with traceable comments that cite this feature. They are not deleted. (Approval item ADR-045.)

| Scenario | File | Change |
|---|---|---|
| US2-S1 | `oauth2_secret_file_rotation_test.go` | Same-identity secret rotation now invalidates the existing session. The new user's connection and refresh still use the rotated secret. |
| US2-S5 | `oauth2_secret_file_rotation_test.go` | The paused source-A callback still fails with `callback_failed`. The refresh now returns the expired-session `409`, and the session is expired. A new B connection succeeds. |
| US5-S3 | `oauth2_secret_file_boundaries_test.go` | Initiation now requires both files. Consent and session metadata stay available during the outage. |

Other Feature 051 scenarios keep their expectations. The `expectServiceUnchanged` service-record equality also still holds, because the generation lives in a separate table (R2).

## R10. Configuration, Helm, and frontend scope

**Decision**:

- Add required `credential_revision_file` to filesystem bindings through the existing configuration port/loader/validator; no default/fallback (API-CONFIG, D-PUBLICATION).
- Update chart credentialFiles documentation/example values and test ConfigMap rendering through its existing mapping. No new top-level chart parameter. Document monotonic publication, cutover and rollback/drain.
- Make no React change. Playwright tests and screenshots cover the existing consent and sessions UI for Journey 2.

**Rationale**: Clarifications revise the initial no-new-key constraint because independent freshness requires operator evidence. Principle VII requires loader, examples and chart parity. No UI change or new dependency.
