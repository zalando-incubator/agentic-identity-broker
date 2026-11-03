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

**Decision**: Add a new table, `thirdparty_credential_states`, with these columns. Rows are created lazily. A missing row means generation 1 with no fingerprint. (Approval item D-SCHEMA.)

| Column | Type | Constraints |
|---|---|---|
| `service_id` | `UUID` | Primary key; foreign key to `thirdparty_oauth2_services` with `ON DELETE CASCADE` |
| `credential_generation` | `BIGINT NOT NULL` | `CHECK > 0` |
| `credential_fingerprint` | `TEXT NULL` | Argon2id PHC prefix check |
| `recognized_at` | `TIMESTAMPTZ NULL` | — |
| `updated_at` | `TIMESTAMPTZ NOT NULL` | — |

**Rationale**:

- The administrative ETag is `strongETag(entity.Version)`. ADR 041 requires metadata operations and service records to be independent of files. If the generation were a service column, a filesystem recognition would change `version` and `updated_at`. That would break operators' `If-Match` and the Feature 051 "service unchanged" assertions in `tests/e2e/oauth2_secret_file_rotation_test.go`. A separate row changes neither.
- Lazy rows avoid a backfill and any lock on `thirdparty_oauth2_services` beyond the foreign-key `SHARE ROW EXCLUSIVE` lock that `CREATE TABLE ... REFERENCES` takes. That lock is brief because the table is new and empty. It follows ADR 041's `NOWAIT` and short `lock_timeout` pattern. The migration is atomic, and no `CONCURRENTLY` exception is needed.
- The table has its own ISP repository interface, `ThirdpartyCredentialStateRepository`, which follows ADR 004. Both stores implement it.

**Alternatives considered**: Columns on `thirdparty_oauth2_services` (rejected; ETag churn). An eager backfill (rejected; it is not needed, and it would lock the table).

## R3. Change detection without persisting credentials

**Decision**:

- **Stored → stored**: The update path compares the candidate client ID with the persisted client ID. It decrypts the persisted secret in memory through the existing `decryptSecret` path and compares it with the candidate plaintext using `crypto/subtle.ConstantTimeCompare`. If decryption fails, the update counts as a change. Public and CIMD services compare only the client ID and the presence of a secret.
- **Filesystem**: The comparison uses a durable **credential fingerprint**. A fingerprint is an Argon2id PHC string over `len(client_id) || client_id || secret`. A process-local HMAC-SHA256 cache under a random per-process key avoids an Argon2id computation on every request. (Approval item D-FINGERPRINT.)
- **Transitions**: A stored-to-filesystem update writes the fingerprint of the decrypted persisted pair and does not increment the generation. A filesystem-to-stored update calls Argon2id `Compare` on the supplied pair against the fingerprint. If the pair differs or no fingerprint exists, the update increments the generation. The fingerprint is then cleared.

**Rationale**:

- ADR 041 forbids persisting file secrets or last-known credentials. A salted, memory-hard one-way hash is not recoverable. The repository already accepts this for broker-issued client secrets (`oauth2server.Argon2Hasher`, `client_credentials.secret_hash`).
- Without durable comparison evidence, a replica or a restarted broker cannot tell a secret-only rotation from an unchanged pair. That would violate FR-005 and Scenario 1.6.
- Argon2id is expensive (64 MiB, t=3) on purpose. The per-process HMAC cache keeps steady-state cost to two file reads and one HMAC computation. The cache entry is keyed by (service, generation), so another replica's recognition makes it miss.
- The library-first rule (Principle III) is met by `golang.org/x/crypto/argon2` (through the existing hasher), `crypto/hmac`, `crypto/sha256`, `crypto/rand`, and `crypto/subtle`.

**Alternatives considered**:

- *Persisting the encrypted pair*. This keeps a recoverable credential. Rejected.
- *A plain SHA-256 hash*. Low-entropy secrets would be vulnerable to offline guessing. Rejected.
- *An HMAC under a configured broker key*. This needs new key material and configuration, and rotating that key would cause mass invalidation. Rejected.
- *A filesystem `generation` symlink or modification time*. It is provider-specific, and ADR 041 forbids generation identifiers. Rejected.

## R4. When filesystem currency is established (C3)

**Decision**: `OAuth2SessionService` establishes credential currency for filesystem-sourced services at these points:

- `initiateOAuth2Flow`
- `HandleCallback`, before the code exchange
- `GetValidAccessToken`, before its fast path and its refresh path
- `refreshLockedSession`, before it takes the session lock. This covers on-demand, forced, background, and sweep refreshes.
- `HandleAuthorization` in `internal/domain/oauth2/service.go`, through a new `ports.CredentialCurrencyChecker` port. It checks every filesystem-sourced service that the grant includes or that the agent requires as mandatory, before session usability is evaluated.

The validated pair is passed to the operation that consumes it, so exchange and refresh still acquire the files once. Metadata-only reads never acquire files: consent detail, session listing, and administrative reads. (Approval items ADR-045 and D-CURRENCY.)

**Rationale**:

- The check must run before the session row lock. Otherwise the recognition transaction would wait for the caller's own lock.
- It must run before the unexpired-token fast path, or Scenario 1.6 fails.
- Including `/oauth2/authorize` admission meets FR-012's "before that operation admits authorization", so SC-003 holds in the filesystem variants.
- Stored services need no check at token use. Their invalidation is complete at commit.

**Cost**:

- Each filesystem token decision makes one primary-key read of the credential state, two file opens, and one HMAC computation.
- File unavailability now fails token supply even for unexpired tokens. This is the availability trade-off in Scenario 1.7.
- The Argon2id computation is limited to 2 concurrent runs per process. This is a fixed internal limit, not a configuration key.

**Alternatives considered**:

- *Initiation reads only the client ID, as ADR 041 does today*. A secret-only change would surface only at the callback fence and force a second login. Rejected.
- *Periodic polling*. This leaves an old-token window. Rejected.

## R5. Fencing concurrent callbacks, refreshes, and replicas

**Decision**:

- **Recognition**:
  1. Lock the service row with `SELECT ... FROM thirdparty_oauth2_services WHERE id=$1 FOR UPDATE`. The service record is not modified.
  2. Upsert the credential state with compare-and-set on the expected generation.
  3. Invalidate the sessions as described in R1.

  These steps run in one transaction.
- **Stored updates**: `PostgresThirdpartyOAuth2ProviderRepository.Update` already locks the row `FOR UPDATE`. Through a transient `CredentialChange` on the entity, it performs the same state and session statements in its transaction.
- **First-connection insert**: The existing `INSERT ... SELECT ... FOR SHARE OF service` in `user_session.go` adds the condition `COALESCE(state.credential_generation, 1) = $expected`. A failed probe reports the existing Conflict.
- **Reconnection**: `createSession`'s `WithLockedSession` callback already requires `current.CallbackID == claims.SessionCallbackID`. Invalidation rotates `callback_id`, so a stale reconnection updates nothing. When the callback updates nothing and the current session is unusable, the callback returns an error (`ErrCredentialGenerationChanged`), which surfaces as the existing `callback_failed` redirect. A competing callback that left a usable session keeps the existing behavior.
- **Before the provider exchange**: `HandleCallback` rejects the callback before any provider request when the sealed `credential_generation` differs from the current generation.
- **Refresh**: It holds the `FOR UPDATE` session row lock during the provider call. The invalidation's `UPDATE` waits for the refresh to finish and then overwrites its result.
- **In-memory store**: The provider repository takes `issuerSessionGate` for writing and acquires each affected session's per-key lock (`lockSession`) before it rewrites the sessions. This gives the same ordering.

**Rationale**:

- The lock order is always service row, then session rows. Refresh never locks the service row, so no lock cycle forms.
- Compare-and-set on the generation lets two replicas that observe the same new pair converge on one increment. The loser rereads the state and finds a matching fingerprint.

**Alternatives considered**: Serializable transactions (rejected; they would cause retries on the hot path). Advisory locks (rejected; row locks already exist).

## R6. Consent and recovery entry points (C2)

**Decision**: The browser authorization flow needs no code change beyond the currency check in R4. Step 4 (`anyDelegatedSessionUnusable`) and Step 5 (`validateMandatoryRequirements`) of `HandleAuthorization` already redirect a user with an active grant to `/agents/{agent-id}?session_token=...` when an included or mandatory session is missing or expired. This happens in every mode, because the decision precedes `ProceedHandler`. The consent page (`AgentGrantDetailPage`) lists such services under "Connect Your Accounts" with **Login** and keeps **Save** disabled until they are connected. `GrantConsent` returns the existing grant unchanged when the submitted selection matches it, so reconnecting does not extend or broaden the delegation. Protocol recovery remains the ADR 044 `error_uri`. (Approval item D-CONSENT.)

**Rationale**: This reuses existing, tested flows (`TestService_HandleAuthorization_SessionExpiry` in `internal/domain/oauth2/service_test.go`, `tests/e2e/frontend/selection_preservation_test.go`) without changing the UI or the API shape.

**Spec reconciliation**: The State A/B/C wireframes now show the existing consent and sessions UI: "Connect Your Accounts", **Login**, **Save**, and the sessions-page "Expired" card with "Log in to reconnect". They no longer show a new reconnect banner. This is recorded in the spec's Clarifications section.

**Alternatives considered**: A new "connection changed" banner. It would need a retained invalidation reason in the API, which is an extra API change that the issue does not require. Rejected.

## R7. API contracts and versions

**Decision**: Response shapes and routes are unchanged. Only documented behavior changes.

- **Admin API 2.1.0 → 2.2.0** (approval item API-ADMIN): `PUT /api/services/{service-id}` documents the invalidation of all user sessions on an effective credential change, its atomicity, and the rule that unchanged values do not invalidate.
- **End-User API 1.1.0 → 1.2.0** (approval item API-ENDUSER): The following behavior is documented:
  - `/oauth2/authorize` redirects to consent for an unusable included or mandatory connection, including an invalidated one, and returns a filesystem source failure as `server_error`.
  - Third-party initiation requires both credential files for a filesystem service.
  - A callback from an authorization that began before a credential change returns `callback_failed`.
  - RFC 8693 `/oauth2/token` returns `400 invalid_grant` with the ADR 044 `error_uri` for invalidated sessions. For filesystem services, an unexpired-token supply can return the existing non-recoverable infrastructure failure.
  - The session list reports invalidated sessions with `is_expired: true` and `has_refresh_token: false`.
  - The session refresh endpoint returns its existing `409` for them.

**Rationale**: These are compatible behavior changes within existing documented error and response sets. A minor version records the documented change. Principle IV requires stakeholder confirmation for any behavior change.

**Alternatives considered**:

- *No version change*. ADR 044 kept 1.0.0 for a target change. Rejected, because the session lifecycle semantics visible to clients change.
- *A major version*. No client-visible shape breaks. Rejected.

## R8. Logging, telemetry, and errors

**Decision**: Three credential-free audit events (see the [ADR draft](adr-draft-credential-change-session-invalidation.md#6-audit)). Recognition and stored updates run inside existing spans (`thirdparty` update, session operations). This plan adds no new metric or span name. No new `oauth2session` error detail is exposed in token-exchange diagnostics. After a recognition, the triggering request reads an expired session and reports the existing `session_missing`, `refresh_unavailable`, or `access_token_expired` recoverable details. A failed recognition commit reports the existing `persistence_failed` or `repository_unavailable` details.

**Rationale**: `coding-standards/LOGGING.md` §2.2 treats event identifiers as contracts. Adding events is additive. Reusing the diagnostic details avoids a dashboard contract change.

## R9. Feature 051 acceptance scenarios affected

**Decision**: The ADR supersession amends these Feature 051 E2E scenarios in `tests/e2e/`. They are amended with traceable comments that cite this feature. They are not deleted. (Approval item ADR-045.)

| Scenario | File | Change |
|---|---|---|
| US2-S1 | `oauth2_secret_file_rotation_test.go` | Same-identity secret rotation now invalidates the existing session. The new user's connection and refresh still use the rotated secret. |
| US2-S5 | `oauth2_secret_file_rotation_test.go` | The paused source-A callback still fails with `callback_failed`. The refresh now returns the expired-session `409`, and the session is expired. A new B connection succeeds. |
| US5-S3 | `oauth2_secret_file_boundaries_test.go` | Initiation now requires both files. Consent and session metadata stay available during the outage. |

Other Feature 051 scenarios keep their expectations. The `expectServiceUnchanged` service-record equality also still holds, because the generation lives in a separate table (R2).

## R10. No configuration, Helm, or frontend code changes

**Decision**:

- Add no configuration key. `third_party_oauth2.credential_files` is unchanged. `docs/configuration.md` and `examples/config/` document the rotation behavior.
- Make no Helm chart change.
- Make no React change. Playwright tests and screenshots cover the existing consent and sessions UI for Journey 2.

**Rationale**: The specification asks for no new setting. Principle VII is not triggered.
