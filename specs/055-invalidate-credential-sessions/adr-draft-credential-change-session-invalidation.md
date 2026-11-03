# ADR 045 (draft): Credential-Change Session Invalidation

**Status**: Proposed
**Date**: 2026-11-03
**Feature**: 055-invalidate-credential-sessions
**Supersedes (narrowly)**: [ADR 041: OAuth2 Client-Secret File Overrides](../../adrs/041-oauth2-client-secret-file-overrides.md), only the rules listed under "Superseded ADR 041 rules"

The number 045 is provisional. Implementation assigns the next free ADR number when it moves this draft into `adrs/`, and updates every reference to it.

## Context

Issue #388 requires the broker to stop using third-party sessions established under replaced client credentials and to guide users to log in again. Today the broker keeps old sessions usable after a credential change:

- A stored client-ID or secret update through `PUT /api/services/{service-id}` leaves every session usable until its tokens expire. A refresh then authenticates with the new credentials, so an old refresh token can keep the old connection alive.
- ADR 041 deliberately supports same-identity filesystem secret rotation "without service writes or restarts", so existing sessions continue. It enforces only a client-ID mismatch, and only when the broker authenticates to the provider during code exchange or refresh. An unexpired access token is returned without any credential acquisition.

The specification's 2026-11-03 clarifications propose three changes: C1 (invalidate on client-ID and secret-only changes from both sources), C2 (consent on the next agent browser authorization), and C3 (establish filesystem credential currency before every authorization admission and token decision). C1 and C3 contradict ADR 041, which is Accepted and merged on `main`. Principle II therefore needs this superseding ADR.

## Decision

### 1. Covered credential changes

A **credential change** is an actual change of a service's **effective outbound client credentials**: the operator-managed client ID and the shared secret, or the absence of a secret.

| Source | Trigger | Effective-at boundary |
|---|---|---|
| Stored | A non-discovery administrative replacement (`PUT /api/services/{service-id}`) whose effective client ID or stored secret differs from the persisted values | Successful commit of that update |
| Filesystem | The broker acquires a coherent pair that differs from the last recognized pair | Successful commit of the recognition transaction, before the triggering operation proceeds |
| Stored ↔ filesystem transition | Covered only when the effective pair differs | Commit of the update (to stored), or first recognition (to filesystem) |

These rules use exact value comparisons. Stored secrets are compared in constant time after the domain decrypts the persisted secret. An update that keeps both values, a display-name or scope edit, and a rejected or conflicting update are not credential changes. If the persisted secret cannot be decrypted, the update counts as a change, so sessions are invalidated instead of being kept.

Not covered: CIMD client identifiers derived by the broker, CIMD client-authentication key rotation, discovery-backed protected-resource refreshes that keep the client, and DCR registration lifecycles. These do not change operator-managed credentials. Existing issuer and audience guards still require zero sessions.

### 2. Credential generation and invalidation

Each service has a **credential generation**: a positive integer, starting at 1, that only increases. Every covered change increments it once. Restoring former values is another change. It increments the generation again and never revives old sessions.

The generation and the last recognized fingerprint are stored in a separate internal **credential state** record (`thirdparty_credential_states`). They are not in the service record. A filesystem recognition therefore changes neither the service `version` (ETag), `updated_at`, nor any administrative representation. ADR 041's metadata independence is preserved.

**Session invalidation** occurs in the transaction that commits the change. It applies to every `user_sessions` row of the service:

- `access_token_expires_at` = the change instant
- `encrypted_refresh_token` = NULL and `refresh_token_expires_at` = NULL
- `callback_id` = a new random value

Rows are retained, not deleted. Every existing reader then treats the connection as unusable with the logic it already has: token supply, refresh, background refresh, sweep, consent gating, session listing (`is_expired: true`), and session-health metrics. User grants are not changed.

### 3. Filesystem currency (C3)

For a filesystem-sourced service, the broker **establishes credential currency** before each of these operations:

1. Third-party authorization initiation
2. Code exchange
3. Each token-supply decision, including returning an unexpired access token
4. Each refresh: on demand, forced, background, or sweep
5. Broker `/oauth2/authorize` admission, for each filesystem-sourced service that the active grant includes or that the agent requires as mandatory

Currency is established through these steps:

1. Acquire one coherent pair with `ReadPair`. ADR 041's acquisition safety rules still apply.
2. Compute an HMAC-SHA256 digest of the length-prefixed pair. Use a 32-byte key that `crypto/rand` generates per process. The digest exists only in memory.
3. If the process cache already holds this digest for this service at the current generation, the pair is current.
4. Otherwise compare the pair with the durable fingerprint. A fingerprint is an Argon2id PHC string over the same length-prefixed pair. It uses the existing `oauth2server.Argon2Hasher` parameters (64 MiB, t=3, p=4, 16-byte salt) and runs with at most 2 concurrent computations.
   - Match: cache the digest.
   - No fingerprint (legacy filesystem state): record a baseline fingerprint with compare-and-set. Do not invalidate. The broker cannot prove a change it never observed.
   - Mismatch: commit a recognition. Lock the service row `FOR UPDATE`, compare-and-set the generation, store the new fingerprint, and invalidate sessions. A concurrent replica can lose the compare-and-set. It then rereads the state and repeats the comparison.
5. Continue the operation with the same validated pair. Code exchange and refresh still make one acquisition per logical operation. Initiation now reads both files, not only the client ID.

Unavailable, incoherent, or invalid files return the existing ADR 041 source errors. They supply no token and use no fallback credential. They are never treated as a change. If the broker cannot load the credential state or commit the recognition, the operation fails closed with the existing repository or persistence failure. No polling or background detection is added. A file publication that is reverted before the next observation cannot be detected. This limit is part of the guarantee.

Stored-to-filesystem updates write a fingerprint of the persisted stored pair, which the domain decrypts in memory, without incrementing the generation. Unchanged files therefore do not invalidate sessions. A later filesystem-to-stored update compares the supplied pair with the fingerprint. It invalidates sessions when the pair differs or no fingerprint exists. The stored source then clears the fingerprint.

### 4. In-flight fencing

- The authorization state JWE seals the current `credential_generation` at initiation.
- The callback compares the sealed generation with the current generation before it contacts the provider.
- A first-connection insert accepts only a matching generation. The insert keeps the service row `FOR SHARE` lock that it already takes, and recognition and stored updates take that row `FOR UPDATE`.
- A reconnection to an existing session is fenced by the rotated `callback_id`. When the fence rejects a reconnection, the callback fails with the existing `callback_failed` redirect. It does not report success with an unusable session.
- Refreshes keep holding the session row lock. An invalidation waits for an in-flight refresh and then overwrites its result.
- A state token without the claim, which an older replica issued during a rolling upgrade, is accepted only while the generation is 1.

A response that the broker authorized before the change committed can still be delivered. Tokens already delivered to agents, the ExtProc cache, or providers are not revoked.

### 5. Recovery and consent (C2)

This decision adds no new user-facing recovery path:

- Broker `/oauth2/authorize` already sends a user with an active grant to the consent page when an included or mandatory service connection is missing or unusable. An invalidated connection is unusable.
- RFC 8693 token exchange returns the existing recoverable `400 invalid_grant` with the service-specific `error_uri` of [ADR 044](../../adrs/044-service-specific-session-recovery-url.md).
- The sessions page offers its existing "Log in to reconnect" action.

### 6. Audit

The following credential-free events follow `coding-standards/LOGGING.md`. [contracts/ports.md](contracts/ports.md#audit-events-coding-standardsloggingmd) defines their levels and messages.

| Event | Attributes |
|---|---|
| `service.thirdparty.credentials_changed` | `service_id`, `trigger` (`stored_update`, `filesystem_recognized`, `source_transition`), `credential_generation`, `session_count`, `outcome` |
| `service.thirdparty.credential_baseline_recorded` | `service_id` |
| `service.thirdparty.credential_change_failed` | `service_id`, `trigger`, `reason` (`state_conflict`, `repository_unavailable`, `persistence_failed`) |
| `session.oauth2.callback_fenced` | `service_id`, `principal`, `reason` (`credential_generation_changed`) |

No event or error contains a credential, digest, fingerprint, path, or token.

### Superseded ADR 041 rules

| ADR 041 rule | Replacement |
|---|---|
| "Fresh acquisition supports same-identity secret rotation without service writes or restarts" (continuity of existing sessions) | A same-identity secret change is a credential change. It invalidates the sessions. It writes the internal credential state, not the service record. |
| Authorization initiation: "One `ReadClientID` call. No secret read." | Initiation performs one `ReadPair` and establishes currency. Only the client ID leaves the domain. |
| Unexpired access tokens are supplied without credential acquisition | A filesystem token-supply decision requires a successful acquisition and currency. This adds an availability dependency on the files. |
| "Publication after validation can leave an in-flight operation using its validated pair" | This remains true for provider authentication. No operation can commit a usable connection from a generation that a recognized change has superseded. |
| "No file secret, file path, descriptor, or last-known credential persists across operations" | Only a salted Argon2id fingerprint of the last recognized pair persists. No plaintext, ciphertext, path, or descriptor persists. |

All other ADR 041 rules remain binding: source selection, bindings, reader safety, identity association and transition evidence, source errors, migrations 040 and 041, and the rollback procedure. ADR 008 AAD, ADR 012 encryption ownership, ADR 038 refresh scheduling, and ADR 044 recovery targets are unchanged.

## Consequences

- Rotating any operator-managed client credential forces every user of that service to reconnect once. Operators should coordinate rotation windows.
- Filesystem token use now depends on file availability, even when the access token has not expired. A cache hit costs two file reads and one HMAC computation. The first observation of each new pair costs one Argon2id computation per process.
- Migration `046` (number provisional) adds one internal table. It is metadata-only and does not rewrite rows.
- The Feature 051 acceptance scenarios that assert session continuity across secret rotation, and initiation that needs only the client ID, are amended (US2-S1, US2-S5, US5-S3).
- Already delivered tokens remain usable at providers until they expire.

## Alternatives Considered

- **Stored-only invalidation (C1 alternative).** This leaves filesystem rotation with different recovery behavior. It was rejected for inconsistency.
- **Periodic or background file polling (C3 alternative).** This leaves an old-token window, adds a background component, and still cannot observe every write. It was rejected.
- **Deleting sessions instead of expiring them.** Deletion removes the existing "Log in to reconnect" action that cancellation recovery needs. It also makes the sessions page forget the service. It was rejected.
- **Storing the encrypted last-known pair for comparison.** This persists a recoverable credential, which conflicts with ADR 041's non-persistence principle. It was rejected for a one-way fingerprint.
- **Generation column on the service record.** A recognition would then change the ETag or record that metadata operations observe. It was rejected for a separate table.
- **Consent-first protocol recovery (C2 alternative).** This requires superseding ADR 044 and changing the `error_uri` target. It was rejected.

## References

- [Specification](spec.md), [plan](plan.md), [research](research.md), [data model](data-model.md)
- [ADR 041](../../adrs/041-oauth2-client-secret-file-overrides.md), [ADR 044](../../adrs/044-service-specific-session-recovery-url.md), [ADR 004](../../adrs/004-storage-layer-architecture.md), [ADR 008](../../adrs/008-encryption-context-optimization.md), [ADR 038](../../adrs/038-proactive-token-refresh.md)
