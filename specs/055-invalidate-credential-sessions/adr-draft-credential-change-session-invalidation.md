# ADR 045 (draft): Credential-Change Session Invalidation

**Status**: Proposed
**Date**: 2026-11-03
**Feature**: 055-invalidate-credential-sessions
**Supersedes (narrowly)**: [ADR 041](../../adrs/041-oauth2-client-secret-file-overrides.md), only the rules listed under "Superseded ADR 041 rules", and [ADR 038](../../adrs/038-proactive-token-refresh.md), only session-only refresh admission locking as described in section 4

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
| Filesystem | A coherent publication has a higher operator revision and differs from the recognized pair | Successful commit of the recognition transaction, before the triggering operation proceeds |
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

#### Publication freshness (Proposed D-PUBLICATION and API-CONFIG)

Each filesystem binding adds required `credential_revision_file` (no default). Its content is a positive decimal int64, optionally followed by one newline. The operator publishes revision, client ID, and secret as one immutable projected-volume generation. `ReadPublication` extends ADR 041's descriptor/coherence checks to all three files; independent overwrites are unsupported. Revisions are service-scoped, not timestamps or symlink names, and strictly increase whenever effective values change, including restoration. All replicas share the same operator revision authority.

Persist `publication_revision` beside the fingerprint. A lower revision is stale; an equal revision with a different pair is invalid publication. Neither can change evidence, invalidate fresh sessions, admit provider authentication, or supply tokens. A higher revision with matching values advances only publication evidence; with different values it increments the generation and invalidates sessions. Restoration must use a new revision: A/41 → B/42 → A/43. Replaying A/41 is not restoration. Reject overflow, never wrap. Source transitions retain the publication revision high-water mark even when clearing the fingerprint for stored credentials; re-entering filesystem requires a revision strictly above that mark. The stored-pair transition fingerprint still prevents unchanged-pair invalidation.

This requires an additional operator file and configuration/deployment work beyond the initial no-new-key proposal. It is recommended because generation CAS cannot distinguish stale material from intentional restoration. The main alternative is stopping/draining all admission, publishing and verifying all mounts, then resuming on every rotation; that adds downtime and lacks independent freshness evidence on a misconfigured replica.

#### Currency operations

For a filesystem-sourced service, the broker **establishes credential currency** before each of these operations:

1. Third-party authorization initiation
2. Code exchange
3. Each token-supply decision, including returning an unexpired access token
4. Each refresh: on demand, forced, background, or sweep
5. Broker `/oauth2/authorize` admission, for each filesystem-sourced service that the active grant includes or that the agent requires as mandatory

Currency is established through these steps:

1. Read a consistent service/state snapshot; acquire one coherent revision and pair with `ReadPublication`; reread the state. If generation or publication revision advanced during acquisition, discard it and fail closed. Never relabel that acquisition against the newer state.
2. Compute an HMAC-SHA256 digest of the length-prefixed pair. Use a 32-byte key that `crypto/rand` generates per process. The digest exists only in memory.
3. Validate publication freshness before any cache hit. A digest match at the current generation and publication revision establishes currency.
4. Otherwise compare the pair with the durable fingerprint. A fingerprint is an Argon2id PHC string over the same length-prefixed pair. It uses the existing `oauth2server.Argon2Hasher` parameters (64 MiB, t=3, p=4, 16-byte salt) and runs with at most 2 concurrent computations.
   - Match at the same revision: cache the digest. At a higher revision, CAS-update only revision evidence before caching; preserve generation and sessions. Treat a known stored-transition fingerprint with no revision similarly, recording first revision and comparing the pair.
   - No fingerprint (legacy filesystem state): initialize fingerprint and revision under the service write lock. If any sessions exist, invalidate them and advance the generation atomically because their establishing pair is unknown. With no sessions, initialize without a generation increment (Proposed D-INITIALIZE).
   - Mismatch at a higher revision: lock the service row `FOR UPDATE`, compare-and-set expected generation and publication revision, store the new fingerprint/revision, and invalidate sessions atomically. A CAS loser fails closed with a sanitized conflict and zero token/provider calls. Diagnostic rereads cannot repeat recognition or relabel the acquired pair. A new request must make a fresh acquisition. Each operation permits at most one mutation attempt. After any successful recognition/evidence initialization, a newly issued consistent observation binds the same verified pair to the transaction result; only that successful transaction, never a CAS loss, may advance the operation's observation.
5. Continue the operation with the same validated pair. Code exchange and refresh still make one acquisition per logical operation. Initiation now reads both files, not only the client ID.

Unavailable, incoherent, or invalid files return the existing ADR 041 source errors. They supply no token and use no fallback credential. They are never treated as a change. If the broker cannot load the credential state or commit the recognition, the operation fails closed with the existing repository or persistence failure. No polling or background detection is added. A file publication that is reverted before the next observation cannot be detected. This limit is part of the guarantee.

Stored-to-filesystem updates write a fingerprint of the persisted stored pair, which the domain decrypts in memory, without incrementing the generation. Unchanged files therefore do not invalidate sessions. A later filesystem-to-stored update compares the supplied pair with the fingerprint. It invalidates sessions when the pair differs or no fingerprint exists. The stored source then clears the fingerprint.

### 4. In-flight fencing

- The authorization state JWE seals the current `credential_generation` at initiation.
- The callback compares the sealed generation with the current generation before it contacts the provider.
- A first-connection insert accepts only a matching generation. The insert keeps the service row `FOR SHARE` lock that it already takes, and recognition and stored updates take that row `FOR UPDATE`.
- A reconnection to an existing session is fenced by the rotated `callback_id`. When the fence rejects a reconnection, the callback fails with the existing `callback_failed` redirect. It does not report success with an unusable session.
- For both stored and filesystem services, acquisition binds provider, source, pair, and generation from a consistent service/state snapshot. Refresh uses `WithLockedSessionAtCredentialGeneration`: service `FOR SHARE`, then session `FOR UPDATE`, then compare current generation, source and optional publication revision with the acquired CredentialObservation before due/identity checks and provider authentication. No acquisition or recognition occurs under these locks. Memory uses `issuerSessionGate.RLock` then the per-session lock. An old acquisition cannot refresh a newly reconnected session. Invalidation takes the service write lock first, waits for an admitted refresh, then overwrites its result. This narrowly extends ADR 038 locking without changing scheduling, singleflight, or retries.
- Every caller, including each singleflight joiner, makes a final token-release decision through the same guarded operation. Under service share/session locks, check acquired CredentialObservation (generation/source/revision) and latest row validity/scopes/identity, decrypt that row's token, and authorize release before unlocking. A copy, refresh result, or unguarded reread never authorizes release. Invalidation committed first means no old-token authorization; a decision completed first permits later network delivery. Research R5 defines the deterministic tests.
- A state token without the claim, which an older replica issued during a rolling upgrade, is accepted only while the generation is 1.

A response whose final guarded release decision completed before the change committed can still be delivered. Tokens already delivered to agents, the ExtProc cache, or providers are not revoked.

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

The prohibition on publication generation identifiers is narrowly replaced by the operator revision file and durable high-water mark above. Coherent acquisition covers three descriptors rather than two. All remaining reader safety rules stay binding.

| ADR 041 rule | Replacement |
|---|---|
| "Fresh acquisition supports same-identity secret rotation without service writes or restarts" (continuity of existing sessions) | A same-identity secret change is a credential change. It invalidates the sessions. It writes the internal credential state, not the service record. |
| Authorization initiation: "One `ReadClientID` call. No secret read." | Initiation performs one `ReadPublication` and establishes currency. Only the client ID leaves the domain. |
| Unexpired access tokens are supplied without credential acquisition | A filesystem token-supply decision requires a successful acquisition and currency. This adds an availability dependency on the files. |
| "Publication after validation can leave an in-flight operation using its validated pair" | This remains true for provider authentication. No operation can commit a usable connection from a generation that a recognized change has superseded. |
| "No file secret, file path, descriptor, or last-known credential persists across operations" | Only a salted Argon2id fingerprint of the last recognized pair persists. No plaintext, ciphertext, path, or descriptor persists. |

All other ADR 041 rules remain binding: source selection, bindings, reader safety, identity association and transition evidence, source errors, migrations 040 and 041, and the rollback procedure. ADR 008 AAD, ADR 012 encryption ownership, ADR 044 recovery targets are unchanged. This draft also narrowly supersedes ADR 038's session-only refresh admission lock with service-share-before-session locking; its scheduler, update-only behavior, bounded contexts, singleflight and drain rules remain binding.

## Consequences

- Rotating any operator-managed client credential forces every user of that service to reconnect once. Operators should coordinate rotation windows.
- Filesystem token use now depends on file availability, even when the access token has not expired. A cache hit costs three coherent file reads and one HMAC computation. The first observation of each new pair costs one Argon2id computation per process.
- Migration `046` (number provisional) adds one internal table including publication revision. It is metadata-only; first acquisition of an unbaselined filesystem service expires any existing sessions (D-INITIALIZE).
- Before cutover, apply schema, publish coherent revision material, and replace all old application replicas before admitting filesystem traffic. Their reader cannot enforce this protocol. Never reset the revision high-water mark during rotation.
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
