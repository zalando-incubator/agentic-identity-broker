# Contract: Internal ports, domain operations, and audit events

**Status**: Internal design. These are not externally visible contracts. The Proposed decisions they implement are D-SCHEMA, D-FINGERPRINT, D-CURRENCY, D-INVALIDATE, D-INITIALIZE and D-PUBLICATION in [plan.md](../plan.md#approval-items).

## New ports (`internal/ports/`)

### `ThirdpartyCredentialStateRepository` (`internal/ports/storage.go`)

```go
// ThirdpartyCredentialStateRepository stores the internal credential generation and
// filesystem fingerprint of each third-party service. A missing record reads as
// generation 1 without a fingerprint. Implementations never expose fingerprints in
// errors, and wrap failures in storage.StorageError.
type ThirdpartyCredentialStateRepository interface {
	// Get returns the current state, including the implicit default for a missing record.
	Get(ctx context.Context, serviceID id.ServiceID) (storage.CredentialState, error)
	// GetWithService returns a consistent provider/source/state snapshot under the service share lock.
	GetWithService(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, storage.CredentialState, error)
	// InitializePublication sets first evidence atomically under service then session locks.
	// Unknown existing sessions are invalidated with a generation increment; an empty service is not.
	// Known stored-transition fingerprint uses normal comparison, not legacy invalidation.
	// Conflict checks both expected generation and publication revision.
	InitializePublication(ctx context.Context, serviceID id.ServiceID, change storage.CredentialChange) (storage.CredentialChangeResult, error)
	// RecognizeChange applies change atomically: it locks the service record without modifying it,
	// compare-and-sets generation and publication revision, stores the fingerprint/revision, and invalidates every user session
	// of the service. A higher revision with unchanged pair advances evidence only. It returns Conflict on stale expected generation/revision and NotFound for a deleted service.
	RecognizeChange(ctx context.Context, serviceID id.ServiceID, change storage.CredentialChange) (storage.CredentialChangeResult, error)
}
```

`ThirdpartyOAuth2ProviderRepository.Update` gains this contract, with no signature change: when `entity.CredentialChange` is non-nil, it applies the change in the same transaction (PostgreSQL) or lock scope (memory) as the service update. It returns no partial result. On a conflict or validation failure, it leaves the service, the credential state, and the sessions unchanged.

### `CredentialFingerprinter` (`internal/ports/credential_file.go`)

```go
// CredentialFingerprinter derives and checks one-way fingerprints of a client credential pair.
// Implementations bound concurrent work and never log inputs.
type CredentialFingerprinter interface {
	Fingerprint(ctx context.Context, clientID, clientSecret string) (string, error)
	// Matches reports false, without an error, for a well-formed fingerprint of another pair.
	Matches(ctx context.Context, fingerprint, clientID, clientSecret string) (bool, error)
}
```

The adapter is `internal/adapters/credentialfile/fingerprint.go`. Add `HashContext(ctx context.Context, secret string) (string, error)` to the existing `internal/domain/oauth2server/argon2_hasher.go`; retain `Hash(secret)` as a compatibility wrapper using `context.Background()`. HashContext uses the same vetted Argon2id primitive, salt, parameters and slot pool. Cancellation while waiting for a slot must return promptly and compute nothing; check context again after obtaining a slot so pre-cancelled requests cannot win a ready-slot race. Running `argon2.IDKey` cannot be interrupted: hold its slot until completion, discard the result if the context expired, and perform no persistence. Test queued cancellation, pre-cancelled entry and running-computation completion without leaked slots. The fingerprinter calls HashContext and the existing context-aware Compare, with a post-computation cancellation check. It wraps `oauth2server.Argon2Hasher`, which is constructed in the builder with `maxConcurrent = 2`. It encodes the pair as `uint32be(len(clientID)) ‖ clientID ‖ clientSecret` and maps the hasher's mismatch result to `false`.

### `CredentialCurrencyChecker` (`internal/ports/oauth2.go`)

```go
// CredentialCurrencyChecker establishes that a filesystem-sourced service's credentials are
// current before an authorization is admitted. It returns nil for other sources.
type CredentialCurrencyChecker interface {
	EnsureCredentialCurrency(ctx context.Context, serviceID id.ServiceID) error
}
```

`*oauth2session.OAuth2SessionService` implements this port. The builder injects it into `oauth2.AuthorizationService` through a `With...` method. Principle XII applies.

## Guarded session operation (`internal/ports/storage.go`)

Extend the existing focused `UserSessionRefreshRepository` with:

```go
WithLockedSessionAtCredentialGeneration(ctx context.Context, principal id.Principal, serviceID id.ServiceID, expected storage.CredentialObservation, use func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error)
```

It takes the service share lock before the session lock, checks expected generation (missing row = 1), credential source and optional acquired publication revision, and invokes the existing update-only callback only on a match. Nil session means no connection. Stale observation returns sanitized StorageError Conflict and never invokes the callback. Memory uses issuerSessionGate.RLock then lockSession, PostgreSQL service FOR SHARE then session FOR UPDATE. GetWithService reads use service share locks for a consistent provider/source/generation association. CredentialObservation contains Generation, Source and optional PublicationRevision, never plaintext. Stale source/revision also rejects the callback, including same-pair source transitions and evidence-only revision advances. No file read, hash, recognition or service-lock acquisition occurs while holding a session lock. Domain maps stale acquired generation to existing recoverable session failure for token callers/409 for explicit refresh; persistence infrastructure failures remain non-recoverable.

Refresh admission, callback reconnection persistence and **each final token-release decision** use this guard. Final release callback returns false (no mutation), validates/decrypts the latest row and authorizes one snapshot while locks are held. Every singleflight joiner executes it independently; the returned leader result is not release authority. The repository does not know token/business predicates. Research R5 defines ordering tests. Existing WithLockedSession remains for unaffected consumers; all feature token/refresh paths must use the generation-aware operation.

## Publication reader (`internal/ports/credential_file.go`)

Add `ReadPublication(ctx context.Context, canonicalID string) (CredentialPublication, error)` alongside existing reader operations; the DTO contains client ID, secret, and positive int64 revision. Extend coherent descriptor validation to all three files and keep DTOs ephemeral. Existing ReadPair consumers remain compatible. Currency caches include revision, check stale/equal mismatch before cache hits, and perform only one CAS mutation attempt. A lost CAS discards material instead of retrying it against newer state.

## Domain operations

| Component | Change |
|---|---|
| `oauth2session.OAuth2SessionService` | Adds `establishCurrency(ctx, entity) (currentCredentials, error)`, with a process-local digest cache keyed by `(service_id, generation, publication_revision)`. `selectCredentials` receives the validated pair instead of reading again for exchange and refresh. Initiation calls `establishCurrency`, seals `CredentialGeneration`, and keeps exposing only the client ID. `GetValidAccessToken` and `refreshLockedSession` acquire a consistent provider/generation for every source; filesystem services additionally establish revision currency before locks. Refresh admission and every final release use the guarded operation above, not a reread. `HandleCallback` rejects a sealed generation that differs from the current generation before any provider request, and returns `ErrCredentialGenerationChanged` when a reconnection is fenced and the current session is unusable. |
| `thirdparty.ThirdpartyOAuth2ProviderService` | `Update` computes `CredentialChange` for non-discovery updates (research R3), including transitions and the decryption-failure rule. It emits the change audit event after a successful commit. |
| `oauth2.AuthorizationService` | `HandleAuthorization` calls `EnsureCredentialCurrency` for each included or mandatory filesystem-sourced service before steps 4 and 5. An error yields the existing `server_error` redirect decision ("Failed to check session status"). |
| `internal/app/builder.go` | Receives the new credential-state repository from the storage factory, wires the fingerprinter and domain services, and injects the currency checker. The repository has memory and PostgreSQL implementations. |

## Error mapping (no new public codes)

| Situation | Domain result | Token exchange (RFC 8693) | Third-party browser routes |
|---|---|---|---|
| Recognition committed by this request | Rereads the expired session. Returns `DetailAccessTokenExpired`, or `DetailSessionMissing` if no session exists | `400 invalid_grant` + ADR 044 `error_uri` | Initiation continues at the new generation |
| Source unavailable, incoherent, stale revision or equal-revision pair mismatch | Existing `DetailCredentialSourceUnavailable` | `500 server_error` (no `error_uri`) | Existing `500`, or `callback_failed` |
| State read or recognition commit failed | Existing `DetailRepositoryUnavailable` or `DetailPersistenceFailed` | `500 server_error` | `500` or `callback_failed` |
| Currency CAS lost | Sanitized state Conflict; discard acquisition, no retry in this operation | `500 server_error` without `error_uri` | `500` or `callback_failed` |
| Sealed generation is stale | `ErrCredentialGenerationChanged`, a new internal sentinel | — | `/sessions?error=callback_failed` |

## Audit events (`coding-standards/LOGGING.md`)

| Message | Level | `event` | Attributes |
|---|---|---|---|
| `"third-party credentials changed"` | Info | `service.thirdparty.credentials_changed` | `service_id`, `trigger` (`stored_update`, `filesystem_recognized`, or `source_transition`), `credential_generation`, `session_count`, `outcome` (`invalidated`) |
| `"third-party credential baseline recorded"` | Info | `service.thirdparty.credential_baseline_recorded` | `service_id` |
| `"third-party credential change failed"` | Error | `service.thirdparty.credential_change_failed` | `service_id`, `trigger`, `reason` (`state_conflict`, `repository_unavailable`, or `persistence_failed`), `error` (StorageError only) |
| `"oauth2 callback fenced"` | Warn | `session.oauth2.callback_fenced` | `service_id`, `principal` (`id.Principal`), `reason` (`credential_generation_changed`) |

The following never appear in any event, error, span, or metric: client IDs read from files, secrets, digests, fingerprints, file paths, or tokens. No new span name or metric instrument is added. Recognition runs within the calling operation's existing span (`coding-standards/OPENTELEMETRY.md`).
